package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/githubclient"
	"github.com/PangIkp/devlens/backend/internal/httpapi"
	"github.com/PangIkp/devlens/backend/internal/organization"
	"github.com/PangIkp/devlens/backend/internal/organizationmember"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	devrepository "github.com/PangIkp/devlens/backend/internal/repository"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
)

type App struct {
	cfg      config.Config
	logger   *slog.Logger
	server   *http.Server
	postgres *postgres.DB
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	logger := newLogger(cfg)

	connectCtx, cancel := context.WithTimeout(ctx, cfg.Postgres.ConnectTimeout)
	defer cancel()

	postgresDB, err := postgres.Open(connectCtx, cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	organizationRepository := organization.NewRepository(postgresDB)
	organizationService := organization.NewService(organizationRepository)
	organizationHandler := httpapi.NewOrganizationHandler(organizationService)
	organizationMemberRepository := organizationmember.NewRepository(postgresDB)
	organizationMemberService := organizationmember.NewService(organizationMemberRepository)
	organizationMemberHandler := httpapi.NewOrganizationMemberHandler(organizationMemberService)
	repositoryStore := devrepository.NewRepository(postgresDB)
	repositoryService := devrepository.NewService(repositoryStore)
	repositoryHandler := httpapi.NewRepositoryHandler(repositoryService)
	githubClient, err := githubclient.New(githubclient.Config{
		BaseURL:        cfg.GitHub.BaseURL,
		UserAgent:      cfg.GitHub.UserAgent,
		HTTPTimeout:    cfg.GitHub.HTTPTimeout,
		MaxRetries:     cfg.GitHub.MaxRetries,
		InitialBackoff: cfg.GitHub.InitialBackoff,
		MaxBackoff:     cfg.GitHub.MaxBackoff,
	}, githubclient.StaticTokenProvider{Value: cfg.GitHub.Token})
	if err != nil {
		return nil, fmt.Errorf("initialize github client: %w", err)
	}
	syncJobRepository := syncjob.NewRepository(postgresDB)
	syncJobService := syncjob.NewService(syncJobRepository, githubClient)
	syncJobHandler := httpapi.NewSyncJobHandler(syncJobService)

	handler := httpapi.NewRouter(logger, httpapi.Dependencies{
		Postgres:            postgresDB,
		Organizations:       organizationHandler,
		OrganizationMembers: organizationMemberHandler,
		Repositories:        repositoryHandler,
		SyncJobs:            syncJobHandler,
	})

	server := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      handler,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	return &App{
		cfg:      cfg,
		logger:   logger,
		server:   server,
		postgres: postgresDB,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer a.close()

	serverErr := make(chan error, 1)

	go func() {
		a.logger.Info("http server listening", "addr", a.server.Addr, "env", a.cfg.AppEnv)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.HTTP.ShutdownTimeout)
		defer cancel()

		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown http server: %w", err)
		}

		return <-serverErr
	case err := <-serverErr:
		return err
	}
}

func (a *App) Logger() *slog.Logger {
	return a.logger
}

func (a *App) close() {
	if a.postgres != nil {
		a.postgres.Close()
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
