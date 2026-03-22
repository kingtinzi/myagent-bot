# PinchBot 接入 `extensions/lobster`：对照 OpenClaw 扩展执行流程

本文把 [openclaw-main 扩展执行流程](https://github.com/openclaw/openclaw/blob/main/docs/dev/extension-execution-flow.zh-CN.md)（下文称「流程文档」）里的阶段，落到 **在 PinchBot（Go）里让 `PinchBot/extensions/lobster` 真正可用** 时要做的具体事。  
实现上可走两条路：**A. Go 实现插件宿主 + Node 执行 TS（Lobster 不改）**；**B. 无 TS 链路时纯 Go 重写 lobster 工具逻辑**。下表默认先按「要对齐 OpenClaw 行为」描述，再在备注里区分路径。

### 策略前提：谁在 Go 里做、Lobster 要不要改

| 目标形态 | Lobster（TS） | PinchBot 主要改哪里 |
|----------|---------------|---------------------|
| **与 OpenClaw 一样**：含「发现 → 读 `openclaw.plugin.json` → **Node（jiti/tsx）执行 `index.ts` → `register(api)`** → 提供与 OpenClaw 等价的 `OpenClawPluginApi` / `PluginRuntime`（至少覆盖 lobster） | **不必改成 Go**，仓库里现有 `extensions/lobster` 继续用 | **用 Go 实现插件宿主与编排**（拉起/管理 Node、IPC、把注册结果并入 Go 侧 `ToolRegistry` 等）。TS 仍在 Node 里跑，不是把插件翻译成 Go。 |
| **仅** Go 里 agent / session / 模型调用与 OpenClaw 同构，**没有** TS 扩展执行链路 | TS 插件**跑不起来** | **具体问题具体分析**；常见做法是 **仍在 Go 侧补一条**（要么补「执行 TS」层，仍不必改 Lobster；要么走路径 **B**，把该工具逻辑 **用 Go 重写**）。**大概率动的是 Go**（补宿主或端口），而不是先改上游 Lobster 源码。 |

一句话：**宿主能力与 OpenClaw 对齐且包含 TS 执行链 → Lobster 保持 TS；缺链时 → 优先在 Go 上补链或端口，Lobster 是否改看个案。**

### 范围：多插件支持（首版启用集）

- **实现上**：TS 插件宿主按 **多插件** 设计与交付——**discovery**（扩展根下多个子包）、**各自** `openclaw.plugin.json`、**按配置启用**、**单 Node 进程内顺序/分组加载**、**插件级错误隔离**（单个失败可记录 diagnostic，不默认拖死宿主）、工具名冲突策略（与 OpenClaw 类似：先定义好优先级或拒绝加载）。
- **首版默认**：配置里 **`plugins.enabled` 仅包含 `lobster`**（或等价：只保证仓库内 **lobster** 一条路径 E2E）；**随仓库放入更多 `extensions/<id>/` 时，无需改宿主架构**，只需安装依赖、写入 enabled 与 allowlist。
- **验收**：除 lobster 外，至少保留 **第二个「空壳/最小」扩展** 的集成测试或文档用例，证明 **多插件加载路径** 可用（可为内部 fixture，不必对外承诺具体插件）。

---

## 流程文档 §1 概念 —— Lobster 实际用到什么

| 概念 | Lobster 情况 |
|------|----------------|
| 扩展包 | `PinchBot/extensions/lobster`，入口 `index.ts`（`package.json` → `openclaw.extensions`）。 |
| 清单 | `openclaw.plugin.json`：`id: lobster`，**configSchema 为空 object**（无额外配置字段）。 |
| 插件 API | 仅 **`register(api)` → `api.registerTool(factory, { optional: true })`**；工厂在 **`ctx.sandboxed`** 时返回 `null`（不注册工具）。 |
| SDK 依赖 | `definePluginEntry`（`openclaw/plugin-sdk/core`）、类型与 `definePluginEntry` 配套类型来自 **`openclaw/plugin-sdk/lobster`**（`runtime-api.ts` re-export）。 |
| 工具本体 | `createLobsterTool(api)`：`spawn("lobster", …)`、解析 JSON envelope；**可选**打 `api.logger?.debug`、`api.runtime?.version`。不调用 `runEmbeddedPiAgent` / channel / hooks。 |

---

## 流程文档 §2 触发加载 —— PinchBot 要在哪 hook

| OpenClaw | PinchBot 要做的事 |
|----------|-------------------|
| CLI `ensurePluginRegistryLoaded` / Gateway `loadGatewayPlugins` | 在 **Gateway 或 Agent 启动**（与注册工具同一时刻）调用你的「插件加载」或 **直接注册 Go 版 `lobster` 工具**。若走 Node：**在子进程/嵌入 Node 启动时**完成加载，再把工具列表同步到 Go。 |
| Gateway 专有的 `setGatewaySubagentRuntime` 等 | **Lobster 不依赖子代理**；PinchBot **可省略**，除非同一加载器还要带其他插件。 |

---

## 流程文档 §3 发现 —— 候选从哪来

| OpenClaw | PinchBot 要做的事 |
|----------|-------------------|
| `discoverOpenClawPlugins` 扫 config / workspace / bundled / global | 至少能 **稳定指向** `PinchBot/extensions/lobster`（或配置里的扩展根目录列表）。不必一开始就做完全部 origin 优先级；**固定相对路径 + 配置覆盖**即可。 |
| 路径安全（越界、权限） | 若允许用户配置扩展目录，建议 **解析为绝对路径并限制在允许根目录内**（对齐 OpenClaw 思路，防路径逃逸）。 |

---

## 流程文档 §4 清单 —— `openclaw.plugin.json`

| OpenClaw | PinchBot 要做的事 |
|----------|-------------------|
| `loadPluginManifestRegistry` | **读取并解析** `openclaw.plugin.json`，得到 **插件 id `lobster`**；校验 **存在 `configSchema`**（可为空 properties）。缺则拒绝加载（与 OpenClaw 一致）。 |
| 与 `package.json` 合并 | 可选；Lobster 主要用 `openclaw.extensions` 指定入口，若你手写路径可暂不强依赖合并逻辑。 |

---

## 流程文档 §5 加载 —— 核心差异在「谁执行 TS」

### 5.1 配置与缓存

| OpenClaw | PinchBot 要做的事 |
|----------|-------------------|
| `normalizePluginsConfig`、`plugins.allow` 等 | 在配置中增加 **是否启用 lobster**、**工具 allowlist**（README 写明需 `agents...tools.allow` 含 lobster；PinchBot 需有 **等价机制**：例如现有工具开关 + 按名 allow）。 |
| Registry 缓存 | 可按需实现；首版可 **每次启动全量加载** 简化问题。 |

### 5.2 Registry 与 PluginRuntime

| OpenClaw | PinchBot 要做的事 |
|----------|-------------------|
| `createPluginRegistry` + 完整 `PluginRuntime` | **路径 A（Node）**：在 Node 里构造 **最小 `OpenClawPluginApi`**：`registerTool`、`pluginConfig`、`logger`、`runtime`（至少 `version` 可常量化），并实现 **`OpenClawPluginToolFactory` 的 `ctx.sandboxed`** 语义。 **路径 B（Go）**：无 TS registry，直接在 Go `ToolRegistry` 注册 **`lobster` 工具**，并在 **沙箱/受限模式** 下不暴露该工具（对齐 `ctx.sandboxed`）。 |

### 5.3 Jiti / 模块加载

| OpenClaw | PinchBot 要做的事 |
|----------|-------------------|
| jiti + `openclaw/plugin-sdk` 别名 | **路径 A**：Node 22+、在扩展目录 `npm install`（依赖含 **`openclaw`** 包以解析 `openclaw/plugin-sdk/*`，与仓库内 `extensions/lobster` 的 import 一致）、用 **jiti 或 tsx** 执行 `index.ts`。 **路径 B**：不加载 TS，**把 `lobster-tool.ts` 行为移植为 Go**（spawn、参数、超时、JSON 解析）。 |

### 5.4 `register(api)` 与工具注册

| OpenClaw | PinchBot 要做的事 |
|----------|-------------------|
| `registerTool(factory, { optional: true })` | **路径 A**：Node 收集注册的工具定义，通过 IPC 交给 Go；Go 侧注册为 **optional**（与现有 agent 工具策略一致）。 **路径 B**：Go 工具标记为可选，仅在 allowlist 启用时交给模型。 |
| TypeBox `parameters` | 映射为 PinchBot `Tool.Parameters() map[string]any` 或项目使用的 JSON Schema 格式，保证 **action run/resume、pipeline、token、approve、cwd…** 与 TS 一致。 |

### 5.5 激活与 hooks

| OpenClaw | PinchBot 要做的事 |
|----------|-------------------|
| `setActivePluginRegistry` + `initializeGlobalHookRunner` | **Lobster 无 hooks**；仅需 **把 `lobster` 工具并入当前 agent 的工具表**。无需实现全局 hook runner，除非统一插件框架要预留。 |

---

## 流程文档 §6 图 —— PinchBot 等价数据流（路径 A）

```mermaid
flowchart LR
  GW[PinchBot 启动 / AgentLoop 初始化]
  CFG[读配置 + openclaw.plugin.json]
  NODE[Node: jiti 加载 index.ts]
  REG[register api.registerTool]
  BR[IPC / 注册表同步]
  GO[Go ToolRegistry: lobster]
  GW --> CFG --> NODE --> REG --> BR --> GO
```

路径 B：省略 NODE/REG/BR，**GW → CFG → Go 实现 lobster 工具**。

---

## Lobster README 中的进阶能力（易遗漏）

- **`openclaw.invoke`**：Lobster 流水线可回调 OpenClaw 的 **`POST /tools/invoke`**。PinchBot **若无等价 HTTP 工具桥**，则文档中的该能力 **不可用**，需在文档或配置里标明「未实现」或后续按 [openclaw-plugin-parity](../.cursor/rules/openclaw-plugin-parity.mdc) 补同构。
- **本机依赖**：`lobster` 可执行文件须在 **PATH**；Windows 下插件内已有 spawn 辅助逻辑，发布与 PATH 需在 PinchBot 安装说明中写明。

---

## 建议落地顺序（第一个插件）

1. **配置与清单**：能读到 `extensions/lobster/openclaw.plugin.json`，并在配置中开关 / allowlist `lobster`。  
2. **最小工具闭环**：优先 **路径 B（Go 移植 spawn + schema）** 或 **路径 A 的最小 Node 加载器 + 单工具 IPC** 二选一，跑通一次 `action=run` / `resume`。  
3. **对齐语义**：`sandboxed` 时不暴露工具；`optional` 与 allowlist 行为与 OpenClaw 文档一致。  
4. **再考虑**：`openclaw.invoke`、缓存与 provenance（多插件 discovery **纳入宿主必做**，见上文「范围」）。

---

## 开发内容清单（WBS）

以下为可拆给迭代/PR 的工作包；**路径 A = Node 加载 TS + 桥接 Go**，**路径 B = Go 原生实现工具**（二选一作为主路线，另一路径可标为后续）。

### P0 决策（0.5d 内）

| 编号 | 内容 | 产出 |
|------|------|------|
| D-01 | 选定主实现路径 A 或 B（性能/维护/与后续 llm-task 一致性） | ADR 或团队结论 |
| D-02 | 是否首版支持 `openclaw.invoke` | 明确「不支持」或立项 |

### P1 配置与清单（与路径无关）

| 编号 | 内容 | 产出 |
|------|------|------|
| C-01 | 扩展根路径策略：默认 `extensions/` 相对可执行文件或工作区 + 可选配置覆盖 | 配置项设计 + 解析代码 |
| C-02 | **多插件**：扫描扩展根下各子目录，读取各自的 `openclaw.plugin.json`，校验 `id`、`configSchema`；无效项记 diagnostic | 单插件失败不拖死全局（策略可配置） |
| C-03 | **`plugins.enabled: string[]`**：首版默认仅 `["lobster"]`；启用多个 id 时均走同一套加载链路 | 与现有 `config` 结构一致（新建 `plugins` 节） |
| C-04 | 工具级策略：**optional** + **allowlist**（多插件时按插件 id / 工具名与 OpenClaw 语义对齐） | 与 `extensions/lobster/README.md` 语义对齐 |
| C-05 | **沙箱/受限执行模式**：该模式下不暴露需隐藏的插件工具 | 对齐 `ctx.sandboxed` 行为 |
| C-06 | **工具名 / 插件 id 冲突**：重复注册时的策略（拒绝后者 / 按来源覆盖）与日志 | 文档写明 |

### P2a 路径 B — Go 原生 `lobster` 工具

| 编号 | 内容 | 产出 |
|------|------|------|
| B-01 | 实现 `pkg/tools/lobster.go`（或等价包）：`Name/Description/Parameters` | JSON Schema 与 TS 版一致（action run/resume、pipeline、token、approve、cwd、timeout、maxStdoutBytes） |
| B-02 | `Execute`：`exec.CommandContext` 调 `lobster`，cwd 相对工作区且禁止逃逸（对齐 TS `resolveCwd`） | 单测覆盖越界 cwd |
| B-03 | stdout 收集、超时、`maxStdoutBytes`、解析 envelope JSON（含尾部 JSON 容忍逻辑） | 与 TS `parseEnvelope` 行为一致或可文档化差异 |
| B-04 | Windows：`argv`/`windowsHide` 等与 TS `windows-spawn` 行为对齐或文档说明差异 | 在 Windows CI 或手测记录 |
| B-05 | 在 `AgentLoop`（或统一注册点）按 C-03～C-06 条件 `Register`（多插件时逐个注册） | 端到端：配置 allow 后模型可调用 |

### P2b 路径 A — Node 加载 + Go 桥接

| 编号 | 内容 | 产出 |
|------|------|------|
| A-01 | 扩展目录 `npm install`（`package.json` 增加 `openclaw` 等），构建/发布流程包含依赖安装 | `scripts` 或 CI 步骤 |
| A-02 | Node 启动脚本：**对 `plugins.enabled` 中每个扩展** 用 jiti/tsx 加载入口，实现最小 `OpenClawPluginApi`（`registerTool`、`pluginConfig`、`logger`、`runtime.version`） | 可本地命令行验证多插件 `register` 无报错 |
| A-03 | 从 Node 聚合各插件的 **工具 schema + pluginId**，经 IPC 交给 Go；`execute` 带 **pluginId + toolName** 路由 | 协议文档 |
| A-04 | Go 侧 **每个工具** `Tool` 适配器（或统一适配器内路由）：IPC 调用 Node，转 `ToolResult` | 与 B-05 相同端到端标准；多工具多插件均可 |
| A-05 | 进程生命周期：Gateway 启停时拉起/回收 **单个** Node 宿主进程，崩溃重试策略 | 运维说明 |
| A-06 | **双插件验收**：lobster + 最小 fixture 扩展同时 enabled 通过集成测试 | 证明多插件支持非预留 |

### P2c 路径 A（graph-memory）— ContextEngine 与事件桥接

| 编号 | 内容 | 产出 |
|------|------|------|
| GM-A-01 | ~~Node graph-memory 扩展~~ **已移除**；graph-memory 为 **Go 原生**（`pkg/graphmemory` + `config.graph-memory.json` 侧车）。Node 宿主仍服务 lobster 等 TS 扩展 | 侧车 `enabled:true` 且 `plugins.enabled` 含 `graph-memory` 时 Go 路径生效 |
| GM-A-02 | IPC 协议扩展：新增 `contextEngine` 通道（`assemble`/`afterTurn`/`compact`/`dispose`）与事件通道（`before_agent_start`、`session_end`） | 协议文档 + Node/Go 双端实现 |
| GM-A-03 | AgentLoop 接线：在构建模型请求前触发 `before_agent_start`；发送前调用 `assemble` 并接入 `systemPromptAddition`；回包后调用 `afterTurn`（携带 `prePromptMessageCount`） | graph-memory 召回和每轮抽取生效 |
| GM-A-04 | 生命周期与兼容：会话结束触发 `session_end`；子代理场景透传 `prepareSubagentSpawn`/`onSubagentEnded`（若当前产品未启用则记录降级策略） | finalize/maintenance 可运行，行为可解释 |
| GM-A-05 | 配置分层：主配置保留 `plugins.enabled`，并加载独立 `config.graph-memory.json` 映射到 `pluginConfig`（不存在或 `enabled=false` 时自动降级旧逻辑） | 开关可控，不影响未启用用户 |
| GM-A-06 | 与 PinchBot 现有摘要策略协调：启用 graph-memory 时定义 `maybeSummarize` 策略（关闭/抬阈值/保留兜底三选一）并固化默认值 | 避免双重压缩冲突 |
| GM-A-07 | 稳定性与性能：单 Node 宿主多插件复用、超时/崩溃重试、`assemble` 关键路径延迟预算（目标 P95） | 运维指标 + 回归基线 |
| GM-A-08 | 集成验收：`lobster + graph-memory + fixture` 三插件同进程启用；验证工具可调用、召回注入、抽取入库、失败回退 | 多插件+记忆链路端到端验收报告 |

#### P2c 默认决策（已确认，可覆盖）

以下为 graph-memory（方案 A）实施基线；若 ADR 变更请同步改本节。

| 议题 | 决策 |
|------|------|
| **GM-A-06 与 `maybeSummarize`** | **抬阈值，不首关摘要**：启用 graph-memory 时提高 `summarize_message_threshold`（约原默认的 3～5 倍）、`summarize_token_percent` 提到约 **90%+**；保留 PinchBot 现有 **emergency compression** 作兜底。稳定后再评估是否改为「关闭摘要」。 |
| **`session_end` 触发（GM-A-04）** | **多条件并存**：① 用户显式新会话（如 `/new`、`/reset`）立即触发；② **空闲超时**（建议 30～60 分钟无新消息，具体值可配置）；③ **进程退出** 时 best-effort 触发（带超时保护）。 |
| **`config.graph-memory.json` 路径（GM-A-05）** | 读取顺序：① 环境变量 **`PINCHBOT_GRAPH_MEMORY_CONFIG`**（绝对路径，若设置则优先）；② **`dirname(config.json)/config.graph-memory.json`**；③ 文件不存在或 `enabled=false` → **完全走现有逻辑**。 |
| **与 OpenClaw 对齐程度** | **两阶段**：**首版最小可用** — `before_agent_start` + `assemble` + `afterTurn`，未启用时 100% 旧逻辑；**二阶段**再补齐 `compact` / `session_end` 严格语义、子代理钩子及事件参数与上游更严同构。 |

**已实现（PinchBot，首版）：** `plugins.node_host: true` 时，`pkg/plugins` 在 `workspace` + `plugins.extensions_dir` 下按 `openclaw.plugin.json` 发现扩展，启动 `pkg/plugins/assets/run.mjs`（stdio 单行 JSON），`init` / `execute` 协议；Go 注册 `bridgeTool`。宿主依赖为 `jiti` + `@sinclair/typebox`；`openclaw/plugin-sdk/*` 由本地 shim + 精简 `windows-spawn` 替代完整 `openclaw` npm 包。**需 Node ≥ 18**（与上游扩展语法一致）。`plugins.enabled` 含 `lobster` 且 Node 宿主成功注册 `lobster` 工具时，不再注册 Go 版 `lobster`；失败则回退 Go 工具。

**A-05（生命周期 / 重试）：** 子进程不再绑定短生命周期 `context`（避免 init 结束后被误杀）。`ManagedPluginHost` 在 **Execute** 遇到可恢复的 IPC/管道类错误时会关闭旧进程、退避后 **重新 spawn + init** 再重试（默认额外 2 次；配置项 `node_host_max_recoveries`，设为 **-1** 可关闭执行期自动恢复）。**启动**阶段 spawn+init 默认最多 **3** 次尝试（`node_host_start_retries`）。网关 `Stop` → `AgentLoop.Stop` → `AgentRegistry.StopPluginHosts` → 每 agent `StopPluginHost()` 显式结束 Node 宿主。

**构建 / CI：** `PinchBot/Makefile` 的 `build` / `build-all` 依赖 `plugin-host-deps`（`npm ci`）。`scripts/build-release.sh` / `build-release.ps1` 在编译前 `npm ci`，并把 `pkg/plugins/assets` 复制为与二进制同目录的 **`plugin-host/`**（运行时通过 `executableAdjacentPluginHostDir` 解析）。GitHub Actions：`build`、`PR test`、`release` 流水线已配置 Node 22 + `npm ci`（或 Makefile）。GoReleaser `before` 钩子含 `npm ci`（便于 Docker 等构建；发布归档若未含 `plugin-host`，请用上述 release 脚本或自行拷贝）。

### P3 联调、测试与文档

| 编号 | 内容 | 产出 |
|------|------|------|
| T-01 | 集成测试或手工脚本：`action=run` 最小 pipeline、`action=resume` 各一条 | 记录在 `docs/` 或测试代码 |
| T-02 | 用户文档：`lobster` CLI 需安装、PATH、与 `openclaw.invoke` 是否支持 | 更新主 README 或 `extensions/lobster/README.md` PinchBot 专节 |
| T-03 | 日志与错误：可执行文件缺失、超时、JSON 解析失败 | 可观测性验收 |

### P4 后续（非首版必做）

| 编号 | 内容 | 说明 |
|------|------|------|
| F-01 | （已合并） | 多插件 discovery / 启用策略见 **P1 C-02～C-06**、**P2b A-02～A-06** |
| F-02 | `POST /tools/invoke` 或等价能力 | 支撑 `openclaw.invoke` |
| F-03 | Registry 缓存、provenance / allowlist 防误加载 | 安全与性能 |

### 依赖关系简图

```text
D-01 → P1(C-01～C-06) → P2a 或 P2b
P2b（若接 graph-memory）→ P2c(GM-A-01～GM-A-08) → P3
  └→ D-02 影响 T-02 文案与 F-02 排期
```

---

*流程文档原文位于 openclaw 仓库：`docs/dev/extension-execution-flow.zh-CN.md`。*
