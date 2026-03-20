# LLM Task (plugin)

Adds an **optional** agent tool `llm-task` for running **JSON-only** LLM tasks
(drafting, summarizing, classifying) with optional JSON Schema validation.

Designed to be called from workflow engines (for example, Lobster via
`openclaw.invoke --each`) without adding new OpenClaw code per workflow.

## PinchBot (native Go)

In **PinchBot**, when `plugins.enabled` contains `llm-task`, the tool is implemented in **Go**
(`pkg/tools/llm_task.go`): no Node `runEmbeddedPiAgent` bridge. The Node plugin host **skips** loading
this extension id so the TS bundle is not required at runtime.

Enable and optional defaults:

```jsonc
{
  "plugins": {
    "enabled": ["llm-task"],
    "llm_task": {
      "default_provider": "openai",
      "default_model": "my-model-name",
      "allowed_models": ["openai/gpt-4o"],
      "max_tokens": 800,
      "timeout_ms": 30000
    }
  }
}
```

`default_model` and `model` tool argument are **model_list** `model_name` values. Combine with
`lobster` (and other tools) by letting the agent call tools in sequence; PinchBot does not ship
`POST /tools/invoke` for Lobster `openclaw.invoke` steps.

## Enable

1. Enable the plugin:

```json
{
  "plugins": {
    "entries": {
      "llm-task": { "enabled": true }
    }
  }
}
```

2. Allowlist the tool (it is registered with `optional: true`):

```json
{
  "agents": {
    "list": [
      {
        "id": "main",
        "tools": { "allow": ["llm-task"] }
      }
    ]
  }
}
```

## Config (optional)

```json
{
  "plugins": {
    "entries": {
      "llm-task": {
        "enabled": true,
        "config": {
          "defaultProvider": "openai-codex",
          "defaultModel": "gpt-5.2",
          "defaultAuthProfileId": "main",
          "allowedModels": ["openai-codex/gpt-5.2"],
          "maxTokens": 800,
          "timeoutMs": 30000
        }
      }
    }
  }
}
```

`allowedModels` is an allowlist of `provider/model` strings. If set, any request
outside the list is rejected.

## Tool API

### Parameters

- `prompt` (string, required)
- `input` (any, optional)
- `schema` (object, optional JSON Schema)
- `provider` (string, optional)
- `model` (string, optional)
- `thinking` (string, optional)
- `authProfileId` (string, optional)
- `temperature` (number, optional)
- `maxTokens` (number, optional)
- `timeoutMs` (number, optional)

### Output

Returns `details.json` containing the parsed JSON (and validates against
`schema` when provided).

## Notes

- The tool is **JSON-only** and instructs the model to output only JSON
  (no code fences, no commentary).
- No tools are exposed to the model for this run.
- Side effects should be handled outside this tool (for example, approvals in
  Lobster) before calling tools that send messages/emails.

## Bundled extension note

This extension depends on OpenClaw internal modules (the embedded agent runner).
It is intended to ship as a **bundled** OpenClaw extension (like `lobster`) and
be enabled via `plugins.entries` + tool allowlists.

It is **not** currently designed to be copied into
`~/.openclaw/extensions` as a standalone plugin directory.
