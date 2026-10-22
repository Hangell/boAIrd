package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"boAIrd/internal/api"
	"boAIrd/internal/config"
	"boAIrd/internal/storage"
)

// Run opens service dependencies and serves HTTP until the context is cancelled.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) (result error) {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}

	database, err := storage.OpenSQLite(ctx, cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("initialize storage: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("close SQLite database: %w", err))
		}
	}()

	logger.Info("SQLite database ready", "path", cfg.DatabasePath)
	if err := api.Serve(ctx, cfg.Address(), api.NewHandler(database), logger); err != nil {
		return fmt.Errorf("run HTTP API: %w", err)
	}
	return nil
}
