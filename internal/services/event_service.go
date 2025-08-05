package services

import (
	"database/sql"

	"github.com/google/uuid"
	"github.com/isdelr/ender-deploy-be/internal/models"
)

// EventServiceProvider defines the interface for event services.
type EventServiceProvider interface {
	CreateEvent(eventType, level, message string, serverID *string) error
	GetRecentEvents(limit int) ([]models.Event, error)
}

// EventService provides business logic for event management.
type EventService struct {
	db *sql.DB
}

// NewEventService creates a new EventService.
func NewEventService(db *sql.DB) *EventService {
	return &EventService{db: db}
}

// CreateEvent logs a new event to the database.
func (s *EventService) CreateEvent(eventType, level, message string, serverID *string) error {
	event := models.Event{
		ID:       uuid.New().String(),
		Type:     eventType,
		Level:    level,
		Message:  message,
		ServerID: serverID,
	}

	stmt, err := s.db.Prepare("INSERT INTO events (id, type, level, message, server_id) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(event.ID, event.Type, event.Level, event.Message, event.ServerID)
	return err
}

// GetRecentEvents retrieves the most recent events from the database.
func (s *EventService) GetRecentEvents(limit int) ([]models.Event, error) {
	rows, err := s.db.Query("SELECT id, type, level, message, server_id, created_at FROM events ORDER BY created_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var event models.Event
		if err := rows.Scan(&event.ID, &event.Type, &event.Level, &event.Message, &event.ServerID, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}
