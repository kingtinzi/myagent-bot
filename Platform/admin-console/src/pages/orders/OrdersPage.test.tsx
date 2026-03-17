import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialog } from '../../components/feedback/ConfirmDialog';
import { OrdersPage } from './OrdersPage';
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

describe('OrdersPage', () => {
  beforeEach(() => {
    useSessionStore.getState().setSession({
      user: { id: 'admin-1', email: 'admin@example.com' },
      operator: {
        email: 'admin@example.com',
        role: 'finance',
        active: true,
        capabilities: ['orders.read', 'orders.write'],
      },
    });

    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.includes('/admin/orders?')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ id: 'ord-1', user_id: 'user-1', amount_fen: 5000, channel: 'alipay', provider: 'alipay', status: 'pending', created_unix: 1710000000 }]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.endsWith('/admin/orders/ord-1/reconcile') && init?.method === 'POST') {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              changed: true,
              order: { id: 'ord-1', user_id: 'user-1', amount_fen: 5000, channel: 'alipay', provider: 'alipay', status: 'paid', created_unix: 1710000000 },
            }),
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

  it('filters by status and opens reconcile confirmation for selected orders', async () => {
    const user = userEvent.setup();

    await act(async () => {
      render(<OrdersPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByText('ord-1')).toBeInTheDocument();
    expect(screen.getByRole('combobox')).toHaveValue('');

    await user.selectOptions(screen.getByRole('combobox'), 'pending');
    await user.click(screen.getByRole('button', { name: '查询订单' }));
    await user.click(screen.getByText('ord-1'));

    expect(await screen.findByRole('dialog', { name: 'ord-1' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: '立即对账' }));

    expect(await screen.findByRole('dialog', { name: '订单对账' })).toBeInTheDocument();
    expect(screen.getByText('即将对账订单 ord-1。')).toBeInTheDocument();
  });
});
