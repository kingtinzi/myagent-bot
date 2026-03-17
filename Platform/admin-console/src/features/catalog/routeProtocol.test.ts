import { describe, expect, it } from 'vitest';

import {
  createEmptyRouteDraft,
  inferRouteProtocol,
  normalizeRouteDrafts,
  redactedSecretPlaceholder,
  serializeRouteDrafts,
} from './routeProtocol';

describe('routeProtocol helpers', () => {
  it('defaults new route drafts to responses protocol', () => {
    expect(createEmptyRouteDraft().protocol).toBe('responses');
  });

  it('infers known protocols and preserves unknown protocols as custom', () => {
    expect(inferRouteProtocol('responses/gpt-5.2')).toMatchObject({
      protocol: 'responses',
      modelValue: 'gpt-5.2',
    });

    expect(inferRouteProtocol('openai/gpt-4.1')).toMatchObject({
      protocol: 'openai',
      modelValue: 'gpt-4.1',
    });

    expect(inferRouteProtocol('volcengine/deepseek-v3')).toMatchObject({
      protocol: 'custom',
      modelValue: 'volcengine/deepseek-v3',
    });
  });

  it('round-trips structured route drafts while preserving existing secret placeholders', () => {
    const drafts = normalizeRouteDrafts([
      {
        public_model_id: 'official-gpt-5-2',
        model_config: {
          model_name: '官方 GPT-5.2',
          model: 'responses/gpt-5.2',
          api_base: 'https://gateway.example.com/v1',
          api_key: redactedSecretPlaceholder,
          request_timeout: 30,
          thinking_level: 'high',
          proxy: '',
          custom_flag: true,
        },
      },
    ]);

    expect(drafts[0]).toMatchObject({
      protocol: 'responses',
      modelValue: 'gpt-5.2',
      keepExistingSecret: true,
      persistedApiKey: redactedSecretPlaceholder,
    });

    const serialized = serializeRouteDrafts(drafts);
    expect(serialized[0]).toMatchObject({
      public_model_id: 'official-gpt-5-2',
      model_config: {
        model_name: '官方 GPT-5.2',
        model: 'responses/gpt-5.2',
        api_base: 'https://gateway.example.com/v1',
        api_key: redactedSecretPlaceholder,
        request_timeout: 30,
        thinking_level: 'high',
        custom_flag: true,
      },
    });
  });
});
