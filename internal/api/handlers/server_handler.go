package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/isdelr/ender-deploy-be/internal/models"
)

// ServerHandler handles HTTP requests related to servers.
type ServerHandler struct {
	service services.ServerServiceProvider
}

// NewServerHandler creates a new ServerHandler.
func NewServerHandler(service services.ServerServiceProvider) *ServerHandler {
	return &ServerHandler{service: service}
}

// GetAll handles the request to get all servers.
func (h *ServerHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	servers, err := h.service.GetAllServers()
	if err != nil {
		http.Error(w, "Failed to retrieve servers", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servers)
}

// Get handles the request to get a single server by its ID.
func (h *ServerHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	server, err := h.service.GetServerByID(id)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(server)
}

// Create handles the request to create a new server.
func (h *ServerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var server models.Server
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Assign a new UUID
	server.ID = uuid.New().String()

	newServer, err := h.service.CreateNewServer(server)
	if err != nil {
		http.Error(w, "Failed to create server", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newServer)
}

// Update handles the request to update an existing server.
func (h *ServerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var server models.Server
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	server.ID = id

	updatedServer, err := h.service.UpdateServer(id, server)
	if err != nil {
		http.Error(w, "Failed to update server", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedServer)
}

// Delete handles the request to delete a server.
func (h *ServerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.service.DeleteServer(id)
	if err != nil {
		http.Error(w, "Failed to delete server", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PerformAction handles state-changing actions like start, stop, restart.
func (h *ServerHandler) PerformAction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload struct {
		Action string `json:"action"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := h.service.PerformServerAction(id, payload.Action)
	if err != nil {
		http.Error(w, "Failed to perform action: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Action '" + payload.Action + "' performed successfully"})
}
