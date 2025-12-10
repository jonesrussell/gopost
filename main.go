package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gopost/integration/internal/config"
	"github.com/gopost/integration/internal/integration"
	"github.com/gopost/integration/internal/logger"
	"github.com/gopost/integration/internal/sources"
)

var (
	// version can be set at build time via -ldflags
	version = "dev"
)

func initializeLogger(cfg *config.Config) (logger.Logger, error) {
	appLogger, err := logger.NewLogger(cfg.Debug)
	if err != nil {
		return nil, err
	}

	// Add service context fields to all log entries
	appLogger = appLogger.With(
		logger.String("service", "gopost"),
		logger.String("version", version),
	)

	return appLogger, nil
}

func handleFlushCache(service *integration.Service, appLogger logger.Logger) {
	const flushCacheTimeout = 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), flushCacheTimeout)
	defer cancel()

	if err := service.FlushCache(ctx); err != nil {
		appLogger.Error("Failed to flush cache",
			logger.Error(err),
		)
		_ = appLogger.Sync()
		os.Exit(1)
	}

	appLogger.Info("Cache flushed successfully")
	_ = appLogger.Sync()
}

func main() {
	var configPath string
	var flushCache bool
	flag.StringVar(&configPath, "config", "config.yml", "Path to configuration file")
	flag.BoolVar(&flushCache, "flush-cache", false, "Flush Redis deduplication cache and exit")
	flag.Parse()

	// Load configuration first (needed to determine debug mode)
	// Load base config to determine debug mode
	baseCfg, err := config.Load(configPath)
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
	appLogger, err := initializeLogger(baseCfg)
	if err != nil {
		// Fallback to temporary logger if logger creation fails
		tempLogger, _ := logger.NewLogger(true)
		tempLogger.Error("Failed to create logger",
			logger.Error(err),
		)
		_ = tempLogger.Sync()
		os.Exit(1)
	}

	// If sources service is enabled, try to fetch cities from it
	var cfg *config.Config
	if baseCfg.Sources.Enabled {
		sourcesClient := sources.NewClient(&baseCfg.Sources, appLogger)
		cfg, err = config.LoadWithSources(configPath, sourcesClient)
		if err != nil {
			appLogger.Warn("Failed to load config with sources, falling back to config file",
				logger.Error(err),
			)
			cfg = baseCfg
		} else {
			appLogger.Info("Loaded cities from sources service",
				logger.Int("city_count", len(cfg.Cities)),
			)
		}
	} else {
		cfg = baseCfg
	}
	defer func() {
		if syncErr := appLogger.Sync(); syncErr != nil {
			// Can't log this error since logger might be closed
			_ = syncErr
		}
	}()

	// Create integration service with logger
	service, err := integration.NewService(cfg, appLogger)
	if err != nil {
		appLogger.Error("Failed to create integration service",
			logger.Error(err),
		)
		_ = appLogger.Sync()
		os.Exit(1)
	}

	// Handle flush-cache flag
	if flushCache {
		handleFlushCache(service, appLogger)
		return
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

	if runErr := service.Run(ctx); runErr != nil && !errors.Is(runErr, context.Canceled) {
		appLogger.Error("Service error",
			logger.Error(runErr),
		)
		_ = appLogger.Sync()
		os.Exit(1)
	}

	appLogger.Info("Service stopped")
}
