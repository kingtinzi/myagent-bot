---
name: user-extensions
description: "用户自行放入扩展目录后，如何完成安装与配置，使 PinchBot 网关/智能体能够加载并调用该扩展（与 workspace 下的 skills/ 说明文档不是同一类东西）。"
metadata: {"nanobot":{"emoji":"📦","requires":{"bins":["node"]}}}
---

# 用户自装扩展：如何被 PinchBot 调用

面向场景：**用户已经把一个扩展文件夹拷到本机**（或解压到某处），希望 **PinchBot 能发现并调用** 其中的工具，而不是「文件放在磁盘上却完全不起作用」。

## 先分清两类「技能」

| 类型 | 路径示例 | 作用 | 会不会被 PinchBot 当插件加载 |
|------|----------|------|------------------------------|
| **OpenClaw 风格扩展** | `…/extensions/<插件名>/` + `openclaw.plugin.json` | 网关通过 **Node 插件宿主** 加载，注册 **工具** 等 | ✅ 会（配置正确时） |
| **Workspace 技能说明** | `workspace/skills/<名>/SKILL.md` | 给 **智能体/人** 阅读的操作说明，**不是**可执行插件 | ❌ 不会自动变成工具 |

本 SKILL 只讲 **第一类**。若用户只是写了 `SKILL.md` 而没有 `openclaw.plugin.json`，需要说明：**那不会自动变成 PinchBot 扩展**，除非按 OpenClaw 扩展规范补齐清单与入口。

## 扩展应放在哪里

PinchBot 解析顺序（简化）：

1. **优先**：`<工作区>/extensions/<你的扩展目录>/`  
   - 工作区目录见用户 `config.json` 里 `agents.defaults.workspace`（常见为 `.openclaw/workspace` 或 `workspace`）。
2. **否则**：与 **网关/可执行文件同级** 的 `extensions/<你的扩展目录>/`（绿色包、安装目录旁常见）。

扩展目录内**必须**有 **`openclaw.plugin.json`**，且其中有 **`id`**（插件 ID，写入配置时用）。

## 安装 checklist（按顺序做）

### 1. 核对清单文件

- 存在 **`openclaw.plugin.json`**，记下 **`id`**（例如 `my-tool`）。
- 若有 **`package.json`**：说明是 Node 类扩展，后面要装依赖。

### 2. 安装依赖（仅 Node 类扩展）

在 **该扩展目录内** 打开终端（不是 PinchBot 仓库根目录）：

```bash
cd /path/to/extensions/<你的扩展目录>
npm ci
# 或没有 lock 文件时：
npm install
```

需要本机已装 **Node.js（建议 ≥ 18）** 与 **npm**。失败时检查网络/镜像（如国内 `npm config set registry …`）。

### 3. 修改 PinchBot 主配置 `config.json`

与当前运行的网关使用**同一份**配置（常在 `.openclaw/config.json` 或 `PINCHBOT_CONFIG` 指向的路径）：

```json
{
  "plugins": {
    "node_host": true,
    "enabled": ["现有插件id", "你的插件id"]
  }
}
```

- **`node_host`: `true`**：否则 Node 扩展**不会加载**。
- **`enabled`**：必须包含 **`openclaw.plugin.json` 里的 `id`**（大小写不敏感），否则**不会发现**该扩展。

若扩展文档要求 **slot**（如 `contextEngine`），再按文档改 **`plugins.slots`**。

### 4. 智能体是否允许调用工具（若使用白名单）

若配置里对智能体使用了 **`tools.allow`**，必须把扩展提供的**工具名**或插件维度加入允许列表（具体以扩展 README 为准；有的写 `allow: ["插件id"]` 即整插件）。

### 5. 重启网关

修改 `config.json` 后需 **重启 PinchBot 网关**（或重启承载网关的 `launcher-chat` / 服务），否则不会重新执行插件发现。

### 6. 验证

- 日志中出现该插件加载成功或 **Discover** 相关信息。
- 在客户端/调试界面查看 **工具列表** 是否出现新工具。

## 常见「放了文件夹却不生效」

| 现象 | 排查 |
|------|------|
| 完全没加载 | 扩展是否在 **`workspace/extensions`** 或 **可执行文件旁 `extensions`**；`openclaw.plugin.json` 是否存在 |
| 仍没有 | **`plugins.enabled`** 是否包含 **`id`**；**`node_host`** 是否为 **true** |
| Node 报错 | 是否在**扩展目录内**执行了 `npm install` |
| 工具不出现 | **智能体 `tools.allow`** 是否过严；扩展是否 `optional` 且未加入 allow |

## 与代理技能的关系

- **本 SKILL**：给用户/协作者一份 **自装扩展的固定步骤**。
- **详细排错与 JSON 片段**：见 **`skills/extensions/SKILL.md`** 与 **`skills/extensions/references/pinchbot-extensions.md`**。
- **Lobster**：除上述外还需系统 **`lobster` CLI**，见 **`skills/lobster/SKILL.md`**。

## 安全提示

扩展若要求 **apiKey、token** 等：写入 `config.json` 或侧车前 **与用户确认**；勿在聊天中粘贴明文密钥。
