# Launcher 聊天小窗 — Wails 版（内嵌网页）

- **主程序**：用户只需运行 **launcher-chat.exe**；启动后会在**当前桌面主进程内**托管聊天网关（18790），并在 `config/platform.env` 存在时自动拉起 **platform-server**（18791）。
- **入口**：任务栏右侧托盘小图标。
- **窗口**：内嵌 WebView，加载 `frontend/` 下的网页。
- **前端**：当前为占位单页；可直接**替换为开源聊天 UI**（如 vue-advanced-chat、chat-ui 等），再改接口对接 `window.go.App.Chat` 与配置页。

**设置页策略**：配置页服务现在由 **launcher-chat.exe 进程内托管**；点击“设置”时会在当前桌面主进程内拉起 `18800` 本地设置服务，不再依赖单独的 `pinchbot-launcher.exe` 子进程。

**可执行文件查找顺序**（与 launcher-chat 同目录优先）：同目录下的 `pinchbot.exe`（兼容回退 `picoclaw.exe` 与 Windows 下 `picoclaw-windows-amd64.exe`）；若不存在则尝试 `PinchBot/build/`（便于开发时与 Makefile 产物一起用）。`pinchbot-launcher.exe` 仍可单独运行做调试/兼容，但桌面端默认不再依赖它。

## 运行

**开发（热重载）：**

```bash
cd Launcher/app-wails
wails dev
```

**生产 exe（推荐，可从任意目录运行）：**

Wails 必须带 build tags 编译，否则运行会报 “Wails applications will not build without the correct build tags”：

```bash
cd Launcher/app-wails
go build -tags "desktop,production" -o launcher-chat.exe -ldflags "-H windowsgui" .
# 得到 Launcher/app-wails/launcher-chat.exe，前端已内嵌，双击即可
```

或用 Wails 打包：

```bash
cd Launcher/app-wails
wails build
# 输出在 build/bin/launcher-chat.exe
```

**若运行的是 `launcher-chat-dev.exe`：**  
开发版会从**当前工作目录**找 `frontend` 文件夹，所以必须**先进入 app-wails 再运行**，否则会报 “assetdir '...\frontend' does not exist”：

```bash
cd D:\ProgramData\PinchBot\PinchBot\Launcher\app-wails
.\build\bin\launcher-chat-dev.exe
```

## 替换为开源前端

1. 将 `frontend/` 改为你的前端项目（Vue/React/纯 HTML 均可）。
2. 若用构建工具（Vite 等），把构建产物放到 `frontend/dist`，并在 `main.go` 里把 `//go:embed all:frontend` 改为 `//go:embed all:frontend/dist`（或按 Wails 文档配置 AssetServer）。
3. 在页面里通过 `window.go.App.Chat(message)` 发消息、`window.go.App.OpenSettings()` 打开设置。
4. 流式回复可用 Wails 的 Events 或后续在 Go 里轮询/SSE 再推事件到前端。

## Go 暴露给前端的方法

- `OpenSettings()` — 打开配置页（默认 18800）。
- `Chat(message string) (string, error)` — 发送消息（当前占位，可改为请求 Gateway/agent）。
