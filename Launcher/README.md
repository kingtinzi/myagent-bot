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
