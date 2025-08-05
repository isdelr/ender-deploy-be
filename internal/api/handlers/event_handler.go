package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/isdelr/ender-deploy-be/internal/services"
	"github.com/rs/zerolog/log"
)

// EventHandler handles HTTP requests related to system events.
type EventHandler struct {
	service services.EventServiceProvider
}

// NewEventHandler creates a new EventHandler.
func NewEventHandler(service services.EventServiceProvider) *EventHandler {
	return &EventHandler{service: service}
}

// GetRecent handles the request to get recent activity/events.
func (h *EventHandler) GetRecent(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20 // Default limit
	}

	events, err := h.service.GetRecentEvents(limit)
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve events")
		http.Error(w, "Failed to retrieve events: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}
