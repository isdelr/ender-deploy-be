package models

import "time"

// Server represents a single Minecraft server instance.
type Server struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Status            string         `json:"status"` // e.g., "online", "offline", "starting"
	Port              int            `json:"port"`
	MinecraftVersion  string         `json:"minecraftVersion"`
	JavaVersion       string         `json:"javaVersion"`
	Players           PlayerInfo     `json:"players"`
	Resources         ResourceUsage  `json:"resources"`
	IPAddress         string         `json:"ipAddress"`
	Modpack           *ModpackInfo   `json:"modpack,omitempty"`
	TemplateID        string         `json:"templateId,omitempty"`
	DockerContainerID string         `json:"-"` // Internal use
	DataPath          string         `json:"-"` // Internal use
	CreatedAt         time.Time      `json:"createdAt"`
	Settings          ServerSettings `json:"settings,omitempty"`
}

// PlayerInfo holds current and max player counts.
type PlayerInfo struct {
	Current int `json:"current"`
	Max     int `json:"max"`
}

// ResourceUsage holds CPU, RAM, and Storage percentages.
type ResourceUsage struct {
	CPU     float64 `json:"cpu"` // As percentage
	RAM     float64 `json:"ram"` // As percentage
	Storage int     `json:"storage"`
}

// ModpackInfo holds details about a server's modpack.
type ModpackInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerSettings represents the editable properties of a server.
type ServerSettings map[string]string

// FileInfo represents a file or directory in the file manager.
type FileInfo struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	IsDir    bool      `json:"isDir"`
	Modified time.Time `json:"modified"`
}

// ResourceDataPoint represents a single point in time for resource usage.
type ResourceDataPoint struct {
	Timestamp      time.Time `json:"timestamp"`
	CPUUsage       float64   `json:"cpuUsage"`
	RAMUsage       float64   `json:"ramUsage"`
	PlayersCurrent int       `json:"playersCurrent"`
}

// OnlinePlayer represents a player currently on the server.
type OnlinePlayer struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// DashboardStats represents the aggregated data for the main dashboard.
type DashboardStats struct {
	TotalServers     int                 `json:"totalServers"`
	OnlineServers    int                 `json:"onlineServers"`
	TotalPlayers     int                 `json:"totalPlayers"`
	MaxPlayers       int                 `json:"maxPlayers"`
	SystemHealth     float64             `json:"systemHealth"` // Example metric
	ServerStatusDist map[string]int      `json:"serverStatusDist"`
	PlayerHistory    []ResourceDataPoint `json:"playerHistory"`
	ResourceHistory  []ResourceDataPoint `json:"resourceHistory"`
}
