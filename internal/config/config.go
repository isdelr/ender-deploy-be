package config

import (
	"os"
	"strconv"
)

// Config holds the application configuration.
type Config struct {
	ServerPort     int
	DatabasePath   string
	ServerDataBase string // Base path for server files
}

// Load loads configuration from environment variables or sets defaults.
func Load() (*Config, error) {
	portStr := getEnv("PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}

	return &Config{
		ServerPort:     port,
		DatabasePath:   getEnv("DATABASE_PATH", "./ender.db"),
		ServerDataBase: getEnv("SERVER_DATA_BASE", "./server-data"),
	}, nil
}

// Helper to get an environment variable with a default value.
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
