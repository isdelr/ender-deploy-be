package models

import "time"

// Event represents a loggable action or alert in the system.
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`  // e.g., "server.start", "system.alert.cpu"
	Level     string    `json:"level"` // e.g., "info", "warn", "error"
	Message   string    `json:"message"`
	ServerID  *string   `json:"serverId,omitempty"` // Nullable for system-wide events
	CreatedAt time.Time `json:"createdAt"`
}
