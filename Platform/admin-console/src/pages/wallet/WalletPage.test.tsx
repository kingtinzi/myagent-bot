import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialog } from '../../components/feedback/ConfirmDialog';
import { WalletPage } from './WalletPage';
import { useSessionStore } from '../../stores/sessionStore';
import { useUIStore } from '../../stores/uiStore';

function createWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{children}</MemoryRouter>
      <ConfirmDialog />
    </QueryClientProvider>
  );
}

describe('WalletPage', () => {
  beforeEach(() => {
    useSessionStore.getState().setSession({
      user: { id: 'admin-1', email: 'admin@example.com' },
      operator: {
        email: 'admin@example.com',
        role: 'finance',
        active: true,
        capabilities: ['wallet.read', 'wallet.write'],
      },
    });

    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.includes('/admin/wallet-adjustments')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ id: 'tx-1', user_id: 'user-1', kind: 'credit', amount_fen: 5000, description: 'manual recharge', reference_type: 'admin_manual_recharge', created_unix: 1710000000 }]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.includes('/admin/users')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                user_id: 'user-1',
                user_no: 1001,
                username: '阿星',
                email: 'user-1@example.com',
                created_unix: 1710000000,
                last_seen_unix: 1710000001,
                balance_fen: 5000,
                currency: 'CNY',
                updated_unix: 1710000001,
              },
            ]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.endsWith('/admin/manual-recharges') && init?.method === 'POST') {
        return Promise.resolve(
          new Response(JSON.stringify({ user_id: 'user-1', balance_fen: 15000, currency: 'CNY', updated_unix: 1710000000 }), {
            status: 201,
            headers: { 'Content-Type': 'application/json' },
          }),
        );
      }

      if (url.endsWith('/admin/wallet-adjustments') && init?.method === 'POST') {
        return Promise.resolve(
          new Response(JSON.stringify({ user_id: 'user-1', balance_fen: 12000, currency: 'CNY', updated_unix: 1710000000 }), {
            status: 201,
            headers: { 'Content-Type': 'application/json' },
          }),
        );
      }

      return Promise.reject(new Error(`Unhandled fetch: ${url}`));
    }) as typeof fetch);
  });

  afterEach(() => {
    useUIStore.getState().resolveConfirmation(false);
    vi.unstubAllGlobals();
    useSessionStore.getState().clearSession();
  });

  it('shows wallet records, validates recharge form, and opens confirm dialog', async () => {
    const user = userEvent.setup();

    await act(async () => {
      render(<WalletPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByText('manual recharge')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: '确认充值' }));
    expect(await screen.findByText('请填写用户标识。')).toBeInTheDocument();

    const inputs = screen.getAllByRole('textbox');
    await user.type(inputs[1], 'user-1');
    await user.type(inputs[2], '300');
    await user.type(inputs[3], '测试充值');
    await user.click(screen.getAllByRole('button', { name: '确认充值' })[0]);

    expect(await screen.findByRole('dialog', { name: '管理员手动充值' })).toBeInTheDocument();
    expect(screen.getByText('即将为用户 阿星（#1001）充值 300 分。')).toBeInTheDocument();
  });

  it('resolves username to the real user id before creating manual recharge', async () => {
    const fetchMock = vi.mocked(globalThis.fetch);
    const user = userEvent.setup();

    await act(async () => {
      render(<WalletPage />, { wrapper: createWrapper() });
    });

    const inputs = screen.getAllByRole('textbox');
    await user.clear(inputs[1]);
    await user.type(inputs[1], '阿星');
    await user.type(inputs[2], '300');
    await user.type(inputs[3], '测试充值');
    await user.click(screen.getByRole('button', { name: '确认充值' }));

    expect(await screen.findByRole('dialog', { name: '管理员手动充值' })).toBeInTheDocument();
    expect(screen.getByText('即将为用户 阿星（#1001）充值 300 分。')).toBeInTheDocument();

    const confirmButtons = screen.getAllByRole('button', { name: '确认充值' });
    const confirmButton = confirmButtons[confirmButtons.length - 1];
    if (!confirmButton) {
      throw new Error('missing confirm button in dialog');
    }
    await user.click(confirmButton);

    expect(await screen.findByText('充值完成，最新余额 CNY 150.00。')).toBeInTheDocument();

    const manualRechargeCall = fetchMock.mock.calls.find((call): call is [RequestInfo | URL, RequestInit | undefined] => {
      const [input, init] = call;
      return String(input).endsWith('/admin/manual-recharges') && init?.method === 'POST';
    });
    expect(manualRechargeCall).toBeTruthy();
    const requestInit = manualRechargeCall?.[1];
    expect(requestInit?.body).toBeTruthy();
    expect(JSON.parse(String(requestInit?.body))).toMatchObject({
      user_id: 'user-1',
      amount_fen: 300,
      description: '测试充值',
    });
  });
});
