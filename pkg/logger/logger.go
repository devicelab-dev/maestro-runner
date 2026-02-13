package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

var (
	globalLogger *log.Logger
	logFile      *os.File
	mu           sync.Mutex
)

// Init initializes the global logger with the specified log file path.
func Init(logPath string) error {
	mu.Lock()
	defer mu.Unlock()

	// Close previous log file if exists
	if logFile != nil {
		_ = logFile.Close()
	}

	// Create log file
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	logFile = f
	globalLogger = log.New(f, "", log.Ltime|log.Lmicroseconds)

	return nil
}

// Close closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
}

// Info logs an info message.
func Info(format string, v ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	if globalLogger != nil {
		globalLogger.Printf("[INFO] "+format, v...)
	}
}

// Debug logs a debug message.
func Debug(format string, v ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	if globalLogger != nil {
		globalLogger.Printf("[DEBUG] "+format, v...)
	}
}

// Error logs an error message.
func Error(format string, v ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	if globalLogger != nil {
		globalLogger.Printf("[ERROR] "+format, v...)
	}
}

// Warn logs a warning message.
func Warn(format string, v ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	if globalLogger != nil {
		globalLogger.Printf("[WARN] "+format, v...)
	}
}

// GetWriter returns the underlying writer for use by drivers.
func GetWriter() io.Writer {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		return logFile
	}
	return io.Discard
}
