package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/PangIkp/devlens/backend/internal/app"
	"github.com/PangIkp/devlens/backend/internal/buildinfo"
	"github.com/PangIkp/devlens/backend/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).Error("load config", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	logger.Info("build info", "version", buildinfo.Version, "commit", buildinfo.Commit, "build_time", buildinfo.BuildTime)

	application, err := app.New(context.Background(), cfg)
	if err != nil {
		logger.Error("initialize application", "error", err)
		os.Exit(1)
	}

	if err := application.Run(context.Background()); err != nil {
		application.Logger().Error("application stopped with error", "error", err)
		os.Exit(1)
	}
}
