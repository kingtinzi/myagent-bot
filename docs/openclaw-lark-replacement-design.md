# OpenClaw Lark Plugin Replacement Design (No Legacy Migration)

## Goal

Replace PinchBot built-in Go `feishu` channel with the official OpenClaw Lark plugin (`openclaw-lark`) for new deployments.

Constraints for this iteration:

- Do not handle legacy user config migration.
- Prioritize official plugin behavior and compatibility with upstream OpenClaw ecosystem.
- Keep rollback surface minimal by preserving a single top-level switch.

## Why Replace Go Built-in Channel

- The official plugin is maintained by Lark/OpenClaw ecosystem owners and receives faster compatibility updates.
- Feature updates and policy changes can be consumed from upstream with less in-house maintenance.
- PinchBot can reduce channel-specific Go maintenance and focus on platform/runtime value.

## Current Gap Summary

PinchBot currently uses:

- built-in Go channel: `pkg/channels/feishu/*`
- snake_case config: `channels.feishu.app_id/app_secret/...`
- plugin enable list: `plugins.enabled`

Official `openclaw-lark` tooling expects:

- plugin-driven channel runtime
- camelCase config: `channels.feishu.appId/appSecret/...`
- OpenClaw plugin toggles: `plugins.entries` and `plugins.allow`

Therefore, "npm install tools only" is not enough to activate official plugin runtime in PinchBot.

## Scope

### In Scope

1. Official plugin as default Feishu runtime path.
2. Config compatibility for OpenClaw-style Feishu fields (camelCase).
3. Plugin activation compatibility (`plugins.entries` + `plugins.allow`).
4. Disable Go built-in Feishu registration in normal runtime path.
5. Add startup validation and diagnostics for missing plugin/config.
6. End-to-end test for message in/out on plugin path.

### Out of Scope

1. Legacy config migration wizard.
2. Dual-run sync mode between Go and plugin channels.
3. Long-term data migration/backfill from historical deployments.

## Target Architecture

1. Gateway starts plugin host (`pkg/plugins/assets/run.mjs`) as today.
2. `openclaw-lark` is discovered from `extensions` and loaded by Node host.
3. Tool/channel capability from plugin runtime is used for Feishu handling.
4. Go built-in `feishu` channel is not registered by default.

## Implementation Plan

### Phase 1: Config Compatibility

Files:

- `pkg/config/config.go`
- `config/config.example.json`

Changes:

- Extend `FeishuConfig` to accept both snake_case and camelCase keys.
- Keep canonical write format documented as camelCase for plugin-first path.
- Ensure env var mapping remains available for required credentials.

Acceptance:

- Loading config with `appId/appSecret` works.
- Existing tests still pass for config parsing.

### Phase 2: Plugin Activation Compatibility

Files:

- `pkg/plugins/*` (loading path)
- config model for plugin fields

Changes:

- Add compatibility read for `plugins.entries` and `plugins.allow`.
- Resolve effective enabled plugin IDs from:
  1. `plugins.enabled`
  2. `plugins.entries[<id>].enabled == true`
- If `plugins.allow` exists, enforce allowlist.

Acceptance:

- `openclaw-lark` can be activated with OpenClaw-style config without extra translation.

### Phase 3: Disable Go Built-in Feishu Path

Files:

- `pkg/channels/feishu/init.go`
- channel manager/bootstrap wiring

Changes:

- Remove default registration of built-in Go `feishu` channel or gate it behind explicit dev switch.
- Emit clear startup message that official plugin path is expected.

Acceptance:

- Runtime does not start Go Feishu channel when official plugin mode is active.

### Phase 4: Runtime Validation and UX

Files:

- startup/bootstrap diagnostics
- docs and runbook updates

Changes:

- Add startup checks:
  - plugin host enabled
  - `openclaw-lark` extension discoverable
  - required `channels.feishu` fields present
- Add actionable error messages with fix hints.

Acceptance:

- Misconfiguration is detected with clear logs and non-ambiguous guidance.

### Phase 5: Test and Verification

Tests:

- Unit:
  - config parse compatibility
  - plugin enabled resolution logic
- Integration:
  - Node host loads `openclaw-lark`
  - Feishu message receive/send path through plugin runtime
- Manual smoke:
  - startup
  - direct chat reply
  - group mention trigger

Release gate:

- All unit/integration tests pass.
- Windows and macOS smoke checks pass.

## Risks and Mitigations

1. **Config ambiguity** between old/new fields.
   - Mitigation: define precedence and warn when both variants exist.
2. **Plugin missing at runtime**.
   - Mitigation: fail fast with explicit startup diagnostics.
3. **Unexpected behavior differences** vs Go channel.
   - Mitigation: add focused smoke checklist for reply, trigger, and error paths.

## Rollout Recommendation (No Legacy Migration)

1. Merge plugin-first runtime support.
2. Keep a short-lived internal fallback switch only for debugging.
3. Announce plugin-first Feishu support in release notes.
4. Remove fallback switch after stabilization window.

## Deliverables

1. Code changes for config + plugin activation + channel bootstrap.
2. Updated runbook/doc for official plugin install and validation.
3. Test evidence (unit, integration, manual smoke summary).
