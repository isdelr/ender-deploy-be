package services

import (
	"database/sql"
	"fmt"

	"github.com/isdelr/ender-deploy-be/internal/models"
)

// TemplateServiceProvider defines the interface for template services.
type TemplateServiceProvider interface {
	GetAllTemplates() ([]models.Template, error)
	GetTemplateByID(id string) (models.Template, error)
	CreateTemplate(template models.Template) (models.Template, error)
	UpdateTemplate(id string, template models.Template) (models.Template, error)
	DeleteTemplate(id string) error
}

// TemplateService provides business logic for template management.
type TemplateService struct {
	db *sql.DB
}

// NewTemplateService creates a new TemplateService.
func NewTemplateService(db *sql.DB) *TemplateService {
	return &TemplateService{db: db}
}

// scanTemplate is a helper to scan a template from a row or rows object.
func scanTemplate(scanner interface{ Scan(...interface{}) error }) (models.Template, error) {
	var tmpl models.Template
	var desc, modpackType, modpackURL, difficulty, iconURL sql.NullString
	var tags, jvm, props, mods, plugins, ops, whitelist sql.NullString
	var datapacks, resourcePacks, bannedPlayers, bannedIPs sql.NullString

	err := scanner.Scan(
		&tmpl.ID, &tmpl.Name, &desc, &tmpl.MinecraftVersion,
		&tmpl.JavaVersion, &tmpl.ServerType, &modpackType, &modpackURL,
		&tmpl.MinMemoryMB, &tmpl.MaxMemoryMB, &difficulty, &iconURL,
		&tags, &jvm, &props, &mods, &plugins, &ops, &whitelist,
		&datapacks, &resourcePacks, &bannedPlayers, &bannedIPs,
	)

	if err != nil {
		return tmpl, err
	}

	// Assign values from nullable types
	tmpl.Description = desc.String
	tmpl.ModpackType = modpackType.String
	tmpl.ModpackURL = modpackURL.String
	tmpl.Difficulty = difficulty.String
	tmpl.IconURL = iconURL.String
	tmpl.TagsJSON = tags.String
	tmpl.JVMArgsJSON = jvm.String
	tmpl.PropertiesJSON = props.String
	tmpl.ModsJSON = mods.String
	tmpl.PluginsJSON = plugins.String
	tmpl.OpsJSON = ops.String
	tmpl.WhitelistJSON = whitelist.String
	tmpl.DatapacksJSON = datapacks.String
	tmpl.ResourcePacksJSON = resourcePacks.String
	tmpl.BannedPlayersJSON = bannedPlayers.String
	tmpl.BannedIPsJSON = bannedIPs.String

	tmpl.PrepareForAPI() // Unmarshal all JSON fields
	return tmpl, nil
}

// GetAllTemplates retrieves all templates from the database.
func (s *TemplateService) GetAllTemplates() ([]models.Template, error) {
	const query = `
		SELECT id, name, description, minecraft_version, java_version, server_type, 
		       modpack_type, modpack_url, min_memory_mb, max_memory_mb, difficulty, icon_url,
		       tags_json, jvm_args_json, properties_json, mods_json, plugins_json, ops_json, whitelist_json,
			   datapacks_json, resource_packs_json, banned_players_json, banned_ips_json 
		FROM templates`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []models.Template
	for rows.Next() {
		tmpl, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, tmpl)
	}
	return templates, nil
}

// GetTemplateByID retrieves a single template by its ID.
func (s *TemplateService) GetTemplateByID(id string) (models.Template, error) {
	const query = `
		SELECT id, name, description, minecraft_version, java_version, server_type,
		       modpack_type, modpack_url, min_memory_mb, max_memory_mb, difficulty, icon_url,
		       tags_json, jvm_args_json, properties_json, mods_json, plugins_json, ops_json, whitelist_json,
			   datapacks_json, resource_packs_json, banned_players_json, banned_ips_json 
		FROM templates WHERE id = ?`
	row := s.db.QueryRow(query, id)

	tmpl, err := scanTemplate(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.Template{}, fmt.Errorf("template with id %s not found", id)
		}
		return models.Template{}, err
	}
	return tmpl, nil
}

// CreateTemplate adds a new template to the database.
func (s *TemplateService) CreateTemplate(template models.Template) (models.Template, error) {
	template.PrepareForSave()
	const query = `
		INSERT INTO templates(id, name, description, minecraft_version, java_version, server_type, 
		                    modpack_type, modpack_url, min_memory_mb, max_memory_mb, difficulty, icon_url,
		                    tags_json, jvm_args_json, properties_json, mods_json, plugins_json, ops_json, whitelist_json,
							datapacks_json, resource_packs_json, banned_players_json, banned_ips_json) 
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	stmt, err := s.db.Prepare(query)
	if err != nil {
		return models.Template{}, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		template.ID, template.Name, template.Description, template.MinecraftVersion,
		template.JavaVersion, template.ServerType, template.ModpackType, template.ModpackURL,
		template.MinMemoryMB, template.MaxMemoryMB, template.Difficulty, template.IconURL,
		template.TagsJSON, template.JVMArgsJSON, template.PropertiesJSON,
		template.ModsJSON, template.PluginsJSON, template.OpsJSON, template.WhitelistJSON,
		template.DatapacksJSON, template.ResourcePacksJSON, template.BannedPlayersJSON, template.BannedIPsJSON,
	)
	if err != nil {
		return models.Template{}, fmt.Errorf("failed to execute statement: %w", err)
	}

	return template, nil
}

// UpdateTemplate updates an existing template in the database.
func (s *TemplateService) UpdateTemplate(id string, template models.Template) (models.Template, error) {
	template.PrepareForSave()
	const query = `
		UPDATE templates SET name = ?, description = ?, minecraft_version = ?, java_version = ?, 
		                    server_type = ?, modpack_type = ?, modpack_url = ?, 
		                    min_memory_mb = ?, max_memory_mb = ?, difficulty = ?, icon_url = ?,
		                    tags_json = ?, jvm_args_json = ?, properties_json = ?,
		                    mods_json = ?, plugins_json = ?, ops_json = ?, whitelist_json = ?,
							datapacks_json = ?, resource_packs_json = ?, banned_players_json = ?, banned_ips_json = ?
		WHERE id = ?`
	stmt, err := s.db.Prepare(query)
	if err != nil {
		return models.Template{}, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		template.Name, template.Description, template.MinecraftVersion, template.JavaVersion,
		template.ServerType, template.ModpackType, template.ModpackURL,
		template.MinMemoryMB, template.MaxMemoryMB, template.Difficulty, template.IconURL,
		template.TagsJSON, template.JVMArgsJSON, template.PropertiesJSON,
		template.ModsJSON, template.PluginsJSON, template.OpsJSON, template.WhitelistJSON,
		template.DatapacksJSON, template.ResourcePacksJSON, template.BannedPlayersJSON, template.BannedIPsJSON,
		id,
	)
	if err != nil {
		return models.Template{}, err
	}

	return s.GetTemplateByID(id)
}

// DeleteTemplate removes a template from the database.
func (s *TemplateService) DeleteTemplate(id string) error {
	_, err := s.db.Exec("DELETE FROM templates WHERE id = ?", id)
	return err
}
