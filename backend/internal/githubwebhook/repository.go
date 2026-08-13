package githubwebhook

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/PangIkp/devlens/backend/internal/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type Repository struct {
	db                   *postgres.DB
	queries              *sqlcgen.Queries
	payloadRetentionDays int
}

type repositoryMatch struct {
	ID string
}

type enqueueResult struct {
	deliveryID       string
	syncJobID        *string
	duplicate        bool
	receivedAt       time.Time
	processingStatus string
}

func NewRepository(db *postgres.DB, retentionDays ...int) *Repository {
	days := 30
	if len(retentionDays) > 0 && retentionDays[0] > 0 {
		days = retentionDays[0]
	}
	return &Repository{db: db, queries: db.Queries(), payloadRetentionDays: days}
}

func (r *Repository) FindRepositoryByGithubID(ctx context.Context, githubID int64) (*repositoryMatch, error) {
	if githubID < 1 {
		return nil, nil
	}

	row, err := r.queries.GetRepositoryByGithubID(ctx, githubID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find repository by github id: %w", err)
	}

	return &repositoryMatch{ID: row.ID.String()}, nil
}

func (r *Repository) EnqueueWebhookSync(ctx context.Context, repositoryID *string, installationID *int64, deliveryID string, eventType string, action *string, payload []byte, enqueueJob bool) (enqueueResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return enqueueResult{}, fmt.Errorf("begin webhook transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := r.queries.WithTx(tx)
	now := time.Now().UTC()
	payloadJSON, err := jsonValue(payload)
	if err != nil {
		return enqueueResult{}, err
	}
	retainUntil := now.Add(time.Duration(r.payloadRetentionDays) * 24 * time.Hour)
	installationUUID, err := r.findInstallationUUID(ctx, tx, installationID)
	if err != nil {
		return enqueueResult{}, err
	}

	var syncJobID *pgtype.UUID
	processingStatus := "ignored"
	if enqueueJob && repositoryID != nil {
		row, err := queries.CreateSyncJob(ctx, sqlcgen.CreateSyncJobParams{
			ID:           newUUID(),
			RepositoryID: parseUUID(*repositoryID),
			Status:       "pending",
			Progress:     0,
			TriggeredBy:  pgtype.UUID{},
			ErrorMessage: pgtype.Text{},
			StartedAt:    pgtype.Timestamptz{},
			FinishedAt:   pgtype.Timestamptz{},
		})
		if err != nil {
			return enqueueResult{}, fmt.Errorf("create sync job from webhook: %w", err)
		}
		syncJobID = &row.ID
		processingStatus = "enqueued"
		if _, err := tx.Exec(ctx, `
			INSERT INTO sync_checkpoints (
				id, sync_job_id, repository_id, resource_type, checkpoint_key, checkpoint_value, status, last_processed_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, 'job', 'mode', 'full', 'pending', NOW(), NOW(), NOW()
			)
			ON CONFLICT (sync_job_id, resource_type, checkpoint_key) DO UPDATE SET
				checkpoint_value = EXCLUDED.checkpoint_value,
				status = EXCLUDED.status,
				last_processed_at = EXCLUDED.last_processed_at,
				updated_at = NOW()`,
			newUUID(),
			row.ID,
			parseUUID(*repositoryID),
		); err != nil {
			return enqueueResult{}, fmt.Errorf("persist webhook sync mode: %w", err)
		}
	} else {
		processingStatus = "ignored"
	}

	var createdDeliveryID pgtype.Text
	var createdSyncJobID pgtype.UUID
	var receivedAt, processedAt pgtype.Timestamptz
	var createdProcessingStatus string
	err = tx.QueryRow(ctx, `
		INSERT INTO webhook_deliveries (
			id,
			repository_id,
			github_installation_id,
			github_delivery_id,
			event_type,
			processed,
			received_at,
			action,
			payload,
			sync_job_id,
			processing_status,
			payload_retention_until,
			error_message,
			processed_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, NOW(), $7, $8, $9, $10, $11, NULL, $12, $13
		)
		ON CONFLICT (github_delivery_id) DO NOTHING
		RETURNING github_delivery_id, sync_job_id, received_at, processed_at, processing_status`,
		newUUID(),
		nullableUUID(repositoryID),
		installationUUID,
		deliveryID,
		eventType,
		!enqueueJob,
		textPointerValue(action),
		payloadJSON,
		nullableUUIDFromValue(syncJobID),
		processingStatus,
		toNullableTimestamp(&retainUntil),
		processedAtForStatus(processingStatus, now),
		toNullableTimestamp(&now),
	).Scan(&createdDeliveryID, &createdSyncJobID, &receivedAt, &processedAt, &createdProcessingStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			existing, getErr := r.getWebhookDeliveryLifecycle(ctx, tx, deliveryID)
			if getErr != nil {
				return enqueueResult{}, fmt.Errorf("load duplicate webhook delivery: %w", getErr)
			}
			result := enqueueResult{
				deliveryID:       deliveryID,
				duplicate:        true,
				receivedAt:       existing.ReceivedAt.Time.UTC(),
				processingStatus: existing.ProcessingStatus,
			}
			if existing.SyncJobID.Valid {
				value := existing.SyncJobID.String()
				result.syncJobID = &value
			}
			return result, nil
		}
		return enqueueResult{}, fmt.Errorf("create webhook delivery: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return enqueueResult{}, fmt.Errorf("commit webhook transaction: %w", err)
	}

	result := enqueueResult{
		deliveryID:       deliveryID,
		duplicate:        false,
		receivedAt:       receivedAt.Time.UTC(),
		processingStatus: createdProcessingStatus,
	}
	if createdSyncJobID.Valid {
		value := createdSyncJobID.String()
		result.syncJobID = &value
	}
	return result, nil
}

func (r *Repository) ProjectRepositoryEvent(ctx context.Context, repositoryID string, eventType string, payload payloadEnvelope) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin webhook projection transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queries := r.queries.WithTx(tx)
	repoUUID := parseUUID(repositoryID)

	switch strings.TrimSpace(eventType) {
	case "pull_request":
		if err := r.projectPullRequest(ctx, queries, repoUUID, payload); err != nil {
			return err
		}
	case "pull_request_review":
		if err := r.projectPullRequest(ctx, queries, repoUUID, payload); err != nil {
			return err
		}
		if err := r.projectReview(ctx, queries, repoUUID, payload); err != nil {
			return err
		}
	case "push":
		if err := r.projectPushCommits(ctx, tx, repoUUID, payload); err != nil {
			return err
		}
	case "workflow_run":
		if err := r.projectWorkflowRun(ctx, tx, repoUUID, payload); err != nil {
			return err
		}
	case "deployment", "deployment_status":
		if err := r.projectDeployment(ctx, tx, repoUUID, payload); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit webhook projection transaction: %w", err)
	}
	return nil
}

func (r *Repository) projectPullRequest(ctx context.Context, queries *sqlcgen.Queries, repositoryID pgtype.UUID, payload payloadEnvelope) error {
	if payload.PullRequest.ID < 1 {
		return nil
	}
	createdAt := payload.PullRequest.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	author := strings.TrimSpace(payload.PullRequest.User.Login)
	if author == "" {
		author = "unknown"
	}
	title := strings.TrimSpace(payload.PullRequest.Title)
	if title == "" {
		title = fmt.Sprintf("PR #%d", payload.PullRequest.Number)
	}
	state := strings.TrimSpace(payload.PullRequest.State)
	if state == "" {
		state = "open"
	}
	_, err := queries.UpsertPullRequest(ctx, sqlcgen.UpsertPullRequestParams{
		ID:           newUUID(),
		RepositoryID: repositoryID,
		GithubPrID:   payload.PullRequest.ID,
		Number:       int32(payload.PullRequest.Number),
		Title:        title,
		Author:       author,
		State:        state,
		CreatedAt:    toNullableTimestamp(&createdAt),
		MergedAt:     toNullableTimestamp(payload.PullRequest.MergedAt),
		ClosedAt:     toNullableTimestamp(payload.PullRequest.ClosedAt),
		Additions:    int32(payload.PullRequest.Additions),
		Deletions:    int32(payload.PullRequest.Deletions),
		FilesChanged: int32(payload.PullRequest.ChangedFiles),
		IsDraft:      payload.PullRequest.Draft,
	})
	if err != nil {
		return fmt.Errorf("project pull request: %w", err)
	}
	return nil
}

func (r *Repository) projectReview(ctx context.Context, queries *sqlcgen.Queries, repositoryID pgtype.UUID, payload payloadEnvelope) error {
	if payload.PullRequest.ID < 1 || payload.Review.ID < 1 {
		return nil
	}
	pullRequestID, err := queries.UpsertPullRequest(ctx, sqlcgen.UpsertPullRequestParams{
		ID:           newUUID(),
		RepositoryID: repositoryID,
		GithubPrID:   payload.PullRequest.ID,
		Number:       int32(payload.PullRequest.Number),
		Title:        fallbackString(strings.TrimSpace(payload.PullRequest.Title), fmt.Sprintf("PR #%d", payload.PullRequest.Number)),
		Author:       fallbackString(strings.TrimSpace(payload.PullRequest.User.Login), "unknown"),
		State:        fallbackString(strings.TrimSpace(payload.PullRequest.State), "open"),
		CreatedAt:    toNullableTimestamp(timeOrNow(payload.PullRequest.CreatedAt)),
		MergedAt:     toNullableTimestamp(payload.PullRequest.MergedAt),
		ClosedAt:     toNullableTimestamp(payload.PullRequest.ClosedAt),
		Additions:    int32(payload.PullRequest.Additions),
		Deletions:    int32(payload.PullRequest.Deletions),
		FilesChanged: int32(payload.PullRequest.ChangedFiles),
		IsDraft:      payload.PullRequest.Draft,
	})
	if err != nil {
		return fmt.Errorf("ensure pull request before review projection: %w", err)
	}
	if err := queries.UpsertPullRequestReview(ctx, sqlcgen.UpsertPullRequestReviewParams{
		ID:                newUUID(),
		PullRequestID:     pullRequestID,
		GithubReviewID:    payload.Review.ID,
		Reviewer:          fallbackString(strings.TrimSpace(payload.Review.User.Login), "unknown"),
		ReviewRequestedAt: pgtype.Timestamptz{},
		FirstReviewAt:     toNullableTimestamp(payload.Review.SubmittedAt),
		ReviewSubmittedAt: toNullableTimestamp(payload.Review.SubmittedAt),
		State:             fallbackString(strings.TrimSpace(payload.Review.State), "commented"),
	}); err != nil {
		return fmt.Errorf("project pull request review: %w", err)
	}
	return nil
}

func (r *Repository) projectPushCommits(ctx context.Context, tx pgx.Tx, repositoryID pgtype.UUID, payload payloadEnvelope) error {
	for _, commit := range payload.Commits {
		if strings.TrimSpace(commit.ID) == "" {
			continue
		}
		authoredAt := time.Now().UTC()
		if commit.Timestamp != nil && !commit.Timestamp.IsZero() {
			authoredAt = commit.Timestamp.UTC()
		}
		author := strings.TrimSpace(commit.Author.Name)
		if author == "" {
			author = strings.TrimSpace(commit.Author.Username)
		}
		if author == "" {
			author = "unknown"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO commit_events (
				id, repository_id, github_commit_sha, author, author_email, message, authored_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, NOW(), NOW()
			)
			ON CONFLICT (github_commit_sha) DO UPDATE SET
				author = EXCLUDED.author,
				author_email = EXCLUDED.author_email,
				message = EXCLUDED.message,
				authored_at = EXCLUDED.authored_at,
				updated_at = NOW()`,
			newUUID(),
			repositoryID,
			strings.TrimSpace(commit.ID),
			author,
			nullableString(strings.TrimSpace(commit.Author.Email)),
			fallbackString(strings.TrimSpace(commit.Message), strings.TrimSpace(commit.ID)),
			toNullableTimestamp(&authoredAt),
		); err != nil {
			return fmt.Errorf("project push commit %s: %w", commit.ID, err)
		}
	}
	return nil
}

func (r *Repository) projectWorkflowRun(ctx context.Context, tx pgx.Tx, repositoryID pgtype.UUID, payload payloadEnvelope) error {
	if payload.WorkflowRun.ID < 1 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO workflow_events (
			id, repository_id, github_workflow_run_id, workflow_name, status, conclusion, started_at, completed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW()
		)
		ON CONFLICT (github_workflow_run_id) DO UPDATE SET
			workflow_name = EXCLUDED.workflow_name,
			status = CASE
				WHEN workflow_events.completed_at IS NOT NULL
				     AND COALESCE(EXCLUDED.completed_at, EXCLUDED.started_at) < COALESCE(workflow_events.completed_at, workflow_events.started_at)
				THEN workflow_events.status
				ELSE EXCLUDED.status
			END,
			conclusion = CASE
				WHEN workflow_events.completed_at IS NOT NULL
				     AND COALESCE(EXCLUDED.completed_at, EXCLUDED.started_at) < COALESCE(workflow_events.completed_at, workflow_events.started_at)
				THEN workflow_events.conclusion
				ELSE EXCLUDED.conclusion
			END,
			started_at = COALESCE(LEAST(workflow_events.started_at, EXCLUDED.started_at), workflow_events.started_at, EXCLUDED.started_at),
			completed_at = COALESCE(GREATEST(workflow_events.completed_at, EXCLUDED.completed_at), workflow_events.completed_at, EXCLUDED.completed_at),
			updated_at = NOW()`,
		newUUID(),
		repositoryID,
		payload.WorkflowRun.ID,
		fallbackString(strings.TrimSpace(payload.WorkflowRun.Name), fmt.Sprintf("workflow-%d", payload.WorkflowRun.ID)),
		fallbackString(strings.TrimSpace(payload.WorkflowRun.Status), "queued"),
		nullableString(strings.TrimSpace(payload.WorkflowRun.Conclusion)),
		toNullableTimestamp(payload.WorkflowRun.RunStartedAt),
		toNullableTimestamp(workflowCompletedAt(payload.WorkflowRun.Conclusion, payload.WorkflowRun.UpdatedAt)),
	)
	if err != nil {
		return fmt.Errorf("project workflow run: %w", err)
	}
	return nil
}

func (r *Repository) projectDeployment(ctx context.Context, tx pgx.Tx, repositoryID pgtype.UUID, payload payloadEnvelope) error {
	if payload.Deployment.ID < 1 {
		return nil
	}
	deployedAt := payload.DeploymentStatus.UpdatedAt
	if deployedAt.IsZero() {
		deployedAt = payload.DeploymentStatus.CreatedAt
	}
	if deployedAt.IsZero() {
		deployedAt = payload.Deployment.UpdatedAt
	}
	if deployedAt.IsZero() {
		deployedAt = payload.Deployment.CreatedAt
	}
	if deployedAt.IsZero() {
		deployedAt = time.Now().UTC()
	}
	status := strings.TrimSpace(payload.DeploymentStatus.State)
	if status == "" {
		status = strings.TrimSpace(payload.Action)
	}
	if status == "" {
		status = "pending"
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO deployments (
			id, repository_id, github_deployment_id, environment, status, deployed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6
		)
		ON CONFLICT (github_deployment_id) DO UPDATE SET
			environment = EXCLUDED.environment,
			status = CASE
				WHEN EXCLUDED.deployed_at >= deployments.deployed_at THEN EXCLUDED.status
				ELSE deployments.status
			END,
			deployed_at = GREATEST(deployments.deployed_at, EXCLUDED.deployed_at)`,
		newUUID(),
		repositoryID,
		payload.Deployment.ID,
		fallbackString(strings.TrimSpace(payload.Deployment.Environment), "production"),
		status,
		toNullableTimestamp(&deployedAt),
	)
	if err != nil {
		return fmt.Errorf("project deployment: %w", err)
	}
	return nil
}

type webhookDeliveryLifecycle struct {
	ReceivedAt       pgtype.Timestamptz
	SyncJobID        pgtype.UUID
	ProcessingStatus string
}

func (r *Repository) getWebhookDeliveryLifecycle(ctx context.Context, tx pgx.Tx, deliveryID string) (webhookDeliveryLifecycle, error) {
	row := tx.QueryRow(ctx, `
		SELECT received_at, sync_job_id, processing_status
		FROM webhook_deliveries
		WHERE github_delivery_id = $1`,
		deliveryID,
	)
	var result webhookDeliveryLifecycle
	if err := row.Scan(&result.ReceivedAt, &result.SyncJobID, &result.ProcessingStatus); err != nil {
		return webhookDeliveryLifecycle{}, err
	}
	return result, nil
}

func (r *Repository) MarkDeliveryStatus(ctx context.Context, deliveryID string, status string, message *string, processedAt *time.Time) error {
	commandTag, err := r.db.Pool().Exec(ctx, `
		UPDATE webhook_deliveries
		SET processing_status = $2::text,
		    error_message = CASE WHEN $2::text = 'failed' THEN $3 ELSE NULL END,
		    next_retry_at = CASE WHEN $2::text = 'failed' THEN next_retry_at ELSE NULL END,
		    processed_at = $4,
		    processed = CASE WHEN $2::text IN ('processed', 'ignored') THEN TRUE ELSE processed END,
		    updated_at = NOW()
		WHERE github_delivery_id = $1`,
		deliveryID,
		status,
		textPointerValue(message),
		toNullableTimestamp(processedAt),
	)
	if err != nil {
		return fmt.Errorf("mark webhook delivery status: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrDeliveryNotFound
	}
	return nil
}

func (r *Repository) GetStoredDelivery(ctx context.Context, deliveryID string) (*StoredDelivery, error) {
	row := r.db.Pool().QueryRow(ctx, `
		SELECT github_delivery_id, event_type, action, repository_id, github_installation_id, payload, processing_status, sync_job_id, received_at, retry_count
		FROM webhook_deliveries
		WHERE github_delivery_id = $1`,
		deliveryID,
	)

	var stored StoredDelivery
	var action pgtype.Text
	var repositoryID, installationUUID, syncJobID pgtype.UUID
	var receivedAt pgtype.Timestamptz
	if err := row.Scan(
		&stored.DeliveryID,
		&stored.EventType,
		&action,
		&repositoryID,
		&installationUUID,
		&stored.Payload,
		&stored.ProcessingStatus,
		&syncJobID,
		&receivedAt,
		&stored.RetryCount,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get stored webhook delivery: %w", err)
	}

	stored.Action = optionalTextPtr(action)
	if repositoryID.Valid {
		value := repositoryID.String()
		stored.RepositoryID = &value
	}
	if installationUUID.Valid {
		var installationID int64
		if err := r.db.Pool().QueryRow(ctx, `SELECT installation_id FROM github_installations WHERE id = $1`, installationUUID).Scan(&installationID); err == nil {
			stored.InstallationID = &installationID
		}
	}
	if syncJobID.Valid {
		value := syncJobID.String()
		stored.SyncJobID = &value
	}
	if receivedAt.Valid {
		stored.ReceivedAt = receivedAt.Time.UTC()
	}
	return &stored, nil
}

func (r *Repository) ScheduleRetry(ctx context.Context, deliveryID string, message string, retryCount int, failedAt time.Time, nextRetryAt time.Time) error {
	commandTag, err := r.db.Pool().Exec(ctx, `
		UPDATE webhook_deliveries
		SET processing_status = 'failed',
		    error_message = $2,
		    retry_count = $3,
		    next_retry_at = $4,
		    processed_at = $5,
		    processed = FALSE,
		    updated_at = NOW()
		WHERE github_delivery_id = $1`,
		deliveryID,
		message,
		retryCount,
		toNullableTimestamp(&nextRetryAt),
		toNullableTimestamp(&failedAt),
	)
	if err != nil {
		return fmt.Errorf("schedule webhook retry: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrDeliveryNotFound
	}
	return nil
}

func (r *Repository) ListRetryableDeliveryIDs(ctx context.Context, limit int, now time.Time) ([]string, error) {
	rows, err := r.db.Pool().Query(ctx, `
		SELECT github_delivery_id
		FROM webhook_deliveries
		WHERE processing_status = 'failed'
		  AND next_retry_at IS NOT NULL
		  AND next_retry_at <= $1
		  AND retry_count < 5
		ORDER BY next_retry_at ASC, received_at ASC
		LIMIT $2`,
		toNullableTimestamp(&now),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list retryable webhook deliveries: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan retryable webhook delivery: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate retryable webhook deliveries: %w", err)
	}
	return ids, nil
}

func newUUID() pgtype.UUID {
	var bytes [16]byte
	_, _ = rand.Read(bytes[:])
	return pgtype.UUID{Bytes: bytes, Valid: true}
}

func parseUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func nullableUUID(value *string) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return parseUUID(*value)
}

func nullableUUIDFromValue(value *pgtype.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return *value
}

func (r *Repository) findInstallationUUID(ctx context.Context, tx pgx.Tx, installationID *int64) (pgtype.UUID, error) {
	if installationID == nil || *installationID < 1 {
		return pgtype.UUID{}, nil
	}

	var id pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM github_installations WHERE installation_id = $1`, *installationID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, nil
		}
		return pgtype.UUID{}, fmt.Errorf("find github installation uuid: %w", err)
	}
	return id, nil
}

func textValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func textPointerValue(value *string) pgtype.Text {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.Text{}
	}
	return textValue(strings.TrimSpace(*value))
}

func optionalTextPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func toNullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func fallbackString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func timeOrNow(value time.Time) *time.Time {
	if value.IsZero() {
		now := time.Now().UTC()
		return &now
	}
	utc := value.UTC()
	return &utc
}

func workflowCompletedAt(conclusion string, updatedAt time.Time) *time.Time {
	if strings.TrimSpace(conclusion) == "" || updatedAt.IsZero() {
		return nil
	}
	utc := updatedAt.UTC()
	return &utc
}

func processedAtForStatus(status string, now time.Time) pgtype.Timestamptz {
	if status == "ignored" {
		return toNullableTimestamp(&now)
	}
	return pgtype.Timestamptz{}
}

func jsonValue(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return []byte("{}"), nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("decode webhook payload: %w", err)
	}
	return []byte(raw), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
