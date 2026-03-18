import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialog } from '../../components/feedback/ConfirmDialog';
import { OperatorsPage } from './OperatorsPage';
import { useSessionStore } from '../../stores/sessionStore';

function createWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{children}</MemoryRouter>
      <ConfirmDialog />
    </QueryClientProvider>
  );
}

function createOperators() {
  return [
    {
      email: 'root@example.com',
      role: 'super_admin',
      active: true,
      capabilities: ['dashboard.read', 'users.read', 'operators.read', 'operators.write'],
      updated_unix: 1710000000,
    },
    {
      email: 'reader@example.com',
      role: 'read_only',
      active: true,
      capabilities: ['dashboard.read', 'users.read', 'operators.read'],
      updated_unix: 1710000001,
    },
  ];
}

describe('OperatorsPage', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useSessionStore.getState().clearSession();
  });

  it('lists operators, previews role capabilities, and saves role changes through confirmation', async () => {
    const user = userEvent.setup();
    let operators = createOperators();

    useSessionStore.getState().setSession({
      user: { id: 'admin-1', email: 'admin@example.com' },
      operator: {
        email: 'admin@example.com',
        role: 'super_admin',
        active: true,
        capabilities: ['operators.read', 'operators.write'],
      },
    });

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.endsWith('/admin/operators') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(new Response(JSON.stringify(operators), { status: 200, headers: { 'Content-Type': 'application/json' } }));
      }

      if (url.endsWith('/admin/operators/reader%40example.com') && init?.method === 'PUT') {
        operators = operators.map(item =>
          item.email === 'reader@example.com'
            ? {
                ...item,
                role: 'finance',
                capabilities: ['dashboard.read', 'users.read', 'orders.read', 'orders.write', 'wallet.read', 'wallet.write', 'audit.read', 'refunds.read', 'refunds.review'],
              }
            : item,
        );

        return Promise.resolve(
          new Response(JSON.stringify(operators[1]), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          }),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });

    await act(async () => {
      render(<OperatorsPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByText('reader@example.com')).toBeInTheDocument();
    expect(screen.getAllByText('只读管理员').length).toBeGreaterThan(0);

    await user.click(screen.getByText('reader@example.com'));
    expect(screen.getByRole('heading', { name: '角色权限预览' })).toBeInTheDocument();
    expect(screen.getAllByText('只读巡检与排障角色，只能查看后台数据，不能执行写操作。').length).toBeGreaterThan(0);

    await user.selectOptions(screen.getByRole('combobox'), 'finance');
    expect(screen.getAllByText('面向充值、钱包、退款场景，负责对账、人工入账与退款审核。').length).toBeGreaterThan(0);
    await user.click(screen.getByRole('button', { name: '保存管理员' }));

    expect(await screen.findByRole('dialog', { name: '管理员角色变更' })).toBeInTheDocument();
    expect(screen.getByText('即将保存管理员 reader@example.com。')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: '确认保存' }));

    expect(await screen.findByText('管理员已保存。')).toBeInTheDocument();
    expect(screen.getAllByText('财务管理员').length).toBeGreaterThan(0);
  });

  it('shows read-only guidance when the current operator lacks write capability', async () => {
    useSessionStore.getState().setSession({
      user: { id: 'admin-1', email: 'readonly@example.com' },
      operator: {
        email: 'readonly@example.com',
        role: 'read_only',
        active: true,
        capabilities: ['operators.read'],
      },
    });

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.endsWith('/admin/operators') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(new Response(JSON.stringify(createOperators()), { status: 200, headers: { 'Content-Type': 'application/json' } }));
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });

    await act(async () => {
      render(<OperatorsPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByText('root@example.com')).toBeInTheDocument();
    expect(screen.getByText('当前账号仅可巡检管理员目录，不能修改角色与启用状态。')).toBeInTheDocument();
    expect(screen.getByText('需要 operators.write 权限才能保存管理员角色与状态。')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '保存管理员' })).not.toBeInTheDocument();
  });
});
