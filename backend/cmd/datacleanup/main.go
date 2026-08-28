package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/PangIkp/devlens/backend/internal/clickhouse"
	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/datalifecycle"
	"github.com/PangIkp/devlens/backend/internal/postgres"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*cfg.Postgres.ConnectTimeout)
	defer cancel()

	pg, err := postgres.Open(ctx, cfg.Postgres, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open postgres: %v\n", err)
		os.Exit(1)
	}
	defer pg.Close()

	var ch *clickhouse.DB
	if cfg.ClickHouse.DSN != "" {
		if db, err := clickhouse.Open(cfg.ClickHouse, nil); err == nil {
			ch = db
			defer ch.Close()
			if err := clickhouse.EnsureSchema(ctx, ch, cfg.DataLifecycle); err != nil {
				fmt.Fprintf(os.Stderr, "ensure clickhouse schema: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Fprintf(os.Stderr, "open clickhouse: %v\n", err)
			os.Exit(1)
		}
	}

	service := datalifecycle.NewService(pg, ch, cfg.DataLifecycle)
	result, err := service.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "run data cleanup: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf(
		"data cleanup complete cleaned_webhook_payloads=%d deleted_organizations=%d deleted_installations=%d purged_analytics_repositories=%d expired_analytics_raw_data_organizations=%d at=%s\n",
		result.CleanedWebhookPayloads,
		result.DeletedOrganizations,
		result.DeletedInstallations,
		result.PurgedAnalyticsRepoCount,
		result.ExpiredAnalyticsRawDataOrgCount,
		time.Now().UTC().Format(time.RFC3339),
	)
}
