# 扩展相关文档（仓库根 `docs/extensions/`）

- **用户自装扩展（文件夹已放好，如何被网关加载）**：工作区模板技能 **`PinchBot/internal/workspacetpl/workspace/skills/user-extensions/SKILL.md`**（随新 workspace 下发；老 workspace 可手动复制到 `workspace/skills/user-extensions/`）。
- **`extension-matrix-template.md`** — 阶段 A「能力矩阵」可复制模板；每个新扩展可另存为 `docs/extensions/<id>-matrix.md` 跟踪准入分析。
- **草稿矩阵扫描**（可选）：在 **`PinchBot/`** 模块根执行  
  `go run ./cmd/scan-extension-matrix -extensions ./extensions`  
  会列出各子目录的 manifest（`id` / `name` / `configSchema` 摘要）并对 `index.ts` 做 **API 子串启发式**（非完整 AST），输出可粘贴到矩阵的 Markdown。详见 `cmd/scan-extension-matrix/main.go`。

PinchBot **随仓库自带的扩展包**仍在 **`PinchBot/extensions/`**（`openclaw.plugin.json` + `index.ts`），与本文档目录不同；二者关系见 `docs/openclaw-extension-adapter-runbook.md`「扩展与 agent 的关系」。

**与 ClawHub（技能市场）的区别**：ClawHub 由 **`pkg/skills/clawhub_registry.go`** 以 Go HTTP 客户端接入 **`clawhub.ai`**，**不依赖 Node**。只有启用 **`plugins.node_host`** 并加载上述 TS 扩展时，才需要本机 **Node ≥ 18** 与插件宿主依赖（`plugin-host` / `npm ci` 等）。

**Gateway 与 Node 扩展**：与 **`/tools/invoke`**、**`GET /plugins/status`**、**`POST /plugins/gateway-method`**（`registerGatewayMethod`）相关的鉴权、限流与可复制 **`curl`** 见 **`docs/snippets/README.md`**；总览与能力表见 **`docs/openclaw-extension-adapter-runbook.md`**（**C.2.4**、**B.2.2**）。
