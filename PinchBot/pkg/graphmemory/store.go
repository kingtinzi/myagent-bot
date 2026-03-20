package graphmemory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

var idSeq atomic.Uint64

func nextID(prefix string) string {
	seq := idSeq.Add(1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), seq)
}

type Node struct {
	ID             string
	Type           string
	Name           string
	Description    string
	Content        string
	ValidatedCount int
	Pagerank       float64
}

func normalizeName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.Join(strings.Fields(s), "-")
	return s
}

func UpsertNode(db *sql.DB, nodeType, name, description, content, sessionID string) (*Node, bool, error) {
	name = normalizeName(name)
	if name == "" {
		return nil, false, fmt.Errorf("name is empty after normalization")
	}
	now := time.Now().UnixMilli()

	var (
		exID             string
		exType           string
		exName           string
		exDesc           string
		exContent        string
		exValidatedCount int
		exPagerank       float64
		srcRaw           string
	)
	err := db.QueryRow(`SELECT id, type, name, description, content, validated_count, pagerank, source_sessions FROM gm_nodes WHERE name=?`, name).
		Scan(&exID, &exType, &exName, &exDesc, &exContent, &exValidatedCount, &exPagerank, &srcRaw)
	if err == nil {
		sessions := []string{}
		_ = json.Unmarshal([]byte(srcRaw), &sessions)
		if sessionID != "" {
			seen := false
			for _, s := range sessions {
				if s == sessionID {
					seen = true
					break
				}
			}
			if !seen {
				sessions = append(sessions, sessionID)
			}
		}
		if len(content) < len(exContent) {
			content = exContent
		}
		if len(description) < len(exDesc) {
			description = exDesc
		}
		sessionsJSON, _ := json.Marshal(sessions)
		_, err = db.Exec(`UPDATE gm_nodes SET description=?, content=?, validated_count=?, source_sessions=?, updated_at=? WHERE id=?`,
			description, content, exValidatedCount+1, string(sessionsJSON), now, exID)
		if err != nil {
			return nil, false, err
		}
		return &Node{
			ID:             exID,
			Type:           exType,
			Name:           exName,
			Description:    description,
			Content:        content,
			ValidatedCount: exValidatedCount + 1,
			Pagerank:       exPagerank,
		}, false, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, err
	}

	id := nextID("n")
	sessions := []string{}
	if sessionID != "" {
		sessions = append(sessions, sessionID)
	}
	sessionsJSON, _ := json.Marshal(sessions)
	_, err = db.Exec(`INSERT INTO gm_nodes (id, type, name, description, content, status, validated_count, source_sessions, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, 'active', 1, ?, ?, ?)`,
		id, strings.ToUpper(strings.TrimSpace(nodeType)), name, description, content, string(sessionsJSON), now, now)
	if err != nil {
		return nil, false, err
	}
	return &Node{
		ID:             id,
		Type:           strings.ToUpper(strings.TrimSpace(nodeType)),
		Name:           name,
		Description:    description,
		Content:        content,
		ValidatedCount: 1,
		Pagerank:       0,
	}, true, nil
}

func LinkSolvedBy(db *sql.DB, fromID, toID, sessionID string) error {
	var exists string
	err := db.QueryRow(`SELECT id FROM gm_edges WHERE from_id=? AND to_id=? AND type='SOLVED_BY'`, fromID, toID).Scan(&exists)
	if err == nil {
		_, err = db.Exec(`UPDATE gm_edges SET instruction=? WHERE id=?`, "关联技能", exists)
		return err
	}
	if err != sql.ErrNoRows {
		return err
	}
	id := nextID("e")
	_, err = db.Exec(`INSERT INTO gm_edges (id, from_id, to_id, type, instruction, condition, session_id, created_at)
	VALUES (?, ?, ?, 'SOLVED_BY', ?, NULL, ?, ?)`, id, fromID, toID, "关联技能", sessionID, time.Now().UnixMilli())
	return err
}

func LinkUsedSkill(db *sql.DB, fromTaskID, toSkillID, sessionID string) error {
	var exists string
	err := db.QueryRow(`SELECT id FROM gm_edges WHERE from_id=? AND to_id=? AND type='USED_SKILL'`, fromTaskID, toSkillID).Scan(&exists)
	if err == nil {
		_, err = db.Exec(`UPDATE gm_edges SET instruction=? WHERE id=?`, "自动关联技能", exists)
		return err
	}
	if err != sql.ErrNoRows {
		return err
	}
	id := nextID("e")
	_, err = db.Exec(`INSERT INTO gm_edges (id, from_id, to_id, type, instruction, condition, session_id, created_at)
	VALUES (?, ?, ?, 'USED_SKILL', ?, NULL, ?, ?)`, id, fromTaskID, toSkillID, "自动关联技能", sessionID, time.Now().UnixMilli())
	return err
}

func SearchNodes(db *sql.DB, query string, limit int) ([]Node, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 6
	}
	terms := strings.Fields(query)
	args := make([]any, 0, len(terms)*3+1)
	whereParts := make([]string, 0, len(terms))
	for _, t := range terms {
		like := "%" + t + "%"
		whereParts = append(whereParts, "(name LIKE ? OR description LIKE ? OR content LIKE ?)")
		args = append(args, like, like, like)
	}
	sqlText := `SELECT id, type, name, description, content, validated_count, pagerank FROM gm_nodes WHERE status='active'`
	if len(whereParts) > 0 {
		sqlText += " AND (" + strings.Join(whereParts, " OR ") + ")"
	}
	sqlText += " ORDER BY pagerank DESC, validated_count DESC, updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Node, 0)
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Type, &n.Name, &n.Description, &n.Content, &n.ValidatedCount, &n.Pagerank); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pagerank > out[j].Pagerank })
	return out, nil
}

// RecomputePageRankHeuristic updates pagerank with a lightweight heuristic:
// validated_count + 0.1 * out_degree + 0.05 * in_degree.
func RecomputePageRankHeuristic(db *sql.DB) (int64, error) {
	rows, err := db.Query(`
		SELECT n.id,
		       n.validated_count,
		       COALESCE((SELECT COUNT(*) FROM gm_edges e WHERE e.from_id=n.id), 0) AS out_deg,
		       COALESCE((SELECT COUNT(*) FROM gm_edges e WHERE e.to_id=n.id), 0) AS in_deg
		FROM gm_nodes n
		WHERE n.status='active'
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type row struct {
		id        string
		validated int
		outDeg    int
		inDeg     int
	}
	all := make([]row, 0, 128)
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.validated, &r.outDeg, &r.inDeg); err != nil {
			return 0, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`UPDATE gm_nodes SET pagerank=?, updated_at=? WHERE id=?`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer stmt.Close()

	now := time.Now().UnixMilli()
	var changed int64
	for _, r := range all {
		score := float64(r.validated) + 0.1*float64(r.outDeg) + 0.05*float64(r.inDeg)
		res, err := stmt.Exec(score, now, r.id)
		if err != nil {
			_ = tx.Rollback()
			return changed, err
		}
		aff, _ := res.RowsAffected()
		changed += aff
	}
	if err := tx.Commit(); err != nil {
		return changed, err
	}
	return changed, nil
}

