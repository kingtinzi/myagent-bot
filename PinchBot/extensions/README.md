# PinchBot Node extensions

Bundled OpenClaw-style plugins live under this directory (each subfolder with `openclaw.plugin.json`).

**graph-memory** is **not** a Node extension: it runs inside the Go binary (`pkg/graphmemory`), uses `config.graph-memory.json` as a sidecar, and creates/opens its SQLite DB at `dbPath`. Release bundles **do not** ship `extensions/graph-memory`.

**lobster** remains a Node extension here; release scripts copy it next to the binary and run `npm ci --omit=dev`.

Agent-facing onboarding for **npm install**, **`config.json` / `plugins.enabled`**, **graph-memory sidecar**, and **secrets handling** lives in the workspace template skill **`workspace/skills/extensions/`** (shipped with new workspaces via `internal/workspacetpl`).

Gateway still runs the embedded plugin host from `pkg/plugins/assets` (`run.mjs`); this folder supplies **extension** packages loaded by that host.
