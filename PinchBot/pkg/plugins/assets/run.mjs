import { createJiti } from "jiti";
import { createRequire } from "node:module";
import path from "node:path";
import readline from "node:readline";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const hostRequire = createRequire(import.meta.url);

let toolsByKey = new Map();
let catalog = [];
let agentWorkspace = process.cwd();

/** OpenClaw typed hook names — api.on(name) for these is no-op + stderr (except before_tool_call, handled in execute). */
const OPENCLAW_TYPED_HOOK_NAMES = new Set([
  "before_model_resolve",
  "before_prompt_build",
  "before_agent_start",
  "llm_input",
  "llm_output",
  "agent_end",
  "before_compaction",
  "after_compaction",
  "before_reset",
  "inbound_claim",
  "message_received",
  "message_sending",
  "message_sent",
  "tool_result_persist",
  "before_message_write",
  "session_start",
  "session_end",
  "subagent_spawning",
  "subagent_delivery_target",
  "subagent_spawned",
  "subagent_ended",
  "gateway_start",
  "gateway_stop",
]);

function logUnsupported(pluginId, method, detail) {
  const msg = detail ? `${method}: ${detail}` : method;
  console.error(`[pinchbot-plugin-host] plugin "${pluginId}" ${msg} (not implemented in PinchBot Node host — extension may be partially broken)`);
}

/** @type {Map<string, Array<(event:any, ctx:any) => any>>} */
let globalEventHandlers = new Map();
/** @type {Map<string, any>} */
let globalContextEngines = new Map();
/** Handlers from api.on('before_tool_call', …) — run in Node before each plugin tool execute. */
let globalBeforeToolCallHandlers = [];
/** Handlers from api.on('after_tool_call', …) — run after successful execute; errors are logged only. */
let globalAfterToolCallHandlers = [];
/** Declared HTTP routes from registerHttpRoute (metadata for Go; Gateway dispatch not wired yet). */
let globalHttpRoutes = [];
/** Declared CLI commands from registerCommand (metadata for Go; execution not wired in PinchBot). */
let globalDeclaredCommands = [];
/** Declared gateway RPC names from registerGatewayMethod (metadata for Go). */
let globalGatewayMethods = [];
/** Key: pluginId + \0 + method — handler for IPC gatewayMethod (PinchBot Gateway POST /plugins/gateway-method). */
let globalGatewayMethodHandlers = new Map();
/** Declared plugin service ids from registerService (metadata for Go; start/stop not wired in PinchBot). */
let globalRegisteredServices = [];
/** Declarations from registerCli(registrar, opts?) — opts.commands (metadata for Go; handler not wired in PinchBot). */
let globalCliRegistrations = [];
/** Declarations from registerProvider (metadata for Go; inference not wired in PinchBot). */
let globalRegisteredProviders = [];
let globalSpeechProviders = [];
let globalMediaUnderstandingProviders = [];
let globalImageGenerationProviders = [];
let globalWebSearchProviders = [];
/** Declarations from registerHook (events metadata; hooks not executed in PinchBot). */
let globalRegisteredHooks = [];
/** Declarations from registerChannel (channel plugin id; channel runtime not wired in PinchBot). */
let globalRegisteredChannels = [];
/** Declarations from registerInteractiveHandler (channel + namespace; handlers not wired in PinchBot). */
let globalInteractiveHandlers = [];
/** One entry per onConversationBindingResolved(handler) call (listener not wired in PinchBot). */
let globalConversationBindingListeners = [];
/** Key: pluginId + method + path — handler kept for future IPC. */
let globalHttpRouteHandlers = new Map();

function normalizeHookEvents(events) {
  const out = [];
  if (typeof events === "string" && events.trim()) {
    out.push(events.trim());
  } else if (Array.isArray(events)) {
    for (const e of events) {
      if (typeof e === "string" && e.trim()) {
        out.push(e.trim());
      }
    }
  }
  return [...new Set(out)].sort();
}

function pushProviderDeclaration(pluginId, provider, bucket, apiName) {
  if (!provider || typeof provider !== "object") {
    console.error(`[pinchbot-plugin-host] ${apiName}: expected object (${pluginId})`);
    return;
  }
  const id = typeof provider.id === "string" ? provider.id.trim() : "";
  if (!id) {
    console.error(`[pinchbot-plugin-host] ${apiName}: need id (${pluginId})`);
    return;
  }
  const label = typeof provider.label === "string" ? provider.label.trim() : "";
  bucket.push({ pluginId, id, label });
}

function send(obj) {
  process.stdout.write(`${JSON.stringify(obj)}\n`);
}

function pluginLogger(prefix) {
  const write = (level, args) => {
    const line = args.map((v) => {
      if (typeof v === "string") return v;
      try {
        return JSON.stringify(v);
      } catch {
        return String(v);
      }
    }).join(" ");
    process.stderr.write(`[${prefix}] ${level}: ${line}\n`);
  };
  return {
    debug: (...args) => write("debug", args),
    info: (...args) => write("info", args),
    warn: (...args) => write("warn", args),
    error: (...args) => write("error", args),
    log: (...args) => write("log", args),
  };
}

function toolResultToContent(result) {
  if (result == null) {
    return "";
  }
  if (typeof result === "string") {
    return result;
  }
  if (typeof result !== "object") {
    return String(result);
  }
  if (Array.isArray(result.content)) {
    return result.content
      .map((part) => {
        if (part && typeof part === "object" && part.type === "text" && typeof part.text === "string") {
          return part.text;
        }
        return JSON.stringify(part);
      })
      .join("\n");
  }
  if (typeof result.content === "string") {
    return result.content;
  }
  return JSON.stringify(result);
}

/** Best-effort JSON-schema-like object for LLM tool lists (TypeBox may omit on JSON.stringify). */
function serializeParameters(raw) {
  if (!raw || typeof raw !== "object") {
    return { type: "object", properties: {} };
  }
  const pick = (o) => {
    const out = {};
    for (const k of Object.keys(o)) {
      const v = o[k];
      const t = typeof v;
      if (t === "string" || t === "number" || t === "boolean" || v === null) {
        out[k] = v;
      } else if (t === "object" && v !== null && !Array.isArray(v)) {
        out[k] = pick(v);
      } else if (Array.isArray(v)) {
        out[k] = v.map((x) =>
          typeof x === "object" && x !== null && !Array.isArray(x) ? pick(x) : x,
        );
      }
    }
    return out;
  };
  const flat = pick(raw);
  if (flat.type === "object" && flat.properties && typeof flat.properties === "object") {
    return flat;
  }
  try {
    const s = JSON.stringify(raw);
    if (s && s !== "{}" && s !== "null") {
      return JSON.parse(s);
    }
  } catch {
    /* ignore */
  }
  return { type: "object", properties: {} };
}

/**
 * Builds api.runtime: merges api.config.runtime from Go (pinchbotConfig), sets version/kind, stubs subagent.spawn.
 */
function buildPinchbotRuntime(pluginId, pinchbotConfig) {
  const pbc = pinchbotConfig && typeof pinchbotConfig === "object" ? pinchbotConfig : {};
  const pr = pbc.runtime && typeof pbc.runtime === "object" ? pbc.runtime : {};
  const ver =
    typeof pr.version === "string" && pr.version.trim()
      ? pr.version.trim()
      : typeof process.env.PINCHBOT_PLUGIN_HOST_VERSION === "string" && process.env.PINCHBOT_PLUGIN_HOST_VERSION.trim()
        ? process.env.PINCHBOT_PLUGIN_HOST_VERSION.trim()
        : "pinchbot-node-host";
  const agentBase = pr.agent && typeof pr.agent === "object" ? { ...pr.agent } : {};
  const subagentStub = {
    spawn: async () => {
      logUnsupported(pluginId, "runtime.agent.subagent.spawn", "not implemented in PinchBot");
      throw new Error("PinchBot: runtime.agent.subagent.spawn is not implemented");
    },
  };
  return {
    ...pr,
    version: ver,
    kind: typeof pr.kind === "string" && pr.kind.trim() ? pr.kind.trim() : "pinchbot",
    agent: { ...agentBase, subagent: subagentStub },
  };
}

/**
 * Resolves a path like OpenClaw: relative paths are against the extension root; absolute paths are normalized only.
 * @param {string} pluginRootAbs absolute filesystem path to the plugin directory (from discover `root`)
 */
function createApi(pluginId, sandboxed, pluginConfig, pinchbotConfig, pluginRootAbs) {
  const cfg = pluginConfig && typeof pluginConfig === "object" ? pluginConfig : {};
  const pbc = pinchbotConfig && typeof pinchbotConfig === "object" ? pinchbotConfig : {};
  const rootAbs =
    typeof pluginRootAbs === "string" && pluginRootAbs.trim()
      ? path.normalize(path.resolve(pluginRootAbs.trim()))
      : "";
  const unsupportedOnce = new Set();
  const once = (key, fn) => {
    if (unsupportedOnce.has(key)) {
      return;
    }
    unsupportedOnce.add(key);
    fn();
  };

  return {
    id: pluginId,
    name: pluginId,
    registrationMode: "full",
    source: "pinchbot-node-host",
    rootDir: rootAbs || undefined,
    version: undefined,
    description: undefined,

    registerTool(factory, _opts) {
      const ctx = { sandboxed };
      let tool;
      try {
        tool = typeof factory === "function" ? factory(ctx) : null;
      } catch (e) {
        console.error(`[pinchbot-plugin-host] registerTool factory error (${pluginId}):`, e);
        return;
      }
      if (!tool || typeof tool.execute !== "function") {
        return;
      }
      const name = tool.name;
      if (!name || typeof name !== "string") {
        return;
      }
      const key = `${pluginId}:${name}`;
      toolsByKey.set(key, tool);
      catalog.push({
        pluginId,
        name,
        description: typeof tool.description === "string" ? tool.description : "",
        parameters: serializeParameters(tool.parameters),
      });
    },
    pluginConfig: cfg,
    config: pbc,
    logger: pluginLogger(pluginId),
    runtime: buildPinchbotRuntime(pluginId, pbc),

    registerHook(events, handler, _opts) {
      if (typeof handler !== "function") {
        console.error(`[pinchbot-plugin-host] registerHook: need handler function (${pluginId})`);
        return;
      }
      const evs = normalizeHookEvents(events);
      if (evs.length === 0) {
        console.error(`[pinchbot-plugin-host] registerHook: need non-empty event name(s) (${pluginId})`);
        return;
      }
      globalRegisteredHooks.push({ pluginId, events: evs });
      once("registerHook", () => logUnsupported(pluginId, "registerHook"));
    },
    registerHttpRoute(params) {
      if (!params || typeof params !== "object") {
        console.error(`[pinchbot-plugin-host] registerHttpRoute: expected options object (${pluginId})`);
        return;
      }
      const method = typeof params.method === "string" ? params.method.trim().toUpperCase() : "";
      const pth = typeof params.path === "string" ? params.path.trim() : "";
      const handler = typeof params.handler === "function" ? params.handler : null;
      if (!method || !pth.startsWith("/") || !handler) {
        console.error(
          `[pinchbot-plugin-host] registerHttpRoute: need method, path (starting with /), handler (${pluginId})`,
        );
        return;
      }
      globalHttpRoutes.push({ pluginId, method, path: pth });
      globalHttpRouteHandlers.set(`${pluginId}\0${method}\0${pth}`, handler);
    },
    registerChannel(registration) {
      const raw = registration && typeof registration === "object" ? registration : null;
      const plugin = raw && raw.plugin && typeof raw.plugin === "object" ? raw.plugin : raw;
      if (!plugin || typeof plugin !== "object") {
        console.error(`[pinchbot-plugin-host] registerChannel: expected channel plugin or { plugin } (${pluginId})`);
        return;
      }
      const cid = typeof plugin.id === "string" ? plugin.id.trim() : "";
      if (!cid) {
        console.error(`[pinchbot-plugin-host] registerChannel: channel plugin needs id (${pluginId})`);
        return;
      }
      globalRegisteredChannels.push({ pluginId, channelId: cid });
      once("registerChannel", () => logUnsupported(pluginId, "registerChannel"));
    },
    registerGatewayMethod(method, handler) {
      if (typeof method !== "string" || !method.trim()) {
        console.error(`[pinchbot-plugin-host] registerGatewayMethod: need non-empty method string (${pluginId})`);
        return;
      }
      if (typeof handler !== "function") {
        console.error(`[pinchbot-plugin-host] registerGatewayMethod: need handler function (${pluginId})`);
        return;
      }
      const m = method.trim();
      globalGatewayMethods.push({ pluginId, method: m });
      globalGatewayMethodHandlers.set(`${pluginId}\0${m}`, handler);
    },
    registerCli(registrar, opts) {
      if (typeof registrar !== "function") {
        console.error(`[pinchbot-plugin-host] registerCli: need registrar function (${pluginId})`);
        return;
      }
      const cmds = [];
      if (opts && typeof opts === "object" && Array.isArray(opts.commands)) {
        for (const c of opts.commands) {
          if (typeof c === "string" && c.trim()) {
            cmds.push(c.trim());
          }
        }
      }
      globalCliRegistrations.push({ pluginId, commands: cmds });
    },
    registerService(service) {
      if (!service || typeof service !== "object") {
        console.error(`[pinchbot-plugin-host] registerService: expected object (${pluginId})`);
        return;
      }
      const sid = typeof service.id === "string" ? service.id.trim() : "";
      if (!sid) {
        console.error(`[pinchbot-plugin-host] registerService: need service.id (${pluginId})`);
        return;
      }
      if (typeof service.start !== "function") {
        console.error(`[pinchbot-plugin-host] registerService: need start() (${pluginId})`);
        return;
      }
      globalRegisteredServices.push({ pluginId, serviceId: sid });
    },
    registerProvider(provider) {
      pushProviderDeclaration(pluginId, provider, globalRegisteredProviders, "registerProvider");
    },
    registerSpeechProvider(provider) {
      pushProviderDeclaration(pluginId, provider, globalSpeechProviders, "registerSpeechProvider");
    },
    registerMediaUnderstandingProvider(provider) {
      pushProviderDeclaration(pluginId, provider, globalMediaUnderstandingProviders, "registerMediaUnderstandingProvider");
    },
    registerImageGenerationProvider(provider) {
      pushProviderDeclaration(pluginId, provider, globalImageGenerationProviders, "registerImageGenerationProvider");
    },
    registerWebSearchProvider(provider) {
      pushProviderDeclaration(pluginId, provider, globalWebSearchProviders, "registerWebSearchProvider");
    },
    registerInteractiveHandler(registration) {
      if (!registration || typeof registration !== "object") {
        console.error(`[pinchbot-plugin-host] registerInteractiveHandler: expected registration object (${pluginId})`);
        return;
      }
      const channel = typeof registration.channel === "string" ? registration.channel.trim() : "";
      const namespace = typeof registration.namespace === "string" ? registration.namespace.trim() : "";
      if (!channel || !namespace) {
        console.error(
          `[pinchbot-plugin-host] registerInteractiveHandler: need channel and namespace (${pluginId})`,
        );
        return;
      }
      globalInteractiveHandlers.push({ pluginId, channel, namespace });
      once("registerInteractiveHandler", () => logUnsupported(pluginId, "registerInteractiveHandler"));
    },
    onConversationBindingResolved(handler) {
      if (typeof handler !== "function") {
        return;
      }
      globalConversationBindingListeners.push({ pluginId });
      once("onConversationBindingResolved", () =>
        logUnsupported(pluginId, "onConversationBindingResolved"),
      );
    },
    registerCommand(command) {
      if (!command || typeof command !== "object") {
        console.error(`[pinchbot-plugin-host] registerCommand: expected object (${pluginId})`);
        return;
      }
      const name = typeof command.name === "string" ? command.name.trim() : "";
      if (!name) {
        console.error(`[pinchbot-plugin-host] registerCommand: need name (${pluginId})`);
        return;
      }
      const description =
        typeof command.description === "string" && command.description.trim()
          ? command.description.trim()
          : "";
      globalDeclaredCommands.push({ pluginId, name, description });
    },
    resolvePath(input) {
      if (typeof input !== "string") {
        return "";
      }
      const s = input.trim();
      if (!s) {
        return "";
      }
      const base = rootAbs || agentWorkspace;
      try {
        if (path.isAbsolute(s)) {
          return path.normalize(s);
        }
        return path.normalize(path.join(base, s));
      } catch {
        return s;
      }
    },

    on(event, handler) {
      if (typeof event !== "string" || !event.trim() || typeof handler !== "function") {
        return;
      }
      const ev = event.trim();
      const low = ev.toLowerCase();
      if (low === "before_tool_call") {
        globalBeforeToolCallHandlers.push({ pluginId, handler });
        return;
      }
      if (low === "after_tool_call") {
        globalAfterToolCallHandlers.push({ pluginId, handler });
        return;
      }
      if (OPENCLAW_TYPED_HOOK_NAMES.has(low)) {
        once(`on:${low}`, () =>
          logUnsupported(
            pluginId,
            `api.on('${ev}', …)`,
            "OpenClaw typed hooks are not forwarded to PinchBot — no-op",
          ),
        );
        return;
      }
      if (!globalEventHandlers.has(ev)) {
        globalEventHandlers.set(ev, []);
      }
      globalEventHandlers.get(ev).push(handler);
    },
    registerContextEngine(id, factory) {
      if (typeof id !== "string" || !id.trim()) {
        return;
      }
      if (typeof factory !== "function") {
        return;
      }
      try {
        const engine = factory();
        if (engine && typeof engine === "object") {
          globalContextEngines.set(id.trim(), engine);
        }
      } catch (e) {
        console.error(`[pinchbot-plugin-host] registerContextEngine error (${pluginId} / ${id}):`, e);
      }
    },
  };
}

async function handleInit(params) {
  toolsByKey = new Map();
  catalog = [];
  globalEventHandlers = new Map();
  globalContextEngines = new Map();
  globalBeforeToolCallHandlers = [];
  globalAfterToolCallHandlers = [];
  globalHttpRoutes = [];
  globalHttpRouteHandlers = new Map();
  globalDeclaredCommands = [];
  globalGatewayMethods = [];
  globalGatewayMethodHandlers = new Map();
  globalRegisteredServices = [];
  globalCliRegistrations = [];
  globalRegisteredProviders = [];
  globalSpeechProviders = [];
  globalMediaUnderstandingProviders = [];
  globalImageGenerationProviders = [];
  globalWebSearchProviders = [];
  globalRegisteredHooks = [];
  globalRegisteredChannels = [];
  globalInteractiveHandlers = [];
  globalConversationBindingListeners = [];
  agentWorkspace = typeof params.workspace === "string" && params.workspace.trim() ? params.workspace : process.cwd();
  const sandboxed = !!params.sandboxed;
  const plugins = Array.isArray(params.plugins) ? params.plugins : [];
  const pinchbotConfig =
    params.pinchbotConfig && typeof params.pinchbotConfig === "object" ? params.pinchbotConfig : {};

  const diagnostics = [];

  for (const p of plugins) {
    const pluginId = typeof p.id === "string" ? p.id.trim() : "";
    const root = typeof p.root === "string" ? p.root.trim() : "";
    if (!pluginId || !root) {
      diagnostics.push({
        pluginId: pluginId || "unknown",
        ok: false,
        error: "each plugin needs id and root",
      });
      console.error("[pinchbot-plugin-host] skip plugin: missing id or root");
      continue;
    }
    try {
      const pluginConfig =
        p.pluginConfig && typeof p.pluginConfig === "object" ? p.pluginConfig : {};
      const indexTs = path.join(root, "index.ts");
      const alias = {
        "openclaw/plugin-sdk/core": path.join(__dirname, "openclaw-core-shim.mjs"),
        "openclaw/plugin-sdk/lobster": path.join(__dirname, "openclaw-lobster-shim.mjs"),
      };
      try {
        alias["@sinclair/typebox"] = hostRequire.resolve("@sinclair/typebox");
      } catch {
        /* extension may bundle its own deps */
      }
      const jiti = createJiti(import.meta.url, {
        interopDefault: true,
        cwd: root,
        alias,
      });
      const mod = jiti(indexTs);
      const entry = mod.default ?? mod;
      if (!entry || typeof entry.register !== "function") {
        throw new Error("missing register()");
      }
      const api = createApi(pluginId, sandboxed, pluginConfig, pinchbotConfig, path.resolve(root));
      await Promise.resolve(entry.register(api));
      diagnostics.push({ pluginId, ok: true });
    } catch (e) {
      const errMsg = e && typeof e === "object" && "message" in e ? String(e.message) : String(e);
      console.error(`[pinchbot-plugin-host] plugin ${pluginId} failed:`, e);
      diagnostics.push({ pluginId, ok: false, error: errMsg });
    }
  }

  return {
    tools: catalog,
    diagnostics,
    httpRoutes: globalHttpRoutes,
    commands: globalDeclaredCommands,
    gatewayMethods: globalGatewayMethods,
    registeredServices: globalRegisteredServices,
    cliRegistrations: globalCliRegistrations,
    registeredProviders: globalRegisteredProviders,
    registeredSpeechProviders: globalSpeechProviders,
    registeredMediaUnderstandingProviders: globalMediaUnderstandingProviders,
    registeredImageGenerationProviders: globalImageGenerationProviders,
    registeredWebSearchProviders: globalWebSearchProviders,
    registeredHooks: globalRegisteredHooks,
    registeredChannels: globalRegisteredChannels,
    registeredInteractiveHandlers: globalInteractiveHandlers,
    conversationBindingResolvedListeners: globalConversationBindingListeners,
  };
}

/**
 * Runs all before_tool_call handlers (registration order). Handlers may return:
 * - `{ blocked: true, error?: string }` — abort execution (error propagates to Go).
 * - `{ args: object }` — replace tool args for this execute (object must be a plain JSON-like map).
 */
async function runBeforeToolCallHandlers(pluginId, toolName, args, ctxPayload) {
  let execArgs =
    args && typeof args === "object" && !Array.isArray(args) ? { ...args } : {};
  for (const { handler } of globalBeforeToolCallHandlers) {
    const out = await Promise.resolve(
      handler(
        { pluginId, tool: toolName, args: execArgs },
        ctxPayload,
      ),
    );
    if (out && typeof out === "object") {
      if (out.blocked === true) {
        const msg =
          typeof out.error === "string" && out.error.trim()
            ? out.error.trim()
            : "tool call blocked by before_tool_call hook";
        throw new Error(`PINCHBOT_TOOL_BLOCKED:${msg}`);
      }
      if (out.args && typeof out.args === "object" && !Array.isArray(out.args)) {
        execArgs = { ...out.args };
      }
    }
  }
  return execArgs;
}

/**
 * Runs all after_tool_call handlers after a successful tool execute. Failures are logged to stderr
 * and do not change the tool result returned to Go.
 */
async function runAfterToolCallHandlers(pluginId, toolName, execArgs, ctxPayload, content) {
  const payload = {
    pluginId,
    tool: toolName,
    args: execArgs && typeof execArgs === "object" && !Array.isArray(execArgs) ? execArgs : {},
    content: typeof content === "string" ? content : "",
  };
  for (const { pluginId: regId, handler } of globalAfterToolCallHandlers) {
    try {
      await Promise.resolve(handler(payload, ctxPayload));
    } catch (e) {
      const msg = e && typeof e === "object" && "message" in e ? String(e.message) : String(e);
      console.error(`[pinchbot-plugin-host] after_tool_call handler error (${regId}):`, msg);
    }
  }
}

async function handleExecute(params) {
  const pluginId = typeof params.pluginId === "string" ? params.pluginId.trim() : "";
  const toolName = typeof params.tool === "string" ? params.tool.trim() : "";
  const args = params.args && typeof params.args === "object" ? params.args : {};
  const key = `${pluginId}:${toolName}`;
  const inst = toolsByKey.get(key);
  if (!inst) {
    throw new Error(`unknown tool ${key}`);
  }
  const ctxPayload = {
    channel: typeof params.channel === "string" ? params.channel : "",
    chatId: typeof params.chatId === "string" ? params.chatId : "",
    agentId: typeof params.agentId === "string" ? params.agentId : "",
  };
  const execArgs = await runBeforeToolCallHandlers(pluginId, toolName, args, ctxPayload);
  const prev = process.cwd();
  try {
    process.chdir(agentWorkspace);
    const result = await inst.execute("", execArgs);
    const content = toolResultToContent(result);
    await runAfterToolCallHandlers(pluginId, toolName, execArgs, ctxPayload, content);
    return { content };
  } finally {
    try {
      process.chdir(prev);
    } catch {
      /* ignore */
    }
  }
}

async function handleHttpRoute(params) {
  const pluginId = typeof params.pluginId === "string" ? params.pluginId.trim() : "";
  const method = typeof params.method === "string" ? params.method.trim().toUpperCase() : "";
  const pth = typeof params.path === "string" ? params.path.trim() : "";
  const key = `${pluginId}\0${method}\0${pth}`;
  const handler = globalHttpRouteHandlers.get(key);
  if (!handler) {
    throw new Error(`no http route handler for ${pluginId} ${method} ${pth}`);
  }
  const query = typeof params.query === "string" ? params.query : "";
  const bodyStr = typeof params.body === "string" ? params.body : "";
  const headers =
    params.headers && typeof params.headers === "object" && params.headers !== null ? params.headers : {};
  const req = {
    method,
    path: pth,
    url: query ? `${pth}?${query}` : pth,
    query,
    body: bodyStr,
    headers,
  };
  let out;
  try {
    out = await Promise.resolve(handler(req));
  } catch (e) {
    const errMsg = e && typeof e === "object" && "message" in e ? String(e.message) : String(e);
    throw new Error(`httpRoute handler error: ${errMsg}`);
  }
  if (out == null) {
    return { status: 204, body: "", headers: {} };
  }
  if (typeof out === "string") {
    return { status: 200, body: out, headers: { "content-type": "text/plain; charset=utf-8" } };
  }
  if (typeof out === "object") {
    const status = typeof out.status === "number" && !Number.isNaN(out.status) ? out.status : 200;
    let body = "";
    if (typeof out.body === "string") {
      body = out.body;
    } else if (out.body != null) {
      body = String(out.body);
    } else if (typeof out.content === "string") {
      body = out.content;
    }
    const hdrs = {};
    if (out.headers && typeof out.headers === "object" && out.headers !== null) {
      for (const [k, v] of Object.entries(out.headers)) {
        if (typeof v === "string") {
          hdrs[k] = v;
        }
      }
    }
    return { status, body, headers: hdrs };
  }
  return { status: 200, body: String(out), headers: {} };
}

/**
 * Invokes a registerGatewayMethod handler (OpenClaw-style ctx.params + ctx.respond, or return value).
 */
async function handleGatewayMethod(params) {
  const pluginId = typeof params.pluginId === "string" ? params.pluginId.trim() : "";
  const method = typeof params.method === "string" ? params.method.trim() : "";
  if (!pluginId || !method) {
    throw new Error("gatewayMethod: need pluginId and method");
  }
  const key = `${pluginId}\0${method}`;
  const handler = globalGatewayMethodHandlers.get(key);
  if (!handler) {
    throw new Error(`no gateway method handler for ${pluginId} ${method}`);
  }
  const inbound =
    params.params && typeof params.params === "object" && !Array.isArray(params.params) ? params.params : {};
  let responded = false;
  let gatewayOk = true;
  let outPayload = null;
  let outError = null;
  const respond = (ok, payload, err) => {
    responded = true;
    gatewayOk = !!ok;
    outPayload = payload !== undefined ? payload : null;
    outError = err != null && err !== undefined ? err : null;
  };
  const stubCtx = {
    params: inbound,
    respond,
    req: { method: "pinchbot.gateway", params: [] },
    client: null,
    isWebchatConnect: () => false,
    context: {},
  };
  let directResult;
  try {
    directResult = await Promise.resolve(handler(stubCtx));
  } catch (e) {
    const errMsg = e && typeof e === "object" && "message" in e ? String(e.message) : String(e);
    throw new Error(`gatewayMethod handler error: ${errMsg}`);
  }
  if (responded) {
    return {
      responded: true,
      gatewayOk,
      payload: outPayload,
      gatewayError: outError,
    };
  }
  if (directResult !== undefined && directResult !== null) {
    return {
      responded: true,
      gatewayOk: true,
      payload: directResult,
      gatewayError: null,
    };
  }
  return { responded: false, gatewayOk: true, payload: null, gatewayError: null };
}

async function handleContextOp(params) {
  const op = typeof params.op === "string" ? params.op.trim() : "";
  if (op === "emit") {
    const event = typeof params.event === "string" ? params.event.trim() : "";
    if (!event) {
      throw new Error("emit: missing event");
    }
    const eventPayload = params.eventPayload && typeof params.eventPayload === "object" ? params.eventPayload : {};
    const ctx = params.ctx && typeof params.ctx === "object" ? params.ctx : {};
    const handlers = globalEventHandlers.get(event) || [];
    for (const h of handlers) {
      await h(eventPayload, ctx);
    }
    return { ok: true };
  }
  if (op === "assemble") {
    const engineId =
      (typeof params.engineId === "string" && params.engineId.trim()) || "graph-memory";
    const eng = globalContextEngines.get(engineId);
    if (!eng || typeof eng.assemble !== "function") {
      throw new Error(`no context engine assemble: ${engineId}`);
    }
    return await eng.assemble({
      sessionId: params.sessionId,
      messages: Array.isArray(params.messages) ? params.messages : [],
      tokenBudget: params.tokenBudget,
    });
  }
  if (op === "afterTurn") {
    const engineId =
      (typeof params.engineId === "string" && params.engineId.trim()) || "graph-memory";
    const eng = globalContextEngines.get(engineId);
    if (!eng || typeof eng.afterTurn !== "function") {
      throw new Error(`no context engine afterTurn: ${engineId}`);
    }
    return await eng.afterTurn({
      sessionId: params.sessionId,
      messages: Array.isArray(params.messages) ? params.messages : [],
      prePromptMessageCount: params.prePromptMessageCount,
      isHeartbeat: !!params.isHeartbeat,
    });
  }
  throw new Error(`unknown contextOp: ${op}`);
}

const rl = readline.createInterface({ input: process.stdin, terminal: false });
rl.on("line", async (line) => {
  const trimmed = line.trim();
  if (!trimmed) {
    return;
  }
  let msg;
  try {
    msg = JSON.parse(trimmed);
  } catch {
    send({ id: null, ok: false, error: "invalid json" });
    return;
  }
  const id = msg.id;
  try {
    let result;
    if (msg.method === "init") {
      result = await handleInit(msg.params || {});
    } else if (msg.method === "execute") {
      result = await handleExecute(msg.params || {});
    } else if (msg.method === "contextOp") {
      result = await handleContextOp(msg.params || {});
    } else if (msg.method === "httpRoute") {
      result = await handleHttpRoute(msg.params || {});
    } else if (msg.method === "gatewayMethod") {
      result = await handleGatewayMethod(msg.params || {});
    } else {
      throw new Error(`unknown method: ${msg.method}`);
    }
    send({ id, ok: true, result });
  } catch (e) {
    const errMsg = e && typeof e === "object" && "message" in e ? String(e.message) : String(e);
    send({ id, ok: false, error: errMsg });
  }
});
