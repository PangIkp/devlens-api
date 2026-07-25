package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/PangIkp/devlens/backend/internal/app"
	"github.com/PangIkp/devlens/backend/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("load config", "error", err)
		os.Exit(1)
	}

	application, err := app.New(context.Background(), cfg)
	if err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("initialize application", "error", err)
		os.Exit(1)
	}

	if err := application.Run(context.Background()); err != nil {
		application.Logger().Error("application stopped with error", "error", err)
		os.Exit(1)
	}
}
