import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ConfirmDialog } from '../../components/feedback/ConfirmDialog';
import { useSessionStore } from '../../stores/sessionStore';
import { InfringementPage } from './InfringementPage';

function createWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{children}</MemoryRouter>
      <ConfirmDialog />
    </QueryClientProvider>
  );
}

describe('InfringementPage', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    useSessionStore.getState().clearSession();
  });

  it('filters infringement reports and updates selected case through confirmation', async () => {
    const user = userEvent.setup();

    useSessionStore.getState().setSession({
      user: { id: 'gov-admin-1', email: 'gov@example.com' },
      operator: {
        email: 'gov@example.com',
        role: 'governance',
        active: true,
        capabilities: ['infringement.read', 'infringement.review'],
      },
    });

    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);

      if (url.includes('/admin/infringement-reports?status=pending&limit=50')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: 'report-1',
                user_id: 'user-1',
                username: '李四',
                subject: '涉嫌侵权内容',
                description: '用户上传了涉嫌侵权的图片素材。',
                evidence_urls: ['https://example.com/evidence/1'],
                status: 'pending',
                created_unix: 1710000000,
                updated_unix: 1710000001,
              },
            ]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.endsWith('/admin/infringement-reports?limit=50')) {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: 'report-1',
                user_id: 'user-1',
                username: '李四',
                subject: '涉嫌侵权内容',
                description: '用户上传了涉嫌侵权的图片素材。',
                evidence_urls: ['https://example.com/evidence/1'],
                status: 'pending',
                created_unix: 1710000000,
                updated_unix: 1710000001,
              },
            ]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
        );
      }

      if (url.endsWith('/admin/infringement-reports/report-1') && init?.method === 'POST') {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: 'report-1',
              user_id: 'user-1',
              username: '李四',
              subject: '涉嫌侵权内容',
              description: '用户上传了涉嫌侵权的图片素材。',
              evidence_urls: ['https://example.com/evidence/1'],
              status: 'resolved',
              resolution: '已下架并记录证据。',
              reviewed_by: 'gov-admin-1',
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
      render(<InfringementPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByText('涉嫌侵权内容')).toBeInTheDocument();
    expect(screen.getByText('打开证据链接')).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText('案件状态'), 'pending');
    await user.click(screen.getByRole('button', { name: '应用筛选' }));
    expect((await screen.findAllByText('待处理')).length).toBeGreaterThan(0);

    await user.selectOptions(screen.getByLabelText('下一状态'), 'resolved');
    await user.type(screen.getByLabelText('处理结论'), '已下架并记录证据。');
    await user.click(screen.getByRole('button', { name: '更新案件状态' }));

    expect(await screen.findByRole('dialog', { name: '更新侵权报告：report-1' })).toBeInTheDocument();
    await user.type(screen.getByLabelText(/请输入/), 'CONFIRM');
    await user.click(screen.getByRole('button', { name: '确认更新' }));

    expect(await screen.findByText('侵权报告 report-1 已更新为「已处置」。')).toBeInTheDocument();
  });

  it('shows read-only guidance when operator cannot review infringement reports', async () => {
    useSessionStore.getState().setSession({
      user: { id: 'gov-admin-2', email: 'readonly-gov@example.com' },
      operator: {
        email: 'readonly-gov@example.com',
        role: 'read_only',
        active: true,
        capabilities: ['infringement.read'],
      },
    });

    vi.mocked(fetch).mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: 'report-1',
            user_id: 'user-1',
            username: '李四',
            subject: '涉嫌侵权内容',
            description: '用户上传了涉嫌侵权的图片素材。',
            status: 'pending',
            created_unix: 1710000000,
            updated_unix: 1710000001,
          },
        ]),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );

    await act(async () => {
      render(<InfringementPage />, { wrapper: createWrapper() });
    });

    expect(await screen.findByText('当前账号只能查看侵权详情，不能提交处理结论。')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '更新案件状态' })).not.toBeInTheDocument();
  });
});
