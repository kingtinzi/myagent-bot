import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { UserDetailDrawer } from './UserDetailDrawer';
import { useSessionStore } from '../../stores/sessionStore';

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

describe('UserDetailDrawer', () => {
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

      if (url.endsWith('/admin/users/user-1/wallet-transactions')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ id: 'tx-1', user_id: 'user-1', kind: 'credit', amount_fen: 5000, description: 'admin recharge', created_unix: 1710000000 }]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.endsWith('/admin/users/user-1/orders')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ id: 'ord-1', user_id: 'user-1', amount_fen: 5000, channel: 'alipay', status: 'paid', created_unix: 1710000000 }]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.endsWith('/admin/users/user-1/agreements')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ user_id: 'user-1', agreement_key: 'privacy', version: 'v1', accepted_unix: 1710000000 }]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.endsWith('/admin/users/user-1/usage')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ id: 'usage-1', user_id: 'user-1', model_id: 'gpt-5.2', charged_fen: 3, created_unix: 1710000000 }]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    }) as typeof fetch);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useSessionStore.getState().clearSession();
  });

  it('switches between overview, wallet, orders, agreements, and usage tabs', async () => {
    const user = userEvent.setup();
    await act(async () => {
      render(
        <UserDetailDrawer onClose={() => undefined} onOpenOrders={() => undefined} onOpenWallet={() => undefined} open userId="user-1" />,
        { wrapper: createWrapper() },
      );
    });

    expect(await screen.findByRole('dialog', { name: '俊夫' })).toBeInTheDocument();
    expect(await screen.findByText('56.00')).toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: '钱包' }));
    expect(await screen.findByText('admin recharge')).toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: '订单' }));
    expect(await screen.findByText('ord-1')).toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: '协议' }));
    expect(await screen.findByText('privacy')).toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: '用量' }));
    expect(await screen.findByText('gpt-5.2')).toBeInTheDocument();
  });
});
