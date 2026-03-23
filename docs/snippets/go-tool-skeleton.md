# Go 原生工具骨架（S.3）

将 OpenClaw / Node 扩展中的某个能力 **迁到 Go** 时，在 `PinchBot/pkg/tools/` 新增 `your_tool.go`（及测试）。注册位置通常在 `pkg/agent/instance.go` 的 `NewAgentInstance` 中按配置 `Register`，与现有 `read_file`、`lobster` 等一致。

以下模板与 **`github.com/sipeed/pinchbot/pkg/tools`** 的 `Tool` 接口、`ToolResult`、`ToolChannel`/`ToolChatID` 对齐；复制后替换 `my_example`、`MyExampleTool`、参数 schema 与业务逻辑。

---

## `your_tool.go`（骨架）

```go
package tools

import "context"

// MyExampleTool is a stub — replace name, schema, and Execute body.
type MyExampleTool struct{}

func NewMyExampleTool() *MyExampleTool {
	return &MyExampleTool{}
}

func (t *MyExampleTool) Name() string {
	return "my_example"
}

func (t *MyExampleTool) Description() string {
	return "Short description for the model (what the tool does)."
}

func (t *MyExampleTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "User query or input.",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MyExampleTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	query, _ := args["query"].(string)
	if query == "" {
		return ErrorResult("query is required")
	}
	_ = ctx
	_ = ToolChannel(ctx)
	_ = ToolChatID(ctx)
	// Return visible user text with ForUser, or SilentResult if only model should see the string.
	return &ToolResult{
		ForLLM:  "result for model: " + query,
		ForUser: "result for user: " + query,
		Silent:  false,
	}
}
```

---

## `your_tool_test.go`（最小单测）

```go
package tools

import (
	"context"
	"testing"
)

func TestMyExampleTool_Execute(t *testing.T) {
	tool := NewMyExampleTool()
	ctx := WithToolContext(context.Background(), "cli", "direct")
	res := tool.Execute(ctx, map[string]any{"query": "hi"})
	if res == nil || res.IsError {
		t.Fatalf("unexpected: %+v", res)
	}
}
```

---

## 接线提示

1. **注册**：在 `NewAgentInstance` 里按 `cfg.Tools` 或 `cfg.Plugins` 条件 `toolsRegistry.Register(NewMyExampleTool())`；若与 Node 扩展同名，需用 `nativeGoPluginExclusiveNodeIDs` 一类策略避免重复（见 `pkg/plugins/register.go`）。
2. **配置开关**：在 `pkg/config` 的 `ToolsConfig` 增加 `MyExample ToolConfig` 与 `IsToolEnabled` 分支（可参考 `read_file`）。
3. **回归**：`go test ./pkg/tools/...`；若工具在 agent 路径与 `/tools/invoke` 都应可用，保持 **registry + `ExecuteWithContext`** 路径一致。
