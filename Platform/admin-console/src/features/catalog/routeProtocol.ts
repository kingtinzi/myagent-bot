import type { OfficialRoute, RouteModelConfig } from '../../services/contracts';

export const redactedSecretPlaceholder = '__KEEP_EXISTING_SECRET__';

export type RouteProtocolOption = {
  value: string;
  label: string;
  prefix: string;
  hint: string;
};

export type RouteDraft = {
  publicModelID: string;
  modelName: string;
  protocol: string;
  modelValue: string;
  apiBase: string;
  apiKeyDraft: string;
  persistedApiKey: string;
  keepExistingSecret: boolean;
  proxy: string;
  authMethod: string;
  connectMode: string;
  workspace: string;
  rpm: string;
  maxTokensField: string;
  requestTimeout: string;
  thinkingLevel: string;
  extraConfigJSON: string;
};

const structuredRouteKeys = new Set([
  'model_name',
  'model',
  'api_base',
  'api_key',
  'proxy',
  'auth_method',
  'connect_mode',
  'workspace',
  'rpm',
  'max_tokens_field',
  'request_timeout',
  'thinking_level',
]);

export function routeProtocolOptions(): RouteProtocolOption[] {
  return [
    { value: 'responses', label: 'Responses API（/responses）', prefix: 'responses/', hint: '适合仅提供 /responses 的上游。' },
    { value: 'openai', label: 'OpenAI Chat Completions（/chat/completions）', prefix: 'openai/', hint: '适合标准 OpenAI 兼容聊天接口。' },
    { value: 'openai-responses', label: 'OpenAI Responses Compatible', prefix: 'openai-responses/', hint: '适合 OpenAI Responses 兼容实现。' },
    { value: 'official', label: 'Platform Official Proxy（/chat/official）', prefix: 'official/', hint: '适合继续转发到另一个官方平台代理。' },
    { value: 'litellm', label: 'LiteLLM / 聚合代理', prefix: 'litellm/', hint: '适合走 LiteLLM 或聚合协议前缀。' },
    { value: 'custom', label: '自定义 / 其他', prefix: '', hint: '保留完整 model 字符串，适配未收录的协议前缀。' },
  ];
}

export function routeProtocolMeta(protocol: string) {
  const normalized = String(protocol ?? '').trim().toLowerCase();
  return routeProtocolOptions().find(item => item.value === normalized) ?? routeProtocolOptions()[routeProtocolOptions().length - 1];
}

export function inferRouteProtocol(model: string) {
  const raw = String(model ?? '').trim();
  if (!raw) {
    return { protocol: 'responses', modelValue: '', fullModel: '' };
  }

  const slashIndex = raw.indexOf('/');
  if (slashIndex <= 0) {
    return { protocol: 'custom', modelValue: raw, fullModel: raw };
  }

  const protocol = raw.slice(0, slashIndex).trim().toLowerCase();
  const modelValue = raw.slice(slashIndex + 1).trim();

  if (routeProtocolMeta(protocol).value === protocol && protocol !== 'custom') {
    return {
      protocol,
      modelValue,
      fullModel: raw,
    };
  }

  return {
    protocol: 'custom',
    modelValue: raw,
    fullModel: raw,
  };
}

function pickExtraConfig(modelConfig: RouteModelConfig) {
  const extras: Record<string, unknown> = {};

  Object.entries(modelConfig ?? {}).forEach(([key, value]) => {
    if (structuredRouteKeys.has(key)) {
      return;
    }
    extras[key] = value;
  });

  return extras;
}

function normalizeNumericString(value: unknown) {
  if (value === undefined || value === null || value === '') {
    return '';
  }
  return String(value);
}

export function createEmptyRouteDraft(): RouteDraft {
  return {
    publicModelID: '',
    modelName: '',
    protocol: 'responses',
    modelValue: '',
    apiBase: '',
    apiKeyDraft: '',
    persistedApiKey: '',
    keepExistingSecret: false,
    proxy: '',
    authMethod: '',
    connectMode: '',
    workspace: '',
    rpm: '',
    maxTokensField: '',
    requestTimeout: '',
    thinkingLevel: '',
    extraConfigJSON: '{}',
  };
}

export function normalizeRouteDrafts(items: OfficialRoute[]) {
  return items.map(item => {
    const inferred = inferRouteProtocol(item.model_config.model);
    const persistedApiKey = String(item.model_config.api_key ?? '').trim();

    return {
      publicModelID: item.public_model_id,
      modelName: item.model_config.model_name,
      protocol: inferred.protocol,
      modelValue: inferred.modelValue,
      apiBase: String(item.model_config.api_base ?? ''),
      apiKeyDraft: '',
      persistedApiKey,
      keepExistingSecret: persistedApiKey !== '' && persistedApiKey === redactedSecretPlaceholder,
      proxy: String(item.model_config.proxy ?? ''),
      authMethod: String(item.model_config.auth_method ?? ''),
      connectMode: String(item.model_config.connect_mode ?? ''),
      workspace: String(item.model_config.workspace ?? ''),
      rpm: normalizeNumericString(item.model_config.rpm),
      maxTokensField: String(item.model_config.max_tokens_field ?? ''),
      requestTimeout: normalizeNumericString(item.model_config.request_timeout),
      thinkingLevel: String(item.model_config.thinking_level ?? ''),
      extraConfigJSON: JSON.stringify(pickExtraConfig(item.model_config), null, 2),
    } satisfies RouteDraft;
  });
}

function buildRouteModelValue(draft: RouteDraft) {
  const protocol = String(draft.protocol ?? '').trim().toLowerCase();
  const modelValue = String(draft.modelValue ?? '').trim();

  if (!modelValue) {
    return '';
  }

  if (protocol === 'custom') {
    return modelValue;
  }

  return `${routeProtocolMeta(protocol).prefix}${modelValue}`;
}

function parseOptionalInteger(value: string) {
  const trimmed = String(value ?? '').trim();
  if (!trimmed) {
    return undefined;
  }

  const parsed = Number.parseInt(trimmed, 10);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function parseExtraConfig(jsonText: string) {
  const trimmed = String(jsonText ?? '').trim();
  if (!trimmed) {
    return {};
  }

  return JSON.parse(trimmed) as Record<string, unknown>;
}

export function serializeRouteDrafts(drafts: RouteDraft[]): OfficialRoute[] {
  return drafts.map(draft => {
    const extraConfig = parseExtraConfig(draft.extraConfigJSON);
    const nextModelConfig: RouteModelConfig = {
      ...extraConfig,
      model_name: draft.modelName.trim(),
      model: buildRouteModelValue(draft),
      api_base: draft.apiBase.trim(),
      api_key: draft.apiKeyDraft.trim() || (draft.keepExistingSecret ? draft.persistedApiKey.trim() : ''),
      proxy: draft.proxy.trim(),
      auth_method: draft.authMethod.trim(),
      connect_mode: draft.connectMode.trim(),
      workspace: draft.workspace.trim(),
      max_tokens_field: draft.maxTokensField.trim(),
      thinking_level: draft.thinkingLevel.trim(),
    };

    const rpm = parseOptionalInteger(draft.rpm);
    const requestTimeout = parseOptionalInteger(draft.requestTimeout);

    if (rpm !== undefined) {
      nextModelConfig.rpm = rpm;
    } else {
      delete nextModelConfig.rpm;
    }

    if (requestTimeout !== undefined) {
      nextModelConfig.request_timeout = requestTimeout;
    } else {
      delete nextModelConfig.request_timeout;
    }

    if (!nextModelConfig.api_base) delete nextModelConfig.api_base;
    if (!nextModelConfig.api_key) delete nextModelConfig.api_key;
    if (!nextModelConfig.proxy) delete nextModelConfig.proxy;
    if (!nextModelConfig.auth_method) delete nextModelConfig.auth_method;
    if (!nextModelConfig.connect_mode) delete nextModelConfig.connect_mode;
    if (!nextModelConfig.workspace) delete nextModelConfig.workspace;
    if (!nextModelConfig.max_tokens_field) delete nextModelConfig.max_tokens_field;
    if (!nextModelConfig.thinking_level) delete nextModelConfig.thinking_level;

    return {
      public_model_id: draft.publicModelID.trim(),
      model_config: nextModelConfig,
    };
  });
}
