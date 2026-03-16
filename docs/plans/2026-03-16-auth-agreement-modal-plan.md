# 注册协议弹窗预览 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将桌面端和设置页的注册协议从“正文直接展示”改为“点击《用户协议》《隐私政策》名称后弹窗预览”。

**Architecture:** 保留现有协议加载与勾选校验逻辑，仅重构前端展示层。桌面端新增轻量协议弹窗；设置页复用现有通用 modal 体系承载协议内容，避免重复实现两套弹窗基础设施。

**Tech Stack:** Wails 桌面前端 HTML/CSS/原生 JS、PinchBot launcher 内嵌 HTML/CSS/原生 JS、Go 静态内容测试。

---

### Task 1: 先写桌面端失败测试

**Files:**
- Modify: `Launcher/app-wails/frontend_content_test.go`
- Test: `Launcher/app-wails/frontend_content_test.go`

**Step 1: Write the failing test**

- 断言桌面端存在协议弹窗容器、可点击协议按钮容器。
- 断言 `renderSignupAgreements()` 不再把 `doc.content` 直接渲染到注册界面列表，而是生成打开弹窗的按钮。

**Step 2: Run test to verify it fails**

Run: `go test . -count=1`

**Step 3: Write minimal implementation**

- 在 `Launcher/app-wails/frontend/index.html` 中增加协议弹窗 DOM、样式与打开/关闭逻辑。
- 将注册协议列表改为“按钮 + 版本信息”的摘要形式。

**Step 4: Run test to verify it passes**

Run: `go test . -count=1`

### Task 2: 先写设置页失败测试

**Files:**
- Modify: `PinchBot/cmd/picoclaw-launcher/ui_content_test.go`
- Test: `PinchBot/cmd/picoclaw-launcher/ui_content_test.go`

**Step 1: Write the failing test**

- 断言设置页注册协议改为“点击标题弹窗查看”。
- 断言加载注册协议后不再把正文直接塞进注册面板。

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/picoclaw-launcher -count=1`

**Step 3: Write minimal implementation**

- 复用现有 `modelModal`，增加协议预览模式。
- `loadAppAuthAgreements()` 改为输出协议按钮与版本信息。

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/picoclaw-launcher -count=1`

### Task 3: 全量回归

**Files:**
- Modify: `Launcher/app-wails/frontend/index.html`
- Modify: `Launcher/app-wails/frontend_content_test.go`
- Modify: `PinchBot/cmd/picoclaw-launcher/internal/ui/index.html`
- Modify: `PinchBot/cmd/picoclaw-launcher/ui_content_test.go`

**Step 1: Run focused tests**

Run:
- `go test . -count=1`
- `go test ./cmd/picoclaw-launcher -count=1`

**Step 2: Run broader regression**

Run:
- `go test ./internal/api ./internal/runtimeconfig ./internal/service -count=1`
- `go test ./pkg/channels ./pkg/providers ./cmd/picoclaw/internal/gateway -count=1`

**Step 3: Diff hygiene**

Run: `git diff --check`
