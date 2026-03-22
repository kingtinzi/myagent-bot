package check

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"

	"github.com/sipeed/pinchbot/cmd/picoclaw/internal"
	"github.com/sipeed/pinchbot/pkg/graphmemory"
)

type graphMemoryEval struct {
	DBPath      string         `json:"db_path"`
	TotalNodes  int            `json:"total_nodes"`
	TotalEdges  int            `json:"total_edges"`
	Communities int            `json:"communities"`
	ByType      map[string]int `json:"by_type"`
	ByEdgeType  map[string]int `json:"by_edge_type"`
	TopNodes    []topNode      `json:"top_nodes"`
}

type topNode struct {
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	Pagerank       float64 `json:"pagerank"`
	ValidatedCount int     `json:"validated_count"`
}

func newGraphMemoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph-memory",
		Short: "Evaluate graph-memory database metrics",
		RunE:  runGraphMemoryCheck,
	}
	return cmd
}

func runGraphMemoryCheck(_ *cobra.Command, _ []string) error {
	cfg, err := internal.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	dbPath := graphmemory.ResolveDBPath(cfg)
	db, err := graphmemory.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	out := graphMemoryEval{
		DBPath:     dbPath,
		ByType:     map[string]int{},
		ByEdgeType: map[string]int{},
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_nodes WHERE status='active'`).Scan(&out.TotalNodes); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			fmt.Printf("{\"db_path\":%q,\"initialized\":false,\"hint\":\"OpenDB runs EnsureSchema on first connect; if you see this, DB file may be empty or from an older build — retry after a restart or check permissions\"}\n", dbPath)
			return nil
		}
		return err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_edges`).Scan(&out.TotalEdges); err != nil {
		return err
	}
	if err := db.QueryRow(`SELECT COUNT(DISTINCT community_id) FROM gm_nodes WHERE status='active' AND community_id IS NOT NULL`).Scan(&out.Communities); err != nil {
		return err
	}
	if err := fillCountMap(db, `SELECT type, COUNT(*) FROM gm_nodes WHERE status='active' GROUP BY type`, out.ByType); err != nil {
		return err
	}
	if err := fillCountMap(db, `SELECT type, COUNT(*) FROM gm_edges GROUP BY type`, out.ByEdgeType); err != nil {
		return err
	}
	top, err := db.Query(`SELECT name, type, pagerank, validated_count FROM gm_nodes WHERE status='active' ORDER BY pagerank DESC, validated_count DESC LIMIT 10`)
	if err != nil {
		return err
	}
	defer top.Close()
	for top.Next() {
		var t topNode
		if err := top.Scan(&t.Name, &t.Type, &t.Pagerank, &t.ValidatedCount); err != nil {
			return err
		}
		out.TopNodes = append(out.TopNodes, t)
	}

	// stable output for diffing
	sort.SliceStable(out.TopNodes, func(i, j int) bool {
		if out.TopNodes[i].Pagerank == out.TopNodes[j].Pagerank {
			return out.TopNodes[i].Name < out.TopNodes[j].Name
		}
		return out.TopNodes[i].Pagerank > out.TopNodes[j].Pagerank
	})

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func fillCountMap(db *sql.DB, query string, out map[string]int) error {
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return err
		}
		out[key] = count
	}
	return rows.Err()
}

