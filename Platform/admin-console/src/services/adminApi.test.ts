import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { adminApi } from './adminApi';
import { AdminApiError } from './http';

describe('adminApi', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sends same-origin credentials for cookie-backed admin session requests', async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          user: { id: 'admin-1', email: 'admin@example.com' },
          operator: { email: 'admin@example.com', role: 'super_admin', active: true, capabilities: ['dashboard.read'] },
        }),
        {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        },
      ),
    );

    await adminApi.getSession();

    expect(fetch).toHaveBeenCalledWith(
      '/admin/session',
      expect.objectContaining({
        credentials: 'include',
        method: 'GET',
      }),
    );
  });

  it('normalizes 401 and 403 responses into user-visible api errors', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response('missing administrator session', { status: 401 }));
    await expect(adminApi.getSession()).rejects.toEqual(
      expect.objectContaining<Partial<AdminApiError>>({
        status: 401,
        code: 'unauthorized',
        message: 'missing administrator session',
      }),
    );

    vi.mocked(fetch).mockResolvedValueOnce(new Response('admin capability denied', { status: 403 }));
    await expect(adminApi.getSession()).rejects.toEqual(
      expect.objectContaining<Partial<AdminApiError>>({
        status: 403,
        code: 'forbidden',
        message: 'admin capability denied',
      }),
    );
  });

  it('captures revision metadata for configuration resources', async () => {
    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          official_routes: [],
          official_models: [],
          pricing_rules: [],
          agreements: [],
        }),
        {
          status: 200,
          headers: {
            'Content-Type': 'application/json',
            'X-Resource-Version': 'rev-123',
          },
        },
      ),
    );

    const response = await adminApi.getRuntimeConfig();

    expect(response.revision).toBe('rev-123');
    expect(response.data).toEqual({
      official_routes: [],
      official_models: [],
      pricing_rules: [],
      agreements: [],
    });
  });
});
