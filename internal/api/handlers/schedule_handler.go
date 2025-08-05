package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/isdelr/ender-deploy-be/internal/models"
	"github.com/isdelr/ender-deploy-be/internal/services"
)

// ScheduleHandler handles HTTP requests related to server schedules.
type ScheduleHandler struct {
	service services.ScheduleServiceProvider
}

// NewScheduleHandler creates a new ScheduleHandler.
func NewScheduleHandler(service services.ScheduleServiceProvider) *ScheduleHandler {
	return &ScheduleHandler{service: service}
}

// GetAllForServer handles the request to get all schedules for a server.
func (h *ScheduleHandler) GetAllForServer(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	schedules, err := h.service.GetSchedulesForServer(serverID)
	if err != nil {
		http.Error(w, "Failed to retrieve schedules: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schedules)
}

// Create handles the request to create a new schedule.
func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	var schedule models.Schedule
	if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	schedule.ID = uuid.New().String()
	schedule.ServerID = serverID

	newSchedule, err := h.service.CreateSchedule(schedule)
	if err != nil {
		http.Error(w, "Failed to create schedule: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newSchedule)
}

// Update handles the request to update an existing schedule.
func (h *ScheduleHandler) Update(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "scheduleId")
	var schedule models.Schedule
	if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	updatedSchedule, err := h.service.UpdateSchedule(scheduleID, schedule)
	if err != nil {
		http.Error(w, "Failed to update schedule: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedSchedule)
}

// Delete handles the request to delete a schedule.
func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "scheduleId")
	err := h.service.DeleteSchedule(scheduleID)
	if err != nil {
		http.Error(w, "Failed to delete schedule: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
