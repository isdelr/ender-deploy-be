package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/isdelr/ender-deploy-be/internal/services"
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
	// We start it in a goroutine and return immediately.
	go func() {
		_, err := h.service.CreateBackup(serverID, payload.Name)
		if err != nil {
			// Log the error. In a real application, you might use a more robust
			// system for notifying the user of background task failures.
			// For now, we'll log it to the console.
			// Note: We can't write an HTTP error response here because the
			// original request has already completed.
			// log.Printf("Failed to create backup for server %s in background: %v", serverID, err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 Accepted indicates the request is being processed.
	json.NewEncoder(w).Encode(map[string]string{"message": "Backup creation started."})
}

// Delete handles the request to delete a backup.
func (h *BackupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	backupID := chi.URLParam(r, "backupId")
	err := h.service.DeleteBackup(backupID)
	if err != nil {
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
		err := h.service.RestoreBackup(backupID)
		if err != nil {
			// log.Printf("Failed to restore backup %s in background: %v", backupID, err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"message": "Backup restoration started. The server will restart."})
}
