# 扩展运行时依赖 - 开发任务单（工程拆解）

基于：

- `docs/extensions-runtime-implementation-checklist.md`
- `docs/openclaw-extension-dependency-contract-draft.md`

---

## A. Go 端任务（按文件拆分）

## A1 `pkg/plugins/runtime_status.go`

- [ ] 定义 `PluginRuntimeStatus/PluginRuntimeCheck/PluginRepairAction`
- [ ] 提供 `NormalizeRuntimeStatus()`（兜底为 `degraded`）
- [ ] 提供 `MergeCheckResult()`（按 `id` 去重）

完成标准：

- [ ] `go test ./pkg/plugins -run RuntimeStatus`

---

## A2 `pkg/plugins/manifest.go`

- [ ] 在 manifest struct 增加：
  - `RuntimeDependencies map[string]any 'json:"runtimeDependencies,omitempty"'`
  - `XRuntimeDependencies map[string]any 'json:"x-runtimeDependencies,omitempty"'`
- [ ] 增加 `ExtractRuntimeDependencies(manifest) map[string]any`
  - 优先 `runtimeDependencies`
  - 回退 `x-runtimeDependencies`

完成标准：

- [ ] 对旧 manifest 零影响
- [ ] 非法字段不会让 `DiscoverEnabled` 崩溃

---

## A3 `pkg/plugins/runtime_checker.go`（新建）

- [ ] `CheckNodeRuntime(...)`
- [ ] `CheckNpmRuntime(...)`
- [ ] `CheckNodeModules(...)`
- [ ] `CheckExecutable(...)`
- [ ] `ResolveExecutablePath(...)`

实现细节：

- [ ] `ResolveExecutablePath` 顺序：`env -> bundled -> PATH`
- [ ] Windows 兼容 `Path/PATHEXT`
- [ ] 版本探测失败返回 `BIN_VERSION_CHECK_FAILED`

完成标准：

- [ ] `go test ./pkg/plugins -run RuntimeChecker`

---

## A4 `pkg/plugins/register.go`

- [ ] 在 `DiscoverEnabled + InitSnapshot` 后挂载 runtime checks
- [ ] 失败插件标记 `runtime_status=blocked/degraded`
- [ ] 不中断其他插件加载

完成标准：

- [ ] 三插件混合场景可通过（1 异常 + 2 正常）

---

## A5 `pkg/gateway/pluginsstatus/handler.go`

- [ ] 响应新增：
  - `runtime_status`
  - `checks`
  - `repair_actions`
- [ ] 兼容旧客户端（字段可缺省，不改变旧字段语义）

完成标准：

- [ ] `go test ./pkg/gateway/pluginsstatus`

---

## A6 `pkg/gateway/pluginsrepair/handler.go`（M3）

- [ ] 新增 `POST /plugins/repair`
- [ ] 支持动作：
  - `explain_fix`（只读）
  - `install_node_deps`
  - `install_bundled_cli`
- [ ] `requires_approval` 动作返回 `needs_approval`

完成标准：

- [ ] 鉴权复用 gateway auth（与 `/tools/invoke` 一致）

---

## A7 `pkg/gatewayservice/runtime.go`

- [ ] 注册路由：`/plugins/repair`
- [ ] 复用 shared rate limiter（与 `/plugins/status`、`/tools/invoke` 同策略）

---

## B. Node host 侧任务

## B1 `pkg/plugins/assets/run.mjs`

- [ ] `init` 返回结构加可选 `runtimeChecks`（透传）
- [ ] 仅透传，不新增复杂判定逻辑（判定由 Go 侧统一做）

完成标准：

- [ ] `node_host_integration_test` 不退化

---

## C. 配置与开关

## C1 `pkg/config/config.go`

- [ ] 新增 `plugins.runtime_dependency_checks`（默认 true）
- [ ] 新增 `plugins.auto_repair_enabled`（默认 false）

完成标准：

- [ ] 反序列化与默认值测试通过

---

## D. 首批测试清单

- [ ] 无 `lobster` 命令：`checks` 出现 `BIN_NOT_FOUND`
- [ ] 设置 `LOBSTER_BIN` 有效：`checks` 通过且 `hit=env`
- [ ] 内置路径存在：`hit=bundled`
- [ ] PATH 命中：`hit=path`
- [ ] `node_modules` 缺失：`NODE_MODULES_MISSING`
- [ ] 关闭 feature flag：不输出新字段

---

## E. 风险与回滚

- [ ] 若 `plugins.status` 新字段引发前端报错，先关 `runtime_dependency_checks`
- [ ] `plugins/repair` 默认关闭执行动作（仅 explain）
- [ ] 所有执行型 repair 动作必须打审计日志

