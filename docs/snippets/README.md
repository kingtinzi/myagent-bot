# 配置片段（`docs/snippets/`）

可复制到 **`PinchBot/config.json`**（或工作区根 `config.json`）的 JSON 片段，按需与现有配置**合并**；不要覆盖未列出的键，除非你清楚含义。

| 文件 | 用途 |
|------|------|
| **`pinchbot-plugins-gateway.example.json`** | OpenClaw 式 **TS 扩展**（`plugins.node_host`）+ **Gateway**（`/tools/invoke`、`/plugins/status`、`/plugins/gateway-method` 鉴权；`gateway.tools` deny/allow 仅作用于 **`/tools/invoke`**） |
| **`go-tool-skeleton.md`** | **S.3** — Go 原生 `Tool` 骨架（`Name` / `Parameters` / `Execute`、最小测试、接线提示） |

### 使用说明

1. 将片段中的 **`plugins`**、**`gateway`** 对象合并进你的配置（同一层级键名与 `pkg/config` 的 JSON tag 一致）。
2. **`plugin_settings`**：键为扩展 **manifest id**（如 `lobster`），值为传给 `register(api).pluginConfig` 的对象；无额外字段时可保留 `{}` 或省略 `plugin_settings`。
3. **`gateway.auth`**：`mode` 为 **`token`** 或 **`password`** 时，`POST /tools/invoke`、**`GET /plugins/status`**、**`POST /plugins/gateway-method`**（`registerGatewayMethod`）以及 **`registerHttpRoute` 挂载的插件 HTTP 路径**均需 `Authorization: Bearer <token|password>`。本地调试可改为 **`none`**（勿用于公网暴露）。
4. **`gateway.rate_limit.requests_per_minute`**：大于 0 时启用**固定窗口**（每分钟）限流；**`invoke`**、**`plugins/status`**、**`plugins/gateway-method`**、**插件 HTTP**（`pluginhttp`）**共用**计数。有 Bearer 时按凭据分桶，否则按 IP（`X-Forwarded-For` 首选段）。示例里 `0` 表示关闭（可改为如 `120`）。
5. **`gateway.tools.deny` / `allow`**：在 OpenClaw 默认 HTTP deny 列表（`sessions_spawn`、`cron`、`gateway` 等）之上微调；`allow` 可从默认 deny 中**放行**某项（见 `pkg/gateway/toolsinvoke/policy.go`）。
6. **内置工具开关**（`read_file`、`lobster` Go 版等）在 **`tools.*`** 各工具的 `enabled` 字段，不在本片段中；Node 扩展的 `lobster` 工具由 **`plugins.enabled`** 与 Node 宿主注册决定。
7. **Per-agent 工具名过滤**：在 **`agents.defaults.tools`** 与 **`agents.list[].tools`** 使用 `allow` / `deny` / `alsoAllow`（OpenClaw 风格）；与 defaults 合并。**`POST /tools/invoke`**：在 Gateway `gateway.tools` / 默认 HTTP deny **之后**应用。**主 agent 聊天** 与 **`spawn` / `subagent` 内 `RunToolLoop`**：同一套 `DeniedByAgentToolsProfile` 规则；子任务若带 `agent_id` 则按**目标 agent** 解析 profile，否则按**发起 spawn 的 agent**（不含 Gateway HTTP 默认 deny）。`deny` 取并集；任一层设置了 **`allow` 或 `alsoAllow`** 时，以该层列表作为白名单（agent 层覆盖 defaults 的 allow/alsoAllow）。实现见 `pkg/config/config.go`、`pkg/agent/agent_tools_profile.go`、`pkg/agent/loop.go`（`SubagentManager`）、`pkg/tools/toolloop.go`、`pkg/gateway/toolsinvoke/handler.go`。

### 示例：`POST /plugins/gateway-method`（`registerGatewayMethod`）

扩展在 `register(api)` 里调用 `api.registerGatewayMethod("my.namespace.method", handler)` 后，可由 Gateway 转发到 **默认 agent** 的 Node 宿主（鉴权与 **`/tools/invoke`** 一致；`plugin_id` 为 manifest **id**）：

```bash
curl -sS -X POST "http://127.0.0.1:18789/plugins/gateway-method" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer REPLACE_WITH_GATEWAY_TOKEN" \
  -d '{"plugin_id":"my-extension-id","method":"my.namespace.method","params":{}}'
```

`pluginId`（camelCase）与 `plugin_id` 等价。成功响应形如 **`{"ok":true,"result":{...}}`**，`result` 为 Node 侧 `respond` / 返回值（字段含义见 **`docs/openclaw-extension-adapter-runbook.md`** **C.2.4**）。

更完整的说明见 **`docs/openclaw-extension-adapter-runbook.md`**（扩展与 agent、`dryRun` 等）。
