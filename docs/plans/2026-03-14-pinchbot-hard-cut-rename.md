# PinchBot Hard-Cut Rename Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove all remaining OpenClaw runtime/development compatibility and hard-cut the repository to PinchBot-only naming.

**Architecture:** Execute the rename in four waves so runtime boundaries are updated before module-path refactors and migration deletion. Each wave must include targeted tests before the next wave begins, and no legacy compatibility layer should remain in running code after the final sweep.

**Tech Stack:** Go, Wails, PowerShell scripts, HTML admin UI, repository documentation, ripgrep, Docker-based Go test runners.

---

### Task 1: Hard-cut runtime boundary identifiers

**Files:**
- Modify: `Launcher/app-wails/update.go`
- Modify: `Launcher/app-wails/update_test.go`
- Modify: `Platform/internal/api/server.go`
- Modify: `Platform/internal/api/server_test.go`
- Modify: `docs/auto-update.md`

**Step 1: Write/extend failing tests for runtime identifiers**

Add or extend tests to assert:

```go
if got := getManifestURL(); got != "https://pinchbot.example.com/manifest.json" { ... }
if got := adminSessionCookieName; got != "pinchbot_admin_session" { ... }
```

**Step 2: Run the focused tests and verify they fail for the old identifiers**

Run:

```bash
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/Launcher/app-wails golang:1.25-bookworm go test update.go update_test.go
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/Platform/internal/api golang:1.25-bookworm go test ./...
```

Expected: failures referencing `OPENCLAW_*`, old pending dir fallback, or `openclaw_admin_session`.

**Step 3: Remove old env and pending-dir compatibility**

Update `Launcher/app-wails/update.go` so it only uses:

- `PINCHBOT_UPDATE_MANIFEST_URL`
- `PINCHBOT_PENDING_DIR`
- `%LOCALAPPDATA%/PinchBot/pending`

Delete legacy fallback constants and helper branches.

**Step 4: Rename the admin session cookie**

Change:

```go
const adminSessionCookieName = "pinchbot_admin_session"
```

and update related tests.

**Step 5: Update docs to describe PinchBot-only runtime behavior**

In `docs/auto-update.md`, remove language that still mentions old env compatibility.

**Step 6: Re-run the focused tests and verify they pass**

Run the same commands from Step 2.

Expected: PASS.

**Step 7: Commit**

```bash
git add Launcher/app-wails/update.go Launcher/app-wails/update_test.go Platform/internal/api/server.go Platform/internal/api/server_test.go docs/auto-update.md
git commit -m "refactor: hard-cut runtime naming to pinchbot"
```

### Task 2: Rename Go modules and import paths

**Files:**
- Modify: `Platform/go.mod`
- Modify: `Launcher/app-wails/go.mod`
- Modify: `Platform/cmd/platform-server/main.go`
- Modify: `Platform/cmd/platform-server/main_test.go`
- Modify: `Platform/internal/authverifier/jwks.go`
- Modify: `Platform/internal/store/pg/store.go`
- Modify: `Platform/internal/store/pg/store_filters_test.go`
- Modify: `Platform/internal/service/service.go`
- Modify: `Platform/internal/service/service_test.go`
- Modify: `Platform/internal/service/governance.go`
- Modify: `Platform/internal/service/memory_store.go`
- Modify: `Platform/internal/runtimeconfig/manager.go`
- Modify: `Platform/internal/runtimeconfig/manager_test.go`
- Modify: `Platform/internal/runtimeconfig/revision.go`
- Modify: `Platform/internal/api/server.go`
- Modify: `Platform/internal/api/server_test.go`
- Modify: `Platform/internal/api/payment_notify_test.go`

**Step 1: Write a failing module-path test or scan assertion**

Add/extend a test or scripted assertion so key files must contain:

```text
pinchbot/platform
pinchbot/launcher/app-wails
```

**Step 2: Run the targeted package tests and confirm the old imports remain**

Run:

```bash
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/Platform/cmd/platform-server golang:1.25-bookworm go test .
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/Platform/internal/payments golang:1.25-bookworm go test ./...
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/Platform/internal/api golang:1.25-bookworm go test ./...
```

Expected: build failures until imports/modules are aligned.

**Step 3: Update both `go.mod` module declarations**

Set:

```go
module pinchbot/platform
module pinchbot/launcher/app-wails
```

**Step 4: Rewrite all `openclaw/...` imports under Platform/Launcher**

Update every affected import to `pinchbot/...` and run `gofmt` on touched Go files.

**Step 5: Re-run package tests**

Use the commands from Step 2 plus:

```bash
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/Launcher/app-wails golang:1.25-bookworm go test update.go update_test.go
```

Expected: PASS.

**Step 6: Commit**

```bash
git add Platform/go.mod Launcher/app-wails/go.mod Platform/cmd/platform-server/main.go Platform/cmd/platform-server/main_test.go Platform/internal/authverifier/jwks.go Platform/internal/store/pg/store.go Platform/internal/store/pg/store_filters_test.go Platform/internal/service/service.go Platform/internal/service/service_test.go Platform/internal/service/governance.go Platform/internal/service/memory_store.go Platform/internal/runtimeconfig/manager.go Platform/internal/runtimeconfig/manager_test.go Platform/internal/runtimeconfig/revision.go Platform/internal/api/server.go Platform/internal/api/server_test.go Platform/internal/api/payment_notify_test.go
git commit -m "refactor: rename platform modules to pinchbot"
```

### Task 3: Delete OpenClaw migration support

**Files:**
- Modify: `PinchBot/pkg/migrate/migrate.go`
- Modify: `PinchBot/pkg/migrate/migrate_test.go`
- Modify: `PinchBot/cmd/picoclaw/internal/migrate/command.go`
- Modify: `PinchBot/cmd/picoclaw/internal/migrate/command_test.go`
- Delete: `PinchBot/pkg/migrate/sources/openclaw/common.go`
- Delete: `PinchBot/pkg/migrate/sources/openclaw/openclaw_config.go`
- Delete: `PinchBot/pkg/migrate/sources/openclaw/openclaw_config_test.go`
- Delete: `PinchBot/pkg/migrate/sources/openclaw/openclaw_handler.go`
- Delete: `PinchBot/pkg/migrate/sources/openclaw/openclaw_handler_test.go`

**Step 1: Write/adjust failing tests for the new migrate command contract**

Update tests so they assert:

```go
cmd.Short == "Migrate PinchBot data within supported PinchBot layouts"
```

and no `--from openclaw` examples remain.

**Step 2: Run the migrate-focused tests and confirm they fail with the old contract**

Run:

```bash
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/PinchBot golang:1.25-bookworm go test ./cmd/picoclaw/internal/migrate ./pkg/migrate/...
```

Expected: failures due to OpenClaw handler and command text still existing.

**Step 3: Remove the OpenClaw handler registration**

In `PinchBot/pkg/migrate/migrate.go`:

- remove `sources/openclaw` import
- remove handler registration
- remove default `source = "openclaw"` fallback
- simplify runtime so only supported PinchBot-native flows remain

**Step 4: Remove CLI flags and copy that reference OpenClaw**

In `PinchBot/cmd/picoclaw/internal/migrate/command.go`:

- remove `--from`
- remove `--source-home` semantics tied to `.openclaw`
- rewrite help text and examples to PinchBot-only wording

**Step 5: Delete the obsolete source directory**

Delete the five files under `PinchBot/pkg/migrate/sources/openclaw/`.

**Step 6: Re-run migrate-focused tests**

Run the command from Step 2.

Expected: PASS.

**Step 7: Commit**

```bash
git add PinchBot/pkg/migrate/migrate.go PinchBot/pkg/migrate/migrate_test.go PinchBot/cmd/picoclaw/internal/migrate/command.go PinchBot/cmd/picoclaw/internal/migrate/command_test.go
git add -u PinchBot/pkg/migrate/sources/openclaw
git commit -m "refactor: remove openclaw migration support"
```

### Task 4: Sweep residual terminology, comments, and docs

**Files:**
- Modify: `Platform/internal/api/admin_ui_test.go`
- Modify: `PinchBot/pkg/providers/error_classifier.go`
- Modify: `PinchBot/pkg/providers/cooldown.go`
- Modify: `docs/auto-update.md`
- Any additional files returned by `rg -n -i "openclaw|OPENCLAW"`

**Step 1: Run the residual scan**

Run:

```bash
rg -n -i "openclaw|OPENCLAW" Platform Launcher PinchBot docs
```

Expected: only the known files listed above.

**Step 2: Replace or remove the remaining references**

- Change comments/documentation to PinchBot wording where still operational.
- Remove test assertions that deliberately allow visible `OpenClaw`.
- Keep wording factual and current; do not leave “legacy fallback” text.

**Step 3: Re-run targeted tests**

Run:

```bash
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/Platform/internal/api golang:1.25-bookworm go test ./...
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/PinchBot golang:1.25-bookworm go test ./pkg/providers ./cmd/picoclaw/internal/migrate ./pkg/migrate/...
```

Expected: PASS.

**Step 4: Verify the scan is clean**

Run:

```bash
rg -n -i "openclaw|OPENCLAW" Platform Launcher PinchBot docs
```

Expected: no matches in runtime code; if any documentation-only historical note remains, explicitly justify it before proceeding.

**Step 5: Commit**

```bash
git add Platform/internal/api/admin_ui_test.go PinchBot/pkg/providers/error_classifier.go PinchBot/pkg/providers/cooldown.go docs/auto-update.md
git commit -m "chore: remove residual openclaw terminology"
```

### Task 5: Final verification and release-note update

**Files:**
- Modify: `docs/plans/2026-03-14-pinchbot-hard-cut-rename-design.md`
- Modify: `docs/plans/2026-03-14-pinchbot-hard-cut-rename.md`

**Step 1: Run the final verification matrix**

Run:

```bash
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/Platform/internal/api golang:1.25-bookworm go test ./...
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/Platform/internal/payments golang:1.25-bookworm go test ./...
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/Platform/cmd/platform-server golang:1.25-bookworm go test .
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/PinchBot golang:1.25-bookworm go test ./cmd/picoclaw/internal/migrate ./pkg/migrate/... ./pkg/providers/...
docker run --rm -v C:/Users/sky/.config/superpowers/worktrees/myagent-bot/platform-remaining-waves:/src -w /src/Launcher/app-wails golang:1.25-bookworm go test update.go update_test.go
rg -n -i "openclaw|OPENCLAW" Platform Launcher PinchBot docs
```

Expected:

- all `go test` commands PASS
- `rg` returns no runtime hits

**Step 2: Record final outcomes in the plan files**

Append a short verification summary to the design and implementation plan files.

**Step 3: Commit**

```bash
git add docs/plans/2026-03-14-pinchbot-hard-cut-rename-design.md docs/plans/2026-03-14-pinchbot-hard-cut-rename.md
git commit -m "docs: capture pinchbot hard-cut rename plan"
```
