const emptyPluginConfigSchema = {
  type: "object",
  additionalProperties: false,
  properties: {},
};

function resolvePluginConfigSchema(configSchema = emptyPluginConfigSchema) {
  return typeof configSchema === "function" ? configSchema() : configSchema;
}

/** Minimal stand-in for openclaw definePluginEntry (bundled extensions only). */
export function definePluginEntry({ id, name, description, kind, configSchema, register }) {
  const cs = configSchema ?? emptyPluginConfigSchema;
  return {
    id,
    name,
    description,
    ...(kind ? { kind } : {}),
    configSchema: resolvePluginConfigSchema(cs),
    register,
  };
}
