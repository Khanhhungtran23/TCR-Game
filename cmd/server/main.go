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
	help      = flag.Bool("help", false, "Show help information")
	ver       = flag.Bool("version", false, "Show version information")
)

func main() {
	flag.Parse()

	// Show help
	if *help {
		showHelp()
		return
	}

	// Show version
	if *ver {
		showVersion()
		return
	}

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

// showHelp displays help information
func showHelp() {
	fmt.Printf(`Clash Royale TCR Server v%s

USAGE:
    %s [OPTIONS]

OPTIONS:
    -port string         Server port (default "8080")
    -host string         Server host (default "localhost")
    -data-dir string     Data directory path (default "./data")
    -log-level string    Set log level (DEBUG, INFO, WARN, ERROR) (default "INFO")
    -log-file string     Set log file path (optional)
    -help               Show this help message
    -version            Show version information

EXAMPLES:
    # Start server with default settings
    %s

    # Start on specific port
    %s -port 9000

    # Start on all interfaces
    %s -host 0.0.0.0 -port 8080

    # Start with debug logging
    %s -log-level DEBUG

    # Start with custom data directory
    %s -data-dir /var/game/data

    # Production setup
    %s -host 0.0.0.0 -port 8080 -log-level WARN -log-file /var/log/tcr-server.log

SERVER FEATURES:
    - TCP socket server for client connections
    - Player authentication and registration
    - Real-time matchmaking system
    - Game state management for multiple concurrent games
    - JSON-based data persistence
    - Comprehensive logging and monitoring

GAME MODES:
    Simple TCR:
    - Turn-based gameplay
    - Players take turns summoning troops
    - Must destroy Guard Towers before King Tower

    Enhanced TCR:
    - Real-time gameplay (3 minutes)
    - Continuous mana regeneration
    - Critical hit system
    - Time-based win conditions

NETWORK PROTOCOL:
    - TCP-based client-server communication
    - JSON message format
    - Real-time event broadcasting
    - Connection health monitoring

ADMINISTRATION:
    - Player data stored in JSON files
    - Game specifications configurable via JSON
    - Automatic cleanup of inactive connections
    - Graceful shutdown handling

For more information, visit: https://github.com/yourusername/clash-royale-tcr
`, version, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

// showVersion displays version information
func showVersion() {
	fmt.Printf(`Clash Royale TCR Server
Version: %s
Build Time: %s
Go Version: go1.21+
Platform: linux/amd64

Server Features:
- TCP networking with clients
- Real-time and turn-based gameplay modes
- JSON-based data persistence
- Player authentication system
- Matchmaking and game management
- Comprehensive logging system

Copyright (c) 2024 Clash Royale TCR Team
`, version, buildTime)
}
