package storage

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// Database wraps the SQLite database connection
type Database struct {
	db *sql.DB
	mu sync.RWMutex

	// Resource version counter
	currentResourceVersion int64
}

// NewDatabase creates a new database connection and initializes the schema
func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_journal_mode=WAL", dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(1) // SQLite works best with a single connection
	db.SetMaxIdleConns(1)

	d := &Database{
		db: db,
	}

	if err := d.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Load current resource version
	if err := d.loadResourceVersion(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load resource version: %w", err)
	}

	return d, nil
}

// initSchema creates the database tables
func (d *Database) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS bundledeployments (
		namespace TEXT NOT NULL,
		name TEXT NOT NULL,
		resource_version INTEGER NOT NULL,
		uid TEXT NOT NULL,
		creation_timestamp INTEGER NOT NULL,
		deletion_timestamp INTEGER,
		generation INTEGER NOT NULL,
		labels TEXT,
		annotations TEXT,
		finalizers TEXT,
		owner_references TEXT,
		spec TEXT NOT NULL,
		status TEXT,
		PRIMARY KEY (namespace, name)
	);

	CREATE INDEX IF NOT EXISTS idx_resource_version ON bundledeployments(resource_version);
	CREATE INDEX IF NOT EXISTS idx_labels ON bundledeployments(labels);
	CREATE INDEX IF NOT EXISTS idx_deletion_timestamp ON bundledeployments(deletion_timestamp);

	CREATE TABLE IF NOT EXISTS watch_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		resource_version INTEGER NOT NULL,
		event_type TEXT NOT NULL,
		namespace TEXT NOT NULL,
		name TEXT NOT NULL,
		timestamp INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_watch_rv ON watch_events(resource_version);
	CREATE INDEX IF NOT EXISTS idx_watch_timestamp ON watch_events(timestamp);

	CREATE TABLE IF NOT EXISTS resource_version (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		current_version INTEGER NOT NULL DEFAULT 1
	);

	INSERT OR IGNORE INTO resource_version (id, current_version) VALUES (1, 1);
	`

	_, err := d.db.Exec(schema)
	return err
}

// loadResourceVersion loads the current resource version from the database
func (d *Database) loadResourceVersion() error {
	row := d.db.QueryRow("SELECT current_version FROM resource_version WHERE id = 1")
	return row.Scan(&d.currentResourceVersion)
}

// NextResourceVersion increments and returns the next resource version
func (d *Database) NextResourceVersion() (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.currentResourceVersion++
	rv := d.currentResourceVersion

	_, err := d.db.Exec("UPDATE resource_version SET current_version = ? WHERE id = 1", rv)
	if err != nil {
		return 0, fmt.Errorf("failed to update resource version: %w", err)
	}

	return rv, nil
}

// CurrentResourceVersion returns the current resource version
func (d *Database) CurrentResourceVersion() int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentResourceVersion
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// Ping checks if the database is accessible
func (d *Database) Ping() error {
	return d.db.Ping()
}

// DB returns the underlying database connection
func (d *Database) DB() *sql.DB {
	return d.db
}
