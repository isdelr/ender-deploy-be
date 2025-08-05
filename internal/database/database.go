package database

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// New creates a new database connection pool.
func New(dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dataSourceName+"?_foreign_keys=on")
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
	CREATE TABLE IF NOT EXISTS servers (
		id TEXT NOT NULL PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT NOT NULL,
		minecraft_version TEXT,
		java_version TEXT,
		players_current INTEGER,
		players_max INTEGER,
		cpu_usage INTEGER,
		ram_usage INTEGER,
		storage_usage INTEGER,
		ip_address TEXT,
		modpack_name TEXT,
		modpack_version TEXT,
		docker_container_id TEXT,
		data_path TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS users (
		id TEXT NOT NULL PRIMARY KEY,
		username TEXT UNIQUE,
		email TEXT UNIQUE,
		password_hash TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

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
	`
	_, err := db.Exec(sqlStmt)
	return err
}
