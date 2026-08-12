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

	"github.com/PangIkp/devlens/backend/internal/clickhouse"
	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/githubapp"
	"github.com/PangIkp/devlens/backend/internal/githubclient"
	"github.com/PangIkp/devlens/backend/internal/githubconnection"
	"github.com/PangIkp/devlens/backend/internal/githubwebhook"
	"github.com/PangIkp/devlens/backend/internal/httpapi"
	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/PangIkp/devlens/backend/internal/metricsbus"
	"github.com/PangIkp/devlens/backend/internal/organization"
	"github.com/PangIkp/devlens/backend/internal/organizationmember"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	devrepository "github.com/PangIkp/devlens/backend/internal/repository"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
)

type App struct {
	cfg             config.Config
	logger          *slog.Logger
	server          *http.Server
	postgres        *postgres.DB
	clickhouse      *clickhouse.DB
	worker          *syncjob.Worker
	metricsBus      *metricsbus.Client
	metricsConsumer *metricsbus.Consumer
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	logger := newLogger(cfg)

	connectCtx, cancel := context.WithTimeout(ctx, cfg.Postgres.ConnectTimeout)
	defer cancel()

	postgresDB, err := postgres.Open(connectCtx, cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	var clickhouseDB *clickhouse.DB
	if db, err := clickhouse.Open(cfg.ClickHouse); err != nil {
		logger.Warn("clickhouse unavailable during startup", "error", err)
	} else {
		if err := clickhouse.EnsureSchema(ctx, db); err != nil {
			logger.Warn("clickhouse schema initialization failed", "error", err)
			db.Close()
		} else {
			clickhouseDB = db
		}
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
	metricsService := metrics.NewService(postgresDB, clickhouseDB)
	metricsHandler := httpapi.NewMetricsHandler(metricsService)
	fallbackGitHubClient, err := githubclient.New(githubclient.Config{
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
	githubAppClient, err := githubapp.New(cfg.GitHub)
	if err != nil {
		return nil, fmt.Errorf("initialize github app client: %w", err)
	}
	githubConnectionRepository := githubconnection.NewRepository(postgresDB)
	syncGitHubClient := githubapp.NewSyncClient(cfg.GitHub, githubAppClient, githubConnectionRepository, fallbackGitHubClient)
	syncJobRepository := syncjob.NewRepository(postgresDB)
	syncJobService := syncjob.NewService(syncJobRepository, syncGitHubClient)
	syncJobHandler := httpapi.NewSyncJobHandler(syncJobService)
	syncWorker := syncjob.NewWorker(logger, syncJobRepository, syncJobService, cfg.Sync.WorkerPollInterval)
	githubConnectionService := githubconnection.NewService(githubConnectionRepository, githubAppClient, syncJobService)
	githubConnectionHandler := httpapi.NewGitHubConnectionHandler(githubConnectionService)
	webhookRepository := githubwebhook.NewRepository(postgresDB)
	webhookService := githubwebhook.NewService(webhookRepository, cfg.GitHub.WebhookSecret, githubConnectionService)
	webhookHandler := httpapi.NewGitHubWebhookHandler(webhookService)

	var metricsBusClient *metricsbus.Client
	var metricsConsumer *metricsbus.Consumer
	if clickhouseDB != nil {
		if client, err := metricsbus.Open(cfg.NATS.URL); err != nil {
			logger.Warn("metrics bus unavailable during startup", "error", err)
		} else {
			metricsBusClient = client
			metricsConsumer = metricsbus.NewConsumer(logger, client, metricsService)
		}
		syncJobService.SetCompletionPublisher(metricsbus.NewPublisher(logger, metricsBusClient, metricsService))
	}

	var clickhouseHealthChecker httpapi.ClickHouseHealthChecker
	if clickhouseDB != nil {
		clickhouseHealthChecker = clickhouseDB
	}

	handler := httpapi.NewRouter(logger, httpapi.Dependencies{
		Postgres:            postgresDB,
		ClickHouse:          clickhouseHealthChecker,
		Organizations:       organizationHandler,
		OrganizationMembers: organizationMemberHandler,
		GitHubConnections:   githubConnectionHandler,
		Repositories:        repositoryHandler,
		Metrics:             metricsHandler,
		SyncJobs:            syncJobHandler,
		GitHubWebhook:       webhookHandler,
	})

	server := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      handler,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	return &App{
		cfg:             cfg,
		logger:          logger,
		server:          server,
		postgres:        postgresDB,
		clickhouse:      clickhouseDB,
		worker:          syncWorker,
		metricsBus:      metricsBusClient,
		metricsConsumer: metricsConsumer,
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

	if a.worker != nil {
		go a.worker.Run(ctx)
	}
	if a.metricsConsumer != nil {
		go func() {
			if err := a.metricsConsumer.Run(ctx); err != nil {
				a.logger.Error("metrics consumer stopped", "error", err)
			}
		}()
	}

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
	if a.clickhouse != nil {
		a.clickhouse.Close()
	}
	if a.metricsBus != nil {
		a.metricsBus.Close()
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
