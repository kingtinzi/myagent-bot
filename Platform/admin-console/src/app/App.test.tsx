import { render } from '@testing-library/react';
import { afterEach, beforeEach, vi } from 'vitest';

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

function createSessionPayload() {
  return {
    user: {
      id: 'admin-1',
      email: 'admin@example.com',
    },
    operator: {
      email: 'admin@example.com',
      role: 'super_admin',
      active: true,
      capabilities: ['dashboard.read', 'users.read', 'orders.read', 'wallet.read', 'operators.read', 'audit.read'],
    },
  };
}

describe('App', () => {
  beforeEach(() => {
    window.location.hash = '';
    useSessionStore.getState().clearSession();
    vi.stubGlobal('fetch', vi.fn());
    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.endsWith('/admin/session') && (!init?.method || init.method === 'GET')) {
        return Promise.resolve(
          new Response(JSON.stringify(createSessionPayload()), {
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
  });

  afterEach(() => {
    window.location.hash = '';
    useSessionStore.getState().clearSession();
    vi.unstubAllGlobals();
  });

  it('renders the admin shell workspace after restoring the admin session', async () => {
    const view = render(<App />);

    expect(await view.findByRole('heading', { name: '平台仪表盘' })).toBeInTheDocument();
    expect(view.getByTestId('admin-shell-root')).toBeInTheDocument();
    expect(view.getByTestId('query-provider-ready')).toHaveTextContent('ready');
    expect(view.getByText('PinchBot 管理后台')).toBeInTheDocument();
    expect(view.getByText('admin@example.com')).toBeInTheDocument();
    expect(view.getByRole('button', { name: '刷新后台' })).toBeInTheDocument();
  });
});
