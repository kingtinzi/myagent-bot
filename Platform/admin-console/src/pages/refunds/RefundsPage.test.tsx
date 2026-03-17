import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialog } from '../../components/feedback/ConfirmDialog';
import { useSessionStore } from '../../stores/sessionStore';
import { RefundsPage } from './RefundsPage';

function createWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{children}</MemoryRouter>
      <ConfirmDialog />
    </QueryClientProvider>
  );
}

describe('RefundsPage', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useSessionStore.getState().clearSession();
  });

  it('filters refund requests and approves selected refunds through confirmation', async () => {
    const user = userEvent.setup();

    useSessionStore.getState().setSession({
      user: { id: 'refund-admin-1', email: 'refunds@example.com' },
      operator: {
        email: 'refunds@example.com',
        role: 'finance',
        active: true,
        capabilities: ['refunds.read', 'refunds.review'],
      },
    });

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.includes('/admin/refund-requests?status=pending&limit=50')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: 'refund-1',
                user_id: 'user-1',
                username: '张三',
                order_id: 'order-1',
                amount_fen: 300,
                reason: '余额误充',
                status: 'pending',
                created_unix: 1710000000,
                updated_unix: 1710000001,
              },
            ]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.endsWith('/admin/refund-requests?limit=50')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: 'refund-1',
                user_id: 'user-1',
                username: '张三',
                order_id: 'order-1',
                amount_fen: 300,
                reason: '余额误充',
                status: 'pending',
                created_unix: 1710000000,
                updated_unix: 1710000001,
              },
            ]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.endsWith('/admin/refund-requests/refund-1/approve') && init?.method === 'POST') {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: 'refund-1',
              user_id: 'user-1',
              username: '张三',
              order_id: 'order-1',
              amount_fen: 300,
              reason: '余额误充',
              status: 'approved_pending_payout',
              review_note: '通过审核',
              refund_provider: 'manual',
              created_unix: 1710000000,
              updated_unix: 1710000010,
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    });

    await act(async () => {
      render(<RefundsPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByText('余额误充')).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText('退款状态'), 'pending');
    await user.click(screen.getByRole('button', { name: '应用筛选' }));

    expect((await screen.findAllByText('待审核')).length).toBeGreaterThan(0);
    await user.type(screen.getByLabelText('审核备注'), '通过审核');
    await user.type(screen.getByLabelText('退款通道'), 'manual');
    await user.click(screen.getByRole('button', { name: '通过退款' }));

    const dialog = await screen.findByRole('dialog', { name: '通过退款：refund-1' });
    await user.type(within(dialog).getByLabelText(/请输入/), 'CONFIRM');
    await user.click(within(dialog).getByRole('button', { name: '通过退款' }));

    expect(await screen.findByText('退款请求 refund-1 已更新为「待打款」。')).toBeInTheDocument();
  });

  it('shows read-only guidance when operator cannot review refunds', async () => {
    useSessionStore.getState().setSession({
      user: { id: 'refund-admin-2', email: 'refunds-readonly@example.com' },
      operator: {
        email: 'refunds-readonly@example.com',
        role: 'read_only',
        active: true,
        capabilities: ['refunds.read'],
      },
    });

    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: 'refund-1',
            user_id: 'user-1',
            username: '张三',
            order_id: 'order-1',
            amount_fen: 300,
            status: 'pending',
            created_unix: 1710000000,
            updated_unix: 1710000001,
          },
        ]),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );

    await act(async () => {
      render(<RefundsPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByText('当前账号只能查看退款详情，不能执行审核、驳回或结算。')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '通过退款' })).not.toBeInTheDocument();
  });
});
