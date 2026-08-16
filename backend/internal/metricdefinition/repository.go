package metricdefinition

import (
	"context"
	"errors"
	"fmt"

	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/PangIkp/devlens/backend/internal/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
)

type Repository struct {
	queries *sqlcgen.Queries
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{queries: db.Queries()}
}

func (r *Repository) LoadMetricsRuleConfig(ctx context.Context, defaults metrics.RuleConfig) (metrics.RuleConfig, error) {
	row, err := r.queries.GetActiveMetricDefinitionByKey(ctx, MetricKeyRepositoryMetrics)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaults, nil
		}
		return metrics.RuleConfig{}, fmt.Errorf("get active metric definition: %w", err)
	}

	cfg, err := decodeRepositoryMetricsRuleConfig(row.ConfigJson, defaults)
	if err != nil {
		return metrics.RuleConfig{}, err
	}

	return cfg, nil
}
