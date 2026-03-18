# PinchBot 本地单进程化重构计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将本地客户端从“桌面壳 + 设置页服务 + 网关进程”逐步收敛到“一个桌面主进程 + 远端平台后端”的结构，先完成桌面壳与设置页服务合并。

**Architecture:** 第一阶段不直接硬切三进程合一，而是先把 `pinchbot-launcher` 的 HTTP 设置服务抽成可复用包，由 `launcher-chat.exe` 进程内托管同一套设置页静态资源与 API；这样可以先消灭本地第二个 UI 进程和 `18800` 外部独立服务。第二阶段再把 `pinchbot gateway` 从 Cobra CLI 入口中拆成可嵌入服务，在同一桌面主进程内启动，最终实现本地单进程。`platform-server` 继续部署在服务器侧，不并入桌面端。

**Tech Stack:** Go、Wails、PinchBot 本地配置/认证模块、内嵌静态资源、HTTP ServeMux

---

### Task 1: 固化重构边界与约束

**Files:**
- Modify: `docs/plans/2026-03-18-local-single-process-refactor-plan.md`
- Reference: `Launcher/app-wails/app.go`
- Reference: `PinchBot/cmd/picoclaw-launcher/main.go`
- Reference: `PinchBot/cmd/picoclaw-launcher/internal/server/*.go`

**Step 1: 记录目标边界**

- 本地合并范围：
  - `launcher-chat.exe`
  - `pinchbot-launcher.exe`
  - 后续 `pinchbot gateway`
- 保持独立：
  - `platform-server`

**Step 2: 明确第一阶段目标**

- 用户继续通过桌面端打开设置
- 设置页仍可走 `http://127.0.0.1:18800`
- 但该端口由 `launcher-chat.exe` 进程内托管
- 不再依赖 `pinchbot-launcher.exe` 子进程

**Step 3: 明确验收标准**

- 不启动 `pinchbot-launcher.exe` 时，点击桌面端“设置”仍能打开并正常使用设置页
- 设置页关键 API 仍返回 200
- 桌面端现有登录/聊天/官方模型链路不回退

**Step 4: 记录风险**

- `pinchbot-launcher` 目前代码在 `cmd/` 下，不能直接作为共享库使用
- 设置页静态资源与嵌入方式要迁移到可复用 package
- 安装包与发布脚本要同步调整

**Step 5: Commit**

```bash
git add docs/plans/2026-03-18-local-single-process-refactor-plan.md
git commit -m "docs: add local single-process refactor plan"
```

### Task 2: 抽离可复用设置服务 package

**Files:**
- Create: `PinchBot/pkg/launcherui/server.go`
- Create: `PinchBot/pkg/launcherui/server_test.go`
- Create: `PinchBot/pkg/launcherui/assets.go`
- Modify: `PinchBot/cmd/picoclaw-launcher/main.go`
- Reference: `PinchBot/cmd/picoclaw-launcher/internal/server/*.go`
- Reference: `PinchBot/cmd/picoclaw-launcher/internal/ui/index.html`

**Step 1: 写失败测试**

- 测试 `launcherui` 能创建统一的 `http.Handler`
- 测试根路径 `/` 能返回设置页 HTML
- 测试 `/api/config`、`/api/auth/status`、`/api/app/session` 仍被注册

**Step 2: 运行测试确认失败**

Run:

```bash
go test ./pkg/launcherui -run TestNewHandler
```

Expected: FAIL（package/handler 尚不存在）

**Step 3: 最小实现**

- 新建 `launcherui` package
- 负责：
  - 嵌入设置页静态资源
  - 组装已有 `RegisterConfigAPI / RegisterAuthAPI / RegisterAppPlatformAPI / RegisterWorkspaceAPI / RegisterProcessAPI`
  - 返回统一 `http.Handler`

**Step 4: 保持 `pinchbot-launcher main` 变薄**

- `cmd/picoclaw-launcher/main.go` 只保留：
  - flag 解析
  - addr 选择
  - basic auth 包装
  - 调用 `launcherui.NewHandler(...)`

**Step 5: 运行测试确认通过**

Run:

```bash
go test ./cmd/picoclaw-launcher/... ./pkg/launcherui/...
```

Expected: PASS

**Step 6: Commit**

```bash
git add PinchBot/pkg/launcherui PinchBot/cmd/picoclaw-launcher/main.go
git commit -m "refactor: extract reusable launcher settings service"
```

### Task 3: 在 launcher-chat 进程内托管设置服务

**Files:**
- Modify: `Launcher/app-wails/app.go`
- Modify: `Launcher/app-wails/app_auth_test.go`
- Modify: `Launcher/app-wails/app_runtime_test.go`
- Modify: `Launcher/app-wails/go.mod`
- Reference: `PinchBot/pkg/launcherui/server.go`

**Step 1: 写失败测试**

- 测试 `launcher-chat` 在不启动 `pinchbot-launcher.exe` 子进程时：
  - 能在本进程启动 `18800`
  - `/api/config` 可访问
  - `OpenSettings()` 仍能打开设置页

**Step 2: 运行测试确认失败**

Run:

```bash
go test ./... -run TestEnsureSettingsServiceStarted
```

Expected: FAIL（当前实现仍依赖外部 `pinchbot-launcher.exe`）

**Step 3: 最小实现**

- 在 `app.go` 中增加 settings HTTP server 生命周期管理
- `ensureSettingsServiceStarted()` 改为：
  - 优先检查本进程内 settings server 是否已启动
  - 若未启动，则直接在当前进程启动 `launcherui` handler
  - 不再启动 `pinchbot-launcher.exe` 子进程

**Step 4: 保持兼容**

- `pinchbot-launcher.exe` 仍可单独运行（便于过渡和调试）
- 但桌面端默认不再依赖它

**Step 5: 跑测试**

Run:

```bash
go test ./...
```

Expected: PASS

**Step 6: Commit**

```bash
git add Launcher/app-wails/app.go Launcher/app-wails/*.go Launcher/app-wails/go.mod
git commit -m "refactor: host settings service inside launcher process"
```

### Task 4: 验证桌面端 + 设置页同进程链路

**Files:**
- Modify: `Launcher/app-wails/frontend_content_test.go`
- Modify: `PinchBot/cmd/picoclaw-launcher/ui_content_test.go`
- Reference: `Launcher/app-wails/frontend/index.html`
- Reference: `PinchBot/cmd/picoclaw-launcher/internal/ui/index.html`

**Step 1: 增加回归测试**

- 桌面端设置入口仍存在
- 设置页 API 仍完整
- 登录/注册/官方模型/工作区/日志/原始配置按钮链路不丢失

**Step 2: 运行测试**

Run:

```bash
go test ./...
go test ./cmd/picoclaw-launcher/...
```

Expected: PASS

**Step 3: 运行态验证**

- 启动 `launcher-chat.exe`
- 不单独启动 `pinchbot-launcher.exe`
- 打开 `http://127.0.0.1:18800`
- 验证关键请求 200：
  - `/`
  - `/api/config`
  - `/api/auth/status`
  - `/api/app/session`

**Step 4: Commit**

```bash
git add Launcher/app-wails/frontend_content_test.go PinchBot/cmd/picoclaw-launcher/ui_content_test.go
git commit -m "test: verify in-process settings service flow"
```

### Task 5: 规划 gateway 内嵌化第二阶段

**Files:**
- Create: `docs/plans/2026-03-18-gateway-embedding-followup.md`
- Reference: `PinchBot/cmd/picoclaw/internal/gateway`
- Reference: `PinchBot/cmd/picoclaw/main.go`
- Reference: `PinchBot/pkg/channels/chat_handler.go`

**Step 1: 分析当前 gateway 可嵌入边界**

- 找出 Cobra 命令层与真正服务启动逻辑边界
- 标出需要抽出的 package API：
  - Start
  - Stop
  - Health
  - Chat handler

**Step 2: 记录第二阶段设计**

- `launcher-chat.exe` 内直接启动 gateway service
- 去掉 `pinchbot.exe gateway` 子进程依赖
- 保留单独 CLI 入口，兼容开发/命令行场景

**Step 3: 定义第二阶段验收**

- 本地仅剩一个桌面主进程
- 聊天链路、设置链路、日志链路都不依赖外部子进程

**Step 4: Commit**

```bash
git add docs/plans/2026-03-18-gateway-embedding-followup.md
git commit -m "docs: plan gateway embedding follow-up"
```
