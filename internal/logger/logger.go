// internal/logger/logger.go
package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Init() {
	// Use ConsoleWriter for human-readable, colorized output in development
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// Set a global log level
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Add a hook to include the caller's file and line number
	log.Logger = log.With().Caller().Logger()
}
