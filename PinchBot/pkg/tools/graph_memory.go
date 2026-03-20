package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/graphmemory"
)

type graphMemoryStatsTool struct {
	cfg *config.Config
}
type graphMemorySearchTool struct{ cfg *config.Config }
type graphMemoryRecordTool struct{ cfg *config.Config }
type graphMemoryMaintainTool struct{ cfg *config.Config }

func NewGraphMemoryStatsTool(cfg *config.Config) Tool {
	return &graphMemoryStatsTool{cfg: cfg}
}
func NewGraphMemorySearchTool(cfg *config.Config) Tool   { return &graphMemorySearchTool{cfg: cfg} }
func NewGraphMemoryRecordTool(cfg *config.Config) Tool   { return &graphMemoryRecordTool{cfg: cfg} }
func NewGraphMemoryMaintainTool(cfg *config.Config) Tool { return &graphMemoryMaintainTool{cfg: cfg} }

func (t *graphMemoryStatsTool) Name() string {
	return "gm_stats"
}

func (t *graphMemoryStatsTool) Description() string {
	return "查看知识图谱统计信息：节点数、边数、社区数与按类型分布。"
}

func (t *graphMemoryStatsTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *graphMemoryStatsTool) Execute(_ context.Context, _ map[string]any) *ToolResult {
	if !graphmemory.Enabled(t.cfg) {
		return ErrorResult("graph-memory is disabled (check config.graph-memory.json enabled=true and plugins.enabled)")
	}

	stats, err := graphmemory.LoadStats(t.cfg)
	if err != nil {
		return ErrorResult(fmt.Sprintf("graph-memory stats failed: %v", err)).WithError(err)
	}

	payload, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return ErrorResult("graph-memory stats encode failed").WithError(err)
	}

	text := []string{
		"知识图谱统计",
		fmt.Sprintf("节点：%d", stats.TotalNodes),
		fmt.Sprintf("边：%d", stats.TotalEdges),
		fmt.Sprintf("社区：%d", stats.Communities),
		"节点类型：" + joinCountMap(stats.ByType),
		"边类型：" + joinCountMap(stats.ByEdgeType),
		"",
		"JSON:",
		string(payload),
	}
	return SilentResult(strings.Join(text, "\n"))
}

func (t *graphMemorySearchTool) Name() string { return "gm_search" }
func (t *graphMemorySearchTool) Description() string {
	return "搜索知识图谱中的经验、技能和关联节点。"
}
func (t *graphMemorySearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "搜索关键词或问题描述"},
		},
		"required": []string{"query"},
	}
}
func (t *graphMemorySearchTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	if !graphmemory.Enabled(t.cfg) {
		return ErrorResult("graph-memory is disabled (check config.graph-memory.json enabled=true and plugins.enabled)")
	}
	q, _ := args["query"].(string)
	q = strings.TrimSpace(q)
	if q == "" {
		return ErrorResult("query is required")
	}
	db, closeFn, err := openGraphDB(t.cfg)
	if err != nil {
		return ErrorResult(fmt.Sprintf("graph-memory open db failed: %v", err)).WithError(err)
	}
	defer closeFn()

	nodes, err := graphmemory.SearchNodes(db, q, 8)
	if err != nil {
		return ErrorResult(fmt.Sprintf("graph-memory search failed: %v", err)).WithError(err)
	}
	if len(nodes) == 0 {
		return SilentResult("图谱中未找到相关记录。")
	}
	lines := make([]string, 0, len(nodes)+1)
	lines = append(lines, fmt.Sprintf("找到 %d 个节点：", len(nodes)))
	for _, n := range nodes {
		content := n.Content
		if len(content) > 240 {
			content = content[:240] + "..."
		}
		lines = append(lines, fmt.Sprintf("[%s] %s (pr:%.3f)\n%s\n%s", n.Type, n.Name, n.Pagerank, n.Description, content))
	}
	return SilentResult(strings.Join(lines, "\n\n"))
}

func (t *graphMemoryRecordTool) Name() string { return "gm_record" }
func (t *graphMemoryRecordTool) Description() string {
	return "手动记录经验到知识图谱，可选关联到已有技能。"
}
func (t *graphMemoryRecordTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":        map[string]any{"type": "string", "description": "节点名称（会标准化）"},
			"type":        map[string]any{"type": "string", "description": "节点类型：TASK、SKILL、EVENT"},
			"description": map[string]any{"type": "string", "description": "一句话描述"},
			"content":     map[string]any{"type": "string", "description": "详细内容"},
			"relatedSkill": map[string]any{
				"type":        "string",
				"description": "可选：关联已有技能名，建立 SOLVED_BY 关系",
			},
		},
		"required": []string{"name", "type", "description", "content"},
	}
}
func (t *graphMemoryRecordTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if !graphmemory.Enabled(t.cfg) {
		return ErrorResult("graph-memory is disabled (check config.graph-memory.json enabled=true and plugins.enabled)")
	}
	name, _ := args["name"].(string)
	nodeType, _ := args["type"].(string)
	description, _ := args["description"].(string)
	content, _ := args["content"].(string)
	relatedSkill, _ := args["relatedSkill"].(string)
	if strings.TrimSpace(name) == "" || strings.TrimSpace(nodeType) == "" || strings.TrimSpace(content) == "" {
		return ErrorResult("name/type/content are required")
	}

	db, closeFn, err := openGraphDB(t.cfg)
	if err != nil {
		return ErrorResult(fmt.Sprintf("graph-memory open db failed: %v", err)).WithError(err)
	}
	defer closeFn()

	sessionID := ToolChatID(ctx)
	if strings.TrimSpace(sessionID) == "" {
		sessionID = "manual"
	}
	node, _, err := graphmemory.UpsertNode(db, nodeType, name, description, content, sessionID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("graph-memory record failed: %v", err)).WithError(err)
	}

	if strings.TrimSpace(relatedSkill) != "" {
		matches, err := graphmemory.SearchNodes(db, relatedSkill, 1)
		if err == nil && len(matches) > 0 {
			_ = graphmemory.LinkSolvedBy(db, node.ID, matches[0].ID, sessionID)
		}
	}
	return SilentResult(fmt.Sprintf("已记录：%s (%s)", node.Name, node.Type))
}

func (t *graphMemoryMaintainTool) Name() string { return "gm_maintain" }
func (t *graphMemoryMaintainTool) Description() string {
	return "触发图维护（当前 Go 原生迁移阶段仅提供状态提示）。"
}
func (t *graphMemoryMaintainTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *graphMemoryMaintainTool) Execute(_ context.Context, _ map[string]any) *ToolResult {
	if !graphmemory.Enabled(t.cfg) {
		return ErrorResult("graph-memory is disabled (check config.graph-memory.json enabled=true and plugins.enabled)")
	}
	db, closeFn, err := openGraphDB(t.cfg)
	if err != nil {
		return ErrorResult(fmt.Sprintf("graph-memory open db failed: %v", err)).WithError(err)
	}
	defer closeFn()
	changed, err := graphmemory.RecomputePageRankHeuristic(db)
	if err != nil {
		return ErrorResult(fmt.Sprintf("graph-memory maintain failed: %v", err)).WithError(err)
	}
	return SilentResult(fmt.Sprintf("graph-memory maintain done: updated pagerank for %d nodes", changed))
}

func joinCountMap(m map[string]int) string {
	if len(m) == 0 {
		return "(empty)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s:%d", k, m[k]))
	}
	return strings.Join(out, ", ")
}

func openGraphDB(cfg *config.Config) (*sql.DB, func(), error) {
	dbPath := graphmemory.ResolveDBPath(cfg)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, func() {}, err
	}
	return db, func() { _ = db.Close() }, nil
}

