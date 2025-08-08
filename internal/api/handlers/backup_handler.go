package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/isdelr/ender-deploy-be/internal/services"
	"github.com/rs/zerolog/log"
)

// BackupHandler handles HTTP requests related to backups.
type BackupHandler struct {
	service services.BackupServiceProvider
}

// NewBackupHandler creates a new BackupHandler.
func NewBackupHandler(service services.BackupServiceProvider) *BackupHandler {
	return &BackupHandler{service: service}
}

// CreateBackupPayload is the expected JSON body for creating a backup.
type CreateBackupPayload struct {
	Name string `json:"name"`
}

// GetAllForServer handles the request to get all backups for a server.
func (h *BackupHandler) GetAllForServer(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	backups, err := h.service.GetBackupsForServer(serverID)
	if err != nil {
		log.Error().Err(err).Str("server_id", serverID).Msg("Failed to retrieve backups for server")
		http.Error(w, "Failed to retrieve backups: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(backups)
}

// Create handles the request to create a new backup.
func (h *BackupHandler) Create(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	var payload CreateBackupPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if payload.Name == "" {
		http.Error(w, "Backup name is required", http.StatusBadRequest)
		return
	}

	// Creating a backup can be a long-running task.
	go func() {
		if _, err := h.service.CreateBackup(serverID, payload.Name); err != nil {
			log.Error().Err(err).Str("server_id", serverID).Str("backup_name", payload.Name).Msg("Failed to create backup in background")
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"message": "Backup creation started."})
}

// Delete handles the request to delete a backup.
func (h *BackupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	backupID := chi.URLParam(r, "backupId")
	if err := h.service.DeleteBackup(backupID); err != nil {
		log.Error().Err(err).Str("backup_id", backupID).Msg("Failed to delete backup")
		http.Error(w, "Failed to delete backup: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Restore handles the request to restore a backup.
func (h *BackupHandler) Restore(w http.ResponseWriter, r *http.Request) {
	backupID := chi.URLParam(r, "backupId")

	// Restoring is a long-running, critical task. We run it in a goroutine.
	go func() {
		if err := h.service.RestoreBackup(backupID); err != nil {
			log.Error().Err(err).Str("backup_id", backupID).Msg("Failed to restore backup in background")
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"message": "Backup restoration started. The server will restart."})
}
