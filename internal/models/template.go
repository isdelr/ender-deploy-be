package models

import (
	"encoding/json"
)

// Template represents a blueprint for creating a new Minecraft server.
type Template struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	MinecraftVersion string            `json:"minecraftVersion"`
	JavaVersion      string            `json:"javaVersion"`
	ServerType       string            `json:"serverType"`           // e.g., Vanilla, Forge, Fabric
	MinMemoryMB      int               `json:"minMemoryMB"`          // Renamed for clarity
	MaxMemoryMB      int               `json:"maxMemoryMB"`          // Renamed for clarity
	TagsJSON         string            `json:"-"`                    // Stored as JSON array string "[\"tag1\", \"tag2\"]"
	JVMArgsJSON      string            `json:"-"`                    // Stored as JSON array string "[\"-Xmx...\", \"-Xms...\"]"
	PropertiesJSON   string            `json:"-"`                    // Stored as JSON object string "{\"key\":\"value\"}"
	Tags             []string          `json:"tags,omitempty"`       // Exposed to frontend
	JVMArgs          []string          `json:"jvmArgs,omitempty"`    // Exposed to frontend
	Properties       map[string]string `json:"properties,omitempty"` // Exposed to frontend
}

// MarshalJSON custom marshaler to handle JSON fields
func (t *Template) MarshalJSON() ([]byte, error) {
	type Alias Template
	return json.Marshal(&struct {
		*Alias
		Tags       []string          `json:"tags,omitempty"`
		JVMArgs    []string          `json:"jvmArgs,omitempty"`
		Properties map[string]string `json:"properties,omitempty"`
	}{
		Alias:      (*Alias)(t),
		Tags:       t.GetTags(),
		JVMArgs:    t.GetJVMArgs(),
		Properties: t.GetProperties(),
	})
}

// UnmarshalJSON custom unmarshaler to handle JSON fields
func (t *Template) UnmarshalJSON(data []byte) error {
	type Alias Template
	aux := &struct {
		Tags       []string          `json:"tags,omitempty"`
		JVMArgs    []string          `json:"jvmArgs,omitempty"`
		Properties map[string]string `json:"properties,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	t.SetTags(aux.Tags)
	t.SetJVMArgs(aux.JVMArgs)
	t.SetProperties(aux.Properties)
	return nil
}

// GetTags returns the tags as a slice of strings.
func (t *Template) GetTags() []string {
	if t.TagsJSON == "" {
		return []string{}
	}
	var tags []string
	json.Unmarshal([]byte(t.TagsJSON), &tags)
	return tags
}

// SetTags sets the JSON string for tags.
func (t *Template) SetTags(tags []string) {
	t.Tags = tags
	jsonBytes, _ := json.Marshal(tags)
	t.TagsJSON = string(jsonBytes)
}

// GetJVMArgs returns the JVM arguments as a slice of strings.
func (t *Template) GetJVMArgs() []string {
	if t.JVMArgsJSON == "" {
		return []string{}
	}
	var args []string
	json.Unmarshal([]byte(t.JVMArgsJSON), &args)
	return args
}

// SetJVMArgs sets the JSON string for JVM args.
func (t *Template) SetJVMArgs(args []string) {
	t.JVMArgs = args
	jsonBytes, _ := json.Marshal(args)
	t.JVMArgsJSON = string(jsonBytes)
}

// GetProperties returns the server properties as a map.
func (t *Template) GetProperties() map[string]string {
	if t.PropertiesJSON == "" {
		return make(map[string]string)
	}
	var properties map[string]string
	json.Unmarshal([]byte(t.PropertiesJSON), &properties)
	return properties
}

// SetProperties sets the JSON string for properties.
func (t *Template) SetProperties(properties map[string]string) {
	t.Properties = properties
	jsonBytes, _ := json.Marshal(properties)
	t.PropertiesJSON = string(jsonBytes)
}
