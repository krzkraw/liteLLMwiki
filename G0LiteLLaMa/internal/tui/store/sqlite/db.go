package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver
)

// schema contains the DDL for all tables used by the persistence layer.
//
// Tables are created with IF NOT EXISTS so the database is self-describing.
// There is no migration machinery. To reset, delete the database file or
// drop/recreate tables manually.
const schema = `
CREATE TABLE IF NOT EXISTS actions (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,
    source      TEXT NOT NULL,
    correlation_id TEXT,
    parent_id   TEXT,
    payload_json TEXT,
    created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
    revision    INTEGER PRIMARY KEY,
    action_id   TEXT NOT NULL,
    type        TEXT NOT NULL,
    payload_json TEXT,
    created_at  TEXT NOT NULL,
    FOREIGN KEY (action_id) REFERENCES actions(id)
);

CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY,
    parent_id   TEXT,
    state_path  TEXT,
    kind        TEXT NOT NULL,
    status      TEXT NOT NULL,
    progress_json TEXT,
    error       TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS snapshots (
    revision    INTEGER PRIMARY KEY,
    state_json  TEXT NOT NULL,
    created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
    key         TEXT PRIMARY KEY,
    value_json  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
`

// OpenDB opens (or creates) the SQLite database at the given path and applies
// the schema. Callers must close the returned *sql.DB when done.
func OpenDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return db, nil
}
