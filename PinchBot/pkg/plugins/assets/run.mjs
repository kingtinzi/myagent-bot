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

/** @type {Map<string, Array<(event:any, ctx:any) => any>>} */
let globalEventHandlers = new Map();
/** @type {Map<string, any>} */
let globalContextEngines = new Map();

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

function createApi(pluginId, sandboxed, pluginConfig, pinchbotConfig) {
  const cfg = pluginConfig && typeof pluginConfig === "object" ? pluginConfig : {};
  const pbc = pinchbotConfig && typeof pinchbotConfig === "object" ? pinchbotConfig : {};
  return {
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
    runtime: { version: "pinchbot", agent: {} },
    on(event, handler) {
      if (typeof event !== "string" || !event.trim() || typeof handler !== "function") {
        return;
      }
      const ev = event.trim();
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
  agentWorkspace = typeof params.workspace === "string" && params.workspace.trim() ? params.workspace : process.cwd();
  const sandboxed = !!params.sandboxed;
  const plugins = Array.isArray(params.plugins) ? params.plugins : [];
  const pinchbotConfig =
    params.pinchbotConfig && typeof params.pinchbotConfig === "object" ? params.pinchbotConfig : {};

  for (const p of plugins) {
    const pluginId = typeof p.id === "string" ? p.id.trim() : "";
    const root = typeof p.root === "string" ? p.root.trim() : "";
    if (!pluginId || !root) {
      throw new Error("each plugin needs id and root");
    }
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
      throw new Error(`plugin ${pluginId}: missing register()`);
    }
    const api = createApi(pluginId, sandboxed, pluginConfig, pinchbotConfig);
    await Promise.resolve(entry.register(api));
  }
  return { tools: catalog };
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
  const prev = process.cwd();
  try {
    process.chdir(agentWorkspace);
    const result = await inst.execute("", args);
    return { content: toolResultToContent(result) };
  } finally {
    try {
      process.chdir(prev);
    } catch {
      /* ignore */
    }
  }
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
    } else {
      throw new Error(`unknown method: ${msg.method}`);
    }
    send({ id, ok: true, result });
  } catch (e) {
    const errMsg = e && typeof e === "object" && "message" in e ? String(e.message) : String(e);
    send({ id, ok: false, error: errMsg });
  }
});
