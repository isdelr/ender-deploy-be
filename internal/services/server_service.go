package services

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
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
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	"github.com/gorcon/rcon"
	"github.com/isdelr/ender-deploy-be/internal/docker"
	"github.com/isdelr/ender-deploy-be/internal/models"
	"github.com/isdelr/ender-deploy-be/internal/websocket"
	"github.com/rs/zerolog/log"
)

const (
	RCONPort     = "25575"
	RCONPassword = "ender-deploy-rcon-password" // This should be configurable per server
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
	rows, err := s.db.Query(`
		SELECT id, name, status, port, minecraft_version, java_version,
		       players_current, players_max, cpu_usage, ram_usage, storage_usage,
		       ip_address, modpack_name, modpack_version, docker_container_id, data_path
		FROM servers
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []models.Server
	for rows.Next() {
		var srv models.Server
		var modpackName, modpackVersion, dockerContainerID, dataPath sql.NullString
		var playersCurrent, playersMax, storageUsage sql.NullInt64
		var cpuUsage, ramUsage sql.NullFloat64
		var port sql.NullInt32
		var ipAddress sql.NullString

		err := rows.Scan(
			&srv.ID, &srv.Name, &srv.Status, &port, &srv.MinecraftVersion, &srv.JavaVersion,
			&playersCurrent, &playersMax, &cpuUsage, &ramUsage, &storageUsage,
			&ipAddress, &modpackName, &modpackVersion, &dockerContainerID, &dataPath,
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
	var modpackName, modpackVersion, containerID, dataPath, templateID, ipAddress sql.NullString
	// Use nullable types to scan from DB
	var playersCurrent, playersMax, storageUsage sql.NullInt64
	var cpuUsage, ramUsage sql.NullFloat64
	var port sql.NullInt32

	row := s.db.QueryRow(`
		SELECT id, name, status, port, minecraft_version, java_version,
		       players_current, players_max, cpu_usage, ram_usage, storage_usage,
		       ip_address, modpack_name, modpack_version, docker_container_id, data_path, template_id
		FROM servers WHERE id = ?`, id)
	err := row.Scan(
		&srv.ID, &srv.Name, &srv.Status, &port, &srv.MinecraftVersion, &srv.JavaVersion,
		&playersCurrent, &playersMax, &cpuUsage, &ramUsage, &storageUsage,
		&ipAddress, &modpackName, &modpackVersion, &containerID, &dataPath, &templateID)
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

	// This block checks if the image exists and pulls it if needed.
	ctx := context.Background()
	imageName := "itzg/minecraft-server:latest"

	_, _, err = s.docker.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		// client.IsErrNotFound is the canonical way to check for a missing image.
		if client.IsErrNotFound(err) {
			log.Info().Str("image", imageName).Msg("Image not found locally. Pulling from Docker Hub...")
			puller, pullErr := s.docker.ImagePull(ctx, imageName, image.PullOptions{})
			if pullErr != nil {
				return models.Server{}, fmt.Errorf("failed to start image pull: %w", pullErr)
			}
			defer puller.Close()
			// This will stream the pull progress to your backend's stdout, which is useful for debugging.
			io.Copy(os.Stdout, puller)
			log.Info().Str("image", imageName).Msg("Image pulled successfully.")
		} else {
			// A different error occurred during image inspection.
			return models.Server{}, fmt.Errorf("failed to inspect docker image '%s': %w", imageName, err)
		}
	} else {
		log.Info().Str("image", imageName).Msg("Image found locally.")
	}

	server := models.Server{
		ID:               uuid.New().String(),
		Name:             name,
		Status:           "offline",
		MinecraftVersion: template.MinecraftVersion,
		JavaVersion:      template.JavaVersion,
		TemplateID:       template.ID,
	}

	// --- OS-INDEPENDENT PATH HANDLING ---
	server.DataPath = filepath.Join(s.serverDataPath, server.ID)
	absDataPath, err := filepath.Abs(server.DataPath)
	if err != nil {
		return server, fmt.Errorf("failed to get absolute path for server data: %w", err)
	}
	if err := os.MkdirAll(absDataPath, 0755); err != nil {
		return server, fmt.Errorf("failed to create server data directory: %w", err)
	}
	// --- END ---

	envVars, err := s.buildEnvVarsFromTemplate(template, name)
	if err != nil {
		return server, fmt.Errorf("failed to build environment variables: %w", err)
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

	exposedPorts := nat.PortSet{
		"25565/tcp": {},
		"25575/tcp": {},
	}
	portBindings := nat.PortMap{
		"25565/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: strconv.Itoa(gamePort)}},
		"25575/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: strconv.Itoa(rconPort)}},
	}

	containerConfig := &container.Config{
		Image:        imageName, // Use the variable defined above
		Env:          envVars,
		Tty:          true,
		ExposedPorts: exposedPorts,
		Labels: map[string]string{
			"com.ender-deploy.managed":  "true",
			"com.ender-deploy.serverId": server.ID,
		},
	}

	hostConfig := &container.HostConfig{
		Mounts:       []mount.Mount{{Type: mount.TypeBind, Source: filepath.ToSlash(absDataPath), Target: "/data"}},
		PortBindings: portBindings,
	}

	containerName := "enderdeploy_" + server.ID
	// Use the same context `ctx` from the image pull operation
	resp, err := s.docker.CreateContainer(ctx, containerConfig, hostConfig, containerName)
	if err != nil {
		return server, fmt.Errorf("failed to create docker container: %w", err)
	}
	server.DockerContainerID = resp.ID
	server.DataPath = absDataPath

	props := template.GetProperties()
	maxPlayers := 20 // default
	if mpStr, ok := props["max-players"]; ok {
		if mp, err := strconv.Atoi(mpStr); err == nil {
			maxPlayers = mp
		}
	}
	server.Players.Max = maxPlayers

	stmt, err := s.db.Prepare(`
		INSERT INTO servers(id, name, status, minecraft_version, java_version, docker_container_id, data_path, template_id, port, ip_address, players_max)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return server, fmt.Errorf("failed to prepare db statement: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(server.ID, server.Name, server.Status, server.MinecraftVersion, server.JavaVersion, server.DockerContainerID, server.DataPath, server.TemplateID, server.Port, server.IPAddress, maxPlayers)
	if err != nil {
		s.docker.RemoveContainer(context.Background(), server.DockerContainerID) // Cleanup container
		return server, fmt.Errorf("failed to write server to database: %w", err)
	}

	newServer, _ := s.GetServerByID(server.ID)
	s.broadcastServerUpdate(newServer)

	s.eventService.CreateEvent("server.create", "info", fmt.Sprintf("Server '%s' was created successfully.", newServer.Name), &newServer.ID)
	log.Info().Str("server_name", server.Name).Str("template_name", template.Name).Str("container_id", server.DockerContainerID).Msg("Successfully created server")
	return newServer, nil
}

// buildEnvVarsFromTemplate creates a slice of "KEY=VALUE" strings for Docker.
func (s *ServerService) buildEnvVarsFromTemplate(template models.Template, serverName string) ([]string, error) {
	env := []string{
		"EULA=TRUE",
		"TYPE=" + template.ServerType,
		"VERSION=" + template.MinecraftVersion,
		"MEMORY=" + strconv.Itoa(template.MaxMemoryMB) + "M",
		"MOTD=" + serverName,
		"ENABLE_RCON=true",
		"RCON_PORT=" + RCONPort,
		"RCON_PASSWORD=" + RCONPassword,
	}

	if len(template.JVMArgs) > 0 {
		env = append(env, "JVM_ARGS="+strings.Join(template.JVMArgs, " "))
	}

	properties := template.GetProperties()
	for key, val := range properties {
		envKey := "CFG_" + strings.ReplaceAll(strings.ToUpper(key), "-", "_")
		env = append(env, fmt.Sprintf("%s=%v", envKey, val))
	}

	return env, nil
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

// PerformServerAction handles start, stop, restart.
func (s *ServerService) PerformServerAction(id, action string) error {
	server, err := s.GetServerByID(id)
	if err != nil {
		return fmt.Errorf("could not find server in DB: %w", err)
	}

	var newStatus, eventLevel, eventMessage string
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
		eventMessage = fmt.Sprintf("Server '%s' has started.", server.Name)
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
	case "reinstall":
		logCtx.Msg("Reinstalling server")
		if err := s.docker.StopContainer(ctx, server.DockerContainerID); err != nil {
			return fmt.Errorf("failed to stop server for reinstall: %w", err)
		}
		time.Sleep(5 * time.Second) // Give it a moment to fully stop

		// Delete contents of data directory
		dir, _ := os.ReadDir(server.DataPath)
		for _, d := range dir {
			os.RemoveAll(filepath.Join(server.DataPath, d.Name()))
		}
		log.Info().Str("server_id", id).Msg("Cleared data directory")

		if err := s.docker.StartContainer(ctx, server.DockerContainerID); err != nil {
			return fmt.Errorf("failed to start server after reinstall: %w", err)
		}
		newStatus = "starting"
		eventLevel = "warn"
		eventMessage = fmt.Sprintf("Server '%s' was reinstalled.", server.Name)
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

	// Find the RCON port from the container info
	containerInfo, err := s.docker.InspectContainer(context.Background(), server.DockerContainerID)
	if err != nil {
		return "", fmt.Errorf("could not inspect container: %w", err)
	}

	rconPortBinding, ok := containerInfo.NetworkSettings.Ports["25575/tcp"]
	if !ok || len(rconPortBinding) == 0 {
		return "", fmt.Errorf("rcon port not bound for server %s", serverID)
	}
	rconAddr := "127.0.0.1:" + rconPortBinding[0].HostPort

	conn, err := rcon.Dial(rconAddr, RCONPassword)
	if err != nil {
		return "", fmt.Errorf("could not connect via rcon: %w", err)
	}
	defer conn.Close()

	response, err := conn.Execute(command)
	if err != nil {
		return "", fmt.Errorf("rcon command failed: %w", err)
	}

	log.Info().Str("command", command).Str("server_name", server.Name).Str("response", response).Msg("RCON command executed")

	// Broadcast command and response to subscribed log viewers
	logMsg := fmt.Sprintf("CMD> %s\n%s", command, response)
	msg := websocket.Message{Action: "log_message", Payload: logMsg}
	jsonMsg, _ := json.Marshal(msg)
	s.hub.BroadcastTo(serverID, jsonMsg)

	return response, nil
}

// StreamServerLogs streams the logs of a container to a websocket client.
// The context passed in is from the HTTP request, which is cancelled on client disconnect.
func (s *ServerService) StreamServerLogs(ctx context.Context, serverID string, sendChan chan []byte) {
	server, err := s.GetServerByID(serverID)
	if err != nil {
		log.Warn().Str("server_id", serverID).Msg("Cannot stream logs, server not found")
		return
	}

	// Do not create a new context; use the one that was passed in.
	// Its cancellation will signal that the client has disconnected.

	logReader, err := s.docker.GetContainerLogs(ctx, server.DockerContainerID, true)
	if err != nil {
		// This error is expected if the context is already cancelled (client disconnected before logs could be fetched).
		if err != context.Canceled {
			log.Error().Err(err).Str("server_id", serverID).Msg("Failed to get container logs")
		}
		return
	}
	defer logReader.Close()

	// Stream logs line by line
	scanner := bufio.NewScanner(logReader)
	for scanner.Scan() {
		// Construct a WebSocket message
		wsMsg := websocket.Message{
			Action:  "log_message",
			Payload: scanner.Text(),
		}
		jsonMsg, _ := json.Marshal(wsMsg)

		// This select block now correctly uses the client's connection context for cancellation.
		// It will block until one of the cases is ready.
		select {
		case <-ctx.Done():
			// The client's connection was closed. Stop streaming.
			log.Info().Str("server_id", serverID).Msg("Client disconnected, stopping log stream.")
			return
		case sendChan <- jsonMsg:
			// Message was successfully sent to the client's channel.
		}
	}

	if err := scanner.Err(); err != nil {
		// Don't log an error if it was caused by the context being canceled.
		if err != context.Canceled {
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
		// FIX: Assign the sql.NullInt64 struct directly.
		dp.PlayersCurrent = totalPlayers
		stats.ResourceHistory = append(stats.ResourceHistory, dp)
	}
	// For simplicity, player history will mirror resource history for the global view.
	stats.PlayerHistory = stats.ResourceHistory

	return stats, nil
}

// GetResourceHistory gets recent resource usage for a specific server.
func (s *ServerService) GetResourceHistory(serverID string) ([]models.ResourceDataPoint, error) {
	rows, err := s.db.Query(`
		SELECT timestamp, cpu_usage, ram_usage, players_current
		FROM resource_history
		WHERE server_id = ? AND timestamp >= ?
		ORDER BY timestamp ASC`, serverID, time.Now().Add(-30*time.Minute))
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
		players[i] = models.OnlinePlayer{Name: name, UUID: name} // Using name as UUID placeholder for cravatar
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

	// Sanitize the path to prevent directory traversal
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

	// Sanitize the path to prevent directory traversal
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

	// Sanitize the path to prevent directory traversal
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
		// If the file doesn't exist yet (e.g., server hasn't started), return empty settings
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
		// Ignore comments and empty lines
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

	// Restart the server for settings to take effect
	return s.PerformServerAction(serverID, "restart")
}
