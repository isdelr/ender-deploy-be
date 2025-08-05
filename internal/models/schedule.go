package models

import (
	"encoding/json"
	"time"
)

// Schedule represents a single automated task for a server.
type Schedule struct {
	ID             string          `json:"id"`
	ServerID       string          `json:"serverId"`
	Name           string          `json:"name"`
	CronExpression string          `json:"cronExpression"` // e.g., "0 4 * * *" for 4 AM daily
	TaskType       string          `json:"taskType"`       // e.g., "restart", "backup", "command"
	PayloadJSON    string          `json:"-"`              // Stored as JSON object string
	Payload        json.RawMessage `json:"payload"`        // Exposed to frontend
	IsActive       bool            `json:"isActive"`
	LastRunAt      *time.Time      `json:"lastRunAt"`
	NextRunAt      *time.Time      `json:"nextRunAt"`
	CreatedAt      time.Time       `json:"createdAt"`
}

// PrepareForDB ensures the payload is correctly marshaled into its JSON string form before saving.
func (s *Schedule) PrepareForDB() {
	if s.Payload != nil {
		s.PayloadJSON = string(s.Payload)
	}
}

// PrepareForAPI ensures the JSON string payload is correctly unmarshaled for API responses.
func (s *Schedule) PrepareForAPI() {
	if s.PayloadJSON != "" {
		s.Payload = []byte(s.PayloadJSON)
	}
}
