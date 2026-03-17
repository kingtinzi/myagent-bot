import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { InlineStatus } from '../../components/feedback/InlineStatus';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import type { SystemNotice } from '../../services/contracts';

type SystemNoticePanelProps = {
  canWrite: boolean;
};

function createEmptyNotice(): SystemNotice {
  return {
    id: '',
    title: '',
    body: '',
    severity: 'info',
    enabled: false,
  };
}

export function SystemNoticePanel({ canWrite }: SystemNoticePanelProps) {
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const [drafts, setDrafts] = useState<SystemNotice[]>([]);
  const [statusMessage, setStatusMessage] = useState('统一维护前台横幅、充值提示和系统公告。');

  const noticesQuery = useQuery({
    queryKey: ['admin', 'governance', 'notices'],
    queryFn: () => adminApi.getSystemNotices(),
    retry: false,
  });

  useEffect(() => {
    if (!noticesQuery.data) {
      return;
    }
    setDrafts(noticesQuery.data.data);
  }, [noticesQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!noticesQuery.data?.revision) {
        throw new Error('系统公告缺少 revision，请重新加载后再保存。');
      }

      const confirmed = await confirmAction({
        title: '保存系统公告',
        message: `即将保存 ${drafts.length} 条系统公告。`,
        hint: '公告发布会直接影响用户端横幅、钱包提示和治理提醒。',
        confirmLabel: '确认保存',
        tone: 'warning',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.saveSystemNotices(drafts, noticesQuery.data.revision);
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('已取消系统公告保存。');
        return;
      }
      setDrafts(result.data);
      setStatusMessage('系统公告已保存。');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'governance', 'notices'] });
    },
  });

  const statusTone = noticesQuery.isError || saveMutation.isError ? 'danger' : noticesQuery.isFetching ? 'info' : 'success';
  const statusText = noticesQuery.isError
    ? noticesQuery.error instanceof Error
      ? noticesQuery.error.message
      : '加载系统公告失败，请稍后重试。'
    : saveMutation.isError
      ? saveMutation.error instanceof Error
        ? saveMutation.error.message
        : '保存系统公告失败，请稍后重试。'
      : noticesQuery.isFetching
        ? '正在加载系统公告…'
        : statusMessage;

  function updateDraft(index: number, patch: Partial<SystemNotice>) {
    setDrafts(current => current.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item)));
  }

  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>系统公告</h2>
          <p>用于前台顶部横幅、钱包充值提醒和运营通知的统一配置。</p>
        </div>
      </div>

      <InlineStatus tone={statusTone}>{statusText}</InlineStatus>

      <div className="resource-toolbar">
        <button className="button button--ghost" onClick={() => setDrafts(noticesQuery.data?.data ?? [])} type="button">
          重置为服务端版本
        </button>
        <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => [...current, createEmptyNotice()])} type="button">
          新增公告
        </button>
        <button className="button button--primary" disabled={!canWrite || saveMutation.isPending} onClick={() => void saveMutation.mutateAsync()} type="button">
          {saveMutation.isPending ? '保存中…' : '保存系统公告'}
        </button>
      </div>

      <div className="list-grid">
        {drafts.length === 0 ? (
          <InlineStatus tone="warning">当前还没有系统公告，可按需新增上线提示、充值说明或风控公告。</InlineStatus>
        ) : (
          drafts.map((draft, index) => (
            <article className="collection-card" key={`${draft.id || 'notice'}-${index}`}>
              <div className="collection-card__header">
                <strong>{draft.title || draft.id || `公告 ${index + 1}`}</strong>
                <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => current.filter((_, itemIndex) => itemIndex !== index))} type="button">
                  删除
                </button>
              </div>

              <div className="form-grid">
                <label className="filter-field">
                  <span>公告 ID</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { id: event.target.value })} value={draft.id} />
                </label>
                <label className="filter-field">
                  <span>公告标题</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { title: event.target.value })} value={draft.title} />
                </label>
                <label className="filter-field">
                  <span>严重级别</span>
                  <select disabled={!canWrite} onChange={event => updateDraft(index, { severity: event.target.value })} value={draft.severity ?? 'info'}>
                    <option value="info">提示</option>
                    <option value="warning">警告</option>
                    <option value="danger">高风险</option>
                    <option value="success">成功</option>
                  </select>
                </label>
                <label className="checkbox-field">
                  <input checked={draft.enabled} disabled={!canWrite} onChange={event => updateDraft(index, { enabled: event.target.checked })} type="checkbox" />
                  <span>启用公告</span>
                </label>
              </div>

              <label className="filter-field">
                <span>公告正文</span>
                <textarea className="textarea-field textarea-field--lg" disabled={!canWrite} onChange={event => updateDraft(index, { body: event.target.value })} value={draft.body} />
              </label>
            </article>
          ))
        )}
      </div>
    </section>
  );
}
