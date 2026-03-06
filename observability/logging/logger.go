package logging

import (
	"io"
	"os"

	"github.com/rs/zerolog"
)

// Logger defines the interface for structured logging
type Logger interface {
	// Debug logs a debug-level message with key-value pairs
	Debug(message string, args ...interface{})

	// Info logs an info-level message with key-value pairs
	Info(message string, args ...interface{})

	// Warn logs a warn-level message with key-value pairs
	Warn(message string, args ...interface{})

	// Error logs an error-level message with key-value pairs
	Error(message string, args ...interface{})
}

// NoOpLogger is a logger that does nothing
type NoOpLogger struct{}

// Debug logs nothing
func (n *NoOpLogger) Debug(message string, args ...interface{}) {}

// Info logs nothing
func (n *NoOpLogger) Info(message string, args ...interface{}) {}

// Warn logs nothing
func (n *NoOpLogger) Warn(message string, args ...interface{}) {}

// Error logs nothing
func (n *NoOpLogger) Error(message string, args ...interface{}) {}

// ZerologLogger is a structured logger using zerolog
type ZerologLogger struct {
	logger zerolog.Logger
}

// ZerologOption is a functional option for ZerologLogger
type ZerologOption func(*ZerologLogger)

// WithOutput sets the output writer
func WithOutput(w io.Writer) ZerologOption {
	return func(zl *ZerologLogger) {
		zl.logger = zerolog.New(w).With().Timestamp().Logger()
	}
}

// WithLevel sets the log level
func WithLevel(level string) ZerologOption {
	return func(zl *ZerologLogger) {
		lvl, err := zerolog.ParseLevel(level)
		if err != nil {
			lvl = zerolog.InfoLevel
		}
		zerolog.SetGlobalLevel(lvl)
	}
}

// NewZerologger creates a new ZerologLogger with options
func NewZerologger(opts ...ZerologOption) *ZerologLogger {
	zl := &ZerologLogger{
		logger: zerolog.New(os.Stderr).With().Timestamp().Logger(),
	}

	for _, opt := range opts {
		opt(zl)
	}

	return zl
}

// Debug logs a debug-level message
func (zl *ZerologLogger) Debug(message string, args ...interface{}) {
	zl.logger.Debug().Fields(parseArgs(args)).Msg(message)
}

// Info logs an info-level message
func (zl *ZerologLogger) Info(message string, args ...interface{}) {
	zl.logger.Info().Fields(parseArgs(args)).Msg(message)
}

// Warn logs a warn-level message
func (zl *ZerologLogger) Warn(message string, args ...interface{}) {
	zl.logger.Warn().Fields(parseArgs(args)).Msg(message)
}

// Error logs an error-level message
func (zl *ZerologLogger) Error(message string, args ...interface{}) {
	zl.logger.Error().Fields(parseArgs(args)).Msg(message)
}

// parseArgs converts key-value pairs to a map
func parseArgs(args []interface{}) map[string]interface{} {
	fields := make(map[string]interface{})

	for i := 0; i < len(args)-1; i += 2 {
		if key, ok := args[i].(string); ok {
			fields[key] = args[i+1]
		}
	}

	return fields
}
