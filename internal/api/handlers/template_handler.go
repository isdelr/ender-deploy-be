package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/isdelr/ender-deploy-be/internal/models"
	"github.com/isdelr/ender-deploy-be/internal/services"
	"github.com/rs/zerolog/log"
)

// TemplateHandler handles HTTP requests related to templates.
type TemplateHandler struct {
	service services.TemplateServiceProvider
}

// NewTemplateHandler creates a new TemplateHandler.
func NewTemplateHandler(service services.TemplateServiceProvider) *TemplateHandler {
	return &TemplateHandler{service: service}
}

// GetAll handles the request to get all templates.
func (h *TemplateHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	templates, err := h.service.GetAllTemplates()
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve templates")
		http.Error(w, "Failed to retrieve templates", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(templates)
}

// Get handles the request to get a single template by its ID.
func (h *TemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	template, err := h.service.GetTemplateByID(id)
	if err != nil {
		log.Warn().Err(err).Str("template_id", id).Msg("Failed to get template by ID")
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(template)
}

// Create handles the request to create a new template.
func (h *TemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	var template models.Template
	if err := json.NewDecoder(r.Body).Decode(&template); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	template.ID = uuid.New().String()

	newTemplate, err := h.service.CreateTemplate(template)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create template")
		http.Error(w, "Failed to create template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newTemplate)
}

// Update handles the request to update an existing template.
func (h *TemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var template models.Template
	if err := json.NewDecoder(r.Body).Decode(&template); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	template.ID = id // Ensure ID is set for the service layer

	updatedTemplate, err := h.service.UpdateTemplate(id, template)
	if err != nil {
		log.Error().Err(err).Str("template_id", id).Msg("Failed to update template")
		http.Error(w, "Failed to update template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedTemplate)
}

// Delete handles the request to delete a template.
func (h *TemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.service.DeleteTemplate(id)
	if err != nil {
		log.Error().Err(err).Str("template_id", id).Msg("Failed to delete template")
		http.Error(w, "Failed to delete template", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
