package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/isdelr/ender-deploy-be/internal/models"
	"github.com/isdelr/ender-deploy-be/internal/services"
)

// ServerHandler handles HTTP requests related to servers.
type ServerHandler struct {
	service services.ServerServiceProvider
}

// NewServerHandler creates a new ServerHandler.
func NewServerHandler(service services.ServerServiceProvider) *ServerHandler {
	return &ServerHandler{service: service}
}

// CreateServerPayload is the expected JSON body for creating a server.
type CreateServerPayload struct {
	Name       string `json:"name"`
	TemplateID string `json:"templateId"`
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

// Create handles the request to create a new server from a template.
func (h *ServerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var payload CreateServerPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	newServer, err := h.service.CreateServerFromTemplate(payload.Name, payload.TemplateID)
	if err != nil {
		http.Error(w, "Failed to create server: "+err.Error(), http.StatusInternalServerError)
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

// GetServerConsoleLogs streams server logs via WebSocket.
func (h *ServerHandler) GetServerConsoleLogs(w http.ResponseWriter, r *http.Request) {
	// This is now handled by the main WebSocket handler, which can
	// accept a subscription message for a specific server's logs.
	// This REST endpoint can be removed or repurposed if needed.
	http.Error(w, "Use WebSocket connection at /api/v1/ws and subscribe to logs", http.StatusNotImplemented)
}

// SendServerConsoleCommand sends a command to the server console.
func (h *ServerHandler) SendServerConsoleCommand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var payload struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if _, err := h.service.SendCommandToServer(id, payload.Command); err != nil {
		http.Error(w, "Failed to send command: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Command sent successfully"})
}

// GetDashboardStats provides aggregated data for the main dashboard.
func (h *ServerHandler) GetDashboardStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.GetDashboardStatistics()
	if err != nil {
		http.Error(w, "Failed to retrieve dashboard stats: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetServerFileContent returns the content of a specific file.
func (h *ServerHandler) GetServerFileContent(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "File path is required", http.StatusBadRequest)
		return
	}

	content, err := h.service.GetFileContent(serverID, filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(content)
}

// ListServerFiles lists files and directories for a server.
func (h *ServerHandler) ListServerFiles(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	dirPath := r.URL.Query().Get("path") // Optional, defaults to root

	files, err := h.service.ListFiles(serverID, dirPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// UpdateServerFile updates the content of a specific file.
func (h *ServerHandler) UpdateServerFile(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")

	var payload struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateFileContent(serverID, payload.Path, []byte(payload.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "File updated successfully."})
}

// GetServerSettings gets the server's parsed server.properties
func (h *ServerHandler) GetServerSettings(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	settings, err := h.service.GetServerSettings(id)
	if err != nil {
		http.Error(w, "Failed to get server settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

// UpdateServerSettings updates the server's properties and restarts it
func (h *ServerHandler) UpdateServerSettings(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var settings map[string]string
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateServerSettings(id, settings); err != nil {
		http.Error(w, "Failed to update server settings: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Settings updated successfully. Server is restarting."})
}

// GetServerResourceHistory handles the request to get resource usage history for a server.
func (h *ServerHandler) GetServerResourceHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// You might add query parameters for time range, e.g., ?last=1h
	// For now, we'll assume a default recent history is fetched by the service.
	history, err := h.service.GetResourceHistory(id)
	if err != nil {
		http.Error(w, "Failed to retrieve resource history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// GetOnlinePlayers gets the list of online players for a server
func (h *ServerHandler) GetOnlinePlayers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	players, err := h.service.GetOnlinePlayers(id)
	if err != nil {
		// Distinguish between a server that's offline and a genuine error
		if err.Error() == "server is not online" {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, "Failed to get online players: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(players)
}

// ManagePlayer handles actions like kicking a player
func (h *ServerHandler) ManagePlayer(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	var payload struct {
		Action string `json:"action"` // "kick", "ban", etc.
		Player string `json:"player"`
		Reason string `json:"reason,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := h.service.ManagePlayer(serverID, payload.Action, payload.Player, payload.Reason)
	if err != nil {
		http.Error(w, "Failed to "+payload.Action+" player: "+err.Error(), http.StatusInternalServerError)
		return
	}

	msg := "Player " + payload.Player + " " + payload.Action + "ed successfully"
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": msg})
}

// BindPort dynamically finds and binds a port for a server
func (h *ServerHandler) BindPort(w http.ResponseWriter, r *http.Request) {
	preferredPortStr := r.URL.Query().Get("preferred")
	preferredPort, _ := strconv.Atoi(preferredPortStr)

	port, err := services.FindAvailablePort(preferredPort)
	if err != nil {
		http.Error(w, "Failed to find available port", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"port": port})
}
