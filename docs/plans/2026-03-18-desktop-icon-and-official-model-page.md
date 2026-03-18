# Desktop Icon and Official-Only Model Page Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 恢复 PinchBot 主程序图标链路，并让用户端设置页模型页只显示后台同步下来的官方模型，不再展示假数据或测试模型。

**Architecture:** 主程序图标修复分为运行时托盘图标与现有 Windows 资源链路两部分，尽量不改窗口标题策略，只恢复图标显示。模型页修复基于当前“平台同步官方模型到本地配置”的架构，前端只渲染 `official/` 模型并将其视为只读展示，不再为无数据场景回退到示例模型。

**Tech Stack:** Go, Wails, 内嵌 HTML/JS, Go 内容测试, PowerShell 打包脚本

---

### Task 1: 恢复主程序托盘图标

**Files:**
- Modify: `Launcher/app-wails/app.go`
- Test: `Launcher/app-wails/app_runtime_test.go` 或 `Launcher/app-wails` 现有内容测试/运行时测试

**Step 1: Write the failing test**

为托盘初始化补一个测试，要求 `runTray()` 在存在嵌入图标时调用图标设置逻辑，并使用 `PinchBot` tooltip。

**Step 2: Run test to verify it fails**

Run: `go test ./... -run Test.*Tray.*Icon`

Expected: FAIL，说明当前没有真正设置托盘图标。

**Step 3: Write minimal implementation**

在 `runTray()` 中恢复：
- `systray.SetIcon(trayIcon)`（仅当 `len(trayIcon) > 0`）
- `systray.SetTooltip("PinchBot")`

**Step 4: Run test to verify it passes**

Run: `go test ./... -run Test.*Tray.*Icon`

Expected: PASS

### Task 2: 让用户端设置页模型页只显示官方模型

**Files:**
- Modify: `PinchBot/pkg/launcherui/...`（实际设置页前后端）
- Modify: `PinchBot/cmd/picoclaw-launcher/internal/ui/index.html`（若仍被共享引用）
- Test: `PinchBot/cmd/picoclaw-launcher/ui_content_test.go`
- Test: `PinchBot/pkg/launcherui/...` 相关测试（如已有）

**Step 1: Write the failing test**

补测试，要求模型页：
- 只渲染 `official/` 模型
- 不展示示例/假数据
- 不展示可编辑的官方内部字段（如 baseurl、api key）
- 无官方模型时显示空态文案，而不是示例模型

**Step 2: Run test to verify it fails**

Run: `go test ./... -run Test.*Official.*Model|Test.*Model.*Page`

Expected: FAIL

**Step 3: Write minimal implementation**

在设置页数据装配与前端渲染中：
- 过滤掉非 `official/` 模型
- 删除示例模型 fallback
- 将官方模型展示改为只读卡片/列表
- 不渲染 `APIBase` 等内部字段

**Step 4: Run test to verify it passes**

Run: `go test ./... -run Test.*Official.*Model|Test.*Model.*Page`

Expected: PASS

### Task 3: 全量验证与打包验证

**Files:**
- Verify only

**Step 1: Run desktop and settings test suites**

Run:
- `cd Launcher/app-wails && go test ./...`
- `cd PinchBot && go test ./...`

Expected: PASS

**Step 2: Run release packaging**

Run:
- `powershell -ExecutionPolicy Bypass -File .\scripts\build-release.ps1 -Version v1.0.0-official-only -Zip -Installer`

Expected: 生成新的 ZIP 和 Setup 安装包

**Step 3: Commit**

只提交本次相关文件，避免带入无关改动。
