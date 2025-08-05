package models

import "time"

// User represents a user account in the system.
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // Never expose this to the client
	CreatedAt    time.Time `json:"createdAt"`
}
