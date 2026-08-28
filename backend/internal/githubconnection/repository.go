package githubconnection

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
	db      *postgres.DB
	queries *sqlcgen.Queries
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db, queries: db.Queries()}
}

func (r *Repository) EnsureOrganizationExists(ctx context.Context, organizationID string) error {
	_, err := r.queries.GetOrganizationByID(ctx, parseUUID(organizationID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrganizationNotFound
		}
		return fmt.Errorf("get organization: %w", err)
	}
	return nil
}

func (r *Repository) GetInstallation(ctx context.Context, organizationID string) (*installationRecord, error) {
	row := r.db.Pool().QueryRow(ctx, `
		WITH repo_status AS (
			SELECT
				gir.github_installation_id,
				gir.linked_repository_id,
				CASE
					WHEN gir.linked_repository_id IS NULL THEN gir.selection_status
					WHEN repo.is_active = FALSE THEN 'not_selected'
					WHEN latest_job.status IN ('pending', 'running') THEN 'syncing'
					WHEN latest_job.status = 'failed' THEN 'sync_failed'
					WHEN latest_job.status = 'completed' THEN 'synced'
					ELSE 'selected'
				END AS derived_selection_status,
				COALESCE(latest_job.error_message, gir.sync_error_message) AS sync_error_message,
				COALESCE(latest_job.finished_at, gir.last_synced_at) AS last_synced_at
			FROM github_installation_repositories gir
			LEFT JOIN repositories repo ON repo.id = gir.linked_repository_id
			LEFT JOIN LATERAL (
				SELECT sj.status, sj.error_message, sj.finished_at
				FROM sync_jobs sj
				WHERE sj.repository_id = gir.linked_repository_id
				ORDER BY sj.created_at DESC, sj.id DESC
				LIMIT 1
			) latest_job ON gir.linked_repository_id IS NOT NULL
		)
		SELECT gi.id, gi.organization_id, COALESCE(gi.installation_id, 0), gi.account_login, gi.account_type, gi.target_type, gi.status, gi.suspended_at,
		       MAX(rs.last_synced_at) AS last_synced_at,
		       MAX(rs.sync_error_message) FILTER (WHERE rs.derived_selection_status = 'sync_failed') AS last_sync_error,
		       COUNT(*) FILTER (WHERE rs.derived_selection_status IN ('selected','syncing','sync_failed','synced'))::int AS connected_repositories,
		       COUNT(*) FILTER (WHERE rs.derived_selection_status = 'syncing')::int AS syncing_repositories,
		       COUNT(*) FILTER (WHERE rs.derived_selection_status = 'sync_failed')::int AS failed_repositories
		FROM github_installations gi
		LEFT JOIN repo_status rs ON rs.github_installation_id = gi.id
		WHERE gi.organization_id = $1
		GROUP BY gi.id`, parseUUID(organizationID))

	var rec installationRecord
	var id, orgID pgtype.UUID
	var installationID int64
	var accountLogin, accountType, targetType pgtype.Text
	var suspendedAt, lastSyncedAt pgtype.Timestamptz
	var lastSyncError pgtype.Text
	if err := row.Scan(
		&id,
		&orgID,
		&installationID,
		&accountLogin,
		&accountType,
		&targetType,
		&rec.Status,
		&suspendedAt,
		&lastSyncedAt,
		&lastSyncError,
		&rec.ConnectedRepositories,
		&rec.SyncingRepositories,
		&rec.FailedRepositories,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get installation: %w", err)
	}

	rec.ID = id.String()
	rec.OrganizationID = orgID.String()
	rec.InstallationID = installationID
	rec.AccountLogin = optionalText(accountLogin)
	rec.AccountType = optionalText(accountType)
	rec.TargetType = optionalText(targetType)
	rec.SuspendedAt = optionalTime(suspendedAt)
	rec.LastSyncedAt = optionalTime(lastSyncedAt)
	rec.LastSyncError = optionalText(lastSyncError)
	return &rec, nil
}

func (r *Repository) ListLinkedRepositoryIDs(ctx context.Context, organizationID string) ([]string, error) {
	installation, err := r.GetInstallation(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if installation == nil {
		return nil, ErrInstallationNotFound
	}

	rows, err := r.db.Pool().Query(ctx, `
		SELECT linked_repository_id
		FROM github_installation_repositories
		WHERE github_installation_id = $1 AND linked_repository_id IS NOT NULL`,
		parseUUID(installation.ID),
	)
	if err != nil {
		return nil, fmt.Errorf("list linked repository ids: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan linked repository id: %w", err)
		}
		ids = append(ids, id.String())
	}
	return ids, nil
}

func (r *Repository) FindOrganizationIDByInstallationID(ctx context.Context, installationID int64) (*string, error) {
	var organizationID pgtype.UUID
	err := r.db.Pool().QueryRow(ctx, `
		SELECT organization_id
		FROM github_installations
		WHERE installation_id = $1
		LIMIT 1`, installationID).Scan(&organizationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find organization by installation id: %w", err)
	}
	value := organizationID.String()
	return &value, nil
}

func (r *Repository) UpsertInstallation(ctx context.Context, organizationID string, installationID int64, accountLogin, accountType, targetType string, status string, permissions map[string]string, installedByGitHubID int64, suspendedAt *time.Time) (*installationRecord, error) {
	permissionsJSON, err := json.Marshal(permissions)
	if err != nil {
		return nil, fmt.Errorf("marshal permissions: %w", err)
	}

	row := r.db.Pool().QueryRow(ctx, `
		INSERT INTO github_installations (
			id, organization_id, installation_id, account_login, account_type, target_type, status, permissions_json, installed_by_github_user_id, suspended_at, installed_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, NOW(), NOW()
		)
		ON CONFLICT (organization_id) DO UPDATE SET
			installation_id = EXCLUDED.installation_id,
			account_login = EXCLUDED.account_login,
			account_type = EXCLUDED.account_type,
			target_type = EXCLUDED.target_type,
			status = EXCLUDED.status,
			permissions_json = EXCLUDED.permissions_json,
			installed_by_github_user_id = EXCLUDED.installed_by_github_user_id,
			suspended_at = EXCLUDED.suspended_at,
			disconnected_at = NULL,
			updated_at = NOW()
		RETURNING id`,
		newUUID(),
		parseUUID(organizationID),
		installationID,
		nullString(accountLogin),
		nullString(accountType),
		nullString(targetType),
		status,
		string(permissionsJSON),
		nullInt64(installedByGitHubID),
		toNullableTimestamp(suspendedAt),
	)

	var id pgtype.UUID
	if err := row.Scan(&id); err != nil {
		return nil, fmt.Errorf("upsert installation: %w", err)
	}

	return r.GetInstallation(ctx, organizationID)
}

func (r *Repository) UpdateInstallationLifecycle(ctx context.Context, installationID int64, status string, suspendedAt *time.Time, disconnectedAt *time.Time) error {
	_, err := r.db.Pool().Exec(ctx, `
		UPDATE github_installations
		SET status = $2,
		    suspended_at = $3,
		    disconnected_at = $4,
		    updated_at = NOW()
		WHERE installation_id = $1`,
		installationID,
		status,
		toNullableTimestamp(suspendedAt),
		toNullableTimestamp(disconnectedAt),
	)
	if err != nil {
		return fmt.Errorf("update installation lifecycle: %w", err)
	}
	return nil
}

func (r *Repository) ReplaceAccessibleRepositories(ctx context.Context, organizationID string, items []accessibleRepositoryRecord) error {
	installation, err := r.GetInstallation(ctx, organizationID)
	if err != nil {
		return err
	}
	if installation == nil {
		return ErrInstallationNotFound
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin replace accessible repositories: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE github_installation_repositories
		SET installation_status = $2,
		    last_discovered_at = NOW(),
		    updated_at = NOW()
		WHERE github_installation_id = $1`,
		parseUUID(installation.ID),
		InstallationStatusInstallationRequired,
	); err != nil {
		return fmt.Errorf("mark existing installation repositories stale: %w", err)
	}

	for _, item := range items {
		permissionsJSON, err := json.Marshal(map[string]string{})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO github_installation_repositories (
				id, github_installation_id, github_repository_id, name, owner_login, full_name, private, default_branch, installation_status, selection_status, permissions_json, linked_repository_id, last_discovered_at, last_synced_at, sync_error_message, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, NULL, NOW(), NULL, NULL, NOW(), NOW()
			)
			ON CONFLICT (github_installation_id, github_repository_id) DO UPDATE SET
				name = EXCLUDED.name,
				owner_login = EXCLUDED.owner_login,
				full_name = EXCLUDED.full_name,
				private = EXCLUDED.private,
				default_branch = EXCLUDED.default_branch,
				installation_status = EXCLUDED.installation_status,
				last_discovered_at = NOW(),
				updated_at = NOW()`,
			newUUID(),
			parseUUID(installation.ID),
			item.GithubRepositoryID,
			item.Name,
			item.OwnerLogin,
			item.FullName,
			item.Private,
			textValue(item.DefaultBranch),
			item.InstallationStatus,
			SelectionStatusNotSelected,
			string(permissionsJSON),
		); err != nil {
			return fmt.Errorf("insert accessible repository %d: %w", item.GithubRepositoryID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit accessible repositories: %w", err)
	}
	return nil
}

// DisconnectInstallation marks an installation as disconnected, clears its
// repository selections, and deactivates every repository that was linked to
// it, all in one transaction. Deactivating the repositories keeps them from
// appearing as syncable in the UI or accepting manual sync requests once the
// GitHub App connection backing them no longer exists.
func (r *Repository) DisconnectInstallation(ctx context.Context, installationID int64, status string, disconnectedAt *time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin disconnect installation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var githubInstallationRowID pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM github_installations WHERE installation_id = $1`, installationID).Scan(&githubInstallationRowID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInstallationNotFound
		}
		return fmt.Errorf("find installation: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE github_installations
		SET status = $2,
		    suspended_at = NULL,
		    disconnected_at = $3,
		    updated_at = NOW()
		WHERE id = $1`,
		githubInstallationRowID,
		status,
		toNullableTimestamp(disconnectedAt),
	); err != nil {
		return fmt.Errorf("update installation lifecycle: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT linked_repository_id
		FROM github_installation_repositories
		WHERE github_installation_id = $1 AND linked_repository_id IS NOT NULL`,
		githubInstallationRowID,
	)
	if err != nil {
		return fmt.Errorf("list linked repositories: %w", err)
	}
	linkedRepositoryIDs := make([]pgtype.UUID, 0)
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan linked repository id: %w", err)
		}
		linkedRepositoryIDs = append(linkedRepositoryIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate linked repositories: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE github_installation_repositories
		SET selection_status = $2,
		    linked_repository_id = NULL,
		    installation_status = $3,
		    updated_at = NOW()
		WHERE github_installation_id = $1`,
		githubInstallationRowID,
		SelectionStatusNotSelected,
		InstallationStatusInstallationRequired,
	); err != nil {
		return fmt.Errorf("clear repository selections: %w", err)
	}

	if len(linkedRepositoryIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE repositories
			SET is_active = FALSE
			WHERE id = ANY($1)`,
			linkedRepositoryIDs,
		); err != nil {
			return fmt.Errorf("deactivate disconnected repositories: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit disconnect installation: %w", err)
	}
	return nil
}

func (r *Repository) ListAccessibleRepositories(ctx context.Context, params ListAccessibleRepositoriesParams) (ListAccessibleRepositoriesResult, error) {
	installation, err := r.GetInstallation(ctx, params.OrganizationID)
	if err != nil {
		return ListAccessibleRepositoriesResult{}, err
	}
	if installation == nil {
		return ListAccessibleRepositoriesResult{}, ErrInstallationNotFound
	}

	search := strings.TrimSpace(params.Search)

	rows, err := r.db.Pool().Query(ctx, `
		SELECT
			gir.id,
			gir.github_installation_id,
			gir.github_repository_id,
			gir.name,
			gir.owner_login,
			gir.full_name,
			gir.private,
			gir.default_branch,
			gir.installation_status,
			CASE
				WHEN gir.linked_repository_id IS NULL THEN gir.selection_status
				WHEN repo.is_active = FALSE THEN 'not_selected'
				WHEN latest_job.status IN ('pending', 'running') THEN 'syncing'
				WHEN latest_job.status = 'failed' THEN 'sync_failed'
				WHEN latest_job.status = 'completed' THEN 'synced'
				ELSE 'selected'
			END AS selection_status,
			gir.linked_repository_id,
			COALESCE(latest_job.error_message, gir.sync_error_message) AS sync_error_message
		FROM github_installation_repositories gir
		LEFT JOIN repositories repo ON repo.id = gir.linked_repository_id
		LEFT JOIN LATERAL (
			SELECT sj.status, sj.error_message
			FROM sync_jobs sj
			WHERE sj.repository_id = gir.linked_repository_id
			ORDER BY sj.created_at DESC, sj.id DESC
			LIMIT 1
		) latest_job ON gir.linked_repository_id IS NOT NULL
		WHERE gir.github_installation_id = $1
		  AND ($4 = '' OR gir.full_name ILIKE '%' || $4 || '%')
		ORDER BY full_name ASC
		LIMIT $2 OFFSET $3`,
		parseUUID(installation.ID),
		params.PageSize,
		(params.Page-1)*params.PageSize,
		search,
	)
	if err != nil {
		return ListAccessibleRepositoriesResult{}, fmt.Errorf("list accessible repositories: %w", err)
	}
	defer rows.Close()

	items := make([]AccessibleRepositoryResponse, 0)
	for rows.Next() {
		rec, err := scanAccessibleRepository(rows)
		if err != nil {
			return ListAccessibleRepositoriesResult{}, err
		}
		items = append(items, toAccessibleResponse(rec))
	}

	var total int
	if err := r.db.Pool().QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM github_installation_repositories
		WHERE github_installation_id = $1
		  AND ($2 = '' OR full_name ILIKE '%' || $2 || '%')`,
		parseUUID(installation.ID),
		search,
	).Scan(&total); err != nil {
		return ListAccessibleRepositoriesResult{}, fmt.Errorf("count accessible repositories: %w", err)
	}

	return ListAccessibleRepositoriesResult{
		Items:      items,
		TotalItems: total,
	}, nil
}

func (r *Repository) GetAccessibleRepositoriesByGitHubIDs(ctx context.Context, organizationID string, ids []int64) ([]accessibleRepositoryRecord, error) {
	installation, err := r.GetInstallation(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if installation == nil {
		return nil, ErrInstallationNotFound
	}

	rows, err := r.db.Pool().Query(ctx, `
		SELECT
			gir.id,
			gir.github_installation_id,
			gir.github_repository_id,
			gir.name,
			gir.owner_login,
			gir.full_name,
			gir.private,
			gir.default_branch,
			gir.installation_status,
			CASE
				WHEN gir.linked_repository_id IS NULL THEN gir.selection_status
				WHEN repo.is_active = FALSE THEN 'not_selected'
				WHEN latest_job.status IN ('pending', 'running') THEN 'syncing'
				WHEN latest_job.status = 'failed' THEN 'sync_failed'
				WHEN latest_job.status = 'completed' THEN 'synced'
				ELSE 'selected'
			END AS selection_status,
			gir.linked_repository_id,
			COALESCE(latest_job.error_message, gir.sync_error_message) AS sync_error_message
		FROM github_installation_repositories gir
		LEFT JOIN repositories repo ON repo.id = gir.linked_repository_id
		LEFT JOIN LATERAL (
			SELECT sj.status, sj.error_message
			FROM sync_jobs sj
			WHERE sj.repository_id = gir.linked_repository_id
			ORDER BY sj.created_at DESC, sj.id DESC
			LIMIT 1
		) latest_job ON gir.linked_repository_id IS NOT NULL
		WHERE gir.github_installation_id = $1 AND gir.github_repository_id = ANY($2)`,
		parseUUID(installation.ID),
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("get accessible repositories: %w", err)
	}
	defer rows.Close()

	items := make([]accessibleRepositoryRecord, 0)
	for rows.Next() {
		rec, err := scanAccessibleRepository(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, rec)
	}
	return items, nil
}

func (r *Repository) LinkRepositoryAndMarkSelected(ctx context.Context, organizationID string, githubRepositoryID int64, autoSync bool) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin link repository: %w", err)
	}
	defer tx.Rollback(ctx)

	var existingID pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT linked_repository_id FROM github_installation_repositories gir JOIN github_installations gi ON gi.id = gir.github_installation_id WHERE gi.organization_id = $1 AND gir.github_repository_id = $2`,
		parseUUID(organizationID), githubRepositoryID).Scan(&existingID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("load linked repository id: %w", err)
	}

	var repositoryID pgtype.UUID
	var installationRepositoryID pgtype.UUID
	if existingID.Valid {
		repositoryID = existingID
	} else {
		var githubRepoID int64
		var name, fullName string
		var defaultBranch pgtype.Text
		err = tx.QueryRow(ctx, `SELECT gir.id, gir.github_repository_id, gir.name, gir.full_name, gir.default_branch FROM github_installation_repositories gir JOIN github_installations gi ON gi.id = gir.github_installation_id WHERE gi.organization_id = $1 AND gir.github_repository_id = $2`,
			parseUUID(organizationID), githubRepositoryID).Scan(&installationRepositoryID, &githubRepoID, &name, &fullName, &defaultBranch)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", ErrAccessibleRepositoryNotFound
			}
			return "", fmt.Errorf("load accessible repository: %w", err)
		}

		err = tx.QueryRow(ctx, `SELECT id FROM repositories WHERE github_id = $1`, githubRepoID).Scan(&repositoryID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				repositoryID = newUUID()
				if _, err := tx.Exec(ctx, `
					INSERT INTO repositories (id, organization_id, github_installation_repository_id, github_id, name, full_name, default_branch, is_active, initial_sync_status, sync_error_message, created_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE, $8, NULL, NOW())`,
					repositoryID, parseUUID(organizationID), installationRepositoryID, githubRepoID, name, fullName, defaultBranch, selectionStatusValue(autoSync),
				); err != nil {
					if isUniqueViolation(err) {
						if err := tx.QueryRow(ctx, `SELECT id FROM repositories WHERE github_id = $1`, githubRepoID).Scan(&repositoryID); err != nil {
							return "", fmt.Errorf("reload repository after conflict: %w", err)
						}
					} else {
						return "", fmt.Errorf("insert repository: %w", err)
					}
				}
			} else {
				return "", fmt.Errorf("check existing repository: %w", err)
			}
		}
	}
	if !installationRepositoryID.Valid {
		if err := tx.QueryRow(ctx, `SELECT gir.id FROM github_installation_repositories gir JOIN github_installations gi ON gi.id = gir.github_installation_id WHERE gi.organization_id = $1 AND gir.github_repository_id = $2`,
			parseUUID(organizationID), githubRepositoryID).Scan(&installationRepositoryID); err != nil {
			return "", fmt.Errorf("load installation repository id: %w", err)
		}
	}

	selectionStatus := SelectionStatusSelected
	if autoSync {
		selectionStatus = SelectionStatusSyncing
	}

	if _, err := tx.Exec(ctx, `
		UPDATE repositories
		SET github_installation_repository_id = $2,
		    initial_sync_status = $3,
		    initial_sync_completed_at = NULL,
		    sync_error_message = NULL,
		    updated_at = NOW()
		WHERE id = $1`,
		repositoryID,
		installationRepositoryID,
		selectionStatus,
	); err != nil {
		return "", fmt.Errorf("update repository installation link: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE github_installation_repositories gir
		SET linked_repository_id = $1, selection_status = $2, updated_at = NOW(), sync_error_message = NULL
		FROM github_installations gi
		WHERE gi.id = gir.github_installation_id AND gi.organization_id = $3 AND gir.github_repository_id = $4`,
		repositoryID, selectionStatus, parseUUID(organizationID), githubRepositoryID); err != nil {
		return "", fmt.Errorf("update installation repository selection: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit repository selection: %w", err)
	}

	return repositoryID.String(), nil
}

func selectionStatusValue(autoSync bool) string {
	if autoSync {
		return SelectionStatusSyncing
	}
	return SelectionStatusSelected
}

func (r *Repository) FindInstallationIDByRepositoryFullName(ctx context.Context, fullName string) (*int64, error) {
	var installationID int64
	err := r.db.Pool().QueryRow(ctx, `
		SELECT gi.installation_id
		FROM github_installation_repositories gir
		JOIN github_installations gi ON gi.id = gir.github_installation_id
		WHERE gir.full_name = $1 AND gir.installation_status = 'accessible'
		ORDER BY gi.updated_at DESC NULLS LAST, gi.installed_at DESC
		LIMIT 1`, fullName).Scan(&installationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find installation by repository full name: %w", err)
	}
	return &installationID, nil
}

func scanAccessibleRepository(row pgx.Row) (accessibleRepositoryRecord, error) {
	var rec accessibleRepositoryRecord
	var id, installationDBID, linkedRepositoryID pgtype.UUID
	var defaultBranch, syncError pgtype.Text
	if err := row.Scan(&id, &installationDBID, &rec.GithubRepositoryID, &rec.Name, &rec.OwnerLogin, &rec.FullName, &rec.Private, &defaultBranch, &rec.InstallationStatus, &rec.SelectionStatus, &linkedRepositoryID, &syncError); err != nil {
		return accessibleRepositoryRecord{}, fmt.Errorf("scan accessible repository: %w", err)
	}
	rec.ID = id.String()
	rec.InstallationDBID = installationDBID.String()
	rec.DefaultBranch = optionalText(defaultBranch)
	if linkedRepositoryID.Valid {
		value := linkedRepositoryID.String()
		rec.LinkedRepositoryID = &value
	}
	rec.LastSyncError = optionalText(syncError)
	return rec, nil
}

func toAccessibleResponse(rec accessibleRepositoryRecord) AccessibleRepositoryResponse {
	return AccessibleRepositoryResponse{
		GithubRepositoryID: rec.GithubRepositoryID,
		FullName:           rec.FullName,
		Name:               rec.Name,
		OwnerLogin:         rec.OwnerLogin,
		Private:            rec.Private,
		DefaultBranch:      rec.DefaultBranch,
		InstallationStatus: rec.InstallationStatus,
		SelectionStatus:    rec.SelectionStatus,
		LinkedRepositoryID: rec.LinkedRepositoryID,
		LastSyncError:      rec.LastSyncError,
	}
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

func optionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	utc := value.Time.UTC()
	return &utc
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func textValue(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: *value != ""}
}

func toNullableTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
