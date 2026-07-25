package postgres

import (
	"context"
	"fmt"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/postgres/sqlcgen"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

func Open(ctx context.Context, cfg config.PostgresConfig) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = cfg.HealthCheckPeriod
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	db := &DB{
		pool:    pool,
		queries: sqlcgen.New(pool),
	}

	if err := db.Check(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("verify postgres connection: %w", err)
	}

	return db, nil
}

func (db *DB) Check(ctx context.Context) error {
	if _, err := db.queries.CheckPostgres(ctx); err != nil {
		return fmt.Errorf("run postgres health query: %w", err)
	}

	return nil
}

func (db *DB) Close() {
	if db == nil || db.pool == nil {
		return
	}

	db.pool.Close()
}
