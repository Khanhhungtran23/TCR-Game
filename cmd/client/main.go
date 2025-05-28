// Clash Royale TCR Client - Main Entry Point
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tcr-game/internal/client"
	"tcr-game/pkg/logger"
)

var (
	version    = "1.0.0"
	buildTime  = "dev"
	serverAddr = flag.String("server", "localhost:8080", "Server address (host:port)")
	logLevel   = flag.String("log-level", "INFO", "Log level (DEBUG, INFO, WARN, ERROR)")
	logFile    = flag.String("log-file", "", "Log file path (optional)")
)

func main() {
	flag.Parse()

	// Initialize logging
	if err := initLogging(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}

	logger.Client.Info("Starting Clash Royale TCR Client v%s", version)
	logger.Client.Info("Connecting to server: %s", *serverAddr)

	// Create client
	gameClient := client.NewClient(*serverAddr)

	// Setup graceful shutdown
	setupGracefulShutdown(gameClient)

	// Start client
	if err := gameClient.Start(); err != nil {
		logger.Client.Error("Client failed to start: %v", err)
		os.Exit(1)
	}

	logger.Client.Info("Client shutting down gracefully")
}

// initLogging sets up the logging system
func initLogging() error {
	// Set log level
	var level logger.LogLevel
	switch *logLevel {
	case "DEBUG":
		level = logger.DEBUG
	case "INFO":
		level = logger.INFO
	case "WARN":
		level = logger.WARN
	case "ERROR":
		level = logger.ERROR
	default:
		level = logger.INFO
	}

	logger.SetGlobalLogLevel(level)

	// Set up file logging if specified
	if *logFile != "" {
		if err := logger.Client.SetFile(*logFile); err != nil {
			return fmt.Errorf("failed to set log file: %w", err)
		}
		logger.Client.Info("Logging to file: %s", *logFile)
	} else {
		// Initialize default file logging
		if err := logger.InitializeFileLogging("./logs"); err != nil {
			// Don't fail if we can't create log directory, just log to console
			logger.Client.Warn("Could not initialize file logging: %v", err)
		}
	}

	return nil
}

// setupGracefulShutdown handles graceful shutdown on interrupt signals
func setupGracefulShutdown(gameClient *client.Client) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		logger.Client.Info("Received shutdown signal, closing client...")
		gameClient.Close()
		os.Exit(0)
	}()
}
