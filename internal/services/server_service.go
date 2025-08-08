// Path: ender-deploy-be/internal/services/server_service.go
package services

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	"github.com/gorcon/rcon"
	"github.com/isdelr/ender-deploy-be/internal/docker"
	"github.com/isdelr/ender-deploy-be/internal/models"
	"github.com/isdelr/ender-deploy-be/internal/websocket"
	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v3/mem"
)

const (
	RCONPort = "25575"
)

// ServerServiceProvider defines the interface for server services.
type ServerServiceProvider interface {
	GetAllServers() ([]models.Server, error)
	GetServerByID(id string) (models.Server, error)
	CreateServerFromTemplate(name, templateId string) (models.Server, error)
	UpdateServer(id string, server models.Server) (models.Server, error)
	DeleteServer(id string) error
	PerformServerAction(id, action string) error
	UpdateServerStats(server models.Server) error
	SendCommandToServer(serverID, command string) (string, error)
	StreamServerLogs(ctx context.Context, serverID string, sendChan chan []byte)
	ListFiles(serverID, path string) ([]models.FileInfo, error)
	GetFileContent(serverID, path string) ([]byte, error)
	UpdateFileContent(serverID, path string, content []byte) error
	GetServerSettings(serverID string) (models.ServerSettings, error)
	UpdateServerSettings(serverID string, settings models.ServerSettings) error
	GetDashboardStatistics() (models.DashboardStats, error)
	GetResourceHistory(serverID string) ([]models.ResourceDataPoint, error)
	GetOnlinePlayers(serverID string) ([]models.OnlinePlayer, error)
	ManagePlayer(serverID, action, playerName, reason string) error
	CreateServerFromUpload(name, javaVersion, serverExecutable string, maxMemoryMB int, fileReader io.Reader) (models.Server, error)
	ExecuteTerminalCommand(ctx context.Context, serverID, command string) (string, error)
	GetSystemResourceStats() (map[string]int, error)
}

// ServerService provides business logic for server management.
type ServerService struct {
	db              *sql.DB
	docker          *docker.Client
	hub             *websocket.Hub
	templateService TemplateServiceProvider
	eventService    EventServiceProvider
	serverDataPath  string
}

// NewServerService creates a new ServerService.
func NewServerService(db *sql.DB, docker *docker.Client, hub *websocket.Hub, templateService TemplateServiceProvider, eventService EventServiceProvider, serverDataPath string) *ServerService {
	return &ServerService{
		db:              db,
		docker:          docker,
		hub:             hub,
		templateService: templateService,
		eventService:    eventService,
		serverDataPath:  serverDataPath,
	}
}
func (s *ServerService) GetAllServers() ([]models.Server, error) {
	rows, err := s.db.Query("SELECT id, name, status, port, minecraft_version, java_version, players_current, players_max, cpu_usage, ram_usage, storage_usage, ip_address, modpack_name, modpack_version, docker_container_id, data_path, rcon_password, max_memory_mb FROM servers")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []models.Server
	for rows.Next() {
		var srv models.Server
		var modpackName, modpackVersion, dockerContainerID, dataPath, rconPassword sql.NullString
		var playersCurrent, playersMax, storageUsage, maxMemoryMB sql.NullInt64
		var cpuUsage, ramUsage sql.NullFloat64
		var port sql.NullInt32
		var ipAddress sql.NullString

		err := rows.Scan(
			&srv.ID, &srv.Name, &srv.Status, &port, &srv.MinecraftVersion, &srv.JavaVersion,
			&playersCurrent, &playersMax, &cpuUsage, &ramUsage, &storageUsage,
			&ipAddress, &modpackName, &modpackVersion, &dockerContainerID, &dataPath, &rconPassword, &maxMemoryMB,
		)
		if err != nil {
			return nil, err
		}

		// Safely assign values from nullable types to the struct
		srv.Port = int(port.Int32)
		srv.IPAddress = ipAddress.String
		srv.Players.Current = int(playersCurrent.Int64)
		srv.Players.Max = int(playersMax.Int64)
		srv.Resources.CPU = cpuUsage.Float64
		srv.Resources.RAM = ramUsage.Float64
		srv.Resources.Storage = int(storageUsage.Int64)
		srv.DockerContainerID = dockerContainerID.String
		srv.DataPath = dataPath.String
		srv.RCONPassword = rconPassword.String
		srv.MaxMemoryMB = int(maxMemoryMB.Int64)

		if modpackName.Valid && modpackVersion.Valid {
			srv.Modpack = &models.ModpackInfo{Name: modpackName.String, Version: modpackVersion.String}
		}
		servers = append(servers, srv)
	}
	return servers, nil
}

// GetServerByID retrieves a single server by its ID.
func (s *ServerService) GetServerByID(id string) (models.Server, error) {
	var srv models.Server
	var modpackName, modpackVersion, containerID, dataPath, templateID, ipAddress, rconPassword sql.NullString
	// Use nullable types to scan from DB
	var playersCurrent, playersMax, storageUsage, maxMemoryMB sql.NullInt64
	var cpuUsage, ramUsage sql.NullFloat64
	var port sql.NullInt32

	row := s.db.QueryRow(`
	SELECT id, name, status, port, minecraft_version, java_version,
	       players_current, players_max, cpu_usage, ram_usage, storage_usage,
	       ip_address, modpack_name, modpack_version, docker_container_id, data_path, template_id, rcon_password, max_memory_mb
	FROM servers WHERE id = ?`, id)
	err := row.Scan(
		&srv.ID, &srv.Name, &srv.Status, &port, &srv.MinecraftVersion, &srv.JavaVersion,
		&playersCurrent, &playersMax, &cpuUsage, &ramUsage, &storageUsage,
		&ipAddress, &modpackName, &modpackVersion, &containerID, &dataPath, &templateID, &rconPassword, &maxMemoryMB)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.Server{}, fmt.Errorf("server with id %s not found", id)
		}
		return models.Server{}, err
	}
	// Safely assign values from nullable types to the struct
	srv.Port = int(port.Int32)
	srv.IPAddress = ipAddress.String
	srv.Players.Current = int(playersCurrent.Int64)
	srv.Players.Max = int(playersMax.Int64)
	srv.Resources.CPU = cpuUsage.Float64
	srv.Resources.RAM = ramUsage.Float64
	srv.Resources.Storage = int(storageUsage.Int64)
	srv.DockerContainerID = containerID.String
	srv.DataPath = dataPath.String
	srv.TemplateID = templateID.String
	srv.RCONPassword = rconPassword.String
	srv.MaxMemoryMB = int(maxMemoryMB.Int64)

	if modpackName.Valid && modpackVersion.Valid {
		srv.Modpack = &models.ModpackInfo{Name: modpackName.String, Version: modpackVersion.String}
	}

	return srv, nil
}

// CreateServerFromTemplate handles the logic for creating a new server instance based on a template.
func (s *ServerService) CreateServerFromTemplate(name, templateId string) (models.Server, error) {
	template, err := s.templateService.GetTemplateByID(templateId)
	if err != nil {
		return models.Server{}, fmt.Errorf("failed to retrieve template: %w", err)
	}

	server := models.Server{
		ID:               uuid.New().String(),
		Name:             name,
		Status:           "offline",
		MinecraftVersion: template.MinecraftVersion,
		JavaVersion:      template.JavaVersion,
		TemplateID:       template.ID,
		RCONPassword:     "ender-rcon-" + uuid.New().String(),
		MaxMemoryMB:      template.MaxMemoryMB,
	}

	// --- Create server directory and provision files ---
	server.DataPath = filepath.Join(s.serverDataPath, server.ID)
	absDataPath, err := filepath.Abs(server.DataPath)
	if err != nil {
		return server, fmt.Errorf("failed to get absolute path for server data: %w", err)
	}
	if err := os.MkdirAll(absDataPath, 0755); err != nil {
		return server, fmt.Errorf("failed to create server data directory: %w", err)
	}

	// --- NEW LOGIC for zip-based templates ---
	// The template.ServerJarURL now holds the path to the template's zip file.
	if template.ServerJarURL == "" {
		return server, fmt.Errorf("template is invalid and has no associated file path")
	}

	// Open the template's zip file for reading
	templateZipFile, err := os.Open(template.ServerJarURL)
	if err != nil {
		return server, fmt.Errorf("failed to open template zip file at %s: %w", template.ServerJarURL, err)
	}
	defer templateZipFile.Close()

	// Unzip the contents into the new server's data directory
	if err := unzip(templateZipFile, absDataPath); err != nil {
		os.RemoveAll(absDataPath) // clean up failed provisioning
		return server, fmt.Errorf("failed to unzip template file into server directory: %w", err)
	}

	// Now that files are unzipped, create the necessary startup scripts and configs.
	// 1. Create start.sh using the command stored in the template
	startScriptPath := filepath.Join(absDataPath, "start.sh")
	if err := os.WriteFile(startScriptPath, []byte("#!/bin/sh\n"+template.StartupCommand), 0755); err != nil {
		return server, fmt.Errorf("failed to write start.sh: %w", err)
	}

	// 2. Ensure eula.txt is present and accepted
	eulaPath := filepath.Join(absDataPath, "eula.txt")
	if err := os.WriteFile(eulaPath, []byte("eula=true\n"), 0644); err != nil {
		return server, fmt.Errorf("failed to write eula.txt: %w", err)
	}

	// 3. Ensure server.properties has RCON enabled for management
	s.ensureRconInProperties(filepath.Join(absDataPath, "server.properties"), server.RCONPassword)
	// --- END NEW LOGIC ---

	// --- Docker Setup ---
	imageName := fmt.Sprintf("eclipse-temurin:%s-jdk", template.JavaVersion)
	ctx := context.Background()
	if err := s.ensureImageExists(ctx, imageName); err != nil {
		return server, err
	}

	gamePort, err := FindAvailablePort(25565)
	if err != nil {
		return server, fmt.Errorf("failed to find an available game port: %w", err)
	}
	rconPort, err := FindAvailablePort(25575)
	if err != nil {
		return server, fmt.Errorf("failed to find an available RCON port: %w", err)
	}
	server.Port = gamePort
	server.IPAddress = fmt.Sprintf("127.0.0.1:%d", gamePort)

	portBindings := nat.PortMap{
		"25565/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: strconv.Itoa(gamePort)}},
		"25575/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: strconv.Itoa(rconPort)}},
	}

	containerConfig := &container.Config{
		Image:      imageName,
		WorkingDir: "/data",
		Cmd:        []string{"/bin/sh", "start.sh"},
		Tty:        true,
		ExposedPorts: nat.PortSet{
			"25565/tcp": {},
			"25575/tcp": {},
		},
		Labels: map[string]string{
			"com.ender-deploy.managed":  "true",
			"com.ender-deploy.serverId": server.ID,
		},
	}

	// Container memory is the user-defined max memory + 512MB for overhead
	containerMemoryBytes := int64(template.MaxMemoryMB+512) * 1024 * 1024

	hostConfig := &container.HostConfig{
		Mounts:       []mount.Mount{{Type: mount.TypeBind, Source: absDataPath, Target: "/data"}},
		PortBindings: portBindings,
		Resources: container.Resources{
			Memory: containerMemoryBytes, // Memory in bytes
		},
	}

	containerName := "enderdeploy_" + server.ID
	resp, err := s.docker.CreateContainer(ctx, containerConfig, hostConfig, containerName)
	if err != nil {
		return server, fmt.Errorf("failed to create docker container: %w", err)
	}
	server.DockerContainerID = resp.ID

	// --- Database Insertion ---
	maxPlayers := 20 // default
	if mpStr, ok := template.Properties["max-players"]; ok {
		if mp, err := strconv.Atoi(mpStr); err == nil {
			maxPlayers = mp
		}
	}
	server.Players.Max = maxPlayers

	stmt, err := s.db.Prepare(`
		INSERT INTO servers(id, name, status, minecraft_version, java_version, docker_container_id, data_path, template_id, port, ip_address, players_max, rcon_password, max_memory_mb)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return server, fmt.Errorf("failed to prepare db statement: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(server.ID, server.Name, server.Status, server.MinecraftVersion, server.JavaVersion, server.DockerContainerID, server.DataPath, server.TemplateID, server.Port, server.IPAddress, maxPlayers, server.RCONPassword, server.MaxMemoryMB)
	if err != nil {
		s.docker.RemoveContainer(context.Background(), server.DockerContainerID) // Cleanup container
		return server, fmt.Errorf("failed to write server to database: %w", err)
	}

	newServer, _ := s.GetServerByID(server.ID)
	s.broadcastServerUpdate(newServer)

	s.eventService.CreateEvent("server.create", "info", fmt.Sprintf("Server '%s' was created successfully.", newServer.Name), &newServer.ID)
	log.Info().Str("server_name", server.Name).Str("template_name", template.Name).Str("container_id", server.DockerContainerID).Msg("Successfully created server from custom template")
	return newServer, nil
}

// CreateServerFromUpload creates a server from an uploaded zip file using the new custom approach.
func (s *ServerService) CreateServerFromUpload(name, javaVersion, serverExecutable string, maxMemoryMB int, fileReader io.Reader) (models.Server, error) {
	server := models.Server{
		ID:               uuid.New().String(),
		Name:             name,
		Status:           "offline",
		MinecraftVersion: "Uploaded", // Can't know this from a zip
		JavaVersion:      javaVersion,
		RCONPassword:     "ender-rcon-" + uuid.New().String(),
		MaxMemoryMB:      maxMemoryMB,
	}

	// --- Create server directory and unzip user files ---
	server.DataPath = filepath.Join(s.serverDataPath, server.ID)
	absDataPath, err := filepath.Abs(server.DataPath)
	if err != nil {
		return server, fmt.Errorf("failed to get absolute path for server data: %w", err)
	}
	if err := os.MkdirAll(absDataPath, 0755); err != nil {
		return server, fmt.Errorf("failed to create server data directory: %w", err)
	}
	if err := unzip(fileReader, absDataPath); err != nil {
		return server, fmt.Errorf("failed to unzip uploaded file: %w", err)
	}

	// --- Provision startup script and EULA ---
	if err := s.provisionServerFilesFromUpload(absDataPath, serverExecutable, maxMemoryMB); err != nil {
		return server, fmt.Errorf("failed to provision startup files: %w", err)
	}

	s.ensureRconInProperties(filepath.Join(absDataPath, "server.properties"), server.RCONPassword)

	// --- Docker Setup ---
	imageName := fmt.Sprintf("eclipse-temurin:%s-jdk", javaVersion)
	ctx := context.Background()
	if err := s.ensureImageExists(ctx, imageName); err != nil {
		return server, err
	}

	gamePort, err := FindAvailablePort(25565)
	if err != nil {
		return server, err
	}
	rconPort, err := FindAvailablePort(25575)
	if err != nil {
		return server, err
	}
	server.Port = gamePort
	server.IPAddress = fmt.Sprintf("127.0.0.1:%d", gamePort)

	portBindings := nat.PortMap{
		"25565/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: strconv.Itoa(gamePort)}},
		"25575/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: strconv.Itoa(rconPort)}},
	}

	containerConfig := &container.Config{
		Image:      imageName,
		WorkingDir: "/data",
		Cmd:        []string{"/bin/sh", "start.sh"},
		Tty:        true,
		ExposedPorts: nat.PortSet{
			"25565/tcp": {},
			"25575/tcp": {},
		},
		Labels: map[string]string{"com.ender-deploy.managed": "true", "com.ender-deploy.serverId": server.ID},
	}

	// Container memory is the user-defined max memory + 512MB for overhead
	containerMemoryBytes := int64(maxMemoryMB+512) * 1024 * 1024

	hostConfig := &container.HostConfig{
		Mounts:       []mount.Mount{{Type: mount.TypeBind, Source: absDataPath, Target: "/data"}},
		PortBindings: portBindings,
		Resources: container.Resources{
			Memory: containerMemoryBytes,
		},
	}

	containerName := "enderdeploy_" + server.ID
	resp, err := s.docker.CreateContainer(ctx, containerConfig, hostConfig, containerName)
	if err != nil {
		return server, err
	}
	server.DockerContainerID = resp.ID

	// --- Database Insertion ---
	stmt, err := s.db.Prepare(`
		INSERT INTO servers(id, name, status, minecraft_version, java_version, docker_container_id, data_path, port, ip_address, players_max, rcon_password, max_memory_mb)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return server, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(server.ID, server.Name, server.Status, server.MinecraftVersion, server.JavaVersion, server.DockerContainerID, server.DataPath, server.Port, server.IPAddress, 20, server.RCONPassword, server.MaxMemoryMB)
	if err != nil {
		s.docker.RemoveContainer(context.Background(), server.DockerContainerID)
		return server, err
	}

	newServer, _ := s.GetServerByID(server.ID)
	s.broadcastServerUpdate(newServer)
	s.eventService.CreateEvent("server.upload", "info", fmt.Sprintf("Server '%s' was created from an upload.", newServer.Name), &newServer.ID)
	return newServer, nil
}

// UpdateServer updates an existing server's settings.
func (s *ServerService) UpdateServer(id string, server models.Server) (models.Server, error) {
	stmt, err := s.db.Prepare("UPDATE servers SET name = ?, minecraft_version = ?, java_version = ?, players_max = ? WHERE id = ?")
	if err != nil {
		return models.Server{}, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(server.Name, server.MinecraftVersion, server.JavaVersion, server.Players.Max, id)
	if err != nil {
		return models.Server{}, err
	}

	updatedServer, err := s.GetServerByID(id)
	if err != nil {
		return models.Server{}, err
	}

	s.broadcastServerUpdate(updatedServer)
	return updatedServer, nil
}

// DeleteServer stops, removes, and deletes a server.
func (s *ServerService) DeleteServer(id string) error {
	server, err := s.GetServerByID(id)
	if err != nil {
		return fmt.Errorf("could not find server to delete: %w", err)
	}

	ctx := context.Background()
	log.Info().Str("container_id", server.DockerContainerID).Msg("Stopping and removing container")
	s.docker.StopContainer(ctx, server.DockerContainerID)
	err = s.docker.RemoveContainer(ctx, server.DockerContainerID)
	if err != nil && !client.IsErrNotFound(err) {
		log.Warn().Err(err).Str("container_id", server.DockerContainerID).Msg("Could not remove container during server deletion")
	}

	log.Info().Str("server_id", id).Msg("Deleting server from database")
	_, err = s.db.Exec("DELETE FROM servers WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete server from DB: %w", err)
	}

	log.Info().Str("data_path", server.DataPath).Msg("Deleting server data")
	if err = os.RemoveAll(server.DataPath); err != nil {
		log.Warn().Err(err).Str("data_path", server.DataPath).Msg("Failed to delete server data directory")
	}

	s.eventService.CreateEvent("server.delete", "warn", fmt.Sprintf("Server '%s' was permanently deleted.", server.Name), nil) // serverId won't exist anymore
	s.hub.Broadcast <- []byte(`{"event": "server_deleted", "id": "` + id + `"}`)
	return nil
}

// pollForRconReady checks periodically if a server's RCON port is available.
func (s *ServerService) pollForRconReady(ctx context.Context, server models.Server) {
	log.Info().Str("server_id", server.ID).Msg("Starting RCON polling to check for server readiness.")

	// Set a timeout for the entire polling process.
	pollingCtx, cancel := context.WithTimeout(ctx, 3*time.Minute) // 3-minute timeout for the server to start
	defer cancel()

	ticker := time.NewTicker(5 * time.Second) // Poll every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-pollingCtx.Done():
			// Polling timed out
			log.Warn().Str("server_id", server.ID).Msg("RCON polling timed out. Server failed to start properly.")
			// Set the server status to offline as it failed to become ready
			s.db.Exec("UPDATE servers SET status = ? WHERE id = ?", "offline", server.ID)
			failedServer, err := s.GetServerByID(server.ID)
			if err == nil {
				s.broadcastServerUpdate(failedServer)
			}
			s.eventService.CreateEvent("server.start.fail", "error", fmt.Sprintf("Server '%s' failed to become ready in time.", server.Name), &server.ID)
			return

		case <-ticker.C:
			// Ensure container is still running before attempting to connect
			containerInfo, err := s.docker.InspectContainer(context.Background(), server.DockerContainerID)
			if err != nil || !containerInfo.State.Running {
				log.Warn().Err(err).Str("server_id", server.ID).Msg("Container stopped running during RCON polling. Marking as offline.")
				s.db.Exec("UPDATE servers SET status = ? WHERE id = ?", "offline", server.ID)
				stoppedServer, dbErr := s.GetServerByID(server.ID)
				if dbErr == nil {
					s.broadcastServerUpdate(stoppedServer)
				}
				return // Stop polling
			}

			rconPortBinding, ok := containerInfo.NetworkSettings.Ports[RCONPort+"/tcp"]
			if !ok || len(rconPortBinding) == 0 {
				log.Warn().Str("server_id", server.ID).Msg("RCON port not bound, cannot poll for readiness.")
				return
			}
			rconAddr := "127.0.0.1:" + rconPortBinding[0].HostPort

			conn, err := rcon.Dial(rconAddr, server.RCONPassword)
			if err == nil {
				// Success! The server is ready.
				conn.Close()
				log.Info().Str("server_id", server.ID).Msg("RCON connection successful. Server is now online.")

				// Update status to online in the DB
				_, err := s.db.Exec("UPDATE servers SET status = ? WHERE id = ?", "online", server.ID)
				if err != nil {
					log.Error().Err(err).Str("server_id", server.ID).Msg("Failed to update server status to online after successful RCON poll.")
					return
				}

				// Fetch the latest server state and broadcast it
				onlineServer, err := s.GetServerByID(server.ID)
				if err == nil {
					s.broadcastServerUpdate(onlineServer)
					s.eventService.CreateEvent("server.start.ready", "info", fmt.Sprintf("Server '%s' is fully loaded and online.", server.Name), &server.ID)
				}
				return // Stop polling
			}
			// If we are here, RCON connection failed, we'll try again on the next tick.
			log.Info().Str("server_id", server.ID).Str("rcon_addr", rconAddr).Msg("RCON ping failed, server not ready yet. Retrying...")
		}
	}
}

// PerformServerAction handles start, stop, restart.
func (s *ServerService) PerformServerAction(id, action string) error {
	server, err := s.GetServerByID(id)
	if err != nil {
		return fmt.Errorf("could not find server in DB: %w", err)
	}

	var newStatus, eventLevel, eventMessage string
	var startPolling bool
	ctx := context.Background()
	logCtx := log.Info().Str("server_id", id).Str("container_id", server.DockerContainerID).Str("action", action)

	switch action {
	case "start":
		logCtx.Msg("Starting container")
		if err := s.docker.StartContainer(ctx, server.DockerContainerID); err != nil {
			return err
		}
		newStatus = "starting"
		eventLevel = "info"
		eventMessage = fmt.Sprintf("Server '%s' is starting.", server.Name)
		startPolling = true
	case "stop":
		logCtx.Msg("Stopping container")
		if err := s.docker.StopContainer(ctx, server.DockerContainerID); err != nil {
			return err
		}
		newStatus = "offline"
		eventLevel = "info"
		eventMessage = fmt.Sprintf("Server '%s' was stopped.", server.Name)
	case "restart":
		logCtx.Msg("Restarting container")
		if err := s.docker.RestartContainer(ctx, server.DockerContainerID); err != nil {
			return err
		}
		newStatus = "starting"
		eventLevel = "info"
		eventMessage = fmt.Sprintf("Server '%s' is restarting.", server.Name)
		startPolling = true
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	_, err = s.db.Exec("UPDATE servers SET status = ? WHERE id = ?", newStatus, id)
	if err != nil {
		return fmt.Errorf("failed to update server status in DB: %w", err)
	}

	updatedServer, _ := s.GetServerByID(id)
	updatedServer.Status = newStatus
	s.broadcastServerUpdate(updatedServer)

	s.eventService.CreateEvent("server."+action, eventLevel, eventMessage, &id)

	if startPolling {
		// Run the RCON polling in the background to not block the API response.
		go s.pollForRconReady(context.Background(), updatedServer)
	}

	return nil
}

// UpdateServerStats updates the resource usage for a server and broadcasts it.
func (s *ServerService) UpdateServerStats(server models.Server) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update the main servers table
	_, err = tx.Exec(`
	UPDATE servers
	SET status = ?, players_current = ?, cpu_usage = ?, ram_usage = ?, storage_usage = ?
	WHERE id = ?`,
		server.Status, server.Players.Current, server.Resources.CPU, server.Resources.RAM, server.Resources.Storage, server.ID)
	if err != nil {
		return err
	}

	// Insert into history table
	_, err = tx.Exec(`
	INSERT INTO resource_history (server_id, cpu_usage, ram_usage, players_current)
	VALUES (?, ?, ?, ?)`,
		server.ID, server.Resources.CPU, server.Resources.RAM, server.Players.Current)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	s.broadcastServerUpdate(server)
	return nil
}

// SendCommandToServer sends a command to a running Minecraft server via RCON.
func (s *ServerService) SendCommandToServer(serverID, command string) (string, error) {
	server, err := s.GetServerByID(serverID)
	if err != nil {
		return "", err
	}

	if server.Status != "online" {
		return "", fmt.Errorf("server is not online")
	}

	containerInfo, err := s.docker.InspectContainer(context.Background(), server.DockerContainerID)
	if err != nil {
		return "", fmt.Errorf("could not inspect container: %w", err)
	}

	rconPortBinding, ok := containerInfo.NetworkSettings.Ports["25565/tcp"]
	if !ok || len(rconPortBinding) == 0 {
		return "", fmt.Errorf("rcon port not bound for server %s", serverID)
	}
	rconAddr := "127.0.0.1:" + rconPortBinding[0].HostPort

	var conn *rcon.Conn
	var dialErr error

	for i := 0; i < 3; i++ {
		conn, dialErr = rcon.Dial(rconAddr, server.RCONPassword)
		if dialErr == nil {
			break // Success
		}
		log.Warn().Err(dialErr).Str("server_id", serverID).Int("attempt", i+1).Msg("RCON connection attempt failed, retrying...")
		time.Sleep(2 * time.Second)
	}

	if dialErr != nil {
		return "", fmt.Errorf("could not connect via rcon after multiple attempts: %w", dialErr)
	}
	defer conn.Close()

	response, err := conn.Execute(command)
	if err != nil {
		return "", fmt.Errorf("rcon command failed: %w", err)
	}

	log.Info().Str("command", command).Str("server_name", server.Name).Str("response", response).Msg("RCON command executed")
	// The response is now returned directly to the handler, not broadcast from here.
	return response, nil
}

// StreamServerLogs streams the logs of a container to a websocket client.
func (s *ServerService) StreamServerLogs(ctx context.Context, serverID string, sendChan chan []byte) {
	server, err := s.GetServerByID(serverID)
	if err != nil {
		log.Warn().Err(err).Str("server_id", serverID).Msg("Cannot stream logs, server not found")
		sendChan <- websocket.NewErrorMessage(err.Error())
		return
	}

	logReader, err := s.docker.GetContainerLogs(ctx, server.DockerContainerID, true)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Str("server_id", serverID).Msg("Failed to get container logs")
			sendChan <- websocket.NewErrorMessage("Failed to get container logs: " + err.Error())
		}
		return
	}
	defer logReader.Close()

	// Strip the 8-byte Docker header from each line.
	// https://docs.docker.com/engine/api/v1.41/#operation/ContainerAttach
	scanner := bufio.NewScanner(logReader)
	for scanner.Scan() {
		lineBytes := scanner.Bytes()
		// Ensure we don't panic on short lines (e.g., empty log lines)
		if len(lineBytes) > 8 {
			lineBytes = lineBytes[8:]
		}

		// Create a structured message
		logMsg := websocket.NewConsoleOutputMessage("docker", "", string(lineBytes))

		select {
		case <-ctx.Done(): // This context is cancelled by the handler when the client unsubscribes or disconnects.
			log.Info().Str("server_id", serverID).Msg("Client disconnected, stopping log stream.")
			return
		case sendChan <- logMsg:
			// Message was successfully sent
		}
	}

	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Str("server_id", serverID).Msg("Error reading logs from container")
		}
	}
}

// broadcastServerUpdate sends a JSON message to all websocket clients with the server's state
func (s *ServerService) broadcastServerUpdate(server models.Server) {
	msg := websocket.Message{
		Action:  "server_update",
		Payload: server,
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Msg("Error marshalling server update for broadcast")
		return
	}
	s.hub.Broadcast <- jsonMsg
}

// findAvailablePort starts from a base port and finds the next available TCP port.
func FindAvailablePort(startPort int) (int, error) {
	for port := startPort; port < 65535; port++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports found")
}
func (s *ServerService) GetDashboardStatistics() (models.DashboardStats, error) {
	servers, err := s.GetAllServers()
	if err != nil {
		return models.DashboardStats{}, err
	}

	stats := models.DashboardStats{
		TotalServers:     len(servers),
		ServerStatusDist: make(map[string]int),
	}

	for _, server := range servers {
		if server.Status == "online" {
			stats.OnlineServers++
			stats.TotalPlayers += server.Players.Current
		}
		stats.MaxPlayers += server.Players.Max
		stats.ServerStatusDist[server.Status]++
	}

	stats.SystemHealth = 99.5

	rows, err := s.db.Query(`
    SELECT timestamp, SUM(cpu_usage) as total_cpu, SUM(ram_usage) as total_ram, SUM(players_current) as total_players
    FROM resource_history
    WHERE timestamp >= ?
    GROUP BY strftime('%Y-%m-%d %H', timestamp)
    ORDER BY timestamp ASC
`, time.Now().Add(-24*time.Hour))

	if err != nil {
		return stats, err
	}
	defer rows.Close()

	for rows.Next() {
		var dp models.ResourceDataPoint
		var totalCPU, totalRAM sql.NullFloat64
		var totalPlayers sql.NullInt64
		if err := rows.Scan(&dp.Timestamp, &totalCPU, &totalRAM, &totalPlayers); err != nil {
			return stats, err
		}
		dp.CPUUsage = totalCPU.Float64
		dp.RAMUsage = totalRAM.Float64
		dp.PlayersCurrent = totalPlayers
		stats.ResourceHistory = append(stats.ResourceHistory, dp)
	}
	stats.PlayerHistory = stats.ResourceHistory

	return stats, nil
}

// GetResourceHistory gets recent resource usage for a specific server.
func (s ServerService) GetResourceHistory(serverID string) ([]models.ResourceDataPoint, error) {
	rows, err := s.db.Query("SELECT timestamp, cpu_usage, ram_usage, players_current FROM resource_history WHERE server_id = ? AND timestamp >= ? ORDER BY timestamp ASC", serverID, time.Now().Add(-30*time.Minute))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []models.ResourceDataPoint
	for rows.Next() {
		var dp models.ResourceDataPoint
		if err := rows.Scan(&dp.Timestamp, &dp.CPUUsage, &dp.RAMUsage, &dp.PlayersCurrent); err != nil {
			return nil, err
		}
		history = append(history, dp)
	}
	return history, nil
}

// GetOnlinePlayers retrieves a list of players currently on the server.
func (s *ServerService) GetOnlinePlayers(serverID string) ([]models.OnlinePlayer, error) {
	response, err := s.SendCommandToServer(serverID, "list")
	if err != nil {
		return nil, err
	}

	parts := strings.SplitN(response, ":", 2)
	if len(parts) < 2 {
		return []models.OnlinePlayer{}, nil
	}

	playerNamesStr := strings.TrimSpace(parts[1])
	if playerNamesStr == "" {
		return []models.OnlinePlayer{}, nil
	}

	playerNames := strings.Split(playerNamesStr, ", ")
	players := make([]models.OnlinePlayer, len(playerNames))
	for i, name := range playerNames {
		players[i] = models.OnlinePlayer{Name: name, UUID: name}
	}

	return players, nil
}

// ManagePlayer executes a player management command (kick, ban, etc.).
func (s *ServerService) ManagePlayer(serverID, action, playerName, reason string) error {
	var command string
	switch action {
	case "kick":
		command = fmt.Sprintf("kick %s %s", playerName, reason)
	case "ban":
		command = fmt.Sprintf("ban %s %s", playerName, reason)
	default:
		return fmt.Errorf("unsupported player action: %s", action)
	}

	_, err := s.SendCommandToServer(serverID, command)
	if err == nil {
		msg := fmt.Sprintf("Player '%s' was %sed.", playerName, action)
		s.eventService.CreateEvent("player."+action, "info", msg, &serverID)
	}
	return err
}

// ListFiles lists files and directories for a server.
func (s *ServerService) ListFiles(serverID, path string) ([]models.FileInfo, error) {
	server, err := s.GetServerByID(serverID)
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(server.DataPath, path)
	if !strings.HasPrefix(fullPath, server.DataPath) {
		return nil, fmt.Errorf("invalid path: access denied")
	}

	dirEntries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("could not read directory: %w", err)
	}

	var fileInfos []models.FileInfo
	for _, entry := range dirEntries {
		info, err := entry.Info()
		if err != nil {
			log.Warn().Err(err).Str("file_name", entry.Name()).Msg("Could not get file info during file listing")
			continue
		}
		fileInfos = append(fileInfos, models.FileInfo{
			Name:     entry.Name(),
			Size:     info.Size(),
			IsDir:    entry.IsDir(),
			Modified: info.ModTime(),
		})
	}

	return fileInfos, nil
}

// GetFileContent reads the content of a file.
func (s *ServerService) GetFileContent(serverID, path string) ([]byte, error) {
	server, err := s.GetServerByID(serverID)
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(server.DataPath, path)
	if !strings.HasPrefix(fullPath, server.DataPath) {
		return nil, fmt.Errorf("invalid path: access denied")
	}

	return os.ReadFile(fullPath)
}

// UpdateFileContent writes new content to a file.
func (s *ServerService) UpdateFileContent(serverID, path string, content []byte) error {
	server, err := s.GetServerByID(serverID)
	if err != nil {
		return err
	}

	fullPath := filepath.Join(server.DataPath, path)
	if !strings.HasPrefix(fullPath, server.DataPath) {
		return fmt.Errorf("invalid path: access denied")
	}

	return os.WriteFile(fullPath, content, 0644)
}

// GetServerSettings reads and parses the server.properties file.
func (s *ServerService) GetServerSettings(serverID string) (models.ServerSettings, error) {
	content, err := s.GetFileContent(serverID, "server.properties")
	if err != nil {
		if os.IsNotExist(err) {
			return make(models.ServerSettings), nil
		}
		return nil, err
	}

	settings := make(models.ServerSettings)
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			settings[parts[0]] = parts[1]
		}
	}

	return settings, nil
}

// UpdateServerSettings writes new settings to server.properties and restarts the server.
func (s *ServerService) UpdateServerSettings(serverID string, settings models.ServerSettings) error {
	var builder strings.Builder
	builder.WriteString("# Minecraft server properties\n")
	builder.WriteString(fmt.Sprintf("# Updated on %s\n", time.Now().Format(time.RFC1123)))
	for key, value := range settings {
		builder.WriteString(fmt.Sprintf("%s=%s\n", key, value))
	}

	err := s.UpdateFileContent(serverID, "server.properties", []byte(builder.String()))
	if err != nil {
		return fmt.Errorf("failed to write to server.properties: %w", err)
	}

	server, _ := s.GetServerByID(serverID)
	msg := fmt.Sprintf("Settings for server '%s' were updated. Restart is in progress.", server.Name)
	s.eventService.CreateEvent("server.settings.update", "info", msg, &serverID)

	return s.PerformServerAction(serverID, "restart")
}

// GetSystemResourceStats calculates total and allocated RAM.
func (s *ServerService) GetSystemResourceStats() (map[string]int, error) {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve system memory stats")
		return nil, fmt.Errorf("could not retrieve system memory stats: %w", err)
	}
	totalRAM := int(vmStat.Total / 1024 / 1024) // Convert from bytes to MB

	servers, err := s.GetAllServers()
	if err != nil {
		return nil, err
	}

	var allocatedRAM int
	for _, server := range servers {
		// The total memory allocated for a container is the Minecraft max memory
		// plus the 512MB overhead we've designated.
		allocatedRAM += server.MaxMemoryMB + 512
	}

	stats := map[string]int{
		"totalRAM":     totalRAM,
		"allocatedRAM": allocatedRAM,
	}
	return stats, nil
}

// --- Helper Functions ---

// provisionServerFilesFromUpload creates the essential files for a server from an upload.
func (s *ServerService) provisionServerFilesFromUpload(dataPath, serverExecutable string, maxMemoryMB int) error {
	// 1. Create or overwrite eula.txt to ensure it's accepted.
	eulaPath := filepath.Join(dataPath, "eula.txt")
	if err := os.WriteFile(eulaPath, []byte("eula=true\n"), 0644); err != nil {
		return fmt.Errorf("failed to write eula.txt: %w", err)
	}

	// 2. Create start.sh with logic to handle .sh or .jar files
	var startScriptContent string
	if strings.HasSuffix(strings.ToLower(serverExecutable), ".sh") {
		// It's a shell script, execute it directly.
		startScriptContent = fmt.Sprintf(
			`#!/bin/sh
# Make sure the user's script is executable
chmod +x ./%s
# Execute the user's start script
./%s
`,
			serverExecutable,
			serverExecutable,
		)
	} else {
		// Assume it's a jar file, using the new memory logic.
		startScriptContent = fmt.Sprintf(
			`#!/bin/sh
java -Xmx%dM -Xms1024M -jar %s nogui
`,
			maxMemoryMB,
			serverExecutable,
		)
	}

	startScriptPath := filepath.Join(dataPath, "start.sh")
	if err := os.WriteFile(startScriptPath, []byte(startScriptContent), 0755); err != nil {
		return fmt.Errorf("failed to write start.sh: %w", err)
	}

	return nil
}

// ensureImageExists pulls a docker image if it's not present locally.
func (s *ServerService) ensureImageExists(ctx context.Context, imageName string) error {
	_, _, err := s.docker.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		if client.IsErrNotFound(err) {
			log.Info().Str("image", imageName).Msg("Image not found locally. Pulling from Docker Hub...")
			puller, pullErr := s.docker.ImagePull(ctx, imageName, image.PullOptions{})
			if pullErr != nil {
				return fmt.Errorf("failed to start image pull for '%s': %w", imageName, pullErr)
			}
			defer puller.Close()
			// Stream pull progress to logs
			if _, err := io.Copy(os.Stdout, puller); err != nil {
				log.Warn().Err(err).Msg("Could not stream docker pull output")
			}
			log.Info().Str("image", imageName).Msg("Image pulled successfully.")
		} else {
			return fmt.Errorf("failed to inspect docker image '%s': %w", imageName, err)
		}
	} else {
		log.Info().Str("image", imageName).Msg("Image found locally.")
	}
	return nil
}

// ensureRconInProperties reads a server.properties file, ensures RCON is configured, and writes it back.
func (s *ServerService) ensureRconInProperties(filePath, rconPassword string) {
	props := make(map[string]string)
	file, err := os.Open(filePath)
	if err != nil {
		log.Warn().Err(err).Str("path", filePath).Msg("Cannot open server.properties to ensure RCON, creating a new one.")
	} else {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				props[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		file.Close()
	}

	// Set/overwrite RCON properties
	props["enable-rcon"] = "true"
	props["rcon.password"] = rconPassword
	props["rcon.port"] = RCONPort

	// Write the properties back
	var builder strings.Builder
	for key, value := range props {
		builder.WriteString(fmt.Sprintf("%s=%s\n", key, value))
	}

	if err := os.WriteFile(filePath, []byte(builder.String()), 0644); err != nil {
		log.Error().Err(err).Str("path", filePath).Msg("Failed to write updated server.properties for RCON.")
	}
}

// unzip decompresses a zip archive from a reader to a destination directory.
func unzip(reader io.Reader, dest string) error {
	// We need to write the reader's content to a temp file to use zip.OpenReader
	tmpFile, err := os.CreateTemp("", "ender-deploy-zip-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	size, err := io.Copy(tmpFile, reader)
	if err != nil {
		return err
	}

	r, err := zip.NewReader(tmpFile, size)
	if err != nil {
		return err
	}

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip vulnerability
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
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
	return nil
}

// ExecuteTerminalCommand runs a shell command inside the server's container.
func (s *ServerService) ExecuteTerminalCommand(ctx context.Context, serverID, command string) (string, error) {
	server, err := s.GetServerByID(serverID)
	if err != nil {
		return "", fmt.Errorf("could not find server to execute command: %w", err)
	}

	if server.Status != "online" {
		return "", fmt.Errorf("server is not online")
	}

	// This is the key for security. The command runs inside the container's isolated environment.
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		WorkingDir:   "/data", // The mounted volume with server files
		Cmd:          []string{"sh", "-c", command},
	}

	execIDResp, err := s.docker.ContainerExecCreate(ctx, server.DockerContainerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec command in container: %w", err)
	}

	attachResp, err := s.docker.ContainerExecAttach(ctx, execIDResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec command in container: %w", err)
	}
	defer attachResp.Close()

	// Use stdcopy to demultiplex the stream into separate stdout and stderr buffers.
	var outBuf, errBuf bytes.Buffer
	_, err = stdcopy.StdCopy(&outBuf, &errBuf, attachResp.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to read output from exec command: %w", err)
	}

	response := outBuf.String()
	// Also include stderr in the response if there is any, to help with debugging.
	if errBuf.Len() > 0 {
		response += "\n--- STDERR ---\n" + errBuf.String()
	}

	return response, nil
}
