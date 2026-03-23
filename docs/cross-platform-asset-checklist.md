# Mac / Windows 双向资源检查清单

用于避免「本机开发正常、另一平台或干净克隆异常」类问题。原则：**凡是运行时要加载的静态文件，必须进 Git 或由发布脚本生成并打入包内**。

## 1. 已发现并处理的问题：`vendor/` 与 `.gitignore`

根目录 `.gitignore` 含有规则 **`vendor/`**。在 Git 中，该规则会忽略**任意路径**下名为 `vendor` 的目录（不限于 Go modules）。

| 现象 | 原因 |
|------|------|
| Mac 上聊天 Markdown 渲染正常，Windows / CI / 新克隆无渲染 | `frontend/index.html` 曾引用 `vendor/marked.min.js` 等，但 `frontend/vendor/` 被全局忽略，从未提交 |
| 修复 | 静态库改到 **`Launcher/app-wails/frontend/third_party/`** 并纳入版本库；单测 `TestDesktopFrontendBundlesMarkdownScriptDeps` 防止回归 |

本地自检：

```bash
git check-ignore -v Launcher/app-wails/frontend/vendor/foo.js
# 若显示被 vendor/ 规则忽略，则该路径不能作为已提交资源使用
```

## 2. Launcher 桌面壳（Wails）— Mac 与 Windows 共用同一套前端

| 项 | 说明 |
|----|------|
| 嵌入资源 | `main.go` 使用 `//go:embed all:frontend`，**Mac / Windows 二进制内嵌同一 `frontend/`** |
| 外链脚本 | 全仓库仅 `frontend/index.html` 使用 `<script src="...">`，且仅指向 **`third_party/*.min.js`**（已提交） |
| 运行时差异 | Windows 用 WebView2，macOS 用 WKWebView；**不复制两份 HTML**，行为差异主要来自 WebView 引擎，而非缺文件 |

发布前建议：在**干净克隆**上各打一次包，确认聊天助手气泡内 Markdown 正常。

## 3. Node：`node_modules` 被忽略（预期行为）

| 路径 | 策略 |
|------|------|
| `PinchBot/pkg/plugins/assets/node_modules/` | 忽略；由 **`scripts/build-release.ps1` / `.sh`** 在构建时 `npm ci` |
| `PinchBot/extensions/**/node_modules/` | 忽略；发布包内扩展（如 **lobster**）由脚本拷贝源码后在 **`dist/.../extensions/<name>/`** 内执行 `npm ci` |

若未安装 Node 或未跑发布脚本，会出现「本机有 node_modules 能用、别人机器没有」的差异——属**构建流程**问题，不是 Mac/Windows 分叉。

## 4. 其他嵌入资源（快速扫）

| 位置 | 嵌入方式 | 注意 |
|------|----------|------|
| `PinchBot/pkg/launcherui` | `//go:embed index.html` | 单文件，无外部 `src` 脚本 |
| `PinchBot/internal/workspacetpl` | `//go:embed workspace` | 工作区模板 |
| `Platform/internal/api` | `admin_index.html` + `admin_console_dist` | SPA 资源应在 `admin_console_dist` 内完整提交 |

## 5. 子进程「黑窗」（Windows）

Node 插件宿主由 Go `exec` 拉起 `node.exe` 时，若未设置 `CREATE_NO_WINDOW`，可能弹出黑色 CMD。与资源遗漏无关，见 `PinchBot/pkg/plugins/node_host_windows.go`（若已合并）。

## 6. 建议的发布前命令（双向）

在 **Windows** 与 **macOS** 各执行一次（或 CI 矩阵）：

1. 干净克隆后 **不**手拷任何 `vendor/` / `node_modules` 到 Launcher frontend。  
2. 按 `docs/build-and-release.md` 完整构建。  
3. 启动 `launcher-chat`，发一条含 **标题、列表、代码块** 的助手回复，确认 Markdown 渲染。  
4. 若启用 Node 插件：确认 `plugin-host` 与扩展目录内 **`node_modules` 已随包安装**（由脚本完成）。

---

**结论（本次审计）**：除已修复的 **`vendor/` 误伤静态库** 外，未发现第二处「HTML 引用本地目录但被 `.gitignore` 全局屏蔽」的同类问题；Mac 与 Windows 共用同一 `frontend/` 嵌入策略，差异主要来自本机是否具备构建时生成的依赖（Node 扩展、plugin-host），而非平台分支两套资源。
