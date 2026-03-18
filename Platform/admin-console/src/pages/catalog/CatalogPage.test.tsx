import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialog } from '../../components/feedback/ConfirmDialog';
import { useSessionStore } from '../../stores/sessionStore';
import { CatalogPage } from './CatalogPage';

function createWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{children}</MemoryRouter>
      <ConfirmDialog />
    </QueryClientProvider>
  );
}

describe('CatalogPage', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useSessionStore.getState().clearSession();
  });

  it('loads official models first and can switch to official routes', async () => {
    const user = userEvent.setup();

    useSessionStore.getState().setSession({
      user: { id: 'admin-1', email: 'catalog@example.com' },
      operator: {
        email: 'catalog@example.com',
        role: 'operations',
        active: true,
        capabilities: ['models.read', 'routes.read'],
      },
    });

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.endsWith('/admin/models') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: 'official-gpt-5-2',
                name: '官方 GPT-5.2',
                description: '默认模型',
                enabled: true,
                pricing_version: 'v20260317',
              },
            ]),
            {
              status: 200,
              headers: { 'Content-Type': 'application/json', 'x-resource-version': 'models-rev-1' },
            },
          ),
        );
      }

      if (url.endsWith('/admin/model-routes') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                public_model_id: 'official-gpt-5-2',
                model_config: {
                  model_name: '官方 GPT-5.2',
                  model: 'responses/gpt-5.2',
                },
              },
            ]),
            {
              status: 200,
              headers: { 'Content-Type': 'application/json', 'x-resource-version': 'routes-rev-1' },
            },
          ),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });

    await act(async () => {
      render(<CatalogPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByRole('heading', { name: '官方模型目录' })).toBeInTheDocument();
    expect(screen.getByDisplayValue('官方 GPT-5.2')).toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: '官方路由' }));

    expect(await screen.findByRole('heading', { name: '官方路由' })).toBeInTheDocument();
    expect(screen.getAllByDisplayValue('官方 GPT-5.2').length).toBeGreaterThan(0);
  });

  it('saves routes with If-Match revision header after confirmation', async () => {
    const user = userEvent.setup();
    let saveIfMatchHeader: string | null = null;

    useSessionStore.getState().setSession({
      user: { id: 'admin-2', email: 'routes@example.com' },
      operator: {
        email: 'routes@example.com',
        role: 'operations',
        active: true,
        capabilities: ['routes.read', 'routes.write'],
      },
    });

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.endsWith('/admin/model-routes') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                public_model_id: 'official-gpt-5-2',
                model_config: {
                  model_name: '官方 GPT-5.2',
                  model: 'responses/gpt-5.2',
                },
              },
            ]),
            {
              status: 200,
              headers: { 'Content-Type': 'application/json', 'x-resource-version': 'routes-rev-1' },
            },
          ),
        );
      }

      if (url.endsWith('/admin/model-routes') && init?.method === 'PUT') {
        saveIfMatchHeader = new Headers(init.headers).get('If-Match');
        return Promise.resolve(
          new Response(
            init.body as BodyInit,
            {
              status: 200,
              headers: { 'Content-Type': 'application/json', 'x-resource-version': 'routes-rev-2' },
            },
          ),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });

    await act(async () => {
      render(<CatalogPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByRole('heading', { name: '官方路由' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: '保存官方路由' }));
    expect(await screen.findByRole('dialog', { name: '保存官方路由' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: '确认保存' }));

    expect(await screen.findByText('官方路由已保存。')).toBeInTheDocument();
    expect(saveIfMatchHeader).toBe('routes-rev-1');
  });

  it('shows only readable tabs and disables write actions in read-only mode', async () => {
    useSessionStore.getState().setSession({
      user: { id: 'admin-3', email: 'pricing@example.com' },
      operator: {
        email: 'pricing@example.com',
        role: 'read_only',
        active: true,
        capabilities: ['pricing.read'],
      },
    });

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.endsWith('/admin/pricing-rules') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                model_id: 'official-gpt-5-2',
                version: 'v1',
                effective_from_unix: 1710000000,
                input_price_micros_per_1k: 3000,
                output_price_micros_per_1k: 6000,
                fallback_price_fen: 30,
              },
            ]),
            {
              status: 200,
              headers: { 'Content-Type': 'application/json', 'x-resource-version': 'pricing-rev-1' },
            },
          ),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });

    await act(async () => {
      render(<CatalogPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByRole('heading', { name: '定价规则' })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: '官方模型' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: '新增规则' })).toBeDisabled();
    expect(screen.getByRole('button', { name: '保存定价规则' })).toBeDisabled();
  });
});
