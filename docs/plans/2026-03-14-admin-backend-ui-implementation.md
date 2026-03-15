# Admin Backend UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a production-ready admin backend UI for MyAgent Bot that covers dashboard statistics, user management, permission control, orders, wallet adjustments, pricing, routes, agreements, audits, refunds, and infringement handling.

**Architecture:** Keep the solution boring and layered. Extend the existing Go admin APIs only where current capabilities are insufficient for a complete operator workflow, then rebuild `Platform/internal/api/admin_index.html` into a responsive single-page admin console that consumes those APIs. Add RBAC checks in the Go API layer first, then expose permission-aware UI modules so operators only see actions they are allowed to use.

**Tech Stack:** Go `net/http`, existing Platform service/store layers, PostgreSQL + memory store test doubles, plain HTML/CSS/JS in `admin_index.html`, Go tests for API/service/store coverage, static admin UI content tests.

---

## Current State Summary

- Existing admin UI only supports:
  - Supabase login
  - runtime config editing
  - official model visibility preview
- Existing backend already has many admin APIs:
  - `/admin/users`
  - `/admin/orders`
  - `/admin/wallet-adjustments`
  - `/admin/model-routes`
  - `/admin/pricing-rules`
  - `/admin/agreement-versions`
  - `/admin/audit-logs`
  - `/admin/refund-requests`
  - `/admin/infringement-reports`
  - `/admin/system-notices`
  - `/admin/risk-rules`
- Missing for a production-grade admin console:
  - unified dashboard metrics API
  - explicit admin self/role/permission API
  - complete admin UI shell/navigation
  - permission-aware UI actions
  - user drill-down workflows
  - responsive QA and security acceptance artifacts

## Non-Negotiable Requirements

- No temporary patch UI. Build a coherent admin app shell.
- All privileged actions must remain server-authorized; UI hiding alone is insufficient.
- Responsive layouts must work on desktop/laptop/tablet widths.
- High-risk actions must have audit logging and confirmation UX.
- Verification before completion:
  - `go test ./...` in `Platform`
  - static admin UI tests
  - targeted manual acceptance checklist

---

## Task 1: Establish admin RBAC and operator context

**Files:**
- Create: `Platform/migrations/0003_admin_rbac.sql`
- Modify: `Platform/internal/service/governance.go`
- Modify: `Platform/internal/service/service.go`
- Modify: `Platform/internal/service/memory_store.go`
- Modify: `Platform/internal/store/pg/store.go`
- Modify: `Platform/internal/api/server.go`
- Modify: `Platform/internal/api/server_test.go`
- Test: `Platform/internal/service/service_test.go`

**Step 1: Write the failing tests**

Add tests that prove:
- admin users can fetch their own operator profile
- non-admins cannot access admin routes
- admin roles/capabilities are enforced for write actions
- role metadata is returned to the UI

Examples to add:
- `TestAdminMeReturnsRoleAndPermissions`
- `TestAdminWriteEndpointRejectsReadOnlyOperator`
- `TestAdminUsersEndpointStillAllowsAuthorizedAdmin`

**Step 2: Run the failing tests**

Run:

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api ./internal/service
```

Expected:
- new tests fail because admin role/capability APIs do not exist yet

**Step 3: Implement minimal RBAC foundation**

Implement:
- admin operator model:
  - `user_id`
  - `email`
  - `role`
  - `capabilities []string`
  - `active`
- server helpers:
  - `GET /admin/me`
  - capability checks for high-risk writes
- migration for persistent RBAC storage
- memory store parity so tests stay fast

Recommended capabilities:
- `dashboard.read`
- `users.read`
- `users.write`
- `orders.read`
- `wallet.write`
- `pricing.write`
- `routes.write`
- `agreements.write`
- `audit.read`
- `refunds.review`
- `infringement.review`

**Step 4: Run tests to verify pass**

Run:

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api ./internal/service ./internal/store/pg
```

Expected:
- RBAC tests pass

**Step 5: Commit**

```powershell
git add Platform/migrations/0003_admin_rbac.sql Platform/internal/service/governance.go Platform/internal/service/service.go Platform/internal/service/memory_store.go Platform/internal/store/pg/store.go Platform/internal/api/server.go Platform/internal/api/server_test.go Platform/internal/service/service_test.go
git commit -m "feat: add admin rbac foundation"
```

---

## Task 2: Add dashboard statistics API for the admin homepage

**Files:**
- Modify: `Platform/internal/service/governance.go`
- Modify: `Platform/internal/service/service.go`
- Modify: `Platform/internal/service/memory_store.go`
- Modify: `Platform/internal/store/pg/store.go`
- Modify: `Platform/internal/api/server.go`
- Modify: `Platform/internal/api/server_test.go`
- Test: `Platform/internal/store/pg/store_filters_test.go`
- Test: `Platform/internal/service/service_test.go`

**Step 1: Write the failing tests**

Add tests for:
- `GET /admin/dashboard`
- totals for users / paid orders / wallet balances
- recent revenue / refund / usage counts
- top official models by recent usage

Suggested payload shape:

```json
{
  "totals": {
    "users": 0,
    "paid_orders": 0,
    "wallet_balance_fen": 0,
    "refund_pending": 0
  },
  "recent": {
    "recharge_fen_7d": 0,
    "consumption_fen_7d": 0,
    "new_users_7d": 0
  },
  "top_models": []
}
```

**Step 2: Run the tests to verify fail**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api ./internal/service ./internal/store/pg
```

Expected:
- dashboard endpoint/tests fail

**Step 3: Implement service + store aggregations**

Implement:
- dashboard DTOs in service layer
- PG queries for:
  - user totals
  - paid order totals
  - recent recharge/consumption/refund windows
  - top model usage summaries
- `GET /admin/dashboard`

Keep queries simple and auditable; no premature analytics framework.

**Step 4: Run tests**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api ./internal/service ./internal/store/pg
```

Expected:
- dashboard tests pass

**Step 5: Commit**

```powershell
git add Platform/internal/service/governance.go Platform/internal/service/service.go Platform/internal/service/memory_store.go Platform/internal/store/pg/store.go Platform/internal/api/server.go Platform/internal/api/server_test.go Platform/internal/store/pg/store_filters_test.go Platform/internal/service/service_test.go
git commit -m "feat: add admin dashboard statistics api"
```

---

## Task 3: Complete user-management drill-down APIs

**Files:**
- Modify: `Platform/internal/api/server.go`
- Modify: `Platform/internal/api/server_test.go`
- Modify: `Platform/internal/service/governance.go`
- Modify: `Platform/internal/service/service.go`
- Modify: `Platform/internal/service/service_test.go`
- Modify: `Platform/internal/store/pg/store.go`
- Modify: `Platform/internal/service/memory_store.go`

**Step 1: Write the failing tests**

Add tests for:
- `GET /admin/users/{id}/overview`
- `GET /admin/users/{id}/wallet-transactions`
- `GET /admin/users/{id}/orders`
- `GET /admin/users/{id}/agreements`
- `GET /admin/users/{id}/usage`

**Step 2: Run tests to verify fail**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api ./internal/service
```

**Step 3: Implement the minimal drill-down layer**

Return a single user overview model that groups:
- profile
- wallet
- latest orders
- latest transactions
- accepted agreements
- recent usage summaries
- pending refund / infringement counts

Prefer composition over duplicating query logic.

**Step 4: Run tests**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api ./internal/service ./internal/store/pg
```

**Step 5: Commit**

```powershell
git add Platform/internal/api/server.go Platform/internal/api/server_test.go Platform/internal/service/governance.go Platform/internal/service/service.go Platform/internal/service/service_test.go Platform/internal/store/pg/store.go Platform/internal/service/memory_store.go
git commit -m "feat: add admin user detail apis"
```

---

## Task 4: Rebuild the admin UI shell and navigation

**Files:**
- Modify: `Platform/internal/api/admin_index.html`
- Modify: `Platform/internal/api/admin_ui_test.go`

**Step 1: Write failing UI content tests**

Add tests that require:
- sidebar/top-nav layout
- dashboard section
- users section
- permissions section
- data tables with filters
- responsive breakpoints and accessible status regions

Suggested test names:
- `TestAdminUIProvidesAppShellAndNavigation`
- `TestAdminUIExposesDashboardCardsAndTables`
- `TestAdminUISupportsResponsiveSections`

**Step 2: Run tests to verify fail**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api
```

**Step 3: Implement the app shell**

Rebuild `admin_index.html` into:
- top bar with admin identity + role badge
- left navigation
- responsive content area
- reusable cards/tables/forms/dialogs/toasts
- centralized auth token handling
- centralized fetch wrapper
- centralized permission gating helpers

Keep it in one file for now unless the file becomes unmaintainable.

**Step 4: Run tests**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api
```

**Step 5: Commit**

```powershell
git add Platform/internal/api/admin_index.html Platform/internal/api/admin_ui_test.go
git commit -m "feat: build admin ui shell and navigation"
```

---

## Task 5: Implement dashboard and user management screens

**Files:**
- Modify: `Platform/internal/api/admin_index.html`
- Modify: `Platform/internal/api/admin_ui_test.go`

**Step 1: Write failing UI tests**

Add tests that require:
- dashboard KPI cards
- trend / summary sections
- user list filters
- user detail drawer/panel
- wallet/order/agreement/usage blocks inside user detail

**Step 2: Run tests to verify fail**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api
```

**Step 3: Implement UI screens**

Implement:
- dashboard data loading from `/admin/dashboard`
- user table from `/admin/users`
- user detail workflow using `/admin/users/{id}/...`
- empty/loading/error states
- mobile/tablet-safe table overflow handling

**Step 4: Run tests**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api
```

**Step 5: Commit**

```powershell
git add Platform/internal/api/admin_index.html Platform/internal/api/admin_ui_test.go
git commit -m "feat: add admin dashboard and user management ui"
```

---

## Task 6: Implement permissions, admin operators, and high-risk action UX

**Files:**
- Modify: `Platform/internal/api/server.go`
- Modify: `Platform/internal/api/server_test.go`
- Modify: `Platform/internal/api/admin_index.html`
- Modify: `Platform/internal/api/admin_ui_test.go`
- Modify: `Platform/internal/service/service.go`
- Modify: `Platform/internal/store/pg/store.go`
- Modify: `Platform/internal/service/memory_store.go`

**Step 1: Write failing tests**

Add tests for:
- listing admin operators
- updating roles/capabilities
- forbidden actions from insufficient-capability operator
- UI hiding/disable states for forbidden actions

**Step 2: Run tests to verify fail**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api ./internal/service
```

**Step 3: Implement**

Add:
- `/admin/operators` list/update endpoints
- permission-aware buttons/forms in UI
- confirmation modal for:
  - wallet adjustments
  - pricing/routing changes
  - agreement publication
  - refund review actions

**Step 4: Run tests**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api ./internal/service ./internal/store/pg
```

**Step 5: Commit**

```powershell
git add Platform/internal/api/server.go Platform/internal/api/server_test.go Platform/internal/api/admin_index.html Platform/internal/api/admin_ui_test.go Platform/internal/service/service.go Platform/internal/store/pg/store.go Platform/internal/service/memory_store.go
git commit -m "feat: add admin operator permissions ui"
```

---

## Task 7: Implement business-management screens (orders, wallet, pricing, routes, agreements, audits)

**Files:**
- Modify: `Platform/internal/api/admin_index.html`
- Modify: `Platform/internal/api/admin_ui_test.go`

**Step 1: Write failing UI tests**

Require screens for:
- orders
- wallet adjustments
- pricing rules
- official routes
- agreements
- audit logs

**Step 2: Run tests to verify fail**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api
```

**Step 3: Implement**

Each module must support:
- list view
- filter/search
- detail preview
- edit/create flow where allowed
- optimistic refresh after save
- error states and audit-friendly confirmations

**Step 4: Run tests**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api
```

**Step 5: Commit**

```powershell
git add Platform/internal/api/admin_index.html Platform/internal/api/admin_ui_test.go
git commit -m "feat: add admin business management screens"
```

---

## Task 8: Implement governance screens (refunds, infringement, notices, risk rules, retention)

**Files:**
- Modify: `Platform/internal/api/admin_index.html`
- Modify: `Platform/internal/api/admin_ui_test.go`

**Step 1: Write failing UI tests**

Add tests for:
- refund review module
- infringement review module
- system notices module
- risk rule module
- data retention module

**Step 2: Run tests to verify fail**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api
```

**Step 3: Implement**

Build operator workflows for:
- approve/reject/settle refunds
- review/update infringement reports
- manage system notices
- manage risk rules
- manage data retention policies

Add explicit warning copy on destructive or legally sensitive actions.

**Step 4: Run tests**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./internal/api
```

**Step 5: Commit**

```powershell
git add Platform/internal/api/admin_index.html Platform/internal/api/admin_ui_test.go
git commit -m "feat: add admin governance screens"
```

---

## Task 9: Full production-readiness review, QA, accessibility, and security validation

**Files:**
- Modify: `Platform/internal/api/admin_ui_test.go`
- Modify: `docs/plans/2026-03-13-myagent-bot-platform-refactor-remaining-todo.md`
- Create: `docs/qa/admin-backend-ui-acceptance-checklist.md`

**Step 1: Add/expand automated coverage**

Add tests for:
- responsive shell markers
- ARIA/live regions
- permission-gated actions
- dangerous action confirmation copy
- required module presence

**Step 2: Run full verification**

```powershell
cd Platform
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./...

cd ..\Launcher\app-wails
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./...

cd ..\..\PinchBot
& 'C:\Users\sky\.cache\codex\go1.25.8\go\bin\go.exe' test ./...

cd ..
git diff --check
```

Expected:
- all tests pass
- no whitespace errors

**Step 3: Manual acceptance checklist**

Create and execute a checklist covering:
- desktop/laptop/tablet widths
- login/logout
- module navigation
- permission-restricted account behavior
- data accuracy against seeded fixtures
- wallet adjustment and refund confirmation flows
- pricing/routing/agreement editing
- audit visibility
- empty/loading/error states

**Step 4: Review and fix until no known issues remain**

Required review loop:
- spec compliance review
- code quality/security review
- responsive/UI polish pass
- re-run verification after each fix batch

**Step 5: Commit**

```powershell
git add Platform/internal/api/admin_ui_test.go docs/qa/admin-backend-ui-acceptance-checklist.md docs/plans/2026-03-13-myagent-bot-platform-refactor-remaining-todo.md
git commit -m "test: verify admin backend ui production readiness"
```

---

## Final Acceptance Criteria

- Admin console exposes:
  - dashboard
  - users
  - permissions/operators
  - orders
  - wallet adjustments
  - pricing rules
  - routes
  - agreements
  - audit logs
  - refunds
  - infringement reports
  - notices/risk/retention governance
- Permission model is enforced server-side and reflected client-side
- Responsive UI works at representative desktop/tablet widths
- High-risk operator actions require explicit confirmation
- Data shown in UI matches backend truth
- Full `go test ./...` passes for `Platform`, `Launcher/app-wails`, and `PinchBot`
- `git diff --check` is clean
- No known P0 defects remain

---

## Suggested Execution Order

1. Task 1 — RBAC foundation
2. Task 2 — dashboard API
3. Task 3 — user drill-down APIs
4. Task 4 — admin shell/navigation
5. Task 5 — dashboard + users UI
6. Task 6 — permissions UI
7. Task 7 — business-management screens
8. Task 8 — governance screens
9. Task 9 — final QA / review / acceptance

