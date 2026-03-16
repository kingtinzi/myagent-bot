# 认证链路错误文案与防御收口 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 统一桌面端、设置页、本地认证桥、平台端的注册/登录错误文案与认证防御，避免透传英文协议错误、底层网络错误和坏 session。

**Architecture:** 以平台端作为用户可见认证错误的主来源，补充统一中文化与脱敏；本地 bridge 与桌面/设置页不再直接显示原始 transport/protocol error。用户登录/注册成功链路增加 access token 非空校验，避免保存损坏会话。

**Tech Stack:** Go (`Platform`, `Launcher`, `PinchBot`), 原生 HTML/JS 前端，Go 测试。

---

### Task 1: 平台端先写失败测试并补认证错误本地化

**Files:**
- Modify: `Platform/internal/api/server_test.go`
- Modify: `Platform/internal/api/server.go`

**Step 1: Write the failing test**
- 为登录/注册/管理员登录补测试，断言：
  - `invalid json` 有中文文案
  - `auth bridge not configured` / `authentication service unavailable` 有中文脱敏文案
  - 协议同步 warning 为中文
  - 用户登录/注册在 access token 为空时返回明确失败

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run 'Test(ServerAuth|AdminSession)' -count=1`

**Step 3: Write minimal implementation**
- 扩展 `localizeUserFacingErrorMessage(...)`
- 在用户登录/注册成功链路增加 access token 非空校验

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api -run 'Test(ServerAuth|AdminSession)' -count=1`

### Task 2: 设置页本地 bridge 先写失败测试再收口错误出口

**Files:**
- Modify: `PinchBot/cmd/picoclaw-launcher/internal/server/app_platform_test.go`
- Modify: `PinchBot/cmd/picoclaw-launcher/internal/server/app_platform.go`

**Step 1: Write the failing test**
- 断言本地 bridge：
  - 登录错误密码不会返回 `platform api returned 401: ...`
  - 本地未登录提示中文化
  - 协议同步 warning 中文化
  - 用户登录/注册 access token 为空时拒绝落盘

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/picoclaw-launcher/internal/server -count=1`

**Step 3: Write minimal implementation**
- 新增本地 bridge 认证错误归一化/中文化函数
- `writePlatformAPIError(...)` 不再直接透传 `err.Error()`
- 用户登录/注册成功链路增加 access token 非空校验

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/picoclaw-launcher/internal/server -count=1`

### Task 3: 桌面端先写失败测试再收口前端错误展示

**Files:**
- Modify: `Launcher/app_auth_test.go`
- Modify: `Launcher/app-wails/frontend_content_test.go`
- Modify: `Launcher/app.go`
- Modify: `Launcher/app-wails/frontend/index.html`

**Step 1: Write the failing test**
- 断言：
  - `APIError.Error()` 风格错误不会再以 `platform api returned ...` 的形式出现在用户文案
  - `GetAuthState()` 的错误提示被中文化/脱敏
  - 前端登录/注册失败不直接展示 transport/protocol 包装前缀

**Step 2: Run test to verify it fails**

Run: `go test . -count=1`

**Step 3: Write minimal implementation**
- 桌面端统一认证错误归一化
- 前端提交错误展示走统一用户文案

**Step 4: Run test to verify it passes**

Run: `go test . -count=1`

### Task 4: 全量回归

**Files:**
- Modify: `Platform/internal/api/server.go`
- Modify: `Platform/internal/api/server_test.go`
- Modify: `PinchBot/cmd/picoclaw-launcher/internal/server/app_platform.go`
- Modify: `PinchBot/cmd/picoclaw-launcher/internal/server/app_platform_test.go`
- Modify: `Launcher/app.go`
- Modify: `Launcher/app_auth_test.go`
- Modify: `Launcher/app-wails/frontend/index.html`
- Modify: `Launcher/app-wails/frontend_content_test.go`

**Step 1: Run focused suites**

Run:
- `go test ./internal/api -count=1`
- `go test ./cmd/picoclaw-launcher/internal/server -count=1`
- `go test . -count=1`

**Step 2: Run broader regression**

Run:
- `go test ./pkg/channels ./pkg/providers ./cmd/picoclaw/internal/gateway ./cmd/picoclaw-launcher -count=1`
- `go test ./internal/api ./internal/runtimeconfig ./internal/service -count=1`

**Step 3: Diff hygiene**

Run: `git diff --check`
