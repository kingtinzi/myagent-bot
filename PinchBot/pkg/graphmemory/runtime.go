package graphmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/sipeed/pinchbot/pkg/providers"
)

// OpenDB opens the SQLite database at dbPath. For file paths it:
//   - creates missing parent directories
//   - verifies the directory is writable (catches permission / read-only / cloud-sync issues early)
//   - pings the handle and applies best-effort PRAGMAs (busy_timeout, WAL)
//
// SQLite error 14 (CANTOPEN) often appears when the parent dir is missing or unwritable; some
// drivers surface confusing "memory" wording — the pre-check and Ping reduce that ambiguity.
func OpenDB(dbPath string) (*sql.DB, error) {
	norm, err := normalizeGraphMemoryDBPath(dbPath)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(norm, ":memory:") {
		db, err := sql.Open("sqlite", norm)
		if err != nil {
			return nil, mapOpenSQLiteError(norm, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, mapOpenSQLiteError(norm, err)
		}
		applyGraphMemoryPragmas(db)
		return db, nil
	}
	if err := ensureSQLiteFSReady(norm); err != nil {
		return nil, err
	}
	dsn := sqliteDSN(norm)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapOpenSQLiteError(norm, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, mapOpenSQLiteError(norm, err)
	}
	if err := EnsureSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("graph-memory: ensure schema: %w", err)
	}
	applyGraphMemoryPragmas(db)
	return db, nil
}

// normalizeGraphMemoryDBPath turns relative paths into absolute paths so opening does not depend
// on the process working directory (gateway/Launcher cwd often differs from the shell).
func normalizeGraphMemoryDBPath(dbPath string) (string, error) {
	p := strings.TrimSpace(dbPath)
	if p == "" || strings.HasPrefix(p, ":memory:") {
		return p, nil
	}
	if strings.HasPrefix(p, "file:") {
		return p, nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("graph-memory: resolve database path %q: %w", dbPath, err)
	}
	return abs, nil
}

// sqliteDSN builds a DSN that modernc.org/sqlite accepts reliably; on Unix, file: URIs handle
// spaces in paths (e.g. ~/Library/Application Support/...) better than raw paths in some setups.
func sqliteDSN(absPath string) string {
	if runtime.GOOS == "windows" {
		return absPath
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}
	return u.String()
}

func ensureSQLiteFSReady(dbPath string) error {
	p := strings.TrimSpace(dbPath)
	if p == "" || strings.HasPrefix(p, ":memory:") {
		return nil
	}
	// Skip for file: URLs — uncommon in our configs; caller manages paths.
	if strings.HasPrefix(p, "file:") {
		return nil
	}
	dir := filepath.Dir(p)
	if dir == "" {
		return nil
	}
	return ensureDirWritableForSQLite(dir)
}

// ensureDirWritableForSQLite creates dir (if needed) and proves we can create/delete a temp file there.
func ensureDirWritableForSQLite(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("graph-memory: create database directory %q: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, ".pinchbot-gm-write-test-*")
	if err != nil {
		return fmt.Errorf("graph-memory: database directory not writable %q: %w (check chmod/ownership, disk space, or avoid read-only/sync-conflicted folders)", dir, err)
	}
	name := f.Name()
	_ = f.Close()
	if err := os.Remove(name); err != nil {
		_ = os.Remove(name)
	}
	return nil
}

func applyGraphMemoryPragmas(db *sql.DB) {
	if db == nil {
		return
	}
	// Best-effort: reduce lock errors under concurrency; WAL uses sidecar files next to the db.
	_, _ = db.Exec(`PRAGMA busy_timeout=5000`)
	_, _ = db.Exec(`PRAGMA journal_mode=WAL`)
}

func mapOpenSQLiteError(dbPath string, err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	hint := ""
	switch {
	case strings.Contains(msg, "14") || strings.Contains(msg, "cantopen") ||
		strings.Contains(msg, "unable to open") || strings.Contains(msg, "could not open"):
		hint = " (SQLite CANTOPEN/14: directory missing, permission denied, read-only volume, or path typo; messages mentioning memory are often misleading)"
	case strings.Contains(msg, "permission") || strings.Contains(msg, "not permitted") || strings.Contains(msg, "operation not permitted"):
		hint = " (permission denied: chmod/chown the database file and its parent directory)"
	case strings.Contains(msg, "disk") && (strings.Contains(msg, "full") || strings.Contains(msg, "quota") || strings.Contains(msg, "space")):
		hint = " (disk full or quota exceeded)"
	case strings.Contains(msg, "readonly") || strings.Contains(msg, "read-only"):
		hint = " (read-only database or filesystem)"
	}
	if hint != "" {
		return fmt.Errorf("open graph-memory sqlite %q: %w%s", dbPath, err, hint)
	}
	return fmt.Errorf("open graph-memory sqlite %q: %w", dbPath, err)
}

func BuildSystemPromptAddition(query string, nodes []Node) string {
	query = strings.TrimSpace(query)
	if len(nodes) == 0 {
		return ""
	}
	lines := []string{
		"# Graph Memory Recall",
	}
	if query != "" {
		lines = append(lines, fmt.Sprintf("Query: %s", query))
	}
	for i, n := range nodes {
		content := n.Content
		if len(content) > 280 {
			content = content[:280] + "..."
		}
		lines = append(lines, fmt.Sprintf("%d. [%s] %s", i+1, n.Type, n.Name))
		if strings.TrimSpace(n.Description) != "" {
			lines = append(lines, "   - "+strings.TrimSpace(n.Description))
		}
		if strings.TrimSpace(content) != "" {
			lines = append(lines, "   - "+strings.TrimSpace(content))
		}
	}
	return strings.Join(lines, "\n")
}

func SaveMessages(db *sql.DB, sessionID string, startTurn int, messages []providers.Message) error {
	if strings.TrimSpace(sessionID) == "" || len(messages) == 0 {
		return nil
	}
	for i, m := range messages {
		content := m.Content
		if strings.TrimSpace(content) == "" && len(m.ToolCalls) > 0 {
			b, _ := json.Marshal(m.ToolCalls)
			content = string(b)
		}
		raw, _ := json.Marshal(m)
		_, err := db.Exec(`INSERT INTO gm_messages (id, session_id, turn_index, role, content, extracted, created_at)
			VALUES (?, ?, ?, ?, ?, 1, strftime('%s','now')*1000)`,
			fmt.Sprintf("m-%s-%d", sessionID, startTurn+i+1),
			sessionID,
			startTurn+i+1,
			roleOrUnknown(m.Role),
			string(raw),
		)
		if err != nil {
			// Best effort ingestion: ignore duplicates from retries.
			if !strings.Contains(strings.ToLower(err.Error()), "constraint") {
				return err
			}
		}
	}
	return nil
}

func roleOrUnknown(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return "unknown"
	}
	return role
}

var splitSentencesRe = regexp.MustCompile(`[。！？!?;\n]+`)

// AutoExtractFromMessages performs a low-risk heuristic extraction path.
// It creates SKILL/EVENT nodes from short, high-signal message fragments.
func AutoExtractFromMessages(db *sql.DB, sessionID string, messages []providers.Message) error {
	if len(messages) == 0 {
		return nil
	}
	var lastTaskID string
	for _, m := range messages {
		text := strings.TrimSpace(m.Content)
		if text == "" {
			continue
		}
		parts := splitSentencesRe.Split(text, -1)
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if len(p) < 12 || len(p) > 220 {
				continue
			}
			nType, ok := classifySnippet(m.Role, p)
			if !ok {
				continue
			}
			name := deriveNodeName(nType, p)
			desc := p
			if len(desc) > 100 {
				desc = desc[:100]
			}
			node, _, err := UpsertNode(db, nType, name, desc, p, sessionID)
			if err != nil {
				return err
			}
			if node == nil {
				continue
			}
			if nType == "TASK" {
				lastTaskID = node.ID
			}
			if nType == "SKILL" && lastTaskID != "" {
				_ = LinkUsedSkill(db, lastTaskID, node.ID, sessionID)
			}
		}
	}
	return nil
}

func classifySnippet(role, text string) (string, bool) {
	l := strings.ToLower(text)
	if strings.Contains(l, "error") || strings.Contains(l, "failed") || strings.Contains(text, "报错") || strings.Contains(text, "失败") {
		return "EVENT", true
	}
	if role == "assistant" {
		if strings.Contains(l, "run ") || strings.Contains(l, "command") ||
			strings.Contains(l, "步骤") || strings.Contains(l, "执行") ||
			strings.Contains(l, "先") || strings.Contains(l, "然后") {
			return "SKILL", true
		}
	}
	if role == "user" && (strings.Contains(l, "需要") || strings.Contains(l, "想要") || strings.Contains(l, "帮我")) {
		return "TASK", true
	}
	return "", false
}

func deriveNodeName(nodeType, text string) string {
	s := strings.ToLower(strings.TrimSpace(text))
	s = strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-").Replace(s)
	filtered := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || (r >= 0x4e00 && r <= 0x9fff) {
			filtered = append(filtered, r)
		}
		if len(filtered) >= 40 {
			break
		}
	}
	name := strings.Trim(string(filtered), "-")
	if name == "" {
		name = fmt.Sprintf("%s-%d", strings.ToLower(nodeType), time.Now().UnixMilli())
	}
	return name
}

