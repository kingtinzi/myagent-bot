import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { InlineStatus } from '../../components/feedback/InlineStatus';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import type { DataRetentionPolicy } from '../../services/contracts';

type DataRetentionPolicyPanelProps = {
  canWrite: boolean;
};

function createEmptyPolicy(): DataRetentionPolicy {
  return {
    data_domain: '',
    retention_days: 30,
    purge_mode: 'manual_review',
    description: '',
    enabled: false,
  };
}

export function DataRetentionPolicyPanel({ canWrite }: DataRetentionPolicyPanelProps) {
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const [drafts, setDrafts] = useState<DataRetentionPolicy[]>([]);
  const [statusMessage, setStatusMessage] = useState('统一管理日志、会话、订单等数据域的保留窗口与清理模式。');

  const retentionQuery = useQuery({
    queryKey: ['admin', 'governance', 'retention'],
    queryFn: () => adminApi.getDataRetentionPolicies(),
    retry: false,
  });

  useEffect(() => {
    if (!retentionQuery.data) {
      return;
    }
    setDrafts(retentionQuery.data.data);
  }, [retentionQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!retentionQuery.data?.revision) {
        throw new Error('数据保留策略缺少 revision，请重新加载后再保存。');
      }

      const confirmed = await confirmAction({
        title: '保存数据保留策略',
        message: `即将保存 ${drafts.length} 条保留策略。`,
        hint: '保留窗口变化会直接影响日志、账本与证据链的保存时长。',
        confirmLabel: '确认保存',
        tone: 'danger',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.saveDataRetentionPolicies(drafts, retentionQuery.data.revision);
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('已取消数据保留策略保存。');
        return;
      }
      setDrafts(result.data);
      setStatusMessage('数据保留策略已保存。');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'governance', 'retention'] });
    },
  });

  const statusTone = retentionQuery.isError || saveMutation.isError ? 'danger' : retentionQuery.isFetching ? 'info' : 'success';
  const statusText = retentionQuery.isError
    ? retentionQuery.error instanceof Error
      ? retentionQuery.error.message
      : '加载数据保留策略失败，请稍后重试。'
    : saveMutation.isError
      ? saveMutation.error instanceof Error
        ? saveMutation.error.message
        : '保存数据保留策略失败，请稍后重试。'
      : retentionQuery.isFetching
        ? '正在加载数据保留策略…'
        : statusMessage;

  function updateDraft(index: number, patch: Partial<DataRetentionPolicy>) {
    setDrafts(current => current.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item)));
  }

  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>数据保留策略</h2>
          <p>确保支付、风控、审计和用户会话等关键数据域有明确保留时长与清理模式。</p>
        </div>
      </div>

      <InlineStatus tone={statusTone}>{statusText}</InlineStatus>

      <div className="resource-toolbar">
        <button className="button button--ghost" onClick={() => setDrafts(retentionQuery.data?.data ?? [])} type="button">
          重置为服务端版本
        </button>
        <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => [...current, createEmptyPolicy()])} type="button">
          新增策略
        </button>
        <button className="button button--primary" disabled={!canWrite || saveMutation.isPending} onClick={() => void saveMutation.mutateAsync()} type="button">
          {saveMutation.isPending ? '保存中…' : '保存数据保留策略'}
        </button>
      </div>

      <div className="list-grid">
        {drafts.length === 0 ? (
          <InlineStatus tone="warning">当前还没有数据保留策略，请先定义关键数据域的保留窗口。</InlineStatus>
        ) : (
          drafts.map((draft, index) => (
            <article className="collection-card" key={`${draft.data_domain || 'retention'}-${index}`}>
              <div className="collection-card__header">
                <strong>{draft.data_domain || `策略 ${index + 1}`}</strong>
                <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => current.filter((_, itemIndex) => itemIndex !== index))} type="button">
                  删除
                </button>
              </div>

              <div className="form-grid">
                <label className="filter-field">
                  <span>数据域</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { data_domain: event.target.value })} value={draft.data_domain} />
                </label>
                <label className="filter-field">
                  <span>保留天数</span>
                  <input
                    disabled={!canWrite}
                    onChange={event => updateDraft(index, { retention_days: Number.parseInt(event.target.value || '0', 10) || 0 })}
                    value={String(draft.retention_days)}
                  />
                </label>
                <label className="filter-field">
                  <span>清理模式</span>
                  <select disabled={!canWrite} onChange={event => updateDraft(index, { purge_mode: event.target.value })} value={draft.purge_mode ?? 'manual_review'}>
                    <option value="manual_review">人工复核</option>
                    <option value="soft_delete">软删除</option>
                    <option value="hard_delete">物理删除</option>
                    <option value="archive">归档</option>
                  </select>
                </label>
                <label className="checkbox-field">
                  <input checked={draft.enabled} disabled={!canWrite} onChange={event => updateDraft(index, { enabled: event.target.checked })} type="checkbox" />
                  <span>启用策略</span>
                </label>
              </div>

              <label className="filter-field">
                <span>策略说明</span>
                <textarea
                  className="textarea-field textarea-field--lg"
                  disabled={!canWrite}
                  onChange={event => updateDraft(index, { description: event.target.value })}
                  value={draft.description ?? ''}
                />
              </label>
            </article>
          ))
        )}
      </div>
    </section>
  );
}
