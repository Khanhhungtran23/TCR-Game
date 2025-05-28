// Clash Royale TCR Server - Main Entry Point
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tcr-game/internal/game"
	"tcr-game/internal/server"
	"tcr-game/pkg/logger"
)

var (
	version   = "1.0.0"
	buildTime = "dev"
	port      = flag.String("port", "8080", "Server port")
	host      = flag.String("host", "localhost", "Server host")
	dataDir   = flag.String("data-dir", "data", "Data directory path")
	logLevel  = flag.String("log-level", "INFO", "Log level (DEBUG, INFO, WARN, ERROR)")
	logFile   = flag.String("log-file", "", "Log file path (optional)")
)

func main() {
	flag.Parse()

	// Initialize logging
	if err := initLogging(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}

	logger.Server.Info("Starting Clash Royale TCR Server v%s", version)

	// Initialize data manager
	dataManager := game.NewDataManager(*dataDir)
	if err := dataManager.Initialize(); err != nil {
		logger.Server.Fatal("Failed to initialize data manager: %v", err)
	}

	logger.Server.Info("Data manager initialized successfully")

	// Create server
	address := fmt.Sprintf("%s:%s", *host, *port)
	gameServer := server.NewServer(address, dataManager)

	// Setup graceful shutdown
	setupGracefulShutdown(gameServer)

	// Start server
	logger.Server.Info("Starting server on %s", address)
	if err := gameServer.Start(); err != nil {
		logger.Server.Fatal("Server failed to start: %v", err)
	}
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
		if err := logger.Server.SetFile(*logFile); err != nil {
			return fmt.Errorf("failed to set log file: %w", err)
		}
		logger.Server.Info("Logging to file: %s", *logFile)
	} else {
		// Initialize default file logging
		if err := logger.InitializeFileLogging("./logs"); err != nil {
			// Don't fail if we can't create log directory, just log to console
			logger.Server.Warn("Could not initialize file logging: %v", err)
		}
	}

	return nil
}

// setupGracefulShutdown handles graceful shutdown on interrupt signals
func setupGracefulShutdown(gameServer *server.Server) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		logger.Server.Info("Received shutdown signal, stopping server...")
		gameServer.Stop()
		os.Exit(0)
	}()
}
