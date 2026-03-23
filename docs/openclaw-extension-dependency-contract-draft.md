# OpenClaw 扩展统一依赖契约（草案）

目标：让 **所有 extensions 走同一诊断框架**，但按依赖类型进入不同修复路径；同时保持与 OpenClaw 生态兼容，不强行改写上游扩展。

相关背景：

- OpenClaw 扩展执行流程（上游）：`docs/dev/extension-execution-flow.zh-CN.md`
- PinchBot 现状：`docs/openclaw-extension-adapter-runbook.md`
- Lobster 痛点：扩展可加载，但外部 `lobster` CLI 未就绪时不可执行。

---

## 1. 设计原则

1. **兼容优先**：不破坏现有 `openclaw.plugin.json` 语义；新增字段应为可选扩展字段。  
2. **声明优先于猜测**：扩展显式声明运行时依赖，宿主只按声明做诊断与修复。  
3. **分层处理**：Node 依赖、外部二进制、服务端依赖、凭据依赖分开治理。  
4. **可审计**：诊断结果和修复动作都可记录到状态接口（建议并入 `/plugins/status`）。  
5. **用户可恢复**：每个失败都给出“下一步可执行动作”，而不是抽象错误。

---

## 2. 契约位置与字段

建议在扩展 manifest 中增加可选字段 `runtimeDependencies`（OpenClaw 未定义时可采用 `x-runtimeDependencies` 作为过渡键）。

示例（建议）：

```json
{
  "id": "lobster",
  "name": "Lobster",
  "configSchema": { "type": "object", "properties": {} },
  "runtimeDependencies": {
    "node": {
      "required": true,
      "minVersion": "18.0.0"
    },
    "executables": [
      {
        "id": "lobster",
        "required": true,
        "resolve": {
          "env": ["LOBSTER_BIN"],
          "bundledRelativePaths": [
            "tools/lobster/lobster.cmd",
            "tools/lobster/bin/lobster.js"
          ],
          "pathLookup": ["lobster"]
        },
        "versionArgs": ["--version"]
      }
    ],
    "npmInstall": {
      "required": true,
      "strategy": "ci-or-install",
      "workingDir": "."
    },
    "credentials": [
      {
        "key": "OPENCLAW_API_TOKEN",
        "required": false,
        "source": "env"
      }
    ]
  }
}
```

说明：

- `runtimeDependencies`：建议最终对齐到 OpenClaw 正式字段；在对齐前用 `x-runtimeDependencies` 过渡。  
- `executables[].resolve`：定义统一查找顺序（见下一节）。  
- `credentials`：只声明“需要什么”，不保存密钥明文。

---

## 3. 统一加载与解析顺序（宿主实现）

对每个扩展执行固定状态机：

1. `manifest_ok`：manifest 合法（含 `configSchema`）。  
2. `node_runtime_ok`：Node 版本满足。  
3. `npm_deps_ok`：扩展目录依赖可用（`node_modules` 就绪）。  
4. `external_bins_ok`：声明的外部命令可解析并可执行。  
5. `credentials_ok`：必需凭据已满足（环境变量或用户配置）。  
6. `ready`：允许注册工具并进入 agent 工具表。

外部命令解析顺序（统一）：

1. `env`：读取 `resolve.env`（如 `LOBSTER_BIN`）。  
2. `bundledRelativePaths`：从安装根/工作区按候选路径查找。  
3. `pathLookup`：最后才查系统 `PATH`（如 `where lobster` / `which lobster`）。

只要某一步失败，记录 `status=degraded` 和 `reasonCode`，不要崩整个插件宿主。

---

## 4. 诊断输出契约（建议并入 `/plugins/status`）

建议每个插件返回：

```json
{
  "plugin_id": "lobster",
  "runtime_status": "degraded",
  "checks": [
    { "id": "node", "ok": true, "detail": "v22.12.0" },
    { "id": "npm_deps", "ok": true, "detail": "node_modules present" },
    { "id": "exe:lobster", "ok": false, "detail": "not found in env/bundled/PATH" }
  ],
  "repair_actions": [
    {
      "id": "install_bundled_cli",
      "label": "安装内置 Lobster 运行时",
      "risk": "medium",
      "requires_approval": true
    }
  ]
}
```

---

## 5. UI 文案模板（统一）

状态标签：

- `就绪`（ready）
- `可用但受限`（degraded）
- `不可用`（blocked）

诊断卡片（示例）：

- 标题：`Lobster 运行时未就绪`
- 描述：`已加载扩展，但未找到 lobster 可执行文件。`
- 建议动作：
  - `一键安装内置运行时（推荐）`
  - `指定本地路径（LOBSTER_BIN）`
  - `查看详细诊断`

高风险动作确认弹窗（示例）：

- 标题：`确认安装扩展运行时`
- 正文：`将下载并安装 lobster CLI 到应用私有目录，不会修改系统全局 PATH。`
- 按钮：`取消` / `继续安装`

---

## 6. 修复路径分流（统一框架，不同动作）

1. **Node 依赖缺失**：在扩展目录执行 `npm ci`（锁文件优先）  
2. **外部 CLI 缺失**：优先安装到应用私有目录 + 写入 `*_BIN` 配置  
3. **凭据缺失**：引导用户填写或绑定，不自动猜值  
4. **平台服务不可达**：显示网络与鉴权检查项，支持重试

---

## 7. 与 OpenClaw 对齐策略

### 7.1 必须对齐（短期）

- manifest 与 `register(api)` 的现有行为不变  
- 插件发现、启用、工具注册语义不变  
- `/tools/invoke`、gateway 鉴权与限流行为保持当前对齐策略

### 7.2 PinchBot 增量（可回收）

- `runtimeDependencies`（或过渡 `x-runtimeDependencies`）  
- 统一诊断状态机 + `repair_actions`  
- 一键修复能力（宿主产品能力，不要求上游扩展实现）

### 7.3 回收计划（当上游给出正式字段）

- 若 OpenClaw 发布官方依赖声明字段，保留双读一段时间：  
  1) 先读官方字段；2) 再读 `x-runtimeDependencies`；3) 输出统一内部结构。  
- 两个版本周期后移除过渡字段写入，只保留读取兼容。

---

## 8. 落地顺序（建议）

1. 先给 `lobster` 落第一个样板（最痛点）。  
2. 在 `plugin host` 加统一解析器与状态机（不改业务工具）。  
3. `/plugins/status` 增加 `runtime_status/checks/repair_actions`。  
4. Launcher 增加“扩展诊断”页和“一键修复”入口。  
5. 再推广到其余扩展（按 `extension-matrix` 优先级）。

---

## 9. 本草案回答的核心问题

- 是否所有 extensions 都能用同方案？**框架能统一，修复动作不能一刀切。**  
- 用户自己安装 extensions 怎么办？**走统一诊断，再按依赖类型分流修复。**  
- 如何对齐 OpenClaw？**不改核心扩展协议，在宿主侧增加可回收增强层。**

