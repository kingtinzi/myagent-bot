---
name: extensions
description: "安装并启用 PinchBot OpenClaw 风格扩展（extensions）：npm 依赖、config.json 注册；graph-memory 为 Go 侧车（非 Node 扩展）。敏感项与用户确认后再写入。发布包通常不含 node_modules。"
metadata: {"nanobot":{"emoji":"🧩","requires":{"bins":["node"]}}}
---

# PinchBot 扩展（extensions）安装与启用

当用户**下载/复制扩展目录**到 `extensions`、提示缺少 `node_modules`、扩展**未出现在工具列表**、或 **Node 插件宿主未加载**时，按本技能操作。

**面向用户自装的步骤清单**（更短、偏「怎么装才生效」）：见 **`skills/user-extensions/SKILL.md`** 与本技能互补。优先用 `read_file` / `list_dir` / `exec`（在允许的环境）完成；需要多步确认时可用 **`lobster`** 工具拆成可暂停、可恢复的工作流。

**Lobster 扩展特例**：除 Node 与 `node_host` 外，工具运行时还会执行系统命令 **`lobster`**（CLI），必须已在 **PATH** 上安装，详见 **`skills/lobster/SKILL.md`**。不要只教用户装扩展目录依赖而漏掉 CLI。

## 先搞清三件事

1. **扩展根目录**里必须有 `openclaw.plugin.json`，其中的 **`id`** 是配置里要启用的插件 ID（与目录名可以不同）。
2. **物理路径**：PinchBot 解析 `plugins.extensions_dir`（默认 `extensions`）时，**优先** `<agent workspace>/extensions`，若不存在则使用 **可执行文件同级的 `extensions`**（发行包常见布局）。必要时读用户 `config.json` 里的 `agents.defaults.workspace` 与 `plugins.extensions_dir` 再拼路径。
3. **Node 类扩展**（含 `index.ts`、需 `npm install` 的）必须 **`plugins.node_host`: `true`**，且 **`plugins.enabled`** 数组包含该 manifest 的 **`id`**（大小写不敏感）。

更细的规则与 JSON 片段见：`references/pinchbot-extensions.md`。

## 推荐流程（按顺序）

### 1. 识别扩展

- 打开 `<extensionsRoot>/<folder>/openclaw.plugin.json`，记录 **`id`**、**`name`**、**`configSchema`**（若有）。
- 若存在 **`package.json`** / `package-lock.json`，发行包通常**未带** `node_modules`，需要本机安装依赖。

### 2. 安装依赖（在扩展目录内）

- 有 `package-lock.json`：优先 `npm ci`；否则 `npm install`。
- **工作目录**必须是该扩展目录（例如 `.../extensions/lobster`），不要用仓库根目录误跑。
- 需要 **Node ≥ 18**（官方扩展说明里常写推荐 **22**）。若命令失败，把完整错误输出给用户并建议检查 Node/npm 版本与网络/registry。

### 3. 修改主配置 `config.json`（与 PinchBot 进程使用的那份一致）

- 设置 **`plugins.node_host`: `true`**（若已是 true 则跳过）。
- 在 **`plugins.enabled`** 中加入 manifest 的 **`id`**（勿重复；保留原有项如 `graph-memory` 仅作插件 ID，其运行时由 Go 提供，无需在 `extensions/` 下安装目录）。
- 若扩展 README 要求 **slot**（例如 `contextEngine` 指向某引擎），按文档更新 **`plugins.slots`**。
- **`extensions_dir`**：若扩展放在可执行文件旁而 workspace 下没有 `extensions`，一般**不必**改（解析会自动 fallback）；若用户把扩展只放在 workspace 外且未被找到，可改为**绝对路径**指向扩展根目录。

### 4. 特例：`graph-memory`（Go 原生，**不是** Node 扩展）

- **无需**在 `extensions/` 下安装任何目录或 `npm install`。运行时由 **`pkg/graphmemory`** 提供。
- 在 **`config.json` 同目录** 放置 **`config.graph-memory.json`**，且 **`"enabled": true`**，并配置 **`dbPath`**（及可选 LLM/embedding 等密钥）。
- 从仓库示例起步：`PinchBot/config/config.graph-memory.example.json`。
- **`enabled`: `true`** 前，与用户确认侧车中 **apiKey 等字段**；可建议用环境变量或脱敏方式，避免把密钥写进聊天日志。

### 5. 敏感项与用户确认（通用）

对 `configSchema` 里 **`apiKey`、`token`、`password`、`secret`** 等：

- **先问用户**再写入文件；不要猜测或套用模板占位符当真值。
- 若扩展支持从环境变量读取，优先帮用户设计 **env + 配置引用**，而不是把明文写进 JSON。

### 6. 收尾

- 明确提示用户 **重启 PinchBot / 网关**，否则不会重新 `DiscoverEnabled`、也不会重启 Node 宿主。
- 验证：日志中出现对应扩展或工具名；若扩展注册了 bridge 工具，在工具列表中应可见。

## 可选：用 `lobster` 做多步协作

若当前环境已启用 **lobster** 工具，可将「读 manifest → `npm ci` → 合并 `config.json` → 侧车确认 → 提示重启」拆成带 **用户批准节点** 的工作流，便于中断后继续。逻辑与本节文字流程一致，不必重复实现不同规则。

## 常见故障

| 现象 | 排查 |
|------|------|
| 工具不出现 | `plugins.enabled` 是否含正确 **`id`**；`node_host` 是否为 true；扩展路径是否在解析到的 `extensionsRoot` 下 |
| graph-memory 不工作 | **`config.graph-memory.json`** 是否存在且 **`enabled`: true** |
| `MODULE_NOT_FOUND` | 在**扩展目录**执行 `npm ci` / `npm install` |
| 远程会话无法 exec | 默认 `tools.exec` 可能限制通道；安装依赖需在允许执行 shell 的环境操作，或请用户在本地终端执行技能里给出的命令 |
