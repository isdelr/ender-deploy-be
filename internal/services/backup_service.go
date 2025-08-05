package services

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/isdelr/ender-deploy-be/internal/models"
)

// BackupServiceProvider defines the interface for backup services.
type BackupServiceProvider interface {
	CreateBackup(serverID, name string) (models.Backup, error)
	GetBackupsForServer(serverID string) ([]models.Backup, error)
	DeleteBackup(backupID string) error
	RestoreBackup(backupID string) error
	GetBackupByID(backupID string) (models.Backup, error)
}

// BackupService provides business logic for backup management.
type BackupService struct {
	db            *sql.DB
	serverService ServerServiceProvider
	eventService  EventServiceProvider
	backupPath    string
}

// NewBackupService creates a new BackupService.
func NewBackupService(db *sql.DB, serverService ServerServiceProvider, eventService EventServiceProvider, backupPath string) *BackupService {
	// Ensure the base directory for backups exists
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		fmt.Printf("Failed to create base backup directory: %v\n", err)
	}
	return &BackupService{
		db:            db,
		serverService: serverService,
		eventService:  eventService,
		backupPath:    backupPath,
	}
}

// CreateBackup creates a new backup for a server.
func (s *BackupService) CreateBackup(serverID, name string) (models.Backup, error) {
	server, err := s.serverService.GetServerByID(serverID)
	if err != nil {
		return models.Backup{}, fmt.Errorf("could not find server: %w", err)
	}

	backup := models.Backup{
		ID:       uuid.New().String(),
		ServerID: serverID,
		Name:     name,
	}

	backupFileName := fmt.Sprintf("%s_%s.zip", serverID, time.Now().Format("20060102150405"))
	backup.Path = filepath.Join(s.backupPath, backupFileName)

	backupFile, err := os.Create(backup.Path)
	if err != nil {
		return models.Backup{}, fmt.Errorf("could not create backup file: %w", err)
	}
	defer backupFile.Close()

	zipWriter := zip.NewWriter(backupFile)
	defer zipWriter.Close()

	err = filepath.Walk(server.DataPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(server.DataPath, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		if info.IsDir() {
			_, err = zipWriter.Create(relPath + "/")
			return err
		}
		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}
		fileToZip, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fileToZip.Close()
		_, err = io.Copy(writer, fileToZip)
		return err
	})

	if err != nil {
		os.Remove(backup.Path) // Clean up partial file
		return models.Backup{}, fmt.Errorf("failed to zip server data: %w", err)
	}

	zipWriter.Close()
	backupFile.Close()

	fi, err := os.Stat(backup.Path)
	if err != nil {
		return models.Backup{}, fmt.Errorf("could not get backup file info: %w", err)
	}
	backup.Size = fi.Size()

	stmt, err := s.db.Prepare("INSERT INTO backups (id, server_id, name, path, size, created_at) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return models.Backup{}, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(backup.ID, backup.ServerID, backup.Name, backup.Path, backup.Size, time.Now())
	if err != nil {
		os.Remove(backup.Path)
		return models.Backup{}, err
	}

	s.eventService.CreateEvent("backup.create", "info", fmt.Sprintf("Backup '%s' created for server '%s'.", backup.Name, server.Name), &server.ID)

	return backup, nil
}

// GetBackupsForServer retrieves all backups for a given server.
func (s *BackupService) GetBackupsForServer(serverID string) ([]models.Backup, error) {
	rows, err := s.db.Query("SELECT id, server_id, name, path, size, created_at FROM backups WHERE server_id = ? ORDER BY created_at DESC", serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []models.Backup
	for rows.Next() {
		var backup models.Backup
		if err := rows.Scan(&backup.ID, &backup.ServerID, &backup.Name, &backup.Path, &backup.Size, &backup.CreatedAt); err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}
	return backups, nil
}

// DeleteBackup deletes a backup from the filesystem and database.
func (s *BackupService) DeleteBackup(backupID string) error {
	backup, err := s.GetBackupByID(backupID)
	if err != nil {
		return err
	}
	server, err := s.serverService.GetServerByID(backup.ServerID)
	if err != nil {
		// Log but don't fail, we should still be able to delete the backup record
		fmt.Printf("Warning: could not find server %s for backup %s\n", backup.ServerID, backup.ID)
	}

	if err := os.Remove(backup.Path); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: could not delete backup file %s: %v\n", backup.Path, err)
	}

	_, err = s.db.Exec("DELETE FROM backups WHERE id = ?", backupID)
	if err == nil && server.ID != "" {
		msg := fmt.Sprintf("Backup '%s' for server '%s' was deleted.", backup.Name, server.Name)
		s.eventService.CreateEvent("backup.delete", "warn", msg, &server.ID)
	}
	return err
}

// RestoreBackup restores a server to a previous state from a backup.
func (s *BackupService) RestoreBackup(backupID string) error {
	backup, err := s.GetBackupByID(backupID)
	if err != nil {
		return err
	}

	server, err := s.serverService.GetServerByID(backup.ServerID)
	if err != nil {
		return fmt.Errorf("could not find server for backup: %w", err)
	}

	msg := fmt.Sprintf("Restoration from backup '%s' started for server '%s'.", backup.Name, server.Name)
	s.eventService.CreateEvent("backup.restore.start", "warn", msg, &server.ID)

	if server.Status == "online" || server.Status == "starting" {
		if err := s.serverService.PerformServerAction(server.ID, "stop"); err != nil {
			return fmt.Errorf("failed to stop server before restoring backup: %w", err)
		}
		// Wait for the server to stop gracefully. A fixed delay is simple but might be unreliable.
		// A better method would be to poll the server status.
		time.Sleep(10 * time.Second)
	}

	// Clean out the server's data directory
	dir, err := os.ReadDir(server.DataPath)
	if err != nil {
		return fmt.Errorf("failed to read server data directory: %w", err)
	}
	for _, d := range dir {
		os.RemoveAll(filepath.Join(server.DataPath, d.Name()))
	}

	// Unzip the backup into the data directory
	zipReader, err := zip.OpenReader(backup.Path)
	if err != nil {
		return fmt.Errorf("failed to open backup archive: %w", err)
	}
	defer zipReader.Close()

	for _, f := range zipReader.File {
		fpath := filepath.Join(server.DataPath, f.Name)

		// Prevent ZipSlip vulnerability
		if !strings.HasPrefix(fpath, filepath.Clean(server.DataPath)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in zip: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	// Start the server again
	if err := s.serverService.PerformServerAction(server.ID, "start"); err != nil {
		return fmt.Errorf("failed to start server after restoring backup: %w", err)
	}

	msg = fmt.Sprintf("Server '%s' successfully restored from backup '%s'.", server.Name, backup.Name)
	s.eventService.CreateEvent("backup.restore.finish", "info", msg, &server.ID)

	return nil
}

// GetBackupByID retrieves a single backup by its ID.
func (s *BackupService) GetBackupByID(backupID string) (models.Backup, error) {
	var backup models.Backup
	row := s.db.QueryRow("SELECT id, server_id, name, path, size, created_at FROM backups WHERE id = ?", backupID)
	err := row.Scan(&backup.ID, &backup.ServerID, &backup.Name, &backup.Path, &backup.Size, &backup.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.Backup{}, fmt.Errorf("backup with id %s not found", backupID)
		}
		return models.Backup{}, err
	}
	return backup, nil
}
