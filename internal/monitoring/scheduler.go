package monitoring

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/isdelr/ender-deploy-be/internal/models"
	"github.com/isdelr/ender-deploy-be/internal/services"
	"github.com/robfig/cron/v3"
)

// Scheduler checks for and executes scheduled tasks.
type Scheduler struct {
	scheduleSvc services.ScheduleServiceProvider
	serverSvc   services.ServerServiceProvider
	backupSvc   services.BackupServiceProvider
	eventSvc    services.EventServiceProvider
	ticker      *time.Ticker
	done        chan bool
}

// NewScheduler creates a new scheduler instance.
func NewScheduler(scheduleSvc services.ScheduleServiceProvider, serverSvc services.ServerServiceProvider, backupSvc services.BackupServiceProvider, eventSvc services.EventServiceProvider) *Scheduler {
	return &Scheduler{
		scheduleSvc: scheduleSvc,
		serverSvc:   serverSvc,
		backupSvc:   backupSvc,
		eventSvc:    eventSvc,
		done:        make(chan bool),
	}
}

// Run starts the scheduler's ticking loop.
func (s *Scheduler) Run() {
	log.Println("Starting background scheduler...")
	s.ticker = time.NewTicker(1 * time.Minute)
	defer s.ticker.Stop()

	// Run once immediately on start
	s.checkAndRunSchedules()

	for {
		select {
		case <-s.done:
			log.Println("Stopping background scheduler.")
			return
		case <-s.ticker.C:
			s.checkAndRunSchedules()
		}
	}
}

// Stop halts the scheduler.
func (s *Scheduler) Stop() {
	s.done <- true
}

// checkAndRunSchedules queries for due tasks and executes them.
func (s *Scheduler) checkAndRunSchedules() {
	schedules, err := s.scheduleSvc.GetAllActiveSchedules()
	if err != nil {
		log.Printf("Scheduler: Failed to retrieve active schedules: %v", err)
		return
	}

	for _, schedule := range schedules {
		cronSchedule, err := cron.ParseStandard(schedule.CronExpression)
		if err != nil {
			log.Printf("Scheduler: Invalid cron expression for schedule %s: %v", schedule.ID, err)
			continue
		}

		now := time.Now()
		// If NextRunAt is in the past, it's time to run
		if schedule.NextRunAt != nil && now.After(*schedule.NextRunAt) {
			go s.executeTask(schedule) // Run in a goroutine to not block the scheduler

			// Update the times for the next run
			lastRun := now
			nextRun := cronSchedule.Next(now)
			s.scheduleSvc.UpdateScheduleRunTimes(schedule.ID, lastRun, nextRun)
		}
	}
}

// executeTask performs the action defined by the schedule.
func (s *Scheduler) executeTask(schedule models.Schedule) {
	log.Printf("Scheduler: Executing task '%s' for server %s", schedule.Name, schedule.ServerID)
	var err error

	switch schedule.TaskType {
	case "start", "stop", "restart":
		err = s.serverSvc.PerformServerAction(schedule.ServerID, schedule.TaskType)
	case "backup":
		var payload struct {
			Name string `json:"name"`
		}
		if schedule.Payload != nil {
			if json.Unmarshal(schedule.Payload, &payload) != nil {
				// Fallback name if payload is invalid
				payload.Name = "Scheduled Backup"
			}
		} else {
			payload.Name = "Scheduled Backup"
		}
		go s.backupSvc.CreateBackup(schedule.ServerID, payload.Name) // Run backup in its own goroutine
	case "command":
		var payload struct {
			Command string `json:"command"`
		}
		if schedule.Payload == nil || json.Unmarshal(schedule.Payload, &payload) != nil || payload.Command == "" {
			err = fmt.Errorf("invalid or missing command in payload for schedule %s", schedule.ID)
		} else {
			_, err = s.serverSvc.SendCommandToServer(schedule.ServerID, payload.Command)
		}
	default:
		err = fmt.Errorf("unknown task type '%s' for schedule %s", schedule.TaskType, schedule.ID)
	}

	if err != nil {
		log.Printf("Scheduler: Error executing task %s: %v", schedule.ID, err)
		msg := fmt.Sprintf("Scheduled task '%s' failed to execute: %v", schedule.Name, err)
		s.eventSvc.CreateEvent("schedule.execute.fail", "error", msg, &schedule.ServerID)
	} else {
		msg := fmt.Sprintf("Scheduled task '%s' executed successfully.", schedule.Name)
		s.eventSvc.CreateEvent("schedule.execute.success", "info", msg, &schedule.ServerID)
	}
}
