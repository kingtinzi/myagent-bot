# PinchBot Node extensions

Bundled OpenClaw-style plugins live under this directory (each subfolder with `openclaw.plugin.json`).

Agent-facing onboarding for **npm install**, **`config.json` / `plugins.enabled`**, **graph-memory sidecar**, and **secrets handling** lives in the workspace template skill **`workspace/skills/extensions/`** (shipped with new workspaces via `internal/workspacetpl`).

## graph-memory

Canonical copy: **`graph-memory/`** (this repo). If you previously cloned the upstream repo elsewhere, use **this directory** as the source of truth.

### After `git clone`

Install dependencies **once** (we do **not** commit `node_modules`):

```bash
cd graph-memory
npm ci
# or: npm install
```

Requires **Node.js ≥ 18** (recommended **22**).

### Configure PinchBot

1. **Defaults (new installs):** `config.json` already sets **`plugins.node_host`: `true`**, **`plugins.enabled`** includes **`graph-memory`** and **`lobster`**, and **`plugins.slots.contextEngine`** is **`graph-memory`**. Enable the sidecar by copying `config/config.graph-memory.example.json` to **`config.graph-memory.json`** next to `config.json` and set **`"enabled": true`** plus LLM/embedding keys. The default data directory next to the binary is **`.openclaw`** (OpenClaw-compatible).
2. **`plugins.extensions_dir`** is resolved relative to the agent **workspace** (default `workspace` under the PinchBot home directory). If that folder is missing, PinchBot falls back to **`extensions` next to the `pinchbot` binary** (Windows/macOS release bundles ship **`extensions/graph-memory`** and **`extensions/lobster`** with production `npm` deps). Alternatively, create `workspace/extensions` and symlink extensions into it, or set **`extensions_dir`** to the **absolute path** of this `PinchBot/extensions` directory.

Gateway still runs the embedded plugin host from `pkg/plugins/assets` (`run.mjs`); this folder supplies **extension** packages loaded by that host.
