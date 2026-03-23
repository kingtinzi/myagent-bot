---
name: lobster
description: "在 PinchBot 中启用并使用 Lobster 工作流工具：区分「扩展插件」与「lobster CLI」两层依赖；Node、node_host、扩展依赖与 PATH 上的 lobster 可执行文件。"
metadata: {"nanobot":{"emoji":"🦞","requires":{"bins":["node","lobster"]}}}
---

# Lobster 工作流（PinchBot）

容易混淆：**仓库里的 `extensions/lobster` 是 PinchBot 的 Node 扩展**（把 `lobster` **工具**注册进智能体）；**真正跑流水线时**，扩展内部会启动子进程执行命令 **`lobster`**（与 `node extensions/...` 不是一回事）。

## 两层依赖（都要满足）

| 层级 | 是什么 | 你要做什么 |
|------|--------|------------|
| **① PinchBot 插件** | `extensions/lobster` + Node 插件宿主 | `plugins.node_host: true`，`plugins.enabled` 含 **`lobster`**；扩展目录内 **`npm ci`/`npm install`**（发行包通常已带好 `node_modules`）；智能体 **`tools.allow`** 含 **`lobster`**（若使用白名单）。 |
| **② Lobster CLI** | 系统上的 **`lobster` 可执行文件**（`PATH` 可查） | 单独安装 **Lobster 命令行**（插件代码里写死 `spawn("lobster", argv)`）。**仅装 Node、仅装好扩展，没有 CLI，仍会失败。** |

验证命令（在用户本机终端执行）：

```bash
node --version   # 建议 ≥ 18
where lobster    # Windows
which lobster    # macOS/Linux
lobster --help   # 应能运行
```

若 `lobster` 找不到：按你所用发行方式安装 CLI（例如部分环境使用 **`npm install -g lobster`** 或官方文档给出的包名；**以 Lobster/OpenClaw 官方说明为准**），直到 `lobster` 在 **运行 PinchBot 网关的同一用户 / 同一环境** 下可用。

## 推荐配置片段（`config.json`）

```json
{
  "plugins": {
    "enabled": ["lobster"],
    "node_host": true
  },
  "agents": {
    "list": [
      {
        "id": "main",
        "tools": {
          "allow": ["lobster"]
        }
      }
    ]
  }
}
```

修改后需 **重启网关 / PinchBot**，插件才会重新加载。

## 与「extensions」通用技能的关系

扩展目录、`npm ci`、路径解析等：**见 `skills/extensions/SKILL.md`** 与 `references/pinchbot-extensions.md`。  
本技能只补充：**Lobster 额外需要 PATH 上的 `lobster` CLI**，不要把说明写成「只要 Node 就行」。

## 常见现象

| 现象 | 可能原因 |
|------|----------|
| 工具列表里没有 `lobster` | `plugins.enabled` / `node_host` / 扩展路径 / 未重启 |
| 调用 `lobster` 报找不到命令 | **未安装 Lobster CLI** 或 PATH 未包含 `lobster` |
| `MODULE_NOT_FOUND`（扩展侧） | 在 **`extensions/lobster`** 目录执行 `npm ci`，不要在仓库根目录误跑 |

## 代理操作建议

1. 先确认 **`lobster` 在终端可执行**，再排查 PinchBot 配置。  
2. 再核对 **`plugins.node_host`** 与 **`plugins.enabled`**。  
3. 再核对 **`agents...tools.allow`**。  
4. 仍失败时收集：**网关日志** + **`lobster --help`** 输出 + **`where lobster`**。
