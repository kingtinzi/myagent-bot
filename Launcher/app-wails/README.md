# Launcher 聊天小窗 — Wails 版（内嵌网页）

- **主程序**：用户只需运行 **launcher-chat.exe**（macOS 为 **launcher-chat.app**）；启动后会在**当前桌面主进程内**托管聊天网关（18790）。**仅当解析到的平台 API 地址为本机**（`127.0.0.1` / `localhost`）时，才会在存在 **`config/platform.env`** 且能找到 **platform-server** 可执行文件的前提下，自动拉起本机 **platform-server**（监听地址由其中的 `PLATFORM_ADDR` 决定，默认 `127.0.0.1:18791`）。若已在 `platform.env` 或环境里配置了 **`PICOCLAW_PLATFORM_API_BASE_URL` 指向远端**，则**不会**再自动启动本机 `platform-server`。
- **远端平台 API（余额/登录等）**：Launcher 启动时会先读取 **`{安装根}/config/platform.env`** 并 `Setenv`。推荐在其中设置 **`PICOCLAW_PLATFORM_API_BASE_URL=http(s)://你的平台主机:端口`**（无尾部 `/`）。**优先级**（后者在未设置时才会用到）：启动参数/ldflags → **`PICOCLAW_PLATFORM_API_BASE_URL` 环境变量** → **`~/Library/Application Support/OpenClaw/config.json`**（或对应 OS 下的 PinchBot 主目录）里的 **`platform_api.base_url`** → 代码内置默认。PinchBot 网关 `LoadConfig` 后会执行 `env.Parse`，因此同一进程内 **`PICOCLAW_PLATFORM_API_BASE_URL` 会覆盖 `config.json` 里的 `platform_api.base_url`**（便于分发包只改 `platform.env` 而不改用户主目录配置）。
- **平台服务未随应用启动？**（仅**本机**平台 API 场景）需同时满足：① 解析到的平台 API 为本机；② 存在 **`{launcher-chat.app 所在文件夹}/config/platform.env`**；③ **`platform-server` 在 PATH 中**，或与 `Contents/MacOS/launcher-chat` 同目录，或与 `.app` 同级等（见下文查找顺序）。缺任一项则不会自动启动本机服务。若你使用**远端** `platform-server`，请改 **`PICOCLAW_PLATFORM_API_BASE_URL`**，不要依赖本机是否拉起进程。
- **把已有 `Platform/config/platform.env` 打进产物：** 执行 **`wails build`** 时，`wails.json` 里的 **postBuildHooks** 会自动把仓库里的 `Platform/config/platform.env` 复制到 **`build/bin/config/platform.env`**（与 `.app` 同级）。若本机还没有该文件则跳过并打印提示。该文件默认在 `.gitignore` 中，请勿把密钥提交到公开仓库。若只用 **`go build`** 而未走 Wails，请手动执行一次：`bash scripts/copy-platform-env.sh`（在任意目录均可）。
- **自动拉起的 `platform-server` 工作目录：** 已固定为 **与 Launcher 相同的安装根**（即含 `config/platform.env` 的那一层），保证与 postBuild 复制到 `build/bin/config/` 的配置一致；不要依赖「可执行文件在 `Platform/` 目录下」来读 env。
- **入口**：任务栏右侧托盘小图标。
- **窗口**：内嵌 WebView，加载 `frontend/` 下的网页。
- **前端**：当前为占位单页；可直接**替换为开源聊天 UI**（如 vue-advanced-chat、chat-ui 等），再改接口对接 `window.go.App.Chat` 与配置页。

**设置页策略**：配置页服务现在由 **launcher-chat.exe 进程内托管**；点击“设置”时会在当前桌面主进程内拉起 `18800` 本地设置服务，不再依赖单独的 `pinchbot-launcher.exe` 子进程。

**可执行文件查找顺序**（与 launcher-chat 同目录优先）：同目录下的 `pinchbot.exe`（兼容回退 `picoclaw.exe` 与 Windows 下 `picoclaw-windows-amd64.exe`）；若不存在则尝试 `PinchBot/build/`（便于开发时与 Makefile 产物一起用）。`pinchbot-launcher.exe` 仍可单独运行做调试/兼容，但桌面端默认不再依赖它。

## macOS 常见问题

1. **双击 `.app` 只有 Dock 弹跳、没有窗口**  
   - 请用 **Dock 图标再点一次** 或 **Command+Tab** 切到应用（部分环境下首次需二次激活）。  
   - **勿**在访达里只双击 `Contents/MacOS/launcher-chat`：那样不会按「应用程序包」方式启动，行为与 `.app` 不一致，且容易与 **App Translocation**（未签名包被系统拷到临时目录）叠加，导致找不到旁边的 `config/`。  
   - 若仍无界面：在终端执行  
     `"/path/to/build/bin/launcher-chat.app/Contents/MacOS/launcher-chat"`  
     查看是否有报错；或对 `.app` 执行 `xattr -cr launcher-chat.app` 后重试；或将 **整个 `build/bin` 文件夹**（含 `.app`、`config/`、`Platform/platform-server` 等）保持相对位置一起移动。

2. **平台服务起不来**  
   - 确认 **`launcher-chat.app` 与 `config` 目录同级**，且存在 `config/platform.env`。  
   - `platform-server` 会从 **上述安装根** 下的 `config/platform.env` 读配置（与 Launcher 判断逻辑一致）。

3. **双击 .app 后闪退**  
   - 常见原因：① 包内或 **App Translocation 只读卷** 无法写入数据。当前 **`.app` 启动时默认将 PinchBot 数据放在** **`~/Library/Application Support/OpenClaw/`**（始终可写）。② 仍请使用 **`wails build` / `package-macos.sh` 重新打包** 后再试。③ 可选：对 `.app` 执行 `xattr -cr launcher-chat.app`，或将整包移到「应用程序」文件夹再双击。

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

**macOS（无控制台、链接 WebKit 时需 UniformTypeIdentifiers）：**

```bash
cd Launcher/app-wails
mkdir -p build/bin
go build -tags "desktop,production" \
  -ldflags '-extldflags "-framework UniformTypeIdentifiers"' \
  -o build/bin/launcher-chat .
```

说明：**系统托盘菜单仅在 Windows 构建中启用**（`getlantern/systray` 与 Wails 在 macOS 上会重复定义 `AppDelegate`，无法同进程链接）。在 macOS 上请用 **Dock** 与主窗口操作应用。

## macOS 给用户的一整包（dist）

在仓库根目录执行 **`./scripts/package-macos.sh`** 会：

1. 清理 **`PinchBot/build`**、**`Launcher/app-wails/build/bin`**（及 `build/darwin`）、仓库内 **`Platform/platform-server`** 等中间产物，并删除旧的 **`dist/PinchBot-*-macOS-*`**；
2. 依次构建 PinchBot、（默认还有）platform-server、Wails Launcher；
3. 在 **`dist/PinchBot-<git版本>-macOS-<amd64|arm64>/`** 生成可直接交给用户的目录：`launcher-chat.app`、`pinchbot`、（默认）`platform-server`、`config/`、`README.txt`。

**纯远端平台**（`platform-server` 只部署在服务器、客户包不带本机二进制）：  
**`./scripts/package-macos.sh --remote-platform`** — 跳过 platform-server 的编译与拷贝；分发包内 **`README.txt`** 会说明需在 **`config/platform.env`** 配置 **`PICOCLAW_PLATFORM_API_BASE_URL`**。

仅清理不构建：`./scripts/package-macos.sh --clean-only`。

### 给客户发 DMG

1. 先打好 **`dist/PinchBot-…-macOS-…/`**（`./scripts/package-macos.sh`）。
2. 再执行：**`./scripts/package-dmg.sh`**（会自动选 `dist` 里最新的 `PinchBot-*-macOS-*`；也可传入目录路径）。
3. 得到 **`dist/PinchBot-…-macOS-….dmg`**，内含完整文件夹 + **「应用程序」** 替身，方便用户把 **`launcher-chat.app`** 拖进应用程序。

**发给陌生客户前**：未签名/未公证的 DMG 可能被 **「无法验证开发者」** 拦截。需用 **Apple Developer** 账号对 **`.app` 内二进制** 做 **`codesign`**，并对 DMG 做 **公证（`notarytool`）**；详见 [Apple 文档](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)。内测可先右键 **打开** 绕过。

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
