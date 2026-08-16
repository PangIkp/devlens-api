package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/PangIkp/devlens/backend/internal/clickhouse"
	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/PangIkp/devlens/backend/internal/postgres"
)

func main() {
	var repositoryID string
	var fromValue string
	var toValue string

	flag.StringVar(&repositoryID, "repository-id", "", "repository UUID to rebuild metrics for")
	flag.StringVar(&fromValue, "from", "", "start date in YYYY-MM-DD")
	flag.StringVar(&toValue, "to", "", "end date in YYYY-MM-DD")
	flag.Parse()

	if repositoryID == "" || fromValue == "" || toValue == "" {
		fmt.Fprintln(os.Stderr, "repository-id, from, and to are required")
		os.Exit(2)
	}

	from, err := time.Parse("2006-01-02", fromValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse from: %v\n", err)
		os.Exit(2)
	}
	to, err := time.Parse("2006-01-02", toValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse to: %v\n", err)
		os.Exit(2)
	}

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

	ch, err := clickhouse.Open(cfg.ClickHouse, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open clickhouse: %v\n", err)
		os.Exit(1)
	}
	defer ch.Close()

	if err := clickhouse.EnsureSchema(ctx, ch, cfg.DataLifecycle); err != nil {
		fmt.Fprintf(os.Stderr, "ensure clickhouse schema: %v\n", err)
		os.Exit(1)
	}

	service := metrics.NewService(pg, ch, metrics.RuleConfig{
		DefaultDayType:         cfg.Metrics.DefaultDayType,
		HotspotCommitWeight:    cfg.Metrics.HotspotCommitWeight,
		HotspotAdditionsWeight: cfg.Metrics.HotspotAdditionsWeight,
		HotspotDeletionsWeight: cfg.Metrics.HotspotDeletionsWeight,
	})
	if err := service.CalculateRepositoryMetrics(ctx, repositoryID, metrics.CalculationRequest{
		From:          from.UTC(),
		To:            to.UTC(),
		MetricVersion: metrics.CurrentMetricVersion,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "rebuild metrics: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("rebuild complete repository_id=%s from=%s to=%s metric_version=%d\n", repositoryID, from.UTC().Format("2006-01-02"), to.UTC().Format("2006-01-02"), metrics.CurrentMetricVersion)
}
