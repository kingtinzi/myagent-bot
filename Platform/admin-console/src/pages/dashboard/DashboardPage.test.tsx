import type { ReactNode } from 'react';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { DashboardPage } from './DashboardPage';

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

describe('DashboardPage', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders KPI cards and model panels from backend data', async () => {
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
          top_models: [
            {
              model_id: 'gpt-5.2',
              usage_count: 88,
              charged_fen: 56300,
              prompt_tokens: 420000,
              completion_tokens: 210000,
            },
          ],
          generated_unix: 1710000000,
        }),
        {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        },
      ),
    );

    render(<DashboardPage />, { wrapper: createWrapper() });

    expect(await screen.findByText('1,284')).toBeInTheDocument();
    expect(screen.getByText('模型消耗排行')).toBeInTheDocument();
    expect(screen.getByText('gpt-5.2')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '最近 30 天' })).toBeInTheDocument();
  });
});
