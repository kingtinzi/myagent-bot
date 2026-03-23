package graphmemory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/sipeed/pinchbot/pkg/config"
)

const defaultDBPath = "~/.openclaw/graph-memory.db"

type Stats struct {
	TotalNodes int            `json:"totalNodes"`
	TotalEdges int            `json:"totalEdges"`
	Communities int           `json:"communities"`
	ByType     map[string]int `json:"byType"`
	ByEdgeType map[string]int `json:"byEdgeType"`
}

func Enabled(cfg *config.Config) bool {
	if cfg == nil || cfg.GraphMemory == nil || !cfg.GraphMemory.Enabled {
		return false
	}
	return cfg.Plugins.IsPluginEnabled("graph-memory")
}

func ResolveDBPath(cfg *config.Config) string {
	if cfg == nil || cfg.GraphMemory == nil {
		return expandHome(defaultDBPath)
	}
	raw := cfg.GraphMemory.Raw
	if raw == nil {
		return expandHome(defaultDBPath)
	}
	if v, ok := raw["dbPath"].(string); ok && strings.TrimSpace(v) != "" {
		return expandHome(strings.TrimSpace(v))
	}
	return expandHome(defaultDBPath)
}

func LoadStats(cfg *config.Config) (*Stats, error) {
	dbPath := ResolveDBPath(cfg)
	db, err := OpenDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	out := &Stats{
		ByType:     map[string]int{},
		ByEdgeType: map[string]int{},
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_nodes WHERE status='active'`).Scan(&out.TotalNodes); err != nil {
		return nil, fmt.Errorf("query gm_nodes: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_edges`).Scan(&out.TotalEdges); err != nil {
		return nil, fmt.Errorf("query gm_edges: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(DISTINCT community_id) FROM gm_nodes WHERE status='active' AND community_id IS NOT NULL`).Scan(&out.Communities); err != nil {
		return nil, fmt.Errorf("query communities: %w", err)
	}

	rows, err := db.Query(`SELECT type, COUNT(*) FROM gm_nodes WHERE status='active' GROUP BY type`)
	if err != nil {
		return nil, fmt.Errorf("query node types: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var typ string
		var cnt int
		if err := rows.Scan(&typ, &cnt); err != nil {
			return nil, err
		}
		out.ByType[typ] = cnt
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	edgeRows, err := db.Query(`SELECT type, COUNT(*) FROM gm_edges GROUP BY type`)
	if err != nil {
		return nil, fmt.Errorf("query edge types: %w", err)
	}
	defer edgeRows.Close()
	for edgeRows.Next() {
		var typ string
		var cnt int
		if err := edgeRows.Scan(&typ, &cnt); err != nil {
			return nil, err
		}
		out.ByEdgeType[typ] = cnt
	}
	if err := edgeRows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		if len(path) > 1 && (path[1] == '/' || path[1] == '\\') {
			return filepath.Join(home, path[2:])
		}
		return home
	}
	return path
}

