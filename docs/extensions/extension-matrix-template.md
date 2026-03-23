# 扩展能力矩阵（模板）

复制本文件为 `docs/extensions/<manifest-id>-matrix.md`（或贴到 issue），按扩展迭代填写。原则见仓库 `.cursor/rules/openclaw-plugin-parity.mdc` 与 `docs/openclaw-extension-adapter-runbook.md`。

**扩展 id / 名称**：  
**仓库路径或来源**（如 `PinchBot/extensions/<dir>/`）：  
**负责人 / 日期**：

---

## 1. 清单 `openclaw.plugin.json`

- [ ] **id**：  
- [ ] **configSchema 摘要**（必填字段、是否可为空 object）：  
- [ ] **channels / providers**（若有）：  

## 2. `register(api)` 实际调用的 API

在 `index.ts`（及 setup 若存在）中勾选：

- [ ] `registerTool`  
- [ ] `registerHook`  
- [ ] `registerHttpRoute`  
- [ ] `registerChannel`  
- [ ] `registerGatewayMethod`  
- [ ] `registerCli`  
- [ ] `registerService`  
- [ ] `registerProvider` / `registerSpeechProvider` / `registerMediaUnderstandingProvider` / `registerImageGenerationProvider` / `registerWebSearchProvider`  
- [ ] `registerContextEngine`  
- [ ] `registerCommand`  
- [ ] `registerInteractiveHandler`  
- [ ] `on` / `onConversationBindingResolved`  
- [ ] 其它（列出）：  

## 3. 外部依赖

- [ ] **二进制**（如 `lobster` 在 PATH）：  
- [ ] **环境变量**：  
- [ ] **HTTP 回调**（如 `openclaw.invoke` → `POST /tools/invoke`；Gateway 地址与鉴权）：  
- [ ] **子代理 / 会话形态**：  

## 4. 路线判定（选一或组合）

- [ ] 宿主 + shim（仅工具 + 已有/可补 shim）  
- [ ] 补底座（缺平台能力但在 PinchBot 内值得做）  
- [ ] 窄 invoke（Lobster / HTTP 工具链）  
- [ ] Sidecar OpenClaw（大量 `registerChannel` 等短期无法 Go 对齐）  
- [ ] 暂不启用  

**简述理由**：

## 5. 验收用例

| 类型 | 步骤 | 预期 |
|------|------|------|
| Happy path | | |
| 失败路径（配置缺失 / 沙箱 / 鉴权） | | |

## 6. PinchBot 对照（填时查阅）

| 主题 | 路径 |
|------|------|
| Node 宿主 | `PinchBot/pkg/plugins/assets/run.mjs` |
| 发现与 manifest | `PinchBot/pkg/plugins/discover.go`、`manifest.go` |
| Gateway `/tools/invoke`、`/plugins/status`、`/plugins/gateway-method` | `PinchBot/pkg/gateway/toolsinvoke/`、`pkg/gateway/pluginsstatus/`、`pkg/gateway/plugingatewaymethod/` |
| **`GET /plugins/status` 声明字段**（排障时核对；含 `registered_hooks`、`registered_channels`、`registered_interactive_handlers`、`conversation_binding_resolved_listeners` 等） | `docs/openclaw-extension-adapter-runbook.md` **B.2.2**；类型 `pkg/plugins/plugin_settings.go` **`PluginInitStatus`** |
| 流程说明 | `docs/pinchbot-lobster-from-openclaw-flow.md` |
