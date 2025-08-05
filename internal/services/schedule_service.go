package services

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/isdelr/ender-deploy-be/internal/models"
	"github.com/robfig/cron/v3"
)

// ScheduleServiceProvider defines the interface for schedule services.
type ScheduleServiceProvider interface {
	CreateSchedule(schedule models.Schedule) (models.Schedule, error)
	GetSchedulesForServer(serverID string) ([]models.Schedule, error)
	GetScheduleByID(scheduleID string) (models.Schedule, error)
	GetAllActiveSchedules() ([]models.Schedule, error)
	UpdateSchedule(scheduleID string, schedule models.Schedule) (models.Schedule, error)
	DeleteSchedule(scheduleID string) error
	UpdateScheduleRunTimes(scheduleID string, lastRun time.Time, nextRun time.Time) error
}

// ScheduleService provides business logic for schedule management.
type ScheduleService struct {
	db           *sql.DB
	eventService EventServiceProvider
}

// NewScheduleService creates a new ScheduleService.
func NewScheduleService(db *sql.DB, eventService EventServiceProvider) *ScheduleService {
	return &ScheduleService{
		db:           db,
		eventService: eventService,
	}
}

// validateCronExpression checks if a cron expression is valid.
func (s *ScheduleService) validateCronExpression(spec string) (cron.Schedule, error) {
	return cron.ParseStandard(spec)
}

// CreateSchedule creates a new schedule and saves it to the database.
func (s *ScheduleService) CreateSchedule(schedule models.Schedule) (models.Schedule, error) {
	cronSchedule, err := s.validateCronExpression(schedule.CronExpression)
	if err != nil {
		return models.Schedule{}, fmt.Errorf("invalid cron expression: %w", err)
	}

	schedule.PrepareForDB()
	nextRun := cronSchedule.Next(time.Now())
	schedule.NextRunAt = &nextRun

	stmt, err := s.db.Prepare(`
		INSERT INTO schedules (id, server_id, name, cron_expression, task_type, payload_json, is_active, next_run_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return models.Schedule{}, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(schedule.ID, schedule.ServerID, schedule.Name, schedule.CronExpression, schedule.TaskType, schedule.PayloadJSON, schedule.IsActive, schedule.NextRunAt)
	if err != nil {
		return models.Schedule{}, err
	}

	s.eventService.CreateEvent("schedule.create", "info", fmt.Sprintf("Schedule '%s' created for server.", schedule.Name), &schedule.ServerID)
	return s.GetScheduleByID(schedule.ID)
}

// GetSchedulesForServer retrieves all schedules for a specific server.
func (s *ScheduleService) GetSchedulesForServer(serverID string) ([]models.Schedule, error) {
	rows, err := s.db.Query(`
		SELECT id, server_id, name, cron_expression, task_type, payload_json, is_active, last_run_at, next_run_at, created_at 
		FROM schedules WHERE server_id = ? ORDER BY created_at DESC`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanSchedules(rows)
}

// GetScheduleByID retrieves a single schedule by its ID.
func (s *ScheduleService) GetScheduleByID(scheduleID string) (models.Schedule, error) {
	row := s.db.QueryRow(`
		SELECT id, server_id, name, cron_expression, task_type, payload_json, is_active, last_run_at, next_run_at, created_at 
		FROM schedules WHERE id = ?`, scheduleID)
	return s.scanSchedule(row)
}

// GetAllActiveSchedules retrieves all active schedules from the database.
func (s *ScheduleService) GetAllActiveSchedules() ([]models.Schedule, error) {
	rows, err := s.db.Query(`
		SELECT id, server_id, name, cron_expression, task_type, payload_json, is_active, last_run_at, next_run_at, created_at 
		FROM schedules WHERE is_active = TRUE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanSchedules(rows)
}

// UpdateSchedule updates an existing schedule.
func (s *ScheduleService) UpdateSchedule(scheduleID string, schedule models.Schedule) (models.Schedule, error) {
	cronSchedule, err := s.validateCronExpression(schedule.CronExpression)
	if err != nil {
		return models.Schedule{}, fmt.Errorf("invalid cron expression: %w", err)
	}

	existing, err := s.GetScheduleByID(scheduleID)
	if err != nil {
		return models.Schedule{}, err
	}

	schedule.PrepareForDB()
	nextRun := cronSchedule.Next(time.Now())
	schedule.NextRunAt = &nextRun

	stmt, err := s.db.Prepare(`
		UPDATE schedules 
		SET name = ?, cron_expression = ?, task_type = ?, payload_json = ?, is_active = ?, next_run_at = ?
		WHERE id = ?
	`)
	if err != nil {
		return models.Schedule{}, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(schedule.Name, schedule.CronExpression, schedule.TaskType, schedule.PayloadJSON, schedule.IsActive, schedule.NextRunAt, scheduleID)
	if err != nil {
		return models.Schedule{}, err
	}

	s.eventService.CreateEvent("schedule.update", "info", fmt.Sprintf("Schedule '%s' updated.", schedule.Name), &existing.ServerID)
	return s.GetScheduleByID(scheduleID)
}

// DeleteSchedule removes a schedule from the database.
func (s *ScheduleService) DeleteSchedule(scheduleID string) error {
	schedule, err := s.GetScheduleByID(scheduleID)
	if err != nil {
		return fmt.Errorf("could not find schedule to delete: %w", err)
	}

	_, err = s.db.Exec("DELETE FROM schedules WHERE id = ?", scheduleID)
	if err == nil {
		s.eventService.CreateEvent("schedule.delete", "warn", fmt.Sprintf("Schedule '%s' was deleted.", schedule.Name), &schedule.ServerID)
	}
	return err
}

// UpdateScheduleRunTimes updates the last and next run times for a schedule after it executes.
func (s *ScheduleService) UpdateScheduleRunTimes(scheduleID string, lastRun time.Time, nextRun time.Time) error {
	_, err := s.db.Exec("UPDATE schedules SET last_run_at = ?, next_run_at = ? WHERE id = ?", lastRun, nextRun, scheduleID)
	return err
}

// scanSchedules is a helper function to scan multiple rows into a slice of Schedules.
func (s *ScheduleService) scanSchedules(rows *sql.Rows) ([]models.Schedule, error) {
	var schedules []models.Schedule
	for rows.Next() {
		schedule, err := s.scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, schedule)
	}
	return schedules, nil
}

// scanSchedule is a helper function to scan a single row into a Schedule struct.
func (s *ScheduleService) scanSchedule(scanner interface{ Scan(...interface{}) error }) (models.Schedule, error) {
	var schedule models.Schedule
	var payloadJSON sql.NullString
	err := scanner.Scan(
		&schedule.ID,
		&schedule.ServerID,
		&schedule.Name,
		&schedule.CronExpression,
		&schedule.TaskType,
		&payloadJSON,
		&schedule.IsActive,
		&schedule.LastRunAt,
		&schedule.NextRunAt,
		&schedule.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.Schedule{}, fmt.Errorf("schedule not found")
		}
		return models.Schedule{}, err
	}
	if payloadJSON.Valid {
		schedule.PayloadJSON = payloadJSON.String
	}
	schedule.PrepareForAPI()
	return schedule, nil
}
