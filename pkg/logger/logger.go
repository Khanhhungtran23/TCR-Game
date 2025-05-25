// Package logger provides a simple logging facility for the application
package logger

import (
	"fmt"
	"io"
	"log" // for logging
	"os"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	// DEBUG level for verbose development information - 0
	DEBUG LogLevel = iota
	// INFO level for general operational information
	INFO
	// WARN level for warning conditions
	WARN
	// ERROR level for error conditions
	ERROR
	// FATAL level for critical errors that cause program termination
	FATAL
)

// String returns the string representation of a LogLevel
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Logger represents a logger with configurable output and log level
type Logger struct {
	level    LogLevel
	prefix   string
	logger   *log.Logger
	mu       sync.Mutex
	logFile  *os.File
	console  bool
	fileName string
}

// New creates a new Logger instance with the specified log level and prefix
func New(level LogLevel, prefix string) *Logger {
	logger := &Logger{
		level:   level,
		prefix:  prefix,
		logger:  log.New(os.Stdout, "", log.LstdFlags),
		console: true,
	}
	return logger
}

// SetOutput sets the output destination for the logger
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.SetOutput(w)
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// SetConsole enables or disables logging to console
func (l *Logger) SetConsole(enable bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.console = enable
	l.updateOutput()
}

// SetFile enables logging to the specified file
func (l *Logger) SetFile(filename string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Close existing file if any
	if l.logFile != nil {
		l.logFile.Close()
		l.logFile = nil
	}

	if filename == "" {
		l.fileName = ""
		l.updateOutput()
		return nil
	}

	// Open new log file
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	l.logFile = f
	l.fileName = filename
	l.updateOutput()
	return nil
}

// updateOutput updates the logger output based on console and file settings
func (l *Logger) updateOutput() {
	var writers []io.Writer
	if l.console {
		writers = append(writers, os.Stdout)
	}
	if l.logFile != nil {
		writers = append(writers, l.logFile)
	}

	if len(writers) == 0 {
		// If no outputs are enabled, use a discard writer
		l.logger.SetOutput(io.Discard)
	} else if len(writers) == 1 {
		// If only one output is enabled, use it directly
		l.logger.SetOutput(writers[0])
	} else {
		// If multiple outputs are enabled, use a multi-writer
		l.logger.SetOutput(io.MultiWriter(writers...))
	}
}

// log logs a message with the specified level if the level is greater than or equal to the logger's level
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Format the log message
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	logMessage := fmt.Sprintf("[%s] [%s] %s: %s", timestamp, level.String(), l.prefix, message)

	// Log the message
	l.logger.Println(logMessage)

	// If fatal, exit the program after logging
	if level == FATAL {
		os.Exit(1)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// Fatal logs a fatal message and exits the program
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, format, args...)
}

// Default logger instances for different components
var (
	Server      = New(INFO, "SERVER")
	Client      = New(INFO, "CLIENT")
	Network     = New(INFO, "NETWORK")
	Game        = New(INFO, "GAME")
	Auth        = New(INFO, "AUTH")
	Persistence = New(INFO, "PERSISTENCE")
)

// InitializeFileLogging sets up file logging for all default loggers
func InitializeFileLogging(directory string) error {
	// Create logs directory if it doesn't exist
	err := os.MkdirAll(directory, 0755)
	if err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Set up file logging for each component
	logFile := fmt.Sprintf("%s/tcr_%s.log", directory, time.Now().Format("2006-01-02"))

	if err := Server.SetFile(logFile); err != nil {
		return err
	}
	if err := Client.SetFile(logFile); err != nil {
		return err
	}
	if err := Network.SetFile(logFile); err != nil {
		return err
	}
	if err := Game.SetFile(logFile); err != nil {
		return err
	}
	if err := Auth.SetFile(logFile); err != nil {
		return err
	}
	if err := Persistence.SetFile(logFile); err != nil {
		return err
	}

	return nil
}

// SetGlobalLogLevel sets the log level for all default loggers
func SetGlobalLogLevel(level LogLevel) {
	Server.SetLevel(level)
	Client.SetLevel(level)
	Network.SetLevel(level)
	Game.SetLevel(level)
	Auth.SetLevel(level)
	Persistence.SetLevel(level)
}
