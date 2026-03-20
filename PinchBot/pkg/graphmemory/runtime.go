package graphmemory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sipeed/pinchbot/pkg/providers"
)

func OpenDB(dbPath string) (*sql.DB, error) {
	return sql.Open("sqlite", dbPath)
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

