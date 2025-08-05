package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/isdelr/ender-deploy-be/internal/docker"
	"github.com/isdelr/ender-deploy-be/internal/models"
	"github.com/isdelr/ender-deploy-be/internal/websocket"
)

// ServerServiceProvider defines the interface for server services.
type ServerServiceProvider interface {
	GetAllServers() ([]models.Server, error)
	GetServerByID(id string) (models.Server, error)
	CreateNewServer(server models.Server) (models.Server, error)
	UpdateServer(id string, server models.Server) (models.Server, error)
	DeleteServer(id string) error
	PerformServerAction(id, action string) error
}

// ServerService provides business logic for server management.
type ServerService struct {
	db             *sql.DB
	docker         *docker.Client
	hub            *websocket.Hub
	serverDataPath string
}

// NewServerService creates a new ServerService.
func NewServerService(db *sql.DB, docker *docker.Client, hub *websocket.Hub, serverDataPath string) *ServerService {
	return &ServerService{
		db:             db,
		docker:         docker,
		hub:            hub,
		serverDataPath: serverDataPath,
	}
}

// GetAllServers retrieves all servers from the database.
func (s *ServerService) GetAllServers() ([]models.Server, error) {
	rows, err := s.db.Query("SELECT id, name, status, minecraft_version, java_version, players_current, players_max, ip_address, modpack_name, modpack_version FROM servers")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []models.Server
	for rows.Next() {
		var srv models.Server
		var modpackName, modpackVersion sql.NullString
		err := rows.Scan(&srv.ID, &srv.Name, &srv.Status, &srv.MinecraftVersion, &srv.JavaVersion, &srv.Players.Current, &srv.Players.Max, &srv.IPAddress, &modpackName, &modpackVersion)
		if err != nil {
			return nil, err
		}
		if modpackName.Valid && modpackVersion.Valid {
			srv.Modpack = &models.ModpackInfo{Name: modpackName.String, Version: modpackVersion.String}
		}
		// TODO: Populate live resource usage
		servers = append(servers, srv)
	}
	return servers, nil
}

// GetServerByID retrieves a single server by its ID.
func (s *ServerService) GetServerByID(id string) (models.Server, error) {
	var srv models.Server
	var modpackName, modpackVersion sql.NullString
	row := s.db.QueryRow("SELECT id, name, status, minecraft_version, java_version, players_current, players_max, ip_address, modpack_name, modpack_version FROM servers WHERE id = ?", id)
	err := row.Scan(&srv.ID, &srv.Name, &srv.Status, &srv.MinecraftVersion, &srv.JavaVersion, &srv.Players.Current, &srv.Players.Max, &srv.IPAddress, &modpackName, &modpackVersion)
	if err != nil {
		return models.Server{}, err
	}
	if modpackName.Valid && modpackVersion.Valid {
		srv.Modpack = &models.ModpackInfo{Name: modpackName.String, Version: modpackVersion.String}
	}
	// TODO: Populate live resource usage
	return srv, nil
}

// CreateNewServer handles the logic for creating a new server instance.
func (s *ServerService) CreateNewServer(server models.Server) (models.Server, error) {
	// 1. Create directory for server data
	server.DataPath = filepath.Join(s.serverDataPath, server.ID)
	if err := os.MkdirAll(server.DataPath, 0755); err != nil {
		return server, fmt.Errorf("failed to create server data directory: %w", err)
	}

	// 2. Prepare Docker configuration
	// Using itzg/minecraft-server image, which is highly configurable via env vars
	envVars := []string{
		"EULA=TRUE",
		"TYPE=PAPER", // Default, can be customized
		"VERSION=" + server.MinecraftVersion,
		"MAX_PLAYERS=" + strconv.Itoa(server.Players.Max),
		"MOTD=" + server.Name,
		// In a real app, memory would come from the template or creation form
		"MEMORY=2G",
	}

	containerConfig := &container.Config{
		Image: "itzg/minecraft-server",
		Env:   envVars,
		Tty:   true,
	}

	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: server.DataPath,
				Target: "/data",
			},
		},
		// Port mapping would be dynamic in a real scenario
		// PortBindings: nat.PortMap{...},
	}

	// 3. Create the Docker container
	containerName := "enderdeploy_" + server.ID
	resp, err := s.docker.CreateContainer(context.Background(), containerConfig, hostConfig, containerName)
	if err != nil {
		return server, fmt.Errorf("failed to create docker container: %w", err)
	}
	server.DockerContainerID = resp.ID
	server.Status = "offline" // Initial status

	// 4. Write initial server record to DB
	stmt, err := s.db.Prepare("INSERT INTO servers(id, name, status, minecraft_version, java_version, players_max, ip_address, docker_container_id, data_path) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return server, fmt.Errorf("failed to prepare db statement: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(server.ID, server.Name, server.Status, server.MinecraftVersion, server.JavaVersion, server.Players.Max, server.IPAddress, server.DockerContainerID, server.DataPath)
	if err != nil {
		// Cleanup container if DB write fails
		s.docker.RemoveContainer(context.Background(), server.DockerContainerID)
		return server, fmt.Errorf("failed to write server to database: %w", err)
	}

	// 5. Broadcast update to clients
	s.broadcastServerUpdate(server)

	log.Printf("Successfully created server '%s' with container ID %s", server.Name, server.DockerContainerID)
	return server, nil
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

	// A restart would be needed for these changes to apply.
	// We can fetch the updated model to return it.
	updatedServer, err := s.GetServerByID(id)
	if err != nil {
		return models.Server{}, err
	}

	s.broadcastServerUpdate(updatedServer)
	return updatedServer, nil
}

// DeleteServer stops, removes, and deletes a server.
func (s *ServerService) DeleteServer(id string) error {
	var containerID, dataPath string
	err := s.db.QueryRow("SELECT docker_container_id, data_path FROM servers WHERE id = ?", id).Scan(&containerID, &dataPath)
	if err != nil {
		return fmt.Errorf("could not find server in DB: %w", err)
	}

	// Stop and Remove container
	log.Printf("Stopping and removing container %s", containerID)
	s.docker.StopContainer(context.Background(), containerID)
	err = s.docker.RemoveContainer(context.Background(), containerID)
	if err != nil && !client.IsErrNotFound(err) {
		log.Printf("Warning: could not remove container %s: %v", containerID, err)
	}

	// Delete from DB
	log.Printf("Deleting server %s from database", id)
	_, err = s.db.Exec("DELETE FROM servers WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete server from DB: %w", err)
	}

	// Delete server files
	log.Printf("Deleting server data at %s", dataPath)
	err = os.RemoveAll(dataPath)
	if err != nil {
		log.Printf("Warning: failed to delete server data directory %s: %v", dataPath, err)
	}

	// Broadcast deletion
	s.hub.Broadcast <- []byte(`{"event": "server_deleted", "id": "` + id + `"}`)
	return nil
}

// PerformServerAction handles start, stop, restart.
func (s *ServerService) PerformServerAction(id, action string) error {
	var containerID string
	err := s.db.QueryRow("SELECT docker_container_id FROM servers WHERE id = ?", id).Scan(&containerID)
	if err != nil {
		return fmt.Errorf("could not find server in DB: %w", err)
	}

	var newStatus string
	ctx := context.Background()

	switch action {
	case "start":
		log.Printf("Starting container %s", containerID)
		if err := s.docker.StartContainer(ctx, containerID); err != nil {
			return err
		}
		newStatus = "online"
	case "stop":
		log.Printf("Stopping container %s", containerID)
		if err := s.docker.StopContainer(ctx, containerID); err != nil {
			return err
		}
		newStatus = "offline"
	case "restart":
		log.Printf("Restarting container %s", containerID)
		if err := s.docker.RestartContainer(ctx, containerID); err != nil {
			return err
		}
		newStatus = "online"
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	// Update status in DB
	_, err = s.db.Exec("UPDATE servers SET status = ? WHERE id = ?", newStatus, id)
	if err != nil {
		return fmt.Errorf("failed to update server status in DB: %w", err)
	}

	updatedServer, _ := s.GetServerByID(id)
	updatedServer.Status = newStatus
	s.broadcastServerUpdate(updatedServer)

	return nil
}

// broadcastServerUpdate sends a JSON message to all websocket clients with the server's state
func (s *ServerService) broadcastServerUpdate(server models.Server) {
	msg := map[string]interface{}{
		"event": "server_update",
		"data":  server,
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshalling server update: %v", err)
		return
	}
	s.hub.Broadcast <- jsonMsg
}
