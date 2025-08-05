package services

import (
	"database/sql"
	"encoding/json"
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
	err := scanner.Scan(
		&tmpl.ID, &tmpl.Name, &tmpl.Description, &tmpl.MinecraftVersion,
		&tmpl.JavaVersion, &tmpl.ServerType, &tmpl.MinMemoryMB, &tmpl.MaxMemoryMB,
		&tmpl.TagsJSON, &tmpl.JVMArgsJSON, &tmpl.PropertiesJSON,
	)
	return tmpl, err
}

// GetAllTemplates retrieves all templates from the database.
func (s *TemplateService) GetAllTemplates() ([]models.Template, error) {
	rows, err := s.db.Query("SELECT id, name, description, minecraft_version, java_version, server_type, min_memory_mb, max_memory_mb, tags_json, jvm_args_json, properties_json FROM templates")
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
	row := s.db.QueryRow(`
		SELECT id, name, description, minecraft_version, java_version, server_type, 
		       min_memory_mb, max_memory_mb, tags_json, jvm_args_json, properties_json 
		FROM templates WHERE id = ?`, id)

	tmpl, err := scanTemplate(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.Template{}, fmt.Errorf("template with id %s not found", id)
		}
		return models.Template{}, err
	}
	return tmpl, nil
}

// prepareTemplateForSave ensures JSON fields are correctly populated before DB insertion.
func (s *TemplateService) prepareTemplateForSave(template *models.Template) {
	tagsBytes, _ := json.Marshal(template.Tags)
	template.TagsJSON = string(tagsBytes)

	jvmArgsBytes, _ := json.Marshal(template.JVMArgs)
	template.JVMArgsJSON = string(jvmArgsBytes)

	propertiesBytes, _ := json.Marshal(template.Properties)
	template.PropertiesJSON = string(propertiesBytes)
}

// CreateTemplate adds a new template to the database.
func (s *TemplateService) CreateTemplate(template models.Template) (models.Template, error) {
	s.prepareTemplateForSave(&template)
	stmt, err := s.db.Prepare(`
		INSERT INTO templates(id, name, description, minecraft_version, java_version, server_type, 
		                    min_memory_mb, max_memory_mb, tags_json, jvm_args_json, properties_json) 
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return models.Template{}, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		template.ID, template.Name, template.Description, template.MinecraftVersion,
		template.JavaVersion, template.ServerType, template.MinMemoryMB, template.MaxMemoryMB,
		template.TagsJSON, template.JVMArgsJSON, template.PropertiesJSON,
	)
	if err != nil {
		return models.Template{}, fmt.Errorf("failed to execute statement: %w", err)
	}

	return template, nil
}

// UpdateTemplate updates an existing template in the database.
func (s *TemplateService) UpdateTemplate(id string, template models.Template) (models.Template, error) {
	s.prepareTemplateForSave(&template)
	stmt, err := s.db.Prepare(`
		UPDATE templates SET name = ?, description = ?, minecraft_version = ?, java_version = ?, 
		                    server_type = ?, min_memory_mb = ?, max_memory_mb = ?, tags_json = ?, 
		                    jvm_args_json = ?, properties_json = ?
		WHERE id = ?`)
	if err != nil {
		return models.Template{}, err
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		template.Name, template.Description, template.MinecraftVersion, template.JavaVersion,
		template.ServerType, template.MinMemoryMB, template.MaxMemoryMB, template.TagsJSON,
		template.JVMArgsJSON, template.PropertiesJSON, id,
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
