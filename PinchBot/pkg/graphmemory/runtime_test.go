package graphmemory

import (
	"testing"

	_ "modernc.org/sqlite"

	"github.com/sipeed/pinchbot/pkg/providers"
)

func TestAutoExtractCreatesNodesAndEdge(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
CREATE TABLE gm_nodes (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  validated_count INTEGER NOT NULL DEFAULT 1,
  source_sessions TEXT NOT NULL DEFAULT '[]',
  community_id TEXT,
  pagerank REAL NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX ux_gm_nodes_name ON gm_nodes(name);
CREATE TABLE gm_edges (
  id TEXT PRIMARY KEY,
  from_id TEXT NOT NULL,
  to_id TEXT NOT NULL,
  type TEXT NOT NULL,
  instruction TEXT NOT NULL,
  condition TEXT,
  session_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
);`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	msgs := []providers.Message{
		{Role: "user", Content: "我需要整理发布流程，帮我做计划。"},
		{Role: "assistant", Content: "先执行检查步骤，然后 run diagnostics command 查看错误日志。"},
	}
	if err := AutoExtractFromMessages(db, "s1", msgs); err != nil {
		t.Fatalf("auto extract: %v", err)
	}

	var nodeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_nodes`).Scan(&nodeCount); err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if nodeCount < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", nodeCount)
	}

	var edgeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_edges WHERE type='USED_SKILL'`).Scan(&edgeCount); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if edgeCount < 1 {
		t.Fatalf("expected at least 1 USED_SKILL edge, got %d", edgeCount)
	}
}

