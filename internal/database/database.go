package database

import (
	"database/sql"

	_ "modernc.org/sqlite" // SQLite driver
)

// New creates a new database connection pool.
func New(dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dataSourceName+"?_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

// Migrate runs the SQL statements to set up the database schema.
func Migrate(db *sql.DB) error {
	const sqlStmt = `
	CREATE TABLE IF NOT EXISTS templates (
		id TEXT NOT NULL PRIMARY KEY,
		name TEXT,
		description TEXT,
		minecraft_version TEXT,
		java_version TEXT,
		server_type TEXT, -- e.g. Vanilla, Forge, Fabric
		min_memory_mb INTEGER,
		max_memory_mb INTEGER,
		-- Store complex fields as JSON text
		tags_json TEXT,
		jvm_args_json TEXT,
		properties_json TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS servers (
		id TEXT NOT NULL PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT NOT NULL,
		port INTEGER,
		minecraft_version TEXT,
		java_version TEXT,
		players_current INTEGER,
		players_max INTEGER,
		cpu_usage REAL,
		ram_usage REAL,
		storage_usage INTEGER,
		ip_address TEXT,
		modpack_name TEXT,
		modpack_version TEXT,
		docker_container_id TEXT,
		data_path TEXT,
		template_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(template_id) REFERENCES templates(id)
	);

	CREATE TABLE IF NOT EXISTS users (
		id TEXT NOT NULL PRIMARY KEY,
		username TEXT UNIQUE,
		email TEXT UNIQUE,
		password_hash TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS resource_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		server_id TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		cpu_usage REAL,
		ram_usage REAL,
		players_current INTEGER,
		FOREIGN KEY(server_id) REFERENCES servers(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS backups (
		id TEXT NOT NULL PRIMARY KEY,
		server_id TEXT NOT NULL,
		name TEXT NOT NULL,
		path TEXT NOT NULL,
		size INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(server_id) REFERENCES servers(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS events (
		id TEXT NOT NULL PRIMARY KEY,
		type TEXT NOT NULL,
		level TEXT NOT NULL,
		message TEXT NOT NULL,
		server_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(server_id) REFERENCES servers(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS schedules (
		id TEXT NOT NULL PRIMARY KEY,
		server_id TEXT NOT NULL,
		name TEXT NOT NULL,
		cron_expression TEXT NOT NULL,
		task_type TEXT NOT NULL,
		payload_json TEXT,
		is_active BOOLEAN NOT NULL DEFAULT TRUE,
		last_run_at DATETIME,
		next_run_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(server_id) REFERENCES servers(id) ON DELETE CASCADE
	);
	`
	_, err := db.Exec(sqlStmt)
	return err
}
