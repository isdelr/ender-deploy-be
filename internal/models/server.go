package models

// This file would contain the Go structs corresponding to your data.
// Based on your frontend code, you'd have structs like these.

// Server represents a single Minecraft server instance.
type Server struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	Status            string        `json:"status"` // e.g., "online", "offline", "starting"
	MinecraftVersion  string        `json:"minecraftVersion"`
	JavaVersion       string        `json:"javaVersion"`
	Players           PlayerInfo    `json:"players"`
	Resources         ResourceUsage `json:"resources"`
	IPAddress         string        `json:"ipAddress"`
	Modpack           *ModpackInfo  `json:"modpack,omitempty"`
	DockerContainerID string        `json:"-"` // Internal use
	DataPath          string        `json:"-"` // Internal use
}

// PlayerInfo holds current and max player counts.
type PlayerInfo struct {
	Current int `json:"current"`
	Max     int `json:"max"`
}

// ResourceUsage holds CPU, RAM, and Storage percentages.
type ResourceUsage struct {
	CPU     int `json:"cpu"`
	RAM     int `json:"ram"`
	Storage int `json:"storage"`
}

// ModpackInfo holds details about a server's modpack.
type ModpackInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ... you would also define User, Template, etc. structs here ...
