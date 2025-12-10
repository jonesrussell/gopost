package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/gopost/integration/internal/config"
	"github.com/gopost/integration/internal/drupal"
	"github.com/gopost/integration/internal/logger"
)

func main() {
	// Load config
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.yml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create logger
	appLogger, err := logger.NewLogger(cfg.Debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = appLogger.Sync()
	}()

	// Create Drupal client
	client, err := drupal.NewClient(
		cfg.Drupal.URL,
		cfg.Drupal.Username,
		cfg.Drupal.Token,
		cfg.Drupal.AuthMethod,
		cfg.Drupal.SkipTLSVerify,
		appLogger,
	)
	if err != nil {
		appLogger.Error("Failed to create Drupal client", logger.Error(err))
		_ = appLogger.Sync()
		os.Exit(1)
	}

	// List nodes first to get a valid UUID
	appLogger.Info("Listing nodes to find valid UUIDs")

	const defaultLimit = 5
	listResult, err := client.ListNodes(context.Background(), defaultLimit)
	if err != nil {
		appLogger.Error("Failed to list nodes", logger.Error(err))
		_ = appLogger.Sync()
		os.Exit(1)
	}

	// Pretty print the list
	jsonBytes, err := json.MarshalIndent(listResult, "", "  ")
	if err != nil {
		appLogger.Error("Failed to marshal JSON", logger.Error(err))
		_ = appLogger.Sync()
		os.Exit(1)
	}

	fmt.Println("=== Node List ===")
	fmt.Println(string(jsonBytes))

	// If a node ID was provided, try to fetch it
	if len(os.Args) > 1 {
		nodeID := os.Args[1]
		appLogger.Info("Fetching specific node",
			logger.String("node_id", nodeID),
		)

		nodeResult, nodeErr := client.GetNode(context.Background(), nodeID)
		if nodeErr != nil {
			appLogger.Error("Failed to fetch node",
				logger.String("node_id", nodeID),
				logger.Error(nodeErr),
			)
			_ = appLogger.Sync()
			os.Exit(1)
		}

		// Pretty print JSON
		nodeJSON, marshalErr := json.MarshalIndent(nodeResult, "", "  ")
		if marshalErr != nil {
			appLogger.Error("Failed to marshal JSON", logger.Error(marshalErr))
			_ = appLogger.Sync()
			os.Exit(1)
		}

		fmt.Println("\n=== Node Details ===")
		fmt.Println(string(nodeJSON))
	}
}
