package datalifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PangIkp/devlens/backend/internal/clickhouse"
	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

type Service struct {
	pg  *postgres.DB
	ch  *clickhouse.DB
	cfg config.DataLifecycleConfig
	now func() time.Time
}

type CleanupResult struct {
	CleanedWebhookPayloads          int64
	DeletedOrganizations            int64
	DeletedInstallations            int64
	PurgedAnalyticsRepoCount        int
	ExpiredAnalyticsRawDataOrgCount int
}

func NewService(pg *postgres.DB, ch *clickhouse.DB, cfg config.DataLifecycleConfig) *Service {
	return &Service{
		pg:  pg,
		ch:  ch,
		cfg: cfg,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Run(ctx context.Context) (CleanupResult, error) {
	var result CleanupResult

	cleaned, err := s.CleanupExpiredWebhookPayloads(ctx)
	if err != nil {
		return CleanupResult{}, err
	}
	result.CleanedWebhookPayloads = cleaned

	deletedInstallations, purgedInstallRepos, err := s.PurgeExpiredDisconnectedInstallations(ctx)
	if err != nil {
		return CleanupResult{}, err
	}
	result.DeletedInstallations = deletedInstallations
	result.PurgedAnalyticsRepoCount += purgedInstallRepos

	deletedOrgs, purgedOrgRepos, err := s.PurgeExpiredSoftDeletedOrganizations(ctx)
	if err != nil {
		return CleanupResult{}, err
	}
	result.DeletedOrganizations = deletedOrgs
	result.PurgedAnalyticsRepoCount += purgedOrgRepos

	expiredRawDataOrgs, err := s.PurgeExpiredAnalyticsRawData(ctx)
	if err != nil {
		return CleanupResult{}, err
	}
	result.ExpiredAnalyticsRawDataOrgCount = expiredRawDataOrgs

	return result, nil
}

// PurgeExpiredAnalyticsRawData enforces each organization's configured (or
// default) raw analytics retention window against the ClickHouse raw event
// tables. Unlike a table-level TTL, this can honor a per-organization value
// because it deletes per organization's own repository/pull-request ids
// rather than applying one cutoff to every row in the table.
func (s *Service) PurgeExpiredAnalyticsRawData(ctx context.Context) (int, error) {
	if s.ch == nil {
		return 0, nil
	}

	defaultDays := s.cfg.AnalyticsRawRetentionDays
	if defaultDays <= 0 {
		defaultDays = 180
	}

	rows, err := s.pg.Pool().Query(ctx, `
SELECT o.id, COALESCE(ors.analytics_raw_retention_days, $1)
FROM organizations o
LEFT JOIN organization_retention_settings ors ON ors.organization_id = o.id
WHERE o.deleted_at IS NULL`, defaultDays)
	if err != nil {
		return 0, fmt.Errorf("list organization retention settings: %w", err)
	}

	type orgRetention struct {
		id   pgtype.UUID
		days int
	}
	orgs := make([]orgRetention, 0)
	for rows.Next() {
		var item orgRetention
		if err := rows.Scan(&item.id, &item.days); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan organization retention: %w", err)
		}
		orgs = append(orgs, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	purged := 0
	for _, org := range orgs {
		if org.days <= 0 {
			continue
		}
		repoIDs, prIDs, err := s.collectOrganizationResourceIDs(ctx, []pgtype.UUID{org.id})
		if err != nil {
			return purged, err
		}
		if len(repoIDs) == 0 && len(prIDs) == 0 {
			continue
		}
		if err := s.purgeExpiredAnalyticsRawDataForOrganization(ctx, repoIDs, prIDs, org.days); err != nil {
			return purged, err
		}
		purged++
	}
	return purged, nil
}

func (s *Service) purgeExpiredAnalyticsRawDataForOrganization(ctx context.Context, repoIDs []string, prIDs []string, retentionDays int) error {
	cutoff := fmt.Sprintf("now() - INTERVAL %d DAY", retentionDays)
	// Deliberately async (no mutations_sync), matching purgeClickHouse below:
	// the shared ClickHouse client's HTTP timeout is tuned for fast
	// interactive queries and is too short to block on a mutation applying,
	// especially against larger tables. ClickHouse applies these mutations
	// in the background on its own.

	if len(prIDs) > 0 {
		prCondition := sqlStringList(prIDs)
		if err := s.ch.Exec(ctx, fmt.Sprintf("ALTER TABLE pull_request_reviews DELETE WHERE pull_request_id IN (%s) AND synced_at < %s", prCondition, cutoff)); err != nil {
			return fmt.Errorf("purge expired pull_request_reviews: %w", err)
		}
		if err := s.ch.Exec(ctx, fmt.Sprintf("ALTER TABLE file_changes DELETE WHERE pull_request_id IN (%s) AND synced_at < %s", prCondition, cutoff)); err != nil {
			return fmt.Errorf("purge expired file_changes: %w", err)
		}
	}

	if len(repoIDs) > 0 {
		repoCondition := sqlStringList(repoIDs)
		for _, table := range []string{"pull_requests", "deployments", "commit_events", "workflow_events"} {
			if err := s.ch.Exec(ctx, fmt.Sprintf("ALTER TABLE %s DELETE WHERE repository_id IN (%s) AND synced_at < %s", table, repoCondition, cutoff)); err != nil {
				return fmt.Errorf("purge expired %s: %w", table, err)
			}
		}
	}
	return nil
}

func (s *Service) CleanupExpiredWebhookPayloads(ctx context.Context) (int64, error) {
	commandTag, err := s.pg.Pool().Exec(ctx, `
UPDATE webhook_deliveries
SET payload = NULL,
    updated_at = NOW()
WHERE payload IS NOT NULL
  AND payload_retention_until IS NOT NULL
  AND payload_retention_until <= $1`, s.now().UTC())
	if err != nil {
		return 0, fmt.Errorf("cleanup expired webhook payloads: %w", err)
	}
	return commandTag.RowsAffected(), nil
}

func (s *Service) PurgeExpiredSoftDeletedOrganizations(ctx context.Context) (int64, int, error) {
	cutoff := s.now().UTC().Add(-time.Duration(s.cfg.SoftDeletedOrganizationRetentionDays) * 24 * time.Hour)
	orgIDs, err := s.listUUIDs(ctx, `
SELECT id
FROM organizations
WHERE deleted_at IS NOT NULL
  AND deleted_at <= $1`, cutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("list soft deleted organizations: %w", err)
	}
	if len(orgIDs) == 0 {
		return 0, 0, nil
	}

	repoIDs, prIDs, err := s.collectOrganizationResourceIDs(ctx, orgIDs)
	if err != nil {
		return 0, 0, err
	}
	if err := s.purgeClickHouse(ctx, repoIDs, prIDs); err != nil {
		return 0, 0, err
	}

	commandTag, err := s.pg.Pool().Exec(ctx, `DELETE FROM organizations WHERE id = ANY($1::uuid[])`, orgIDs)
	if err != nil {
		return 0, 0, fmt.Errorf("delete soft deleted organizations: %w", err)
	}
	return commandTag.RowsAffected(), len(repoIDs), nil
}

func (s *Service) PurgeExpiredDisconnectedInstallations(ctx context.Context) (int64, int, error) {
	cutoff := s.now().UTC().Add(-time.Duration(s.cfg.DisconnectedInstallationRetentionDays) * 24 * time.Hour)
	installationIDs, err := s.listUUIDs(ctx, `
SELECT id
FROM github_installations
WHERE disconnected_at IS NOT NULL
  AND disconnected_at <= $1`, cutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("list disconnected installations: %w", err)
	}
	if len(installationIDs) == 0 {
		return 0, 0, nil
	}

	repoIDs, prIDs, err := s.collectInstallationResourceIDs(ctx, installationIDs)
	if err != nil {
		return 0, 0, err
	}
	if err := s.purgeClickHouse(ctx, repoIDs, prIDs); err != nil {
		return 0, 0, err
	}

	tx, err := s.pg.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin installation purge transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM webhook_deliveries WHERE github_installation_id = ANY($1::uuid[])`, installationIDs); err != nil {
		return 0, 0, fmt.Errorf("delete installation webhook deliveries: %w", err)
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM repositories
WHERE github_installation_repository_id IN (
	SELECT id
	FROM github_installation_repositories
	WHERE github_installation_id = ANY($1::uuid[])
)`, installationIDs); err != nil {
		return 0, 0, fmt.Errorf("delete installation repositories: %w", err)
	}
	commandTag, err := tx.Exec(ctx, `DELETE FROM github_installations WHERE id = ANY($1::uuid[])`, installationIDs)
	if err != nil {
		return 0, 0, fmt.Errorf("delete disconnected installations: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit installation purge transaction: %w", err)
	}

	return commandTag.RowsAffected(), len(repoIDs), nil
}

func (s *Service) collectOrganizationResourceIDs(ctx context.Context, organizationIDs []pgtype.UUID) ([]string, []string, error) {
	repoIDs, err := s.listStrings(ctx, `
SELECT id::text
FROM repositories
WHERE organization_id = ANY($1::uuid[])`, organizationIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("list repositories for organizations: %w", err)
	}
	prIDs, err := s.listStrings(ctx, `
SELECT pr.id::text
FROM pull_requests pr
INNER JOIN repositories r ON r.id = pr.repository_id
WHERE r.organization_id = ANY($1::uuid[])`, organizationIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("list pull requests for organizations: %w", err)
	}
	return repoIDs, prIDs, nil
}

func (s *Service) collectInstallationResourceIDs(ctx context.Context, installationIDs []pgtype.UUID) ([]string, []string, error) {
	repoIDs, err := s.listStrings(ctx, `
SELECT r.id::text
FROM repositories r
INNER JOIN github_installation_repositories gir ON gir.id = r.github_installation_repository_id
WHERE gir.github_installation_id = ANY($1::uuid[])`, installationIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("list repositories for installations: %w", err)
	}
	prIDs, err := s.listStrings(ctx, `
SELECT pr.id::text
FROM pull_requests pr
INNER JOIN repositories r ON r.id = pr.repository_id
INNER JOIN github_installation_repositories gir ON gir.id = r.github_installation_repository_id
WHERE gir.github_installation_id = ANY($1::uuid[])`, installationIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("list pull requests for installations: %w", err)
	}
	return repoIDs, prIDs, nil
}

func (s *Service) purgeClickHouse(ctx context.Context, repoIDs []string, prIDs []string) error {
	if s.ch == nil {
		return nil
	}
	if len(repoIDs) == 0 && len(prIDs) == 0 {
		return nil
	}

	if len(prIDs) > 0 {
		prCondition := sqlStringList(prIDs)
		if err := s.ch.Exec(ctx, fmt.Sprintf("ALTER TABLE pull_request_reviews DELETE WHERE pull_request_id IN (%s)", prCondition)); err != nil {
			return fmt.Errorf("purge clickhouse pull_request_reviews: %w", err)
		}
		if err := s.ch.Exec(ctx, fmt.Sprintf("ALTER TABLE file_changes DELETE WHERE pull_request_id IN (%s)", prCondition)); err != nil {
			return fmt.Errorf("purge clickhouse file_changes: %w", err)
		}
	}

	if len(repoIDs) > 0 {
		repoCondition := sqlStringList(repoIDs)
		for _, table := range []string{"pull_requests", "deployments", "commit_events", "workflow_events", "metrics_daily"} {
			if err := s.ch.Exec(ctx, fmt.Sprintf("ALTER TABLE %s DELETE WHERE repository_id IN (%s)", table, repoCondition)); err != nil {
				return fmt.Errorf("purge clickhouse %s: %w", table, err)
			}
		}
	}
	return nil
}

func (s *Service) listUUIDs(ctx context.Context, query string, args ...any) ([]pgtype.UUID, error) {
	rows, err := s.pg.Pool().Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]pgtype.UUID, 0)
	for rows.Next() {
		var item pgtype.UUID
		if err := rows.Scan(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) listStrings(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := s.pg.Pool().Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var item string
		if err := rows.Scan(&item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func sqlStringList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, "'"+strings.ReplaceAll(value, "'", "''")+"'")
	}
	return strings.Join(quoted, ",")
}
