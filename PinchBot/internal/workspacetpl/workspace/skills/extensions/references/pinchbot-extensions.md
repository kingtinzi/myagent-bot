# PinchBot extensions 参考

## `extensions` 目录如何解析

实现逻辑：`pkg/plugins/discover.go` 中 `ResolveExtensionsDir(workspace, plugins.extensions_dir)`。

- `extensions_dir` 默认 **`extensions`**（相对路径）。
- 若 `workspace/extensions` 存在且为目录 → 使用该绝对路径。
- 否则 → 尝试 **`<可执行文件目录>/extensions`**（发行包把扩展放在二进制旁边时常用）。
- 若配置为绝对路径 → 直接使用该路径。

Agent 帮用户排查「找不到扩展」时，应对上述两处都做 `list_dir` 核对。

## `openclaw.plugin.json`（节选）

```json
{
  "id": "my-plugin",
  "name": "My Plugin",
  "configSchema": { "type": "object", "properties": {} }
}
```

- **`id`**：必须出现在 **`plugins.enabled`** 中才会被 `DiscoverEnabled` 加载。
- **`configSchema`**：给人看的契约；敏感字段写入前与用户确认。

## `config.json` 中与 Node 扩展相关的片段

```json
{
  "plugins": {
    "enabled": ["graph-memory", "lobster"],
    "extensions_dir": "extensions",
    "node_host": true,
    "node_binary": "",
    "slots": {
      "contextEngine": "graph-memory"
    }
  }
}
```

- **`node_host`**：为 `false` 时不会启动 Node 插件宿主，TS 扩展不会加载。
- **`slots`**：仅当扩展文档要求时修改；不要随意改导致与 README 不一致。

## Lobster 扩展（`id: lobster`）

- 与多数 Node 扩展一样：需要 **`plugins.node_host`: `true`**、`plugins.enabled` 含 **`lobster`**，且扩展目录 **`npm ci`**。
- **额外要求**：插件实现会对子进程执行命令 **`lobster`**（见 `extensions/lobster/src/lobster-tool.ts` 中 `execPath = "lobster"`）。必须在运行网关的同一环境下安装 **Lobster CLI** 并保证 **`lobster` 在 PATH 中**。仅装好扩展的 `node_modules` **不能**替代 CLI。
- 工作区技能说明：`skills/lobster/SKILL.md`。

## graph-memory 侧车

文件位置：**与 `config.json` 同目录**，文件名 **`config.graph-memory.json`**。

结构因版本而异；仓库内示例：`PinchBot/config/config.graph-memory.example.json`。至少需 **`"enabled": true`**。graph-memory 由 **Go 运行时**实现（`pkg/graphmemory`），**不会**作为 Node 扩展加载。

## 与 Go 原生插件的区别

- **`llm-task`** 等可能由 Go 直接注册工具，**不一定**在 `extensions/` 下有 `openclaw.plugin.json`。
- 本技能主要针对 **`extensions/<name>/openclaw.plugin.json` + Node 宿主** 这一类扩展。
