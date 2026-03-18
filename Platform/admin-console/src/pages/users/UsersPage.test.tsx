import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { UsersPage } from './UsersPage';
import { useSessionStore } from '../../stores/sessionStore';

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>
  );
}

describe('UsersPage', () => {
  beforeEach(() => {
    useSessionStore.getState().setSession({
      user: { id: 'admin-1', email: 'admin@example.com' },
      operator: {
        email: 'admin@example.com',
        role: 'super_admin',
        active: true,
        capabilities: ['users.read', 'wallet.read', 'orders.read', 'agreements.read', 'usage.read'],
      },
    });

    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      const url = String(input);

      if (url.includes('/admin/users?')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                user_id: 'user-1',
                user_no: 1001,
                username: '俊夫',
                email: 'kingtinzi@sina.com',
                balance_fen: 5600,
                currency: 'CNY',
                updated_unix: 1710000000,
                order_count: 3,
              },
            ]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.endsWith('/admin/users/user-1/overview')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              user: {
                user_id: 'user-1',
                user_no: 1001,
                username: '俊夫',
                email: 'kingtinzi@sina.com',
                balance_fen: 5600,
                currency: 'CNY',
                updated_unix: 1710000000,
                order_count: 3,
              },
              wallet: { user_id: 'user-1', balance_fen: 5600, currency: 'CNY', updated_unix: 1710000000 },
              recent_orders: [],
              recent_transactions: [],
              agreements: [],
              recent_usage: [],
              pending_refund_count: 1,
              pending_infringement_count: 0,
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.endsWith('/admin/users/user-1/orders')) {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { 'Content-Type': 'application/json' } }));
      }

      if (url.endsWith('/admin/users/user-1/wallet-transactions')) {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { 'Content-Type': 'application/json' } }));
      }

      if (url.endsWith('/admin/users/user-1/agreements')) {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { 'Content-Type': 'application/json' } }));
      }

      if (url.endsWith('/admin/users/user-1/usage')) {
        return Promise.resolve(new Response(JSON.stringify([]), { status: 200, headers: { 'Content-Type': 'application/json' } }));
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    }) as typeof fetch);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useSessionStore.getState().clearSession();
  });

  it('searches by keyword, shows username and number, and opens the detail drawer', async () => {
    const user = userEvent.setup();
    await act(async () => {
      render(<UsersPage />, { wrapper: createWrapper() });
    });

    await user.type(screen.getByPlaceholderText('用户名 / 用户编号 / 邮箱'), '俊夫');
    await user.click(screen.getByRole('button', { name: '搜索用户' }));

    expect(await screen.findByText('俊夫')).toBeInTheDocument();
    expect(screen.getByText('#1001')).toBeInTheDocument();

    await act(async () => {
      await user.click(screen.getByText('俊夫'));
    });

    expect(await screen.findByRole('dialog', { name: '俊夫' })).toBeInTheDocument();
    expect(await screen.findByText('56.00')).toBeInTheDocument();
    expect(await screen.findByRole('tab', { name: '概览' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '钱包' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '订单' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '手动充值' })).toBeInTheDocument();
  });
});
