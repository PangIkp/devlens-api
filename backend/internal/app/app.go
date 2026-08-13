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
	"time"

	"github.com/PangIkp/devlens/backend/internal/auditlog"
	"github.com/PangIkp/devlens/backend/internal/auth"
	"github.com/PangIkp/devlens/backend/internal/authorization"
	"github.com/PangIkp/devlens/backend/internal/clickhouse"
	"github.com/PangIkp/devlens/backend/internal/config"
	"github.com/PangIkp/devlens/backend/internal/githubapp"
	"github.com/PangIkp/devlens/backend/internal/githubclient"
	"github.com/PangIkp/devlens/backend/internal/githubconnection"
	"github.com/PangIkp/devlens/backend/internal/githubwebhook"
	"github.com/PangIkp/devlens/backend/internal/httpapi"
	"github.com/PangIkp/devlens/backend/internal/insightbus"
	"github.com/PangIkp/devlens/backend/internal/insights"
	"github.com/PangIkp/devlens/backend/internal/metrics"
	"github.com/PangIkp/devlens/backend/internal/metricsbus"
	"github.com/PangIkp/devlens/backend/internal/observability"
	"github.com/PangIkp/devlens/backend/internal/organization"
	"github.com/PangIkp/devlens/backend/internal/organizationmember"
	"github.com/PangIkp/devlens/backend/internal/postgres"
	"github.com/PangIkp/devlens/backend/internal/pullrequest"
	devrepository "github.com/PangIkp/devlens/backend/internal/repository"
	"github.com/PangIkp/devlens/backend/internal/syncjob"
	"github.com/PangIkp/devlens/backend/internal/userprofile"
)

type App struct {
	cfg             config.Config
	logger          *slog.Logger
	server          *http.Server
	postgres        *postgres.DB
	clickhouse      *clickhouse.DB
	worker          *syncjob.Worker
	webhookWorker   *githubwebhook.Worker
	metricsBus      *metricsbus.Client
	metricsConsumer *metricsbus.Consumer
	insightBus      *insightbus.Client
	insightConsumer *insightbus.Consumer
	tracing         *observability.Tracing
	appMetrics      *observability.Metrics
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	logger := newLogger(cfg)
	appMetrics := observability.NewMetrics()

	tracing, err := observability.SetupTracing(ctx, observability.TracingConfig{
		Enabled:          cfg.Tracing.Enabled,
		ServiceName:      cfg.Tracing.ServiceName,
		ExporterEndpoint: cfg.Tracing.ExporterEndpoint,
		Insecure:         cfg.Tracing.Insecure,
		SampleRatio:      cfg.Tracing.SampleRatio,
	})
	if err != nil {
		return nil, fmt.Errorf("setup tracing: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, cfg.Postgres.ConnectTimeout)
	defer cancel()

	postgresDB, err := postgres.Open(connectCtx, cfg.Postgres, appMetrics)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	var clickhouseDB *clickhouse.DB
	if db, err := clickhouse.Open(cfg.ClickHouse, appMetrics); err != nil {
		logger.Warn("clickhouse unavailable during startup", "error", err)
	} else {
		if err := clickhouse.EnsureSchema(ctx, db, cfg.DataLifecycle); err != nil {
			logger.Warn("clickhouse schema initialization failed", "error", err)
			db.Close()
		} else {
			clickhouseDB = db
		}
	}

	organizationRepository := organization.NewRepository(postgresDB)
	organizationService := organization.NewService(organizationRepository)
	authRepository := auth.NewRepository(postgresDB)
	authService := auth.NewService(authRepository, 15*time.Minute, 30*24*time.Hour)
	auditService := auditlog.NewService(postgresDB)
	authHandler := httpapi.NewAuthHandler(authService, auditService)
	authorizationRepository := authorization.NewRepository(postgresDB)
	authorizationService := authorization.NewService(authorizationRepository)
	organizationHandler := httpapi.NewOrganizationHandler(organizationService, authorizationService, auditService)
	userRepository := userprofile.NewRepository(postgresDB)
	userService := userprofile.NewService(userRepository)
	meHandler := httpapi.NewMeHandler(userService)
	organizationMemberRepository := organizationmember.NewRepository(postgresDB)
	organizationMemberService := organizationmember.NewService(organizationMemberRepository)
	organizationMemberHandler := httpapi.NewOrganizationMemberHandler(organizationMemberService, authorizationService, auditService)
	pullRequestRepository := pullrequest.NewRepository(postgresDB)
	pullRequestService := pullrequest.NewService(pullRequestRepository)
	pullRequestHandler := httpapi.NewPullRequestHandler(pullRequestService, authorizationService)
	repositoryStore := devrepository.NewRepository(postgresDB)
	repositoryService := devrepository.NewService(repositoryStore)
	repositoryHandler := httpapi.NewRepositoryHandler(repositoryService, authorizationService, auditService)
	metricsService := metrics.NewService(postgresDB, clickhouseDB, metrics.RuleConfig{
		DefaultDayType:         cfg.Metrics.DefaultDayType,
		HotspotCommitWeight:    cfg.Metrics.HotspotCommitWeight,
		HotspotAdditionsWeight: cfg.Metrics.HotspotAdditionsWeight,
		HotspotDeletionsWeight: cfg.Metrics.HotspotDeletionsWeight,
	})
	metricsHandler := httpapi.NewMetricsHandler(metricsService, authorizationService)
	insightRules := insights.RuleConfig{
		LargePR: insights.LargePRRuleConfig{
			FilesThreshold:              cfg.Insights.LargePRFilesThreshold,
			TotalChangesThreshold:       cfg.Insights.LargePRTotalChangesThreshold,
			HighSeverityFilesThreshold:  cfg.Insights.LargePRHighSeverityFilesThreshold,
			HighSeverityChangeThreshold: cfg.Insights.LargePRHighSeverityChangesThreshold,
		},
		SlowReview: insights.SlowReviewRuleConfig{
			WaitHoursThreshold:             cfg.Insights.SlowReviewWaitHoursThreshold,
			HighSeverityWaitHoursThreshold: cfg.Insights.SlowReviewHighSeverityWaitHours,
		},
		Hotspot: insights.HotspotRuleConfig{
			ScoreThreshold:             cfg.Insights.HotspotScoreThreshold,
			HighSeverityScoreThreshold: cfg.Insights.HotspotHighSeverityScoreThreshold,
			TopFilesLimit:              cfg.Insights.HotspotTopFilesLimit,
		},
		DeploymentFailure: insights.DeploymentFailureRuleConfig{
			MinimumDeployments:      cfg.Insights.DeploymentMinimumCount,
			FailureRateThreshold:    cfg.Insights.DeploymentFailureRateThreshold,
			HighSeverityFailureRate: cfg.Insights.DeploymentHighSeverityFailureRate,
		},
		ReviewConcentration: insights.ReviewConcentrationRuleConfig{
			MinimumReviewCount:         cfg.Insights.ReviewConcentrationMinimumCount,
			ShareThreshold:             cfg.Insights.ReviewConcentrationShareThreshold,
			HighSeverityShareThreshold: cfg.Insights.ReviewConcentrationHighSeverityShare,
		},
		Bottleneck: insights.BottleneckRuleConfig{
			MinimumMergedCount:              cfg.Insights.BottleneckMinimumMergedCount,
			AverageCycleHoursThreshold:      cfg.Insights.BottleneckAverageCycleHoursThreshold,
			HighSeverityCycleHoursThreshold: cfg.Insights.BottleneckHighSeverityCycleHours,
			StaleOpenCountThreshold:         cfg.Insights.BottleneckStaleOpenCountThreshold,
			HighSeverityStaleOpenThreshold:  cfg.Insights.BottleneckHighSeverityStaleOpenCount,
			StaleOpenAgeDays:                cfg.Insights.BottleneckStaleOpenAgeDays,
		},
		AutoReopen: insights.AutoReopenRuleConfig{
			OnReviewed:      cfg.Insights.AutoReopenOnReviewed,
			OnDismissed:     cfg.Insights.AutoReopenOnDismissed,
			MinimumSeverity: cfg.Insights.AutoReopenMinimumSeverity,
		},
		Deduplicate: insights.DeduplicateRuleConfig{
			Enabled: cfg.Insights.DeduplicateEnabled,
			Version: cfg.Insights.DeduplicateVersion,
		},
	}
	insightRepository := insights.NewRepository(postgresDB, insightRules)
	insightService := insights.NewService(insightRepository, insightRules)
	insightHandler := httpapi.NewInsightHandler(insightService, authorizationService, auditService)
	fallbackGitHubClient, err := githubclient.New(githubclient.Config{
		BaseURL:        cfg.GitHub.BaseURL,
		UserAgent:      cfg.GitHub.UserAgent,
		HTTPTimeout:    cfg.GitHub.HTTPTimeout,
		MaxRetries:     cfg.GitHub.MaxRetries,
		InitialBackoff: cfg.GitHub.InitialBackoff,
		MaxBackoff:     cfg.GitHub.MaxBackoff,
		Metrics:        appMetrics,
	}, githubclient.StaticTokenProvider{Value: cfg.GitHub.Token})
	if err != nil {
		return nil, fmt.Errorf("initialize github client: %w", err)
	}
	githubAppClient, err := githubapp.New(cfg.GitHub)
	if err != nil {
		return nil, fmt.Errorf("initialize github app client: %w", err)
	}
	githubConnectionRepository := githubconnection.NewRepository(postgresDB)
	syncGitHubClient := githubapp.NewSyncClient(cfg.GitHub, githubAppClient, githubConnectionRepository, fallbackGitHubClient, appMetrics)
	syncJobRepository := syncjob.NewRepository(postgresDB)
	syncJobService := syncjob.NewService(syncJobRepository, syncGitHubClient, appMetrics)
	syncJobService.ConfigureRateLimitThrottle(cfg.Sync.GitHubRateLimitRemaining)
	syncJobHandler := httpapi.NewSyncJobHandler(syncJobService, authorizationService, auditService)
	syncWorker := syncjob.NewWorker(logger, syncJobRepository, syncJobService, cfg.Sync.WorkerPollInterval, cfg.Sync.WorkerBatchSize, cfg.Sync.WorkerConcurrency, cfg.Sync.JobTimeout, appMetrics)
	githubConnectionService := githubconnection.NewService(githubConnectionRepository, githubAppClient, syncJobService)
	githubConnectionHandler := httpapi.NewGitHubConnectionHandler(githubConnectionService, authorizationService, auditService)
	webhookRepository := githubwebhook.NewRepository(postgresDB, cfg.DataLifecycle.WebhookPayloadRetentionDays)
	webhookService := githubwebhook.NewService(webhookRepository, cfg.GitHub.WebhookSecret, githubConnectionService, appMetrics)
	webhookService.ConfigureRetryProcessing(cfg.Sync.WebhookRetryConcurrency, cfg.Sync.WebhookRetryTimeout)
	webhookHandler := httpapi.NewGitHubWebhookHandler(webhookService, auditService)
	webhookWorker := githubwebhook.NewWorker(logger, webhookService, cfg.Sync.WebhookRetryInterval, cfg.Sync.WebhookRetryBatchSize, appMetrics)

	var metricsBusClient *metricsbus.Client
	var metricsConsumer *metricsbus.Consumer
	var insightBusClient *insightbus.Client
	var insightConsumer *insightbus.Consumer
	if clickhouseDB != nil {
		if client, err := metricsbus.Open(cfg.NATS.URL, appMetrics); err != nil {
			logger.Warn("metrics bus unavailable during startup", "error", err)
		} else {
			metricsBusClient = client
			metricsConsumer = metricsbus.NewConsumer(logger, client, metricsService)
		}
		if client, err := insightbus.Open(cfg.NATS.URL, appMetrics); err != nil {
			logger.Warn("insight bus unavailable during startup", "error", err)
		} else {
			insightBusClient = client
			insightConsumer = insightbus.NewConsumer(logger, client, insightService)
		}
		syncJobService.SetCompletionPublisher(syncjob.NewCompositePublisher(
			metricsbus.NewPublisher(logger, metricsBusClient, metricsService),
			insightbus.NewPublisher(logger, insightBusClient),
		))
	}

	var clickhouseHealthChecker httpapi.ClickHouseHealthChecker
	if clickhouseDB != nil {
		clickhouseHealthChecker = clickhouseDB
	}

	handler := httpapi.NewRouter(logger, httpapi.Dependencies{
		Postgres:            postgresDB,
		ClickHouse:          clickhouseHealthChecker,
		AppMetrics:          appMetrics,
		AllowedOrigins:      cfg.HTTP.AllowedOrigins,
		RateLimitRequests:   cfg.HTTP.RateLimit.Requests,
		RateLimitWindow:     cfg.HTTP.RateLimit.Window,
		Auth:                authHandler,
		Authenticator:       authService,
		Me:                  meHandler,
		Organizations:       organizationHandler,
		OrganizationMembers: organizationMemberHandler,
		GitHubConnections:   githubConnectionHandler,
		PullRequests:        pullRequestHandler,
		Repositories:        repositoryHandler,
		Metrics:             metricsHandler,
		Insights:            insightHandler,
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
		webhookWorker:   webhookWorker,
		metricsBus:      metricsBusClient,
		metricsConsumer: metricsConsumer,
		insightBus:      insightBusClient,
		insightConsumer: insightConsumer,
		tracing:         tracing,
		appMetrics:      appMetrics,
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
	if a.webhookWorker != nil {
		go a.webhookWorker.Run(ctx)
	}
	if a.metricsConsumer != nil {
		go func() {
			if err := a.metricsConsumer.Run(ctx); err != nil {
				a.logger.Error("metrics consumer stopped", "error", err)
			}
		}()
	}
	if a.insightConsumer != nil {
		go func() {
			if err := a.insightConsumer.Run(ctx); err != nil {
				a.logger.Error("insight consumer stopped", "error", err)
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
	if a.insightBus != nil {
		a.insightBus.Close()
	}
	if a.tracing != nil {
		_ = a.tracing.Shutdown(context.Background())
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
