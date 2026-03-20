# Graph Memory Node vs Go 对比评估

## 目标

用同一批会话数据，对比 `node-bridge` 与 `go-native` 两种运行模式在以下指标上的差异：

- 图谱规模：节点数、边数、社区数
- 召回质量：命中节点类型分布、Top 节点稳定性
- 上下文注入：系统注入长度（近似）
- 性能：端到端耗时、维护耗时

---

## 1. 评估前准备

1. 准备同一份业务对话样本（建议 20~50 轮，包含修复、排障、复盘）。
2. 准备两份 sidecar 配置：
   - `config.graph-memory.node.json`（`graph_memory_go_native=false`）
   - `config.graph-memory.native.json`（`graph_memory_go_native=true`）
3. 每轮评估前清空或切换独立 DB，避免数据串扰。

建议 DB 路径：

- Node: `~/.openclaw/graph-memory-node.db`
- Native: `~/.openclaw/graph-memory-native.db`

---

## 2. 运行模式切换

在 `config.json` 中切换：

```json
{
  "plugins": {
    "graph_memory_go_native": false
  }
}
```

或：

```json
{
  "plugins": {
    "graph_memory_go_native": true
  }
}
```

每次切换后重启进程，保证配置生效。

---

## 3. 建议采集指标（SQL）

对对应 DB 执行：

```sql
SELECT COUNT(*) AS total_nodes FROM gm_nodes WHERE status='active';
SELECT COUNT(*) AS total_edges FROM gm_edges;
SELECT COUNT(DISTINCT community_id) AS communities
FROM gm_nodes
WHERE status='active' AND community_id IS NOT NULL;
```

节点类型分布：

```sql
SELECT type, COUNT(*) AS c
FROM gm_nodes
WHERE status='active'
GROUP BY type
ORDER BY c DESC;
```

边类型分布：

```sql
SELECT type, COUNT(*) AS c
FROM gm_edges
GROUP BY type
ORDER BY c DESC;
```

Top 节点（用于稳定性比较）：

```sql
SELECT name, type, pagerank, validated_count
FROM gm_nodes
WHERE status='active'
ORDER BY pagerank DESC, validated_count DESC
LIMIT 10;
```

---

## 4. 运行日志采样

推荐采样以下日志关键词（两种模式都采）：

- `graph-memory: active`（确认模式）
- `graph-memory native auto maintain done`
- `graph-memory native session_end maintain done`
- `graph-memory assemble failed` / `afterTurn` 失败告警

对比项：

- 错误率（每 100 轮）
- 维护触发频次
- 单轮平均耗时（若日志有 duration 字段）

---

## 5. 对比结论模板

可直接按下面模板填：

```text
数据集：<名称>
轮次：<N>

1) 图谱规模
- Node:   nodes=<>, edges=<>, communities=<>
- Native: nodes=<>, edges=<>, communities=<>
- 差异：<>

2) 召回质量
- Top10 重叠率：<>
- TASK/SKILL/EVENT 分布差异：<>
- 结论：<>

3) 性能
- 平均轮次耗时：Node <>ms / Native <>ms
- 维护耗时：Node <>ms / Native <>ms
- 结论：<>

4) 稳定性
- 错误告警次数：Node <> / Native <>
- 是否存在阻断：<>

最终建议：
- [ ] 保持 go-native 默认
- [ ] 调整 nativeAutoMaintain 周期为 <>
- [ ] 开启/关闭 nativeLLMExtract
```

---

## 6. 当前推荐阈值

- `nativeAutoExtract=true`
- `nativeAutoMaintain=true`
- `nativeMaintainEveryTurns=20`
- `nativeLLMExtract=false`（先稳定，再灰度打开）

如果 Native 图谱规模明显低于 Node（例如节点数 < 70%），再考虑开启 `nativeLLMExtract`。

