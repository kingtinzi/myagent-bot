# 扩展运行时依赖改造清单（Go / Node host）

范围：将 `docs/openclaw-extension-dependency-contract-draft.md` 落地为可实现任务。  
目标：统一扩展依赖诊断与状态输出，先覆盖 `lobster`，再推广全部 extensions。

---

## 0. 里程碑与验收

### M1（最小闭环）

- `lobster` 能输出依赖诊断（node/npm/bin）。
- `/plugins/status` 返回 `runtime_status` 与 `checks`。
- 插件加载失败不拖垮宿主，状态可见。

### M2（可修复）

- 提供 `repair_actions`（先只读建议，不执行）。
- Launcher 能读取并展示可修复动作。

### M3（一键修复）

- 实现后端 `repair` API（审批可选）。
- Launcher 完成动作执行与回显。

---

## 1. 数据模型（Go）

文件建议：`PinchBot/pkg/plugins/runtime_status.go`

- [ ] 新增类型：`PluginRuntimeStatus`
  - `PluginID string`
  - `RuntimeStatus string` (`ready|degraded|blocked`)
  - `Checks []PluginRuntimeCheck`
  - `RepairActions []PluginRepairAction`
  - `UpdatedAt time.Time`
- [ ] 新增类型：`PluginRuntimeCheck`
  - `ID string`（如 `node`, `npm_deps`, `exe:lobster`）
  - `OK bool`
  - `Detail string`
  - `ReasonCode string`（如 `BIN_NOT_FOUND`, `NODE_TOO_OLD`）
- [ ] 新增类型：`PluginRepairAction`
  - `ID string`
  - `Label string`
  - `Risk string`（`low|medium|high`）
  - `RequiresApproval bool`
  - `ArgsSchema map[string]any`（可选）

验收：

- [ ] `go test` 新增单测：状态序列化、默认值、空值兼容。

---

## 2. Manifest 扩展字段读取（Go）

文件建议：`PinchBot/pkg/plugins/manifest.go`

- [ ] 增加可选读取：
  - 优先 `runtimeDependencies`
  - 兼容 `x-runtimeDependencies`
- [ ] 字段缺失时保持现有行为（不影响旧扩展）。
- [ ] 非法结构仅记诊断，不直接 panic。

验收：

- [ ] 单测覆盖：仅新字段、仅过渡字段、双字段并存、非法值。

---

## 3. 依赖检查器（Go）

文件建议：`PinchBot/pkg/plugins/runtime_checker.go`

### 3.1 Node/npm 检查

- [ ] 检查 Node 是否可执行与版本满足（最小版本可来自 manifest）。
- [ ] npm 可执行性检查（仅扩展声明需要 npm 时检查）。

### 3.2 扩展目录依赖检查

- [ ] 检查扩展目录 `node_modules` 是否存在（声明 `npmInstall.required` 时）。
- [ ] 可选：检查 lockfile 存在并标注建议动作（`npm ci` / `npm install`）。

### 3.3 外部可执行文件检查

- [ ] 解析顺序固定：
  1) `env`（如 `LOBSTER_BIN`）
  2) `bundledRelativePaths`
  3) `pathLookup`
- [ ] 记录命中来源（`env|bundled|path`）。
- [ ] 记录失败原因（未找到/不可执行/版本检查失败）。

验收：

- [ ] Windows/Linux path 分支单测。
- [ ] `lobster` 用例快照测试。

---

## 4. Node host 集成（Go <-> Node）

文件建议：

- `PinchBot/pkg/plugins/register.go`
- `PinchBot/pkg/plugins/managed_host.go`
- `PinchBot/pkg/plugins/assets/run.mjs`（只需透传，不做复杂逻辑）

任务：

- [ ] 在插件 discover + init 后挂载 `PluginRuntimeStatus`。
- [ ] 保持“单插件失败不阻断其他插件”。
- [ ] 日志输出增加统一字段：
  - `plugin_id`
  - `runtime_status`
  - `failed_checks`

验收：

- [ ] integration test：三插件场景，一插件缺依赖时 status=degraded，其余可用。

---

## 5. `/plugins/status` 响应扩展（Go Gateway）

文件建议：`PinchBot/pkg/gateway/pluginsstatus/handler.go`

- [ ] 在现有插件信息中追加：
  - `runtime_status`
  - `checks`
  - `repair_actions`
- [ ] 保持旧字段不变（向后兼容）。
- [ ] 鉴权与限流行为保持现状（与 `/tools/invoke` 对齐）。

验收：

- [ ] handler 测试：旧客户端可解析；新字段结构正确。

---

## 6. 修复动作 API（M3）

文件建议：`PinchBot/pkg/gateway/pluginsrepair/handler.go`

- [ ] 新增 `POST /plugins/repair`（建议）
  - 请求：`plugin_id`, `action_id`, `args`
  - 响应：`ok`, `job_id|result`, `logs`
- [ ] 风险动作要求确认 token（可选）：
  - `requires_approval=true` 时，先返回 `needs_approval`
- [ ] 先实现只读动作：
  - `explain_fix`
- [ ] 再实现可执行动作：
  - `install_bundled_cli`
  - `install_node_deps`

验收：

- [ ] API 单测 + 权限单测 + 幂等性单测。

---

## 7. 首批 reasonCode 规范

- `MANIFEST_INVALID`
- `NODE_NOT_FOUND`
- `NODE_TOO_OLD`
- `NPM_NOT_FOUND`
- `NODE_MODULES_MISSING`
- `BIN_NOT_FOUND`
- `BIN_NOT_EXECUTABLE`
- `BIN_VERSION_CHECK_FAILED`
- `CREDENTIAL_MISSING`
- `UNKNOWN_ERROR`

要求：

- [ ] 所有检查失败必须落 `reasonCode`，不返回自由文本当主判断条件。

---

## 8. 回滚与兼容

- [ ] Feature flag：`plugins.runtime_dependency_checks`（默认 true）。
- [ ] 关闭 flag 时回退旧行为，仅保留日志。
- [ ] 不修改现有扩展包结构即可运行。

---

## 9. 排期建议（两周）

第 1 周：

- [ ] 数据模型 + manifest 读取 + checker + status 输出

第 2 周：

- [ ] Launcher 展示联调 + repair API 骨架 + lobster 一键修复

