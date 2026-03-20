# PinchBot Node extensions

Bundled OpenClaw-style plugins live under this directory (each subfolder with `openclaw.plugin.json`).

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

1. Set **`plugins.node_host`: `true`** and include **`graph-memory`** in **`plugins.enabled`** in `config.json`.
2. **`plugins.extensions_dir`** is resolved relative to the agent **workspace** (default `workspace` under `PINCHBOT_HOME`). Either:
   - Create `workspace/extensions` and symlink `graph-memory` into it, or  
   - Set **`extensions_dir`** to the **absolute path** of this `PinchBot/extensions` directory.
3. Add **`config.graph-memory.json`** next to `config.json` (see `config/config.graph-memory.example.json`). Set `"enabled": true` and LLM/embedding keys as needed.

Gateway still runs the embedded plugin host from `pkg/plugins/assets` (`run.mjs`); this folder supplies **extension** packages loaded by that host.
