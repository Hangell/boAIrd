package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"boAIrd/internal/app"
	"boAIrd/internal/config"
	"boAIrd/internal/logging"
)

func main() {
	logger := slog.New(logging.NewRedactingHandler(slog.NewJSONHandler(os.Stderr, nil)))
	slog.SetDefault(logger)
	if len(os.Args) != 2 || os.Args[1] != "serve" {
		logger.Error("invalid command", "usage", "boaird serve")
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	defer clear(cfg.MasterKey[:])

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg, logger); err != nil {
		logger.Error("boAIrd stopped with an error", "error", err)
		os.Exit(1)
	}
}
