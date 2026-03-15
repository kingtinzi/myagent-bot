# Platform Auth Login/Signup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make desktop and launcher signup/login reliably establish a valid platform session with Supabase, or fail with a clear actionable error instead of silently storing an incomplete session.

**Architecture:** Keep the existing Platform → Supabase bridge boundary, but tighten response parsing and make signup fall back to an immediate password login when Supabase does not return a session directly. Preserve all current frontend APIs and fix behavior through server-side validation plus regression tests.

**Tech Stack:** Go, net/http, httptest, Wails desktop app, PinchBot platform client, Supabase Auth REST API.

---

### Task 1: Lock down the expected Supabase auth behavior with failing tests

**Files:**
- Modify: `Platform/internal/authbridge/supabase_test.go`
- Modify: `Platform/internal/api/server_test.go`
- Modify: `Launcher/app-wails/app_auth_test.go`
- Modify: `PinchBot/cmd/picoclaw-launcher/internal/server/app_platform_test.go`

**Step 1: Write the failing tests**

- Add authbridge tests for:
  - signup returns direct session
  - signup returns only user, then login fallback succeeds
  - signup returns only user, then login fallback fails with actionable error
- Add integration-facing tests to confirm callers do not treat incomplete signup data as authenticated success.

**Step 2: Run tests to verify they fail**

Run:

```bash
bash -lc 'cd /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/Platform && ./.tools/linux-go/go/bin/go test -count=1 ./internal/authbridge ./internal/api'
bash -lc 'cd /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/Launcher/app-wails && /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/.tools/linux-go/go/bin/go test -count=1 ./...'
bash -lc 'cd /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/PinchBot && /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/.tools/linux-go/go/bin/go test -count=1 ./cmd/picoclaw-launcher/internal/server ./pkg/platformapi'
```

Expected: new tests fail because signup currently accepts an incomplete session.

### Task 2: Fix Supabase auth bridge semantics

**Files:**
- Modify: `Platform/internal/authbridge/supabase.go`
- Test: `Platform/internal/authbridge/supabase_test.go`

**Step 1: Implement minimal production code**

- Introduce a small internal response model for Supabase auth responses.
- Validate that a “successful” auth result must contain:
  - non-empty `access_token`
  - non-empty `user.id`
- For `signup`:
  - first call `/auth/v1/signup`
  - if response already contains session, return it
  - else immediately call password login once
  - if fallback also cannot produce a valid session, return a clear API error

**Step 2: Run targeted tests**

Run:

```bash
bash -lc 'cd /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/Platform && ./.tools/linux-go/go/bin/go test -count=1 ./internal/authbridge'
```

Expected: authbridge tests pass.

### Task 3: Verify upstream callers surface the fixed behavior cleanly

**Files:**
- Modify: `Platform/internal/api/server_test.go`
- Modify: `Launcher/app-wails/app_auth_test.go`
- Modify: `PinchBot/cmd/picoclaw-launcher/internal/server/app_platform_test.go`

**Step 1: Keep caller behavior strict**

- Confirm platform API preserves actionable signup/login errors.
- Confirm desktop and launcher flows only persist real sessions.
- Confirm no token leakage in public auth/session responses.

**Step 2: Run relevant test suites**

Run:

```bash
bash -lc 'cd /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/Platform && ./.tools/linux-go/go/bin/go test -count=1 ./internal/api'
bash -lc 'cd /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/Launcher/app-wails && /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/.tools/linux-go/go/bin/go test -count=1 ./...'
bash -lc 'cd /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/PinchBot && /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/.tools/linux-go/go/bin/go test -count=1 ./cmd/picoclaw-launcher/internal/server ./pkg/platformapi'
```

Expected: all relevant suites pass.

### Task 4: Update operator-facing docs for the one required Supabase toggle

**Files:**
- Modify: `Platform/README.md`

**Step 1: Document the runtime expectation**

- State that if immediate signup-login is desired, the Supabase project must not require email confirmation for this flow, or must allow unverified email sign-ins.
- Keep wording aligned with the new actionable runtime error.

**Step 2: Run documentation sanity checks**

Run any existing release/docs tests that cover README assumptions.

### Task 5: Final verification

**Files:**
- No new files

**Step 1: Run the combined verification set**

```bash
bash -lc 'cd /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/Platform && ./.tools/linux-go/go/bin/go test -count=1 ./internal/authbridge ./internal/api'
bash -lc 'cd /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/Launcher/app-wails && /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/.tools/linux-go/go/bin/go test -count=1 ./...'
bash -lc 'cd /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/PinchBot && /mnt/c/Users/sky/.config/superpowers/worktrees/myagent-bot/integration-platform-auth-billing/.tools/linux-go/go/bin/go test -count=1 ./cmd/picoclaw-launcher/internal/server ./pkg/platformapi'
```

Expected: all tests green, no incomplete-session regressions.
