import { definePluginEntry } from "openclaw/plugin-sdk/core";

export default definePluginEntry({
  id: "fixture-second",
  name: "Fixture Second",
  description: "Second minimal test plugin.",
  register(api: any) {
    api.registerTool(
      (_ctx: any) => ({
        name: "fixture_second_echo",
        description: "Echo args (second fixture).",
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
                text: JSON.stringify({ ok: true, fixture: "second", params }, null, 2),
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
