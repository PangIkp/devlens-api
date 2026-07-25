package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Postgres.ConnectTimeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.Postgres.ConnectionString())
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	var version int64
	var dirty bool

	err = pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
	if err == nil {
		fmt.Printf("version=%d dirty=%t\n", version, dirty)
		return
	}

	if errors.Is(err, pgx.ErrNoRows) {
		fmt.Println("version=0 dirty=false")
		return
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		fmt.Println("version=0 dirty=false")
		return
	}

	fmt.Fprintf(os.Stderr, "query migration status: %v\n", err)
	os.Exit(1)
}
