package e2e

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/PangIkp/devlens/backend/internal/clickhouse"
	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/githubapp"
	"github.com/PangIkp/devlens/backend/internal/githubclient"
	"github.com/PangIkp/devlens/backend/internal/githubconnection"
	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/PangIkp/devlens/backend/internal/metricsbus"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestGitHubAppInstallSelectSyncAndMetricsReadyIntegration(t *testing.T) {
	t.Parallel()

	cfg, db, ch := openIntegrationDependencies(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	orgID := seedOrganization(t, ctx, db)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	githubRepoID := int64(42001)
	installationID := time.Now().UTC().UnixNano()
	app := integrationGitHubApp{
		installation: githubapp.Installation{
			ID:           installationID,
			AccountLogin: "pangikp",
			AccountType:  "User",
			TargetType:   "all",
			Permissions: map[string]string{
				"metadata":      "read",
				"contents":      "read",
				"pull_requests": "read",
				"actions":       "read",
				"deployments":   "read",
			},
		},
		repositories: []githubapp.AccessibleRepository{
			{
				GithubRepositoryID: githubRepoID,
				Name:               "devlens-api",
				OwnerLogin:         "pangikp",
				FullName:           "pangikp/devlens-api",
				Private:            true,
				DefaultBranch:      stringPtr("main"),
			},
		},
	}

	connectionRepo := githubconnection.NewRepository(db)
	connectionSvc := githubconnection.NewService(connectionRepo, app, nil)

	connection, err := connectionSvc.CompleteInstallation(ctx, orgID, installationID, orgID+":"+strconv.FormatInt(time.Now().UTC().Unix(), 10), "")
	if err != nil {
		t.Fatalf("complete installation: %v", err)
	}
	if connection.State != githubconnection.StateConnected {
		t.Fatalf("expected connected state after installation, got %q", connection.State)
	}

	accessible, err := connectionSvc.ListAccessibleRepositories(ctx, githubconnection.ListAccessibleRepositoriesParams{
		OrganizationID: orgID,
		Page:           1,
		PageSize:       20,
	})
	if err != nil {
		t.Fatalf("list accessible repositories: %v", err)
	}
	if len(accessible.Items) != 1 {
		t.Fatalf("expected 1 accessible repo, got %d", len(accessible.Items))
	}

	selected, err := connectionSvc.SelectRepositories(ctx, orgID, githubconnection.SelectRepositoriesRequest{
		RepositoryIDs: []int64{githubRepoID},
		AutoSync:      boolPtr(false),
	})
	if err != nil {
		t.Fatalf("select repositories: %v", err)
	}
	if len(selected.CreatedRepositoryIDs) != 1 {
		t.Fatalf("expected 1 created repository id, got %#v", selected.CreatedRepositoryIDs)
	}
	repositoryID := selected.CreatedRepositoryIDs[0]

	metricsSvc := metrics.NewService(db, ch)
	syncRepo := syncjob.NewRepository(db)
	syncSvc := syncjob.NewService(syncRepo, integrationGitHubClient{baseTime: now}, nil)
	syncSvc.SetCompletionPublisher(metricsbus.NewPublisher(nil, nil, metricsSvc))

	job, err := syncSvc.Create(ctx, repositoryID, syncjob.CreateSyncRequest{Mode: syncjob.ModeFull})
	if err != nil {
		t.Fatalf("run initial sync: %v", err)
	}
	if job.Status != syncjob.StatusCompleted {
		t.Fatalf("expected completed sync job, got %q", job.Status)
	}

	connection, err = connectionSvc.GetConnection(ctx, orgID)
	if err != nil {
		t.Fatalf("reload connection: %v", err)
	}
	if connection.ConnectedRepositories != 1 {
		t.Fatalf("expected one connected repository, got %d", connection.ConnectedRepositories)
	}

	query := metrics.DeploymentQueryParams{
		QueryParams: metrics.QueryParams{
			From:     now.AddDate(0, 0, -2),
			To:       now,
			Interval: metrics.IntervalDay,
			DayType:  metrics.DayTypeCalendar,
		},
	}
	result, err := metricsSvc.GetRepositoryMetrics(ctx, repositoryID, query)
	if err != nil {
		t.Fatalf("get repository metrics: %v", err)
	}

	if result.Summary.PRCycleTimeMinutes <= 0 {
		t.Fatalf("expected PR cycle time > 0, got %f", result.Summary.PRCycleTimeMinutes)
	}
	if result.Reviews.ReviewCoverage <= 0 {
		t.Fatalf("expected review coverage > 0, got %f", result.Reviews.ReviewCoverage)
	}
	if result.Deployments.DeploymentCount != 1 {
		t.Fatalf("expected 1 deployment, got %d", result.Deployments.DeploymentCount)
	}
	if len(result.Hotspots) == 0 {
		t.Fatal("expected hotspot data after sync and metrics calculation")
	}

	var initialSyncStatus string
	if err := db.Pool().QueryRow(ctx, `SELECT initial_sync_status FROM repositories WHERE id = $1`, parseUUID(repositoryID)).Scan(&initialSyncStatus); err != nil {
		t.Fatalf("load repository sync status: %v", err)
	}
	if initialSyncStatus != "synced" {
		t.Fatalf("expected repository initial_sync_status synced, got %q", initialSyncStatus)
	}

	_ = cfg
}

type integrationGitHubApp struct {
	installation githubapp.Installation
	repositories []githubapp.AccessibleRepository
}

func (a integrationGitHubApp) Enabled() bool { return true }
func (a integrationGitHubApp) InstallURL(string, string) (string, error) {
	return "http://example.test/install", nil
}
func (a integrationGitHubApp) GetInstallation(context.Context, int64) (githubapp.Installation, error) {
	return a.installation, nil
}
func (a integrationGitHubApp) CreateInstallationToken(context.Context, int64) (githubapp.Token, error) {
	return githubapp.Token{Value: "token", ExpiresAt: time.Now().UTC().Add(time.Hour)}, nil
}
func (a integrationGitHubApp) ListInstallationRepositories(context.Context, int64, int, int) (githubapp.RepositoryPage, error) {
	return githubapp.RepositoryPage{Items: a.repositories}, nil
}

type integrationGitHubClient struct {
	baseTime time.Time
}

func (c integrationGitHubClient) GetRepository(context.Context, string, string) (githubclient.Repository, error) {
	return githubclient.Repository{
		ID:            42001,
		Name:          "devlens-api",
		FullName:      "pangikp/devlens-api",
		DefaultBranch: "main",
		Private:       true,
	}, nil
}

func (c integrationGitHubClient) GetPullRequest(context.Context, string, string, int) (githubclient.PullRequest, error) {
	created := c.baseTime.Add(-36 * time.Hour)
	merged := c.baseTime.Add(-12 * time.Hour)
	return githubclient.PullRequest{
		ID:           9001,
		Number:       12,
		Title:        "Improve metrics pipeline",
		State:        "closed",
		Draft:        false,
		User:         githubclient.User{Login: "pangikp", ID: 101},
		CreatedAt:    created,
		UpdatedAt:    merged,
		ClosedAt:     &merged,
		MergedAt:     &merged,
		Additions:    25,
		Deletions:    5,
		ChangedFiles: 1,
	}, nil
}

func (c integrationGitHubClient) ListPullRequests(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.PullRequest], error) {
	created := c.baseTime.Add(-36 * time.Hour)
	merged := c.baseTime.Add(-12 * time.Hour)
	return githubclient.Page[githubclient.PullRequest]{
		Items: []githubclient.PullRequest{{
			ID:           9001,
			Number:       12,
			Title:        "Improve metrics pipeline",
			State:        "closed",
			Draft:        false,
			User:         githubclient.User{Login: "pangikp", ID: 101},
			CreatedAt:    created,
			UpdatedAt:    merged,
			ClosedAt:     &merged,
			MergedAt:     &merged,
			Additions:    25,
			Deletions:    5,
			ChangedFiles: 1,
		}},
	}, nil
}

func (c integrationGitHubClient) ListReviews(context.Context, string, string, int, githubclient.ListOptions) (githubclient.Page[githubclient.Review], error) {
	submitted := c.baseTime.Add(-24 * time.Hour)
	return githubclient.Page[githubclient.Review]{
		Items: []githubclient.Review{{
			ID:          8001,
			State:       "approved",
			User:        githubclient.User{Login: "reviewer", ID: 202},
			CommitID:    "abc123",
			SubmittedAt: &submitted,
		}},
	}, nil
}

func (c integrationGitHubClient) ListCommits(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.Commit], error) {
	committed := c.baseTime.Add(-30 * time.Hour)
	return githubclient.Page[githubclient.Commit]{
		Items: []githubclient.Commit{{
			SHA:    "abc123",
			Author: &githubclient.User{Login: "pangikp", ID: 101},
			Commit: githubclient.CommitDetail{
				Message: "Improve metrics pipeline",
				Author: githubclient.CommitAuthor{
					Name:  "Pang",
					Email: "pang@example.com",
					Date:  committed,
				},
			},
		}},
	}, nil
}

func (c integrationGitHubClient) ListPullRequestFiles(context.Context, string, string, int, githubclient.ListOptions) (githubclient.Page[githubclient.PullRequestFile], error) {
	return githubclient.Page[githubclient.PullRequestFile]{
		Items: []githubclient.PullRequestFile{{
			Filename:  "backend/internal/metrics/service.go",
			Additions: 25,
			Deletions: 5,
			Changes:   30,
		}},
	}, nil
}

func (c integrationGitHubClient) ListWorkflowRuns(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.WorkflowRun], error) {
	started := c.baseTime.Add(-10 * time.Hour)
	updated := c.baseTime.Add(-9 * time.Hour)
	return githubclient.Page[githubclient.WorkflowRun]{
		Items: []githubclient.WorkflowRun{{
			ID:           7001,
			Name:         "CI",
			Status:       "completed",
			Conclusion:   "success",
			RunStartedAt: &started,
			CreatedAt:    started,
			UpdatedAt:    updated,
		}},
	}, nil
}

func (c integrationGitHubClient) ListDeployments(context.Context, string, string, githubclient.ListOptions) (githubclient.Page[githubclient.Deployment], error) {
	created := c.baseTime.Add(-8 * time.Hour)
	return githubclient.Page[githubclient.Deployment]{
		Items: []githubclient.Deployment{{
			ID:          6001,
			Environment: "production",
			CreatedAt:   created,
			UpdatedAt:   created,
		}},
	}, nil
}

func (c integrationGitHubClient) ListDeploymentStatuses(context.Context, string, string, int64, githubclient.ListOptions) (githubclient.Page[githubclient.DeploymentStatus], error) {
	updated := c.baseTime.Add(-7 * time.Hour)
	return githubclient.Page[githubclient.DeploymentStatus]{
		Items: []githubclient.DeploymentStatus{{
			ID:        6002,
			State:     "success",
			CreatedAt: updated,
			UpdatedAt: updated,
		}},
	}, nil
}

func openIntegrationDependencies(t *testing.T) (config.Config, *postgres.DB, *clickhouse.DB) {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Postgres.ConnectTimeout)
	defer cancel()

	db, err := postgres.Open(ctx, cfg.Postgres, nil)
	if err != nil {
		t.Skipf("skip integration test: postgres unavailable: %v", err)
	}
	t.Cleanup(db.Close)

	ch, err := clickhouse.Open(cfg.ClickHouse, nil)
	if err != nil {
		t.Skipf("skip integration test: clickhouse unavailable: %v", err)
	}
	t.Cleanup(ch.Close)

	chCtx, chCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer chCancel()
	if err := clickhouse.EnsureSchema(chCtx, ch, cfg.DataLifecycle); err != nil {
		t.Skipf("skip integration test: ensure clickhouse schema failed: %v", err)
	}

	return cfg, db, ch
}

func seedOrganization(t *testing.T, ctx context.Context, db *postgres.DB) string {
	t.Helper()

	orgID := newUUID()
	suffix := time.Now().UTC().UnixNano()
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO organizations (id, github_id, name, slug, created_at)
		VALUES ($1, $2, $3, $4, NOW())`,
		orgID,
		suffix,
		"E2E Org",
		fmt.Sprintf("e2e-org-%d", suffix),
	); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	return orgID.String()
}

func newUUID() pgtype.UUID {
	var value pgtype.UUID
	value.Valid = true
	copy(value.Bytes[:], []byte(fmt.Sprintf("%016x%016x", time.Now().UTC().UnixNano(), time.Now().UTC().UnixNano())))
	return value
}

func parseUUID(value string) pgtype.UUID {
	var parsed pgtype.UUID
	_ = parsed.Scan(value)
	return parsed
}

func boolPtr(value bool) *bool {
	return &value
}

func stringPtr(value string) *string {
	return &value
}
