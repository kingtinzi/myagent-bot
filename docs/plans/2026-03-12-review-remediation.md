# Review Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close the review findings that currently block safe release of auth, billing, official-model, and desktop packaging flows.

**Architecture:** Stabilize the platform in layers: first make packaged startup deterministic, then repair the official-model user flow, then harden auth/session/security boundaries, and finally finish lower-risk UX/admin/distribution issues. Keep changes boring, test-first, and scoped to one risk area at a time so regressions stay attributable.

**Tech Stack:** Go, Wails desktop app, plain HTML/JS launcher UI, PowerShell/Bash release scripts, Supabase auth bridge, EasyPay callback flow.

---

### Task 1: Make packaged startup deterministic

**Priority:** P0

**Files:**
- Modify: `Platform/cmd/platform-server/main.go`
- Modify: `Platform/internal/config/envfile.go`
- Test: `Platform/internal/config/envfile_test.go`
- Modify: `scripts/build-release.ps1`
- Modify: `scripts/build-release.sh`
- Modify: `docs/build-and-release.md`

**Step 1: Write the failing tests**

Add tests that prove:
- release startup does not auto-load `platform.example.env` as live configuration
- env loading still honors `config/platform.env` when it exists
- release scripts do not copy example files into active runtime file paths

**Step 2: Run the targeted tests and verify failure**

Run: `go test ./internal/config ./cmd/platform-server`

Expected: failure or missing coverage for “example config is treated as active config”.

**Step 3: Implement the minimal fix**

- Split “load live env” from “load example env for docs/dev only”
- Make `platform-server` read only explicit live config by default
- Keep example files in release output only as examples, never as `runtime-config.json`
- Update release docs to explain first-run config expectations

**Step 4: Add release smoke verification**

Run:
- `go test ./internal/config ./cmd/platform-server`
- `powershell -ExecutionPolicy Bypass -File scripts/build-release.ps1 -Version smoke-config`
- `bash scripts/build-release.sh smoke-config`

Expected:
- tests pass
- release directories contain examples only as examples
- no active file is silently seeded from an example

**Step 5: Commit**

```bash
git add Platform/cmd/platform-server/main.go Platform/internal/config/envfile.go Platform/internal/config/envfile_test.go scripts/build-release.ps1 scripts/build-release.sh docs/build-and-release.md
git commit -m "fix: make release config startup explicit"
```

### Task 2: Repair official-model sync so synced models are immediately usable

**Priority:** P0

**Files:**
- Modify: `PicoClaw/cmd/picoclaw-launcher/internal/server/app_platform.go`
- Modify: `PicoClaw/cmd/picoclaw-launcher/internal/ui/index.html`
- Test: `PicoClaw/cmd/picoclaw-launcher/internal/server/app_platform_test.go`
- Test: `PicoClaw/cmd/picoclaw-launcher/internal/server/server_test.go`

**Step 1: Write the failing tests**

Add tests that prove:
- syncing official models creates entries that the UI will consider selectable
- default model switching is explicit and stable when synced models are removed
- sync preserves custom aliases only when behavior is intentional

**Step 2: Run the targeted tests and verify failure**

Run: `go test ./cmd/picoclaw-launcher/internal/server`

Expected: failure or missing assertions around model availability after sync.

**Step 3: Implement the minimal fix**

- Align the server-side synced model shape with the UI availability contract
- Or update the UI availability logic so `official/...` models with valid `api_base` are treated as available
- Add a visible distinction between official managed models and custom API-key models
- Prevent silent default-model mutation without explicit user feedback

**Step 4: Verify the launcher flow**

Run:
- `go test ./cmd/picoclaw-launcher/internal/server`
- `go build ./cmd/picoclaw-launcher`

Manual smoke:
- sync official models
- confirm a synced model can be set primary
- confirm the UI does not mark it unavailable

**Step 5: Commit**

```bash
git add PicoClaw/cmd/picoclaw-launcher/internal/server/app_platform.go PicoClaw/cmd/picoclaw-launcher/internal/server/app_platform_test.go PicoClaw/cmd/picoclaw-launcher/internal/server/server_test.go PicoClaw/cmd/picoclaw-launcher/internal/ui/index.html
git commit -m "fix: make synced official models selectable"
```

### Task 3: Unify session expiry handling and isolate chat history per user

**Priority:** P0

**Files:**
- Modify: `Launcher/app-wails/app.go`
- Test: `Launcher/app-wails/app_auth_test.go`
- Modify: `Launcher/app-wails/frontend/index.html`

**Step 1: Write the failing tests**

Add tests that prove:
- chat requests clear stored session and surface an auth-required state on 401
- auth-state refresh and chat path behave consistently on expired tokens

**Step 2: Run the targeted tests and verify failure**

Run: `go test ./Launcher/app-wails/...`

Expected: failure or missing assertions around chat-path unauthorized handling.

**Step 3: Implement the minimal backend fix**

- Normalize 401 handling in `Chat()` to match `GetAuthState()`
- Return a structured auth-expired signal that the frontend can act on

**Step 4: Implement the minimal frontend fix**

- Key local history by user identity instead of one global key
- Do not render prior history until auth/session resolution is known
- Clear or switch history correctly on logout/login transitions
- Show a persistent “session expired, please sign in again” message when chat hits 401

**Step 5: Verify**

Run:
- `go test ./Launcher/app-wails/...`
- `go build -tags "desktop,production" ./Launcher/app-wails`

Manual smoke:
- login as user A, create history, logout
- login as user B, confirm user A history is not shown
- expire session, send a message, confirm forced re-login

**Step 6: Commit**

```bash
git add Launcher/app-wails/app.go Launcher/app-wails/app_auth_test.go Launcher/app-wails/frontend/index.html
git commit -m "fix: unify auth expiry and isolate launcher history"
```

### Task 4: Lock down default network exposure

**Priority:** P1

**Files:**
- Modify: `Platform/internal/config/config.go`
- Modify: `Platform/cmd/platform-server/main.go`
- Modify: `PicoClaw/cmd/picoclaw-launcher/main.go`
- Modify: `PicoClaw/cmd/picoclaw-launcher/internal/server/server.go`
- Test: `Platform/internal/api/server_test.go`
- Test: `PicoClaw/cmd/picoclaw-launcher/internal/server/server_test.go`
- Modify: `PicoClaw/cmd/picoclaw-launcher/README.md`
- Modify: `PicoClaw/cmd/picoclaw-launcher/README.zh.md`

**Step 1: Write the failing tests**

Add tests that prove:
- default `platform-server` bind is loopback-only
- “public” launcher mode requires an explicit secondary auth mechanism or refuses dangerous routes

**Step 2: Run the targeted tests and verify failure**

Run:
- `go test ./Platform/internal/api`
- `go test ./PicoClaw/cmd/picoclaw-launcher/internal/server`

**Step 3: Implement the minimal fix**

- Change default bind to `127.0.0.1`
- Gate `-public` with shared-secret auth or carve dangerous endpoints out of public mode
- Update docs so “public” is clearly documented as advanced/admin-only

**Step 4: Verify**

Run:
- `go test ./Platform/internal/api`
- `go test ./PicoClaw/cmd/picoclaw-launcher/internal/server`
- `go build ./Platform/cmd/platform-server`
- `go build ./PicoClaw/cmd/picoclaw-launcher`

**Step 5: Commit**

```bash
git add Platform/internal/config/config.go Platform/cmd/platform-server/main.go Platform/internal/api/server_test.go PicoClaw/cmd/picoclaw-launcher/main.go PicoClaw/cmd/picoclaw-launcher/internal/server/server.go PicoClaw/cmd/picoclaw-launcher/internal/server/server_test.go PicoClaw/cmd/picoclaw-launcher/README.md PicoClaw/cmd/picoclaw-launcher/README.zh.md
git commit -m "fix: reduce default network exposure"
```

### Task 5: Make consent and recharge UX legally and functionally explicit

**Priority:** P1

**Files:**
- Modify: `PicoClaw/pkg/platformapi/types.go`
- Modify: `Platform/internal/service/service.go`
- Modify: `Platform/internal/api/server.go`
- Test: `Platform/internal/api/server_test.go`
- Modify: `Platform/internal/api/admin_index.html`
- Modify: `PicoClaw/cmd/picoclaw-launcher/internal/ui/index.html`

**Step 1: Write the failing tests**

Add tests that prove:
- agreement documents can carry readable body text or URLs, not just title/version
- recharge order creation rejects acceptance payloads that do not match current readable agreements

**Step 2: Run the targeted tests and verify failure**

Run: `go test ./Platform/internal/api ./Platform/internal/service`

**Step 3: Implement the minimal fix**

- Extend agreement DTOs with readable content fields (`content`, `url`, or equivalent)
- Update admin UI to manage those fields explicitly
- Update settings/recharge UI to show readable materials before acceptance
- Separate “I have read” from “create order” so acceptance is visible and auditable

**Step 4: Verify**

Run:
- `go test ./Platform/internal/api ./Platform/internal/service`
- `go build ./cmd/picoclaw-launcher`

Manual smoke:
- open recharge panel
- confirm agreement text/link is visible before payment
- confirm order creation still works only after acceptance

**Step 5: Commit**

```bash
git add PicoClaw/pkg/platformapi/types.go Platform/internal/service/service.go Platform/internal/api/server.go Platform/internal/api/server_test.go Platform/internal/api/admin_index.html PicoClaw/cmd/picoclaw-launcher/internal/ui/index.html
git commit -m "feat: require readable recharge consent materials"
```

### Task 6: Reduce token exposure and harden payment callback validation

**Priority:** P2

**Files:**
- Modify: `PicoClaw/cmd/picoclaw-launcher/internal/server/app_platform.go`
- Modify: `PicoClaw/pkg/platformapi/session_store.go`
- Modify: `PicoClaw/pkg/platformapi/types.go`
- Modify: `Platform/internal/payments/easypay.go`
- Modify: `Platform/internal/service/service.go`
- Test: `Platform/internal/payments/easypay_test.go`
- Test: `Platform/internal/api/server_test.go`

**Step 1: Write the failing tests**

Add tests that prove:
- browser-facing session responses exclude access and refresh tokens
- EasyPay callback rejects mismatched amount and invalid callback payloads

**Step 2: Run the targeted tests and verify failure**

Run:
- `go test ./Platform/internal/payments ./Platform/internal/api`
- `go test ./PicoClaw/pkg/platformapi ./PicoClaw/cmd/picoclaw-launcher/internal/server`

**Step 3: Implement the minimal fix**

- Split browser-safe session DTO from persisted internal session
- Stop returning raw tokens to the browser
- If feasible on current platform, prepare secure-at-rest storage abstraction for later OS keychain integration
- Validate callback amount/currency/status before crediting wallet

**Step 4: Verify**

Run:
- `go test ./Platform/internal/payments ./Platform/internal/api`
- `go test ./PicoClaw/cmd/picoclaw-launcher/internal/server`

**Step 5: Commit**

```bash
git add PicoClaw/cmd/picoclaw-launcher/internal/server/app_platform.go PicoClaw/pkg/platformapi/session_store.go PicoClaw/pkg/platformapi/types.go Platform/internal/payments/easypay.go Platform/internal/service/service.go Platform/internal/payments/easypay_test.go Platform/internal/api/server_test.go
git commit -m "fix: minimize token exposure and validate payment amounts"
```

### Task 7: Preserve upstream auth and official-provider errors

**Priority:** P2

**Files:**
- Modify: `Platform/internal/authbridge/supabase.go`
- Modify: `Platform/internal/api/server.go`
- Test: `Platform/internal/api/server_test.go`
- Modify: `PicoClaw/pkg/providers/official_provider.go`
- Test: `PicoClaw/pkg/providers/official_provider_test.go`

**Step 1: Write the failing tests**

Add tests that prove:
- auth 4xx errors are surfaced as user-correctable responses, not generic 502s
- official provider preserves useful upstream response text/body for diagnostics

**Step 2: Run the targeted tests and verify failure**

Run:
- `go test ./Platform/internal/api`
- `go test ./PicoClaw/pkg/providers`

**Step 3: Implement the minimal fix**

- Preserve upstream auth error details in a safe, non-secret form
- Map user errors to 4xx and infrastructure errors to 5xx
- Include bounded upstream error payloads from official provider responses

**Step 4: Verify**

Run:
- `go test ./Platform/internal/api`
- `go test ./PicoClaw/pkg/providers`

**Step 5: Commit**

```bash
git add Platform/internal/authbridge/supabase.go Platform/internal/api/server.go Platform/internal/api/server_test.go PicoClaw/pkg/providers/official_provider.go PicoClaw/pkg/providers/official_provider_test.go
git commit -m "fix: preserve actionable auth and provider errors"
```

### Task 8: Keep official-model billing token through retry and summarize paths

**Priority:** P2

**Files:**
- Modify: `PicoClaw/pkg/agent/loop.go`
- Modify: `PicoClaw/pkg/providers/official_provider.go`
- Test: `PicoClaw/pkg/agent/loop_platform_test.go`
- Test: `PicoClaw/pkg/providers/official_provider_test.go`

**Step 1: Write the failing tests**

Add tests that prove:
- official-model calls retain `user_access_token` through retry paths
- summarize/merge paths keep the same billing/auth context as the initial request

**Step 2: Run the targeted tests and verify failure**

Run:
- `go test ./PicoClaw/pkg/agent`
- `go test ./PicoClaw/pkg/providers`

**Step 3: Implement the minimal fix**

- Thread platform auth metadata through retry/summarize helper paths
- Keep one source of truth for provider call context so later paths cannot silently drop auth state

**Step 4: Verify**

Run:
- `go test ./PicoClaw/pkg/agent`
- `go test ./PicoClaw/pkg/providers`
- `go build ./cmd/picoclaw`

**Step 5: Commit**

```bash
git add PicoClaw/pkg/agent/loop.go PicoClaw/pkg/agent/loop_platform_test.go PicoClaw/pkg/providers/official_provider.go PicoClaw/pkg/providers/official_provider_test.go
git commit -m "fix: retain platform token through official retries"
```

### Task 9: Finish admin/runtime UX and desktop accessibility/distribution polish

**Priority:** P2

**Files:**
- Modify: `Platform/internal/api/admin_index.html`
- Modify: `PicoClaw/cmd/picoclaw-launcher/internal/ui/index.html`
- Modify: `Launcher/app-wails/frontend/index.html`
- Modify: `scripts/build-release.sh`
- Modify: `docs/build-and-release.md`

**Step 1: Write the failing checks**

Create a checklist-based verification file or UI test notes that cover:
- dialog semantics
- `aria-live` status regions
- keyboard reachability for custom controls
- mac packaging output expectations
- admin runtime config validation and preview behavior

**Step 2: Implement the minimal fix**

- Replace clickable non-buttons where practical
- Add labels, live regions, dialog roles, and focus management
- Add structured validation/previews to admin runtime config editor
- Upgrade mac packaging from raw binary folder toward `.app` bundle layout

**Step 3: Verify**

Run:
- `go build -tags "desktop,production" ./Launcher/app-wails`
- `go build ./cmd/picoclaw-launcher`
- `bash scripts/build-release.sh smoke-mac`

Manual smoke:
- keyboard-only navigation across login, settings, recharge, modal
- screen-reader friendly status messaging spot-check
- confirm mac output is closer to distributable bundle format

**Step 4: Commit**

```bash
git add Platform/internal/api/admin_index.html PicoClaw/cmd/picoclaw-launcher/internal/ui/index.html Launcher/app-wails/frontend/index.html scripts/build-release.sh docs/build-and-release.md
git commit -m "fix: polish admin and desktop accessibility flows"
```

### Final verification batch

**Run after all tasks complete:**

```bash
go test ./Platform/...
go build ./Platform/cmd/platform-server
go test ./PicoClaw/cmd/picoclaw-launcher/internal/server ./PicoClaw/pkg/providers ./PicoClaw/pkg/agent
go build ./PicoClaw/cmd/picoclaw ./PicoClaw/cmd/picoclaw-launcher
go test ./Launcher/app-wails/...
go build -tags "desktop,production" ./Launcher/app-wails
powershell -ExecutionPolicy Bypass -File scripts/build-release.ps1 -Version final-review-remediation
bash scripts/build-release.sh final-review-remediation
```

**Release checklist:**
- packaged startup no longer depends on example config
- synced official models are selectable and bill correctly
- expired sessions force re-login consistently
- chat history is user-isolated
- payment consent is readable before payment
- loopback is the default exposure boundary
- tokens are not sent to browser clients
- EasyPay callback validates amount

