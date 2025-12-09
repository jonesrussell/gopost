package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/gopost/integration/internal/config"
	"github.com/gopost/integration/internal/integration"
	"github.com/gopost/integration/internal/logger"
)

var (
	// version can be set at build time via -ldflags
	version = "dev"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yml", "Path to configuration file")
	flag.Parse()

	// Load configuration first (needed to determine debug mode)
	cfg, err := config.Load(configPath)
	if err != nil {
		// Use a temporary logger for early errors before config is loaded
		tempLogger, _ := logger.NewLogger(true)
		tempLogger.Error("Failed to load config",
			logger.String("config_path", configPath),
			logger.Error(err),
		)
		_ = tempLogger.Sync()
		os.Exit(1)
	}

	// Create logger based on debug mode from config
	appLogger, err := logger.NewLogger(cfg.Debug)
	if err != nil {
		// Fallback to temporary logger if logger creation fails
		tempLogger, _ := logger.NewLogger(true)
		tempLogger.Error("Failed to create logger",
			logger.Error(err),
		)
		_ = tempLogger.Sync()
		os.Exit(1)
	}
	defer func() {
		if err := appLogger.Sync(); err != nil {
			// Can't log this error since logger might be closed
			_ = err
		}
	}()

	// Add service context fields to all log entries
	appLogger = appLogger.With(
		logger.String("service", "gopost"),
		logger.String("version", version),
	)

	// Create integration service with logger
	service, err := integration.NewService(cfg, appLogger)
	if err != nil {
		appLogger.Error("Failed to create integration service",
			logger.Error(err),
		)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		appLogger.Info("Shutting down",
			logger.String("signal", sig.String()),
		)
		cancel()
	}()

	appLogger.Info("Starting integration service",
		logger.String("config_path", configPath),
		logger.Bool("debug", cfg.Debug),
	)

	if err := service.Run(ctx); err != nil && err != context.Canceled {
		appLogger.Error("Service error",
			logger.Error(err),
		)
		os.Exit(1)
	}

	appLogger.Info("Service stopped")
}
