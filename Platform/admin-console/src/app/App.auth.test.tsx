import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import App from './App';
import { useSessionStore } from '../stores/sessionStore';

function createDashboardPayload() {
  return {
    totals: {
      users: 1284,
      paid_orders: 276,
      wallet_balance_fen: 932340,
      refund_pending: 5,
      infringement_pending: 2,
    },
    recent: {
      recharge_fen_7d: 342100,
      consumption_fen_7d: 125400,
      new_users_7d: 39,
      window_days: 30,
    },
    top_models: [],
    generated_unix: 1710000000,
  };
}

function createSessionPayload(capabilities: string[]) {
  return {
    user: {
      id: 'admin-1',
      email: 'admin@example.com',
    },
    operator: {
      email: 'admin@example.com',
      role: 'finance',
      active: true,
      capabilities,
    },
  };
}

describe('App auth flow', () => {
  beforeEach(() => {
    window.location.hash = '';
    useSessionStore.getState().clearSession();
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    window.location.hash = '';
    vi.unstubAllGlobals();
    useSessionStore.getState().clearSession();
  });

  it('shows the admin login screen when session restore returns unauthorized', async () => {
    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith('/admin/session')) {
        return Promise.resolve(new Response('missing administrator session', { status: 401 }));
      }
      throw new Error(`Unhandled fetch: ${url}`);
    });

    render(<App />);

    expect(await screen.findByRole('heading', { name: '管理员登录' })).toBeInTheDocument();
    expect(screen.getByLabelText('邮箱')).toBeInTheDocument();
    expect(screen.getByLabelText('密码')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '登录后台' })).toBeInTheDocument();
  });

  it('submits login and restores the dashboard workspace', async () => {
    const user = userEvent.setup();

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/admin/session') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(new Response('missing administrator session', { status: 401 }));
      }
      if (url.endsWith('/admin/session/login') && init?.method === 'POST') {
        return Promise.resolve(
          new Response(JSON.stringify(createSessionPayload(['dashboard.read', 'wallet.read', 'orders.read'])), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          }),
        );
      }
      if (url.includes('/admin/dashboard')) {
        return Promise.resolve(
          new Response(JSON.stringify(createDashboardPayload()), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          }),
        );
      }
      throw new Error(`Unhandled fetch: ${url}`);
    });

    render(<App />);

    await user.type(await screen.findByLabelText('邮箱'), 'admin@example.com');
    await user.type(screen.getByLabelText('密码'), 'secret');
    await user.click(screen.getByRole('button', { name: '登录后台' }));

    expect(await screen.findByRole('heading', { name: '平台仪表盘' })).toBeInTheDocument();
    expect(screen.getByText('admin@example.com')).toBeInTheDocument();
  });

  it('hides modules that the current operator cannot access', async () => {
    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/admin/session') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(
          new Response(JSON.stringify(createSessionPayload(['dashboard.read', 'wallet.read', 'orders.read'])), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          }),
        );
      }
      if (url.includes('/admin/dashboard')) {
        return Promise.resolve(
          new Response(JSON.stringify(createDashboardPayload()), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          }),
        );
      }
      throw new Error(`Unhandled fetch: ${url}`);
    });

    render(<App />);

    expect(await screen.findByRole('heading', { name: '平台仪表盘' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /订单.*充值订单、支付链路与对账/ })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /钱包.*手动充值、调账与账本流水/ })).toBeInTheDocument();
    expect(screen.queryByText('用户')).not.toBeInTheDocument();
    expect(screen.queryByText('管理员')).not.toBeInTheDocument();
    expect(screen.queryByText('治理')).not.toBeInTheDocument();
  });
});
