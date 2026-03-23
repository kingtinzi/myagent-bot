# `before_tool_call`：PinchBot 对齐草案

面向 runbook **C.2**「`before_tool_call` 与插件 hook」：先固定**语义与挂载点**，再分阶段实现，避免与 graph-memory 的 `emit` 事件混淆（见 `run.mjs` 当前行为）。**P0（Go `ToolRegistry` hook）已实现**；Node / OpenClaw 同构仍为后续阶段。

## 1. OpenClaw 侧（参考）

- 扩展可 `registerHook` / 或等价机制在**工具真正执行前**拦截或改写参数、拒绝调用（具体名称与 payload 以上游 `openclaw` 与扩展 SDK 为准）。
- 典型用途：审计、脱敏、按会话/渠道追加策略。

## 2. PinchBot 现状

| 路径 | 行为 |
|------|------|
| Node `run.mjs` | `api.on('before_tool_call', …)` / **`api.on('after_tool_call', …)`** 在 **`execute`** 路径生效（前拦截/改参；后审计，处理器异常不影响工具返回值）。仍**不**混入 graph-memory 自定义 `emit` 表。 |
| Go `ToolRegistry.ExecuteWithContext` | `WithToolContext` 之后、执行前可运行可选 **`BeforeToolCallHook`**（`SetBeforeToolCall`）；可拒绝或改写 `args`。见 `pkg/tools/before_tool_call.go`。 |
| `POST /tools/invoke` | 走同一 `ExecuteWithContext`；策略过滤在 agent tools profile / gateway deny 层。 |

## 3. 建议的 Hook 契约（实现时）

- **调用时机**：在 `Get(name)` 成功之后、`Execute` / `ExecuteAsync` 之前；与 `DeniedByAgentToolsProfile` 等策略的关系：**策略层先过滤，hook 再运行**（hook 可视为「最后可改 args / 拒绝」的扩展点）。
- **输入**（最小）：`toolName`、`args`（map）、`ctx`（`ToolChannel` / `ToolChatID` / **`ToolAgentID`** 由调用路径注入，见 `pkg/tools/base.go`）。
- **输出**（选一）：
  - **通过**：可选 `args` 改写（新 map 或原地约定）。
  - **拒绝**：返回 `ToolResult` 风格错误（与现有 `ErrorResult` 一致），不执行工具。
- **异步工具**：若走 `ExecuteAsync`，hook 须在异步分支**之前**同步执行，避免竞态。

## 4. 分阶段落地（建议）

1. **P0 — Go 内部**（**已完成**）：`ToolRegistry.SetBeforeToolCall` + `BeforeToolCallHook`（`pkg/tools/before_tool_call.go`）；单测见 `registry_test.go`（`TestToolRegistry_BeforeToolCall_*`）。
2. **P1 — Node 插件（桥接 execute，已完成基线）**：`run.mjs` 在调用插件 `registerTool` 的 `execute` 前运行全局注册的 `before_tool_call`；Go `NodeHost.Execute` 传入 `channel` / `chatId` / `agentId`。与上游 OpenClaw **字段级**一致仍见 **P2**。
3. **P2 — 真同构**：与上游 `runBeforeToolCallHook` 字段级对齐 + 回归 fixture。

## 5. 与 per-agent `tools` 的关系

- **`agents.*.tools`**：静态允许/拒绝（已实现）。
- **`before_tool_call`**：动态、可带上下文（会话、渠道、插件状态）。二者应**可组合**：静态 deny 仍应短路，不进入 hook。

## 6. 参考代码

- `PinchBot/pkg/tools/before_tool_call.go` — `BeforeToolCallHook` 语义
- `PinchBot/pkg/tools/registry.go` — `ExecuteWithContext`
- `PinchBot/pkg/plugins/assets/run.mjs` — `createApi`、`api.on`
- `docs/openclaw-extension-adapter-runbook.md` — C.2
