# Platform Auth Login/Signup Design

**Date:** 2026-03-13

**Goal:** 让桌面端与配置页的平台注册/登录在当前 Supabase 接入下稳定可用，并在注册无法立即建立会话时给出明确、可执行的提示。

## Current Context

- 桌面端 `Launcher/app-wails` 在启动后会强制弹出注册/登录弹窗。
- 设置页 `PinchBot/cmd/picoclaw-launcher/internal/server/app_platform.go` 也复用了同一条平台注册/登录链路。
- 平台服务 `Platform/internal/api/server.go` 将 `/auth/login` 与 `/auth/signup` 转发到 `Platform/internal/authbridge/supabase.go`。
- 现状中，`signup` 会把 Supabase 返回内容直接映射到 `platformapi.Session`，但没有校验 `access_token` 是否存在。

## Problem

Supabase 在开启 email confirmation 时，`signup` 可能只返回 `user` 而不返回 `session/access_token`。当前代码会：

1. 保存一个不完整 session；
2. 后续 `GetAuthState()` / session store 读取失败；
3. 前端表现为“注册看似成功，但仍未登录”。

同时，bridge 当前对 key/header 的使用方式也过于死板，错误提示不够明确，不利于快速定位 Supabase 配置问题。

## Chosen Approach

### 1. 明确建模 Supabase auth 响应

给 auth bridge 增加受控响应解析：

- 允许 `login/signup` 正常解析会话；
- 若 `signup` 只有 user 没有 session，则不再返回“空 session 成功”；
- 返回明确错误，说明“当前项目未返回登录会话，需要关闭邮箱确认或允许未验证邮箱直接登录”。

### 2. 对 signup 增加一次登录回退

当 `signup` 返回 user 但没有 session 时：

- 立即调用一次 `login(password grant)`；
- 若登录成功，则完成自动登录；
- 若仍失败，则把最有操作性的错误返回给前端。

这样能覆盖：

- 已关闭邮箱确认的项目；
- 已允许未验证邮箱直接登录的项目；
- 避免“注册成功但仍然是未登录”的假成功状态。

### 3. 强化错误提示与测试

- 平台层测试：覆盖 signup 无 session、signup 后 login 回退成功、signup 后 login 回退失败。
- 桌面端/设置页测试：确保不会把不完整 session 写入本地，也不会把“假成功”展示给用户。

## Non-Goals

- 这轮不接入邮箱验证流程页；
- 不修改充值/钱包业务逻辑；
- 不把 Supabase PAT 或管理 API 带入应用运行时。

## Validation

- `Platform/internal/authbridge` 单测覆盖新增分支；
- `Platform/internal/api` 单测验证接口返回；
- `Launcher/app-wails` 与 `PinchBot/.../app_platform` 单测确认 session 行为正确。
