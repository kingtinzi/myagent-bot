# Graph Memory Go Native 一步到位落地文档

## 1. 目标与范围

将 `graph-memory` 从 Node 扩展（`extensions/graph-memory` + Node host）改为 PinchBot 的 Go 原生插件能力，保留原有插件 ID `graph-memory` 与现有配置语义，减少发布体积并消除用户侧 Node/npm 运行依赖。

本次按“一步到位”目标执行，但上线策略仍建议保留一个可控回滚开关。

---

## 2. 当前状态（基于现有代码）

- `graph-memory` 现由 Node host 加载，插件入口是 `extensions/graph-memory/index.ts`。
- Agent 主循环中，图谱相关流程通过 `pkg/plugins/graph_memory_bridge.go` 间接调用 Node context engine：
  - `before_agent_start` 触发召回
  - `assemble` 注入 `systemPromptAddition` 并裁剪会话尾部
  - `afterTurn` 做 ingest + 异步提取
- Node 侧工具包括：`gm_search`、`gm_record`、`gm_stats`、`gm_maintain`。
- 目前 Go 原生排他加载（`nativeGoPluginExclusiveNodeIDs`）只包含 `llm-task`。

---

## 3. 最终架构（目标态）

### 3.1 设计原则

- 插件 ID 保持 `graph-memory`，不改用户 `plugins.enabled` 和 `plugins.slots.contextEngine` 配置方式。
- Graph Memory 运行时不再依赖 Node host。
- 保持 sidecar 文件 `config.graph-memory.json` 的启用语义：`enabled=true` 时激活。
- Node 版 `extensions/graph-memory` 可保留一段过渡期（不加载），但不再作为运行路径。

### 3.2 Go 侧模块划分

- `pkg/graphmemory/`（新增）：核心实现
  - `types.go`：配置与结构体定义
  - `store/`：SQLite 表结构与 CRUD（`gm_messages`, `gm_nodes`, `gm_edges`）
  - `recall/`：FTS/向量召回、图扩展、PageRank 排序
  - `extract/`：LLM 提取、finalize 规则
  - `format/`：assemble 注入内容构造、对话裁剪与修复
  - `maintenance/`：去重、社区、PageRank 维护
- `pkg/plugins/graph_memory_runtime.go`（新增）：插件运行时门面
  - `RuntimeActive(cfg)`：替代 `GraphMemoryRuntimeActive(cfg, host)` 对 host 的依赖
  - `BeforeAgentStart/Assemble/AfterTurn/SessionEnd`：Go 直接调用
- `pkg/tools/graph_memory.go`（新增）：原生工具注册
  - `gm_search`, `gm_record`, `gm_stats`, `gm_maintain`

---

## 4. 文件级改造清单

## 4.1 必改文件

- `pkg/plugins/discover_filter.go`
  - 将 `graph-memory` 加入 `nativeGoPluginExclusiveNodeIDs`。
  - 目的：同名 Node 插件永不加载，避免双栈冲突。

- `pkg/plugins/graph_memory_bridge.go`
  - 改造成 Go 运行时适配层，或保留文件名但移除 Node host 调用。
  - 删除 `host.ContextOp(...)` 相关逻辑，改为调用 Go runtime。

- `pkg/plugins/register.go`
  - `effectiveNodeHostEnabled` 保持 sidecar gating 语义，但 `graph-memory` 不再需要 Node host。
  - `LogGraphMemoryStartupStatus` 文案更新：不再依赖 “node plugin host did not start”。

- `pkg/agent/loop.go`
  - 当前图谱流程判断条件：`plugins.GraphMemoryRuntimeActive(al.cfg, agent.PluginHost)`。
  - 调整为：Go runtime 激活判断（不依赖 `agent.PluginHost`）。
  - 触发点保持：
    - 请求前：before + assemble
    - 请求后：afterTurn
    - 会话结束：session_end（如原先通过事件触发，改为 Go 生命周期钩子）

- `pkg/agent/instance.go`
  - 在工具注册时，当 `graph-memory` 启用，注册 Go 原生 gm_* 工具。
  - 不再依赖 Node catalog 中桥接出的 `gm_*`。

## 4.2 新增文件

- `pkg/graphmemory/...`（按第 3.2 节拆分）
- `pkg/tools/graph_memory.go`
- 对应单元测试文件（`*_test.go`）

## 4.3 可选清理（上线稳定后）

- `extensions/graph-memory` 保留源码用于对照迁移；运行包中可不再携带。
- 更新 `extensions/README.md`，注明 graph-memory 已改为 Go native。

---

## 5. 功能映射表（Node -> Go）

| Node 实现位置 | Go 目标模块 | 备注 |
|---|---|---|
| `index.ts` 事件钩子 | `pkg/plugins/graph_memory_runtime.go` | 保留事件语义与时序 |
| `src/store/*` | `pkg/graphmemory/store/*` | SQL 与索引尽量兼容 |
| `src/recaller/recall.ts` | `pkg/graphmemory/recall/*` | 召回质量需回归 |
| `src/extractor/extract.ts` | `pkg/graphmemory/extract/*` | 提取提示词与规则对齐 |
| `src/format/assemble.ts` | `pkg/graphmemory/format/*` | prompt 注入格式保持 |
| `src/graph/maintenance.ts` | `pkg/graphmemory/maintenance/*` | 维护任务移植 |
| Node `gm_*` tools | `pkg/tools/graph_memory.go` | tool schema 保持兼容 |

---

## 6. 配置兼容策略

- 继续使用 `config.graph-memory.json`，由 `pkg/config/graph_memory.go` 读取。
- `enabled=false` 或 sidecar 不存在：Graph Memory 全路径禁用（与现状一致）。
- `plugins.enabled` 仍需包含 `graph-memory`（与现状一致）。
- `plugins.slots.contextEngine="graph-memory"` 仍为必需（与现状一致）。

---

## 7. 验收标准（必须满足）

- 功能验收
  - `gm_search/gm_record/gm_stats/gm_maintain` 工具可用，输出结构与原逻辑一致。
  - 新会话触发召回，`assemble` 能注入上下文。
  - 多轮对话后 `gm_messages/gm_nodes/gm_edges` 正常增长。
  - `session_end` 后可观察到维护任务日志。

- 兼容验收
  - 不安装 Node/npm 也能运行 graph-memory。
  - 现有 `config.graph-memory.json` 无需变更即可生效。
  - 旧库数据可读取（SQLite schema 不破坏或带迁移逻辑）。

- 性能验收
  - 单轮新增延迟不可明显劣化（以当前 Node 版为基线）。
  - 召回和 assemble 在常见会话规模下保持可接受时延。

---

## 8. 风险与回滚

## 8.1 主要风险

- Node 与 Go 版本行为差异导致召回质量波动。
- SQLite schema 兼容性问题导致旧数据不可用。
- 事件时序偏差（例如 afterTurn 边界）影响提取效果。

## 8.2 回滚策略

- 保留配置开关 `plugins.graph_memory_go_native`（建议新增，默认 true）。
- 当开关关闭时，退回当前 Node bridge 路径（需要同时保留旧逻辑一段周期）。
- 回滚不要求用户改配置文件，仅需重启进程。

---

## 9. 实施步骤（执行顺序）

1. 新建 Go 核心模块骨架与工具骨架（可编译）。
2. 迁移 store 与基础查询，打通 `gm_stats`。
3. 迁移 `gm_record/gm_search`，完成工具链路。
4. 迁移 `before + assemble + afterTurn` 主流程。
5. 迁移 `session_end + maintenance`。
6. 将 `graph-memory` 加入 `nativeGoPluginExclusiveNodeIDs`，禁用 Node 同名加载。
7. 完整回归测试与灰度发布。

---

## 10. 文档与运维更新项

- 更新 `extensions/README.md`：说明 graph-memory 已原生化。
- 更新 workspace 技能文档：
  - `internal/workspacetpl/workspace/skills/extensions/SKILL.md`
  - `internal/workspacetpl/workspace/skills/extensions/references/pinchbot-extensions.md`
- 增加故障排查节：Go native 模式的日志关键词与常见错误。

---

## 11. 需要你确认的决策（请回复）

1. **是否需要保留回滚开关** `plugins.graph_memory_go_native`？
   - 建议：保留，默认 `true`，至少 1-2 个版本后再移除。

2. **数据库兼容策略**：
   - A. 完全复用现有 SQLite schema（推荐）
   - B. 新 schema + 自动迁移脚本

3. **发布策略**：
   - A. 直接切换（一步到位）
   - B. 单版本灰度（默认 Go，问题时可开关回退）

4. **`extensions/graph-memory` 目录处理**：
   - A. 保留源码（仅开发参考，不参与发布）
   - B. 直接删除（更干净，但不便对照）

---

## 11.1 已确认决策（2026-03-20）

- 回滚开关：**保留** `plugins.graph_memory_go_native`（默认 `true`）。
- DB 策略：**复用旧 schema**（仅在必要时做兼容性补齐，不做破坏式迁移）。
- 发布策略：**直接切换**（不做灰度）。
- `extensions/graph-memory`：**保留源码用于参考，但不参与发包**。

---

## 12. 执行备注

本文档是“直接开工版”的总控文档。你确认第 11 节后，可按第 9 节逐项落地代码与测试，不再需要额外方案文档。

---

## 13. 当前进度（2026-03-20）

- 已完成：
  - 新增 `plugins.graph_memory_go_native`（默认 `true`）并接入运行时判定。
  - Go 原生工具已接入：`gm_stats`、`gm_search`、`gm_record`、`gm_maintain`（最小可用维护）。
  - `loop` 已切到统一入口（按配置自动走 native 或 node-bridge）。
  - native `assemble` 已可做基础召回注入；native `afterTurn` 已完成消息入库。
  - native 自动提取（规则版）已上线：可从对话中生成 `TASK/SKILL/EVENT` 节点。
  - native 自动维护已上线：按 `nativeMaintainEveryTurns` 周期执行轻量 pagerank 重算。
  - 开关与节奏参数已加入 `config.graph-memory.example.json`：
    - `nativeAutoExtract`
    - `nativeAutoMaintain`
    - `nativeMaintainEveryTurns`
  - go-native 模式下，Node host 已排除加载 `graph-memory` 扩展，避免双栈冲突。

- 待完成：
  - `session_end` 语义与 finalize 能力迁移（当前仅有最小维护链路）。
  - 提取链路从规则版升级到更高质量（含边类型丰富化、可选 LLM 提取）。
  - 完整质量回归（召回命中率、上下文质量、时延对比）。
