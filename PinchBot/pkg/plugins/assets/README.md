# PinchBot Node plugin host

PinchBot loads OpenClaw-style extensions (`openclaw.plugin.json` + `index.ts`) when `plugins.node_host` is true.

Install dependencies once:

```bash
cd pkg/plugins/assets && npm install
```

Requires **Node.js 18+** on `PATH` (or set `plugins.node_binary` in config). The host does not install the full `openclaw` npm package; it uses small local shims plus vendored Windows spawn helpers.

Release bundles place this directory next to the `pinchbot` binary as `plugin-host/` (see `scripts/build-release.*`). Optional config: `node_host_start_retries`, `node_host_max_recoveries` (use `-1` to disable execute-time restart), `node_host_restart_delay_ms`.
