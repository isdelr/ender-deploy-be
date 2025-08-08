package models

import (
	"encoding/json"
)

// Template represents a blueprint for creating a new Minecraft server.
type Template struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	MinecraftVersion string `json:"minecraftVersion"`
	JavaVersion      string `json:"javaVersion"`
	ServerType       string `json:"serverType"`
	ServerJarURL     string `json:"serverJarURL,omitempty"` // URL to the server JAR file
	StartupCommand   string `json:"startupCommand,omitempty"`
	MinMemoryMB      int    `json:"minMemoryMB"`
	MaxMemoryMB      int    `json:"maxMemoryMB"`
	IconURL          string `json:"iconURL,omitempty"`

	// New direct properties
	Difficulty string `json:"difficulty,omitempty"`

	// JSON string fields for DB storage
	TagsJSON          string `json:"-"`
	JVMArgsJSON       string `json:"-"`
	PropertiesJSON    string `json:"-"`
	ModsJSON          string `json:"-"`
	PluginsJSON       string `json:"-"`
	OpsJSON           string `json:"-"`
	WhitelistJSON     string `json:"-"`
	DatapacksJSON     string `json:"-"`
	ResourcePacksJSON string `json:"-"`
	BannedPlayersJSON string `json:"-"`
	BannedIPsJSON     string `json:"-"`

	// Slice/Map fields for API interaction
	Tags          []string          `json:"tags,omitempty"`
	JVMArgs       []string          `json:"jvmArgs,omitempty"`
	Properties    map[string]string `json:"properties,omitempty"`
	Mods          []string          `json:"mods,omitempty"`
	Plugins       []string          `json:"plugins,omitempty"`
	Ops           []string          `json:"ops,omitempty"`
	Whitelist     []string          `json:"whitelist,omitempty"`
	Datapacks     []string          `json:"datapacks,omitempty"`
	ResourcePacks []string          `json:"resourcePacks,omitempty"`
	BannedPlayers []string          `json:"bannedPlayers,omitempty"`
	BannedIPs     []string          `json:"bannedIPs,omitempty"`
}

// PrepareForSave marshals all slice/map fields into their respective JSON strings for DB storage.
func (t *Template) PrepareForSave() {
	tagsBytes, _ := json.Marshal(t.Tags)
	t.TagsJSON = string(tagsBytes)

	jvmArgsBytes, _ := json.Marshal(t.JVMArgs)
	t.JVMArgsJSON = string(jvmArgsBytes)

	propertiesBytes, _ := json.Marshal(t.Properties)
	t.PropertiesJSON = string(propertiesBytes)

	modsBytes, _ := json.Marshal(t.Mods)
	t.ModsJSON = string(modsBytes)

	pluginsBytes, _ := json.Marshal(t.Plugins)
	t.PluginsJSON = string(pluginsBytes)

	opsBytes, _ := json.Marshal(t.Ops)
	t.OpsJSON = string(opsBytes)

	whitelistBytes, _ := json.Marshal(t.Whitelist)
	t.WhitelistJSON = string(whitelistBytes)

	datapacksBytes, _ := json.Marshal(t.Datapacks)
	t.DatapacksJSON = string(datapacksBytes)

	resourcePacksBytes, _ := json.Marshal(t.ResourcePacks)
	t.ResourcePacksJSON = string(resourcePacksBytes)

	bannedPlayersBytes, _ := json.Marshal(t.BannedPlayers)
	t.BannedPlayersJSON = string(bannedPlayersBytes)

	bannedIPsBytes, _ := json.Marshal(t.BannedIPs)
	t.BannedIPsJSON = string(bannedIPsBytes)
}

// PrepareForAPI unmarshals all JSON string fields into their respective slice/map fields for API responses.
func (t *Template) PrepareForAPI() {
	if t.TagsJSON != "" {
		json.Unmarshal([]byte(t.TagsJSON), &t.Tags)
	}
	if t.JVMArgsJSON != "" {
		json.Unmarshal([]byte(t.JVMArgsJSON), &t.JVMArgs)
	}
	if t.PropertiesJSON != "" {
		json.Unmarshal([]byte(t.PropertiesJSON), &t.Properties)
	}
	if t.ModsJSON != "" {
		json.Unmarshal([]byte(t.ModsJSON), &t.Mods)
	}
	if t.PluginsJSON != "" {
		json.Unmarshal([]byte(t.PluginsJSON), &t.Plugins)
	}
	if t.OpsJSON != "" {
		json.Unmarshal([]byte(t.OpsJSON), &t.Ops)
	}
	if t.WhitelistJSON != "" {
		json.Unmarshal([]byte(t.WhitelistJSON), &t.Whitelist)
	}
	if t.DatapacksJSON != "" {
		json.Unmarshal([]byte(t.DatapacksJSON), &t.Datapacks)
	}
	if t.ResourcePacksJSON != "" {
		json.Unmarshal([]byte(t.ResourcePacksJSON), &t.ResourcePacks)
	}
	if t.BannedPlayersJSON != "" {
		json.Unmarshal([]byte(t.BannedPlayersJSON), &t.BannedPlayers)
	}
	if t.BannedIPsJSON != "" {
		json.Unmarshal([]byte(t.BannedIPsJSON), &t.BannedIPs)
	}
}
