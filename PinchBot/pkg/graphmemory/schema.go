package graphmemory

import (
	"database/sql"
	"fmt"
)

// EnsureSchema creates core gm_* tables on first open of the SQLite file.
// Without this, a fresh .db opens but queries fail with "no such table".
// (Historical note: the old Node/TS extension had parallel migrations; graph-memory is Go-only now.)
func EnsureSchema(db *sql.DB) error {
	if db == nil {
		return nil
	}
	for i, q := range graphMemorySchemaSteps {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("graph-memory schema step %d: %w", i+1, err)
		}
	}
	return nil
}

var graphMemorySchemaSteps = []string{
	// m1_core (subset: tables + indexes used by Go runtime)
	`CREATE TABLE IF NOT EXISTS gm_nodes (
		id              TEXT PRIMARY KEY,
		type            TEXT NOT NULL,
		name            TEXT NOT NULL,
		description     TEXT NOT NULL DEFAULT '',
		content         TEXT NOT NULL,
		status          TEXT NOT NULL DEFAULT 'active',
		validated_count INTEGER NOT NULL DEFAULT 1,
		source_sessions TEXT NOT NULL DEFAULT '[]',
		community_id    TEXT,
		pagerank        REAL NOT NULL DEFAULT 0,
		created_at      INTEGER NOT NULL,
		updated_at      INTEGER NOT NULL
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS ux_gm_nodes_name ON gm_nodes(name)`,
	`CREATE INDEX IF NOT EXISTS ix_gm_nodes_type_status ON gm_nodes(type, status)`,
	`CREATE INDEX IF NOT EXISTS ix_gm_nodes_community ON gm_nodes(community_id)`,
	`CREATE TABLE IF NOT EXISTS gm_edges (
		id          TEXT PRIMARY KEY,
		from_id     TEXT NOT NULL,
		to_id       TEXT NOT NULL,
		type        TEXT NOT NULL,
		instruction TEXT NOT NULL,
		condition   TEXT,
		session_id  TEXT NOT NULL,
		created_at  INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS ix_gm_edges_from ON gm_edges(from_id)`,
	`CREATE INDEX IF NOT EXISTS ix_gm_edges_to ON gm_edges(to_id)`,
	// m2_messages
	`CREATE TABLE IF NOT EXISTS gm_messages (
		id          TEXT PRIMARY KEY,
		session_id  TEXT NOT NULL,
		turn_index  INTEGER NOT NULL,
		role        TEXT NOT NULL,
		content     TEXT NOT NULL,
		extracted   INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS ix_gm_msg_session ON gm_messages(session_id, turn_index)`,
}
