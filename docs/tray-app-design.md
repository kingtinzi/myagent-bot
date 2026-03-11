# Windows 托盘应用形态设计

> 目标：系统托盘图标 + 点击聊天小窗 + 设置窗口，普通人可配置，任务在本地执行。

---

## 一、产品形态

### 1. 系统托盘（任务栏右侧图标）

- 常驻 Windows 托盘区，单图标（如 PicoClaw/Claw  logo）。
- **左键单击**：弹出「聊天小窗」。
- **右键菜单**：
  - 打开设置
  - 显示/隐藏聊天
  - 开机自启 开/关
  - 退出

### 2. 聊天小窗（主入口）

- 小弹窗（例如 400×500 或可拖拽缩放），紧贴托盘或屏幕一侧，不抢焦点时可半透明。
- **功能**：
  - 输入框 + 发送，直接与 BOT 对话。
  - 对话在**本机**执行（调本地 picoclaw 进程 / 内嵌 agent），结果流式展示。
  - 可选：最近会话、快捷指令（写邮件、查日程等）。
- **与现有能力**：复用现有 agent/模型/技能，不新造一套逻辑，只做 UI 壳。

### 3. 设置窗口（点击「设置」打开）

一个完整的设置程序框，包含以下配置模块（与现有 config 对应）：

| 配置模块 | 说明 | 对应现有能力 |
|----------|------|----------------|
| **使用模型** | 选择/配置主模型、备用模型、API Key、端点 | `config.model_list`、`agents.defaults.model_name`，launcher 已有模型管理 |
| **SKILL / 插件** | 安装、启用/禁用、列表展示 | `picoclaw skills`，workspace/skills，可复用 skill 列表与安装接口 |
| **MCP 脚本** | 配置 MCP 服务器、脚本路径、开关 | 对应 config 中 MCP 相关配置，需在设置里暴露 |
| **社交软件联通** | Telegram、微信、钉钉、飞书、QQ 等通道的配置 | `config.channels`，launcher 已有频道表单，可直接复用或嵌入 |
| **BOT 本地权限** | 文件访问范围、是否允许执行命令、网络访问、可访问目录白名单等 | 需在 config 中增加「本地权限」段（如 `local_permissions`），在设置里做开关与路径配置 |

- **实现建议**：  
  - 方案 A：设置窗口内嵌**现有 Launcher 前端**（localhost:18800 的 Web UI），以独立窗口或嵌入式浏览器打开，无需重写一套配置页。  
  - 方案 B：用原生控件（如 Fyne/Wails）重写设置页，与 config JSON 绑定，适合后续做「离线/单文件」分发。

---

## 二、和现有代码的关系

- **picoclaw**：核心进程，负责 agent、模型、技能、MCP、通道。托盘应用不替代它，而是**调用/内嵌**它（例如本地起子进程或 in-process 调用）。
- **picoclaw-launcher**：当前为 Web 配置编辑器（HTTP 18800），可直接作为「设置」的后端与 UI 来源；托盘进程可先启动 launcher 的 HTTP 服务，再在「设置」里打开浏览器或嵌入 WebView。
- **picoclaw-launcher-tui**：TUI 版，与托盘形态并行，不影响。
- **config 路径**：继续使用 `~/.picoclaw/config.json`（或 Windows 下等效路径），设置窗口读写的仍是同一份配置。

---

## 三、技术实现要点

### 托盘 + 多窗口

- **Go 方案**：
  - 托盘： [getlantern/systray](https://github.com/getlantern/systray) 或 [gen2brain/dlgs](https://github.com/gen2brain/dlgs) 等，Windows 下可用 `shell_notifyicon` 或现成 Go 封装。
  - 聊天小窗：可用 **Wails** 或 **Fyne** 做一个小窗口 + 输入框 + 消息列表；若希望用 Web 技术，可用 **Wails** 的 WebView 加载本地或内嵌的聊天页，通过 JS↔Go 绑定调用本机 agent。
  - 设置窗口：  
    - 复用 Launcher：托盘进程启动 launcher 的 HTTP 服务，点击「设置」时用默认浏览器打开 `http://localhost:18800`，或 Wails 里用 WebView 加载该 URL（推荐先这样快速落地）。  
    - 或后续用 Fyne/Wails 做纯原生设置界面，直接读写在 `pkg/config` 的 Config 结构上。

### 本地任务与权限

- 所有「和 BOT 对话、执行任务」都在本机完成：要么启动本地 picoclaw 进程并与之通信（stdin/stdout、HTTP 或本地 socket），要么在托盘进程内直接引用 picoclaw 的 agent 包跑 loop。
- **BOT 本地权限** 建议在 config 中增加一节，例如：
  - `allow_shell`、`allow_file_read`、`allow_file_write`、`allow_network`
  - `workspace_path` 或 `allowed_dirs` 白名单
  - 在设置界面用勾选 + 路径列表编辑即可，agent 执行前检查这些字段。

### 安装与分发

- 安装包：NSIS / Inno Setup / MSIX 等，安装后写注册表实现开机自启（可选）。
- 单文件：Go 编译为单一 exe，托盘 + 内嵌 launcher 时需把 launcher 的 HTTP 和静态资源一起打进同一进程（当前 launcher 已 embed 前端，可复用）。

---

## 四、建议实现顺序

1. **托盘 + 菜单**：仅实现托盘图标与「打开设置 / 退出」，点击设置用默认浏览器打开 `http://localhost:18800`（需先启动 launcher 或把 launcher 集成进同一进程）。
2. **聊天小窗**：小窗口 + 输入框 + 调用本机 agent 的接口（先简单轮询或本地 HTTP），流式输出到小窗。
3. **设置窗口内嵌**：用 Wails 或 Electron 的 WebView 内嵌 Launcher 页面，不再依赖外部浏览器。
4. **配置项补全**：在 config 与 Launcher UI 中增加 MCP 配置、BOT 本地权限配置。
5. **体验优化**：开机自启、小窗位置记忆、主题等。

---

## 五、小结

- **托盘**：入口统一，不占任务栏主区域。
- **聊天小窗**：直接对话、本地执行，符合「在本地电脑上处理任务」。
- **设置**：模型、SKILL、插件、MCP、社交联通、BOT 本地权限一屏配置，可先复用现有 Launcher，再逐步原生化。
- 与现有 PicoClaw/Launcher 兼容，扩展 config 与权限模型即可，无需推倒重来。
