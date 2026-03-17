import { render } from '@testing-library/react';
import { afterEach, beforeEach, vi } from 'vitest';

import App from './App';

describe('App', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
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
        }),
        {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        },
      ),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders the admin shell workspace', async () => {
    const view = render(<App />);

    expect(await view.findByRole('heading', { name: '平台仪表盘' })).toBeInTheDocument();
    expect(view.getByTestId('admin-shell-root')).toBeInTheDocument();
    expect(view.getByTestId('query-provider-ready')).toHaveTextContent('ready');
    expect(view.getByText('PinchBot 管理后台')).toBeInTheDocument();
    expect(view.getByRole('button', { name: '刷新后台' })).toBeInTheDocument();
  });
});
