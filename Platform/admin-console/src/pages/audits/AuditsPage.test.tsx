import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useSessionStore } from '../../stores/sessionStore';
import { AuditsPage } from './AuditsPage';

function createWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>
  );
}

describe('AuditsPage', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
    vi.stubGlobal('open', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useSessionStore.getState().clearSession();
  });

  it('applies filters and shows audit details for the selected row', async () => {
    const user = userEvent.setup();

    useSessionStore.getState().setSession({
      user: { id: 'audit-1', email: 'audit@example.com' },
      operator: {
        email: 'audit@example.com',
        role: 'super_admin',
        active: true,
        capabilities: ['audit.read'],
      },
    });

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);

      if (url.includes('/admin/audit-logs?action=admin.manual_recharge.created&risk_level=high&limit=50')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: 'audit-2',
                actor_user_id: 'admin-1',
                actor_email: 'ops@example.com',
                action: 'admin.manual_recharge.created',
                target_type: 'wallet_account',
                target_id: 'user-2',
                risk_level: 'high',
                detail: '管理员手动充值。',
                created_unix: 1710000001,
              },
            ]),
            {
              status: 200,
              headers: { 'Content-Type': 'application/json' },
            },
          ),
        );
      }

      if (url.includes('/admin/audit-logs?limit=50')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: 'audit-1',
                actor_user_id: 'admin-0',
                actor_email: 'root@example.com',
                action: 'admin.login',
                target_type: 'admin_session',
                target_id: 'session-1',
                risk_level: 'low',
                detail: '管理员登录后台。',
                created_unix: 1710000000,
              },
            ]),
            {
              status: 200,
              headers: { 'Content-Type': 'application/json' },
            },
          ),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });

    await act(async () => {
      render(<AuditsPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByText('admin.login')).toBeInTheDocument();
    expect(screen.getByText('管理员登录后台。')).toBeInTheDocument();

    await user.type(screen.getByLabelText('动作'), 'admin.manual_recharge.created');
    await user.selectOptions(screen.getByLabelText('风险等级'), 'high');
    await user.click(screen.getByRole('button', { name: '应用筛选' }));

    expect(await screen.findByText('管理员手动充值。')).toBeInTheDocument();
    expect(screen.getAllByText('admin.manual_recharge.created').length).toBeGreaterThan(0);

    await user.click(screen.getAllByText('admin.manual_recharge.created')[0]);
    expect(screen.getAllByText('wallet_account / user-2').length).toBeGreaterThan(0);
    expect(screen.getAllByText('高风险').length).toBeGreaterThan(0);
  });

  it('exports current audit filters as csv', async () => {
    const user = userEvent.setup();

    useSessionStore.getState().setSession({
      user: { id: 'audit-2', email: 'audit-export@example.com' },
      operator: {
        email: 'audit-export@example.com',
        role: 'super_admin',
        active: true,
        capabilities: ['audit.read'],
      },
    });

    vi.mocked(fetch).mockImplementation(() =>
      Promise.resolve(
        new Response(JSON.stringify([]), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    );

    await act(async () => {
      render(<AuditsPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByText('当前筛选条件下未找到审计日志。')).toBeInTheDocument();

    await user.type(screen.getByLabelText('动作'), 'admin.order.reconcile');
    await user.click(screen.getByRole('button', { name: '应用筛选' }));
    await user.click(screen.getByRole('button', { name: '导出 CSV' }));

    expect(globalThis.open).toHaveBeenCalledWith(
      expect.stringContaining('/admin/audit-logs?action=admin.order.reconcile&limit=50&format=csv'),
      '_blank',
      'noopener,noreferrer',
    );
    expect(await screen.findByText('已发起审计日志 CSV 导出。')).toBeInTheDocument();
  });
});
