import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialog } from '../../components/feedback/ConfirmDialog';
import { useSessionStore } from '../../stores/sessionStore';
import { GovernancePage } from './GovernancePage';

function createWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{children}</MemoryRouter>
      <ConfirmDialog />
    </QueryClientProvider>
  );
}

describe('GovernancePage', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useSessionStore.getState().clearSession();
  });

  it('loads system notices first and can switch to risk rules', async () => {
    const user = userEvent.setup();

    useSessionStore.getState().setSession({
      user: { id: 'gov-1', email: 'gov@example.com' },
      operator: {
        email: 'gov@example.com',
        role: 'governance',
        active: true,
        capabilities: ['notices.read', 'risk.read'],
      },
    });

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.endsWith('/admin/system-notices') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: 'notice-1',
                title: '充值说明',
                body: '充值资金仅用于官方模型调用。',
                severity: 'info',
                enabled: true,
              },
            ]),
            {
              status: 200,
              headers: { 'Content-Type': 'application/json', 'x-resource-version': 'notices-rev-1' },
            },
          ),
        );
      }

      if (url.endsWith('/admin/risk-rules') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                key: 'manual_review',
                name: '人工复核',
                description: '命中高风险订单时需要人工复核。',
                enabled: true,
              },
            ]),
            {
              status: 200,
              headers: { 'Content-Type': 'application/json', 'x-resource-version': 'risk-rev-1' },
            },
          ),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });

    await act(async () => {
      render(<GovernancePage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByRole('heading', { name: '系统公告' })).toBeInTheDocument();
    expect(await screen.findByDisplayValue('充值说明')).toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: '风控规则' }));

    expect(await screen.findByRole('heading', { name: '风控规则' })).toBeInTheDocument();
    expect(await screen.findByDisplayValue('人工复核')).toBeInTheDocument();
  });

  it('saves system notices with If-Match revision header after confirmation', async () => {
    const user = userEvent.setup();
    let saveIfMatchHeader: string | null = null;

    useSessionStore.getState().setSession({
      user: { id: 'gov-2', email: 'notice-admin@example.com' },
      operator: {
        email: 'notice-admin@example.com',
        role: 'governance',
        active: true,
        capabilities: ['notices.read', 'notices.write'],
      },
    });

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.endsWith('/admin/system-notices') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: 'notice-1',
                title: '充值说明',
                body: '充值资金仅用于官方模型调用。',
                severity: 'info',
                enabled: true,
              },
            ]),
            {
              status: 200,
              headers: { 'Content-Type': 'application/json', 'x-resource-version': 'notices-rev-1' },
            },
          ),
        );
      }

      if (url.endsWith('/admin/system-notices') && init?.method === 'PUT') {
        saveIfMatchHeader = new Headers(init.headers).get('If-Match');
        return Promise.resolve(
          new Response(
            init.body as BodyInit,
            {
              status: 200,
              headers: { 'Content-Type': 'application/json', 'x-resource-version': 'notices-rev-2' },
            },
          ),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });

    await act(async () => {
      render(<GovernancePage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByRole('heading', { name: '系统公告' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: '保存系统公告' }));
    expect(await screen.findByRole('dialog', { name: '保存系统公告' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: '确认保存' }));

    expect(await screen.findByText('系统公告已保存。')).toBeInTheDocument();
    expect(saveIfMatchHeader).toBe('notices-rev-1');
  });

  it('shows only readable tabs and disables write actions in read-only mode', async () => {
    useSessionStore.getState().setSession({
      user: { id: 'gov-3', email: 'retention@example.com' },
      operator: {
        email: 'retention@example.com',
        role: 'read_only',
        active: true,
        capabilities: ['retention.read'],
      },
    });

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.endsWith('/admin/data-retention-policies') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                data_domain: 'audit_logs',
                retention_days: 365,
                purge_mode: 'archive',
                description: '审计日志保留一年后归档。',
                enabled: true,
              },
            ]),
            {
              status: 200,
              headers: { 'Content-Type': 'application/json', 'x-resource-version': 'retention-rev-1' },
            },
          ),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });

    await act(async () => {
      render(<GovernancePage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByRole('heading', { name: '数据保留策略' })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: '系统公告' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: '新增策略' })).toBeDisabled();
    expect(screen.getByRole('button', { name: '保存数据保留策略' })).toBeDisabled();
  });
});
