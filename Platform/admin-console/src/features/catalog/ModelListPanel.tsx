import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { InlineStatus } from '../../components/feedback/InlineStatus';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import type { OfficialModel } from '../../services/contracts';

type ModelListPanelProps = {
  canWrite: boolean;
};

function createEmptyModel(): OfficialModel {
  return {
    id: '',
    name: '',
    description: '',
    enabled: false,
    fallback_priority: 0,
    pricing_version: '',
  };
}

export function ModelListPanel({ canWrite }: ModelListPanelProps) {
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const [drafts, setDrafts] = useState<OfficialModel[]>([]);
  const [statusMessage, setStatusMessage] = useState('可在这里管理官方模型目录与前台展示信息。');

  const modelsQuery = useQuery({
    queryKey: ['admin', 'catalog', 'models'],
    queryFn: () => adminApi.getOfficialModels(),
    retry: false,
  });

  useEffect(() => {
    if (!modelsQuery.data) {
      return;
    }
    setDrafts(modelsQuery.data.data);
  }, [modelsQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!modelsQuery.data?.revision) {
        throw new Error('模型目录缺少 revision，请重新加载后再保存。');
      }

      const confirmed = await confirmAction({
        title: '保存官方模型目录',
        message: `即将保存 ${drafts.length} 条官方模型配置。`,
        hint: '该操作会直接影响前台用户可见模型目录。',
        confirmLabel: '确认保存',
        tone: 'warning',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.saveOfficialModels(drafts, modelsQuery.data.revision);
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('已取消模型目录保存。');
        return;
      }

      setDrafts(result.data);
      setStatusMessage('官方模型目录已保存。');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'catalog', 'models'] });
    },
  });

  const statusTone = modelsQuery.isError || saveMutation.isError ? 'danger' : modelsQuery.isFetching ? 'info' : 'success';
  const statusText = modelsQuery.isError
    ? modelsQuery.error instanceof Error
      ? modelsQuery.error.message
      : '加载官方模型目录失败，请稍后重试。'
    : saveMutation.isError
      ? saveMutation.error instanceof Error
        ? saveMutation.error.message
        : '保存官方模型目录失败，请稍后重试。'
      : modelsQuery.isFetching
        ? '正在加载官方模型目录…'
        : statusMessage;

  function updateDraft(index: number, patch: Partial<OfficialModel>) {
    setDrafts(current =>
      current.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item)),
    );
  }

  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>官方模型目录</h2>
          <p>维护前台模型名称、描述、启用状态、默认 pricing version 与回退优先级（数字越小越先尝试）。</p>
          <p>前台仅展示统一“官方模型”入口，不向用户展示这里配置的具体官方模型名称。</p>
        </div>
      </div>

      <InlineStatus tone={statusTone}>{statusText}</InlineStatus>

      <div className="resource-toolbar">
        <button className="button button--ghost" onClick={() => setDrafts(modelsQuery.data?.data ?? [])} type="button">
          重置为服务端版本
        </button>
        <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => [...current, createEmptyModel()])} type="button">
          新增模型
        </button>
        <button className="button button--primary" disabled={!canWrite || saveMutation.isPending} onClick={() => void saveMutation.mutateAsync()} type="button">
          {saveMutation.isPending ? '保存中…' : '保存模型目录'}
        </button>
      </div>

      <div className="list-grid">
        {drafts.length === 0 ? (
          <InlineStatus tone="warning">当前还没有官方模型，请先新增至少一个模型条目。</InlineStatus>
        ) : (
          drafts.map((draft, index) => (
            <article className="collection-card" key={`${draft.id || 'model'}-${index}`}>
              <div className="collection-card__header">
                <strong>{draft.name || draft.id || `模型 ${index + 1}`}</strong>
                <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => current.filter((_, itemIndex) => itemIndex !== index))} type="button">
                  删除
                </button>
              </div>

              <div className="form-grid">
                <label className="filter-field">
                  <span>模型 ID</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { id: event.target.value })} value={draft.id} />
                </label>

                <label className="filter-field">
                  <span>显示名称</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { name: event.target.value })} value={draft.name} />
                </label>

                <label className="filter-field">
                  <span>计费版本</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { pricing_version: event.target.value })} value={draft.pricing_version ?? ''} />
                </label>

                <label className="filter-field">
                  <span>回退优先级</span>
                  <input
                    disabled={!canWrite}
                    min={0}
                    onChange={event => {
                      const raw = event.target.value.trim();
                      const parsed = Number.parseInt(raw || '0', 10);
                      updateDraft(index, { fallback_priority: Number.isFinite(parsed) && parsed >= 0 ? parsed : 0 });
                    }}
                    type="number"
                    value={draft.fallback_priority ?? 0}
                  />
                </label>

                <label className="checkbox-field">
                  <input checked={draft.enabled} disabled={!canWrite} onChange={event => updateDraft(index, { enabled: event.target.checked })} type="checkbox" />
                  <span>前台可用</span>
                </label>
              </div>

              <label className="filter-field">
                <span>模型说明</span>
                <textarea
                  className="textarea-field"
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
