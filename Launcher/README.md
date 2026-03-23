# Launcher — 独立配置与聊天小窗

Launcher 与主程序（PinchBot）分离，单独放在本目录，**不修改主程序**。包含：

- **独立配置文件**：MCP、BOT 本地权限、小窗 UI 配置，与主程序 `config.json` 分开，避免主配置过长。
- **聊天小窗**：类似参考图的窄高弹窗，可直接与 BOT 对话；设置通过菜单打开配置页（如 18800）。

## 目录结构

```
Launcher/
├── config/
│   └── launcher.example.json   # MCP + 本地权限 + 小窗配置示例
├── app-wails/                   # 聊天小窗（Go + Wails，内嵌网页，已接 Gateway）
│   ├── main.go
│   ├── app.go
│   └── frontend/                # 内嵌的网页，可替换为开源聊天前端
│       └── index.html
└── README.md
```

## 配置文件说明

- **路径建议**：`~/.PinchBot/launcher.json` 或与主 config 同目录下的 `launcher.json`。
- **mcp**：MCP 服务器列表（与主 config 中 `tools.mcp.servers` 结构一致），便于在 Launcher 设置里单独维护，不撑大主 config。
- **bot_permissions**：BOT 在本机的权限（是否允许执行命令、读写文件、网络、工作区与目录白名单等）。主程序暂不读取此文件，由 Launcher 使用；后续若主程序支持，可仅增加一个可选引用字段。
- **launcher_ui**：小窗标题、设置页 URL、Gateway 地址等。

主程序 `config.json` 保持不变；Launcher 只读本目录/独立路径下的 `launcher.json`（或你指定的路径）。

## 聊天小窗 — `app-wails/`

- **入口**：任务栏右侧（托盘）小图标，点击弹出窗口。
- **内部**：窗口里直接**内嵌网页**（WebView），可用任意前端技术 + 开源聊天 UI 改一版。
- **技术**：**Wails**（Go 壳 + WebView2）。Go 负责窗口、与 Gateway 通信；前端在 `frontend/`，可打成静态资源嵌入。
- **开源前端**：把 `app-wails/frontend/` 换成你选的开源聊天界面（如 [vue-advanced-chat](https://github.com/advanced-chat/vue-advanced-chat)、[chat-ui](https://github.com/nvima/chat-ui)），对接 `window.go.App.Chat` 与 `OpenSettings`。
- **运行**：`cd Launcher/app-wails && wails dev`；生产构建须带 tags，见 `app-wails/README.md`（Windows / macOS 命令不同）。

## 与主程序的关系

- **不修改 PinchBot**：不往主 config、主二进制里加 Launcher 逻辑。
- **可选联动**：若将来主程序需要「按文件限制权限」，可在主 config 中增加可选字段（如 `permissions_file`）指向本配置；当前阶段仅 Launcher 使用。

## 聊天通道、exec 与外部渠道（规划）

Launcher 小窗通过 Gateway 的 **`POST /api/chat`** 与 Agent 对话；在 PinchBot 里该会话的 **`Channel` 为 `launcher`**（与 Telegram、钉钉等「外部渠道」并列，由消息总线区分）。

- **`tools.exec`（本机 shell）**  
  - 默认 **`tools.exec.allow_remote` 为 `false`** 时，仅允许在**本地信任通道**上执行 shell（含 **`launcher`**、以及 `cli` / `system` / `subagent` 等）；避免未显式授权时，任意 IM 用户触发本机命令。  
  - 若希望 **所有渠道**（含远程 IM）都能 `exec`，需在主配置里将 **`allow_remote` 设为 `true`**（或环境变量 `PinchBot_TOOLS_EXEC_ALLOW_REMOTE`），并理解安全风险。  
  - 实现上，`launcher` 与纯「内部通道」区分：`IsInternalChannel` 仍用于心跳记录、出站分发等语义；**exec 信任列表**见 PinchBot `pkg/constants/channels.go` 中的 **`IsExecTrustedChannel`**。

- **后续：外部渠道通信配置**  
  计划在主配置或独立配置中补充：**按渠道**限制工具（含 `exec`）、速率、绑定 Agent 等，使「本机 Launcher」与「公网 IM」策略可分开。当前以全局 `tools.exec` + 上述通道判断为主；文档与配置项随功能落地再更新。

## 分发包内的扩展与 Node 依赖（macOS）

发布包若随 **`.app`** 附带 **`extensions/`**（例如 Lobster），依赖应**在打包阶段**已执行 `npm ci` / `npm install` 并带上 **`node_modules`**，**一般用户无需自己装 Node 依赖**。

若遇扩展加载异常、或你自行替换了 `extensions/` 目录，可在本机用「终端」进入扩展目录补装（需已安装 **Node.js** 与 **npm**）：

```bash
cd "/Applications/launcher-chat.app/Contents/MacOS/extensions/lobster"
npm ci
```

路径说明：**访达**中右键 **`launcher-chat.app`** → **显示包内容** → **`Contents` → `MacOS` → `extensions` → `lobster`**；上述 `cd` 路径与之一致。Windows 安装布局不同时，以安装目录下与 **`pinchbot` 可执行文件同级**的 **`extensions/lobster`** 为准（见 `docs/build-and-release.md` 发布包结构）。

产品侧后续可做：**首次启动检测 `node_modules` 缺失时提示**，或 **设置页一键修复**（需开发排期）；当前以文档 + 支持人员协助为主。
