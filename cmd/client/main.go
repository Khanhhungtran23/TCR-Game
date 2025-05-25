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
	help       = flag.Bool("help", false, "Show help information")
	ver        = flag.Bool("version", false, "Show version information")
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

// showHelp displays help information
func showHelp() {
	fmt.Printf(`Clash Royale TCR Client v%s

USAGE:
    %s [OPTIONS]

OPTIONS:
    -server string       Server address to connect to (default "localhost:8080")
    -log-level string    Set log level (DEBUG, INFO, WARN, ERROR) (default "INFO")
    -log-file string     Set log file path (optional)
    -help               Show this help message
    -version            Show version information

EXAMPLES:
    # Start client with default settings
    %s

    # Connect to specific server
    %s -server "192.168.1.100:8080"

    # Start with debug logging
    %s -log-level DEBUG

    # Start with custom log file  
    %s -log-file ./my-game.log

    # Connect to remote server with debug logging
    %s -server "game.example.com:8080" -log-level DEBUG

    - Use 'surrender' to forfeit the match
    - Use 'info' to show detailed game information

TROOPS:
    Pawn     - HP: 50,  ATK: 150, DEF: 100, MANA: 3
    Bishop   - HP: 100, ATK: 200, DEF: 150, MANA: 4  
    Rook     - HP: 250, ATK: 200, DEF: 200, MANA: 5
    Knight   - HP: 200, ATK: 300, DEF: 150, MANA: 5
    Prince   - HP: 500, ATK: 400, DEF: 300, MANA: 6
    Queen    - Special: Heals lowest HP tower by 300, MANA: 5

TOWERS:
    King Tower  - HP: 2000, ATK: 500, DEF: 300, CRIT: 10%%
    Guard Tower - HP: 1000, ATK: 300, DEF: 100, CRIT: 5%%

NETWORK:
    The client uses TCP to communicate with the server.
    Authentication is required before gameplay.
    Real-time game events are synchronized between players.

For more information, visit: https://github.com/yourusername/clash-royale-tcr
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

// showVersion displays version information
func showVersion() {
	fmt.Printf(`Clash Royale TCR Client
Version: %s
Build Time: %s
Go Version: go1.21+
Platform: linux/amd64

Features:
- TCP networking with server
- Real-time and turn-based gameplay modes
- JSON-based data persistence
- Colored terminal output
- Authentication system
- Leveling and progression system
- Critical hit mechanics
- Mana management system

Copyright (c) 2024 Clash Royale TCR Team
`, version, buildTime)
}
