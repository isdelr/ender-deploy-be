package models

import "time"

// Backup represents a backup of a server's data.
type Backup struct {
	ID        string    `json:"id"`
	ServerID  string    `json:"serverId"`
	Name      string    `json:"name"`
	Path      string    `json:"-"` // Internal use, not exposed to client
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}
