package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gopost/integration/internal/config"
	"github.com/gopost/integration/internal/integration"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	service, err := integration.NewService(cfg)
	if err != nil {
		log.Fatalf("Failed to create integration service: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	log.Println("Starting integration service...")
	if err := service.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("Service error: %v", err)
	}

	log.Println("Service stopped")
}
