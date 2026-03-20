import { definePluginEntry } from "openclaw/plugin-sdk/core";

export default definePluginEntry({
  id: "echo-fixture",
  name: "Echo Fixture",
  description: "Test-only plugin that echoes tool args.",
  register(api: any) {
    api.registerTool(
      (_ctx: any) => ({
        name: "echo_fixture",
        description: "Echo args as JSON (fixture).",
        parameters: {
          type: "object",
          properties: {
            message: { type: "string" },
          },
          required: ["message"],
          additionalProperties: true,
        },
        async execute(_id: string, params: Record<string, unknown>) {
          return {
            content: [
              {
                type: "text",
                text: JSON.stringify({ ok: true, params }, null, 2),
              },
            ],
            details: { ok: true, params },
          };
        },
      }),
      { optional: false },
    );
  },
});

