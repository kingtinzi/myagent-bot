import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { InlineStatus } from '../../components/feedback/InlineStatus';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import type { AgreementDocument } from '../../services/contracts';

type AgreementEditorPanelProps = {
  canWrite: boolean;
};

function createEmptyAgreement(): AgreementDocument {
  return {
    key: '',
    version: '',
    title: '',
    content: '',
    url: '',
    effective_from_unix: 0,
  };
}

export function AgreementEditorPanel({ canWrite }: AgreementEditorPanelProps) {
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const [drafts, setDrafts] = useState<AgreementDocument[]>([]);
  const [statusMessage, setStatusMessage] = useState('维护用户协议、隐私政策与充值知情材料的版本内容。');

  const agreementsQuery = useQuery({
    queryKey: ['admin', 'catalog', 'agreements'],
    queryFn: () => adminApi.getAgreementVersions(),
    retry: false,
  });

  useEffect(() => {
    if (!agreementsQuery.data) {
      return;
    }
    setDrafts(agreementsQuery.data.data);
  }, [agreementsQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!agreementsQuery.data?.revision) {
        throw new Error('协议版本缺少 revision，请重新加载后再保存。');
      }

      const confirmed = await confirmAction({
        title: '保存协议版本',
        message: `即将保存 ${drafts.length} 份协议文档。`,
        hint: '协议发布会直接影响注册、充值或知情确认展示内容。',
        confirmLabel: '确认保存',
        tone: 'danger',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.saveAgreementVersions(drafts, agreementsQuery.data.revision);
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('已取消协议版本保存。');
        return;
      }
      setDrafts(result.data);
      setStatusMessage('协议版本已保存。');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'catalog', 'agreements'] });
    },
  });

  const statusTone = agreementsQuery.isError || saveMutation.isError ? 'danger' : agreementsQuery.isFetching ? 'info' : 'success';
  const statusText = agreementsQuery.isError
    ? agreementsQuery.error instanceof Error
      ? agreementsQuery.error.message
      : '加载协议版本失败，请稍后重试。'
    : saveMutation.isError
      ? saveMutation.error instanceof Error
        ? saveMutation.error.message
        : '保存协议版本失败，请稍后重试。'
      : agreementsQuery.isFetching
        ? '正在加载协议版本…'
        : statusMessage;

  function updateDraft(index: number, patch: Partial<AgreementDocument>) {
    setDrafts(current => current.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item)));
  }

  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>协议版本</h2>
          <p>支持维护协议 key、版本号、标题、正文、跳转链接与生效时间。</p>
        </div>
      </div>

      <InlineStatus tone={statusTone}>{statusText}</InlineStatus>

      <div className="resource-toolbar">
        <button className="button button--ghost" onClick={() => setDrafts(agreementsQuery.data?.data ?? [])} type="button">
          重置为服务端版本
        </button>
        <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => [...current, createEmptyAgreement()])} type="button">
          新增协议
        </button>
        <button className="button button--primary" disabled={!canWrite || saveMutation.isPending} onClick={() => void saveMutation.mutateAsync()} type="button">
          {saveMutation.isPending ? '保存中…' : '保存协议版本'}
        </button>
      </div>

      <div className="list-grid">
        {drafts.length === 0 ? (
          <InlineStatus tone="warning">当前还没有协议版本，请先新增用户协议或隐私政策条目。</InlineStatus>
        ) : (
          drafts.map((draft, index) => (
            <article className="collection-card" key={`${draft.key || 'agreement'}-${draft.version || index}`}>
              <div className="collection-card__header">
                <strong>{draft.title || `${draft.key || '协议'} ${draft.version || ''}`.trim()}</strong>
                <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => current.filter((_, itemIndex) => itemIndex !== index))} type="button">
                  删除
                </button>
              </div>

              <div className="form-grid">
                <label className="filter-field">
                  <span>协议 Key</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { key: event.target.value })} value={draft.key} />
                </label>
                <label className="filter-field">
                  <span>版本号</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { version: event.target.value })} value={draft.version} />
                </label>
                <label className="filter-field">
                  <span>标题</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { title: event.target.value })} value={draft.title} />
                </label>
                <label className="filter-field">
                  <span>跳转链接</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { url: event.target.value })} value={draft.url ?? ''} />
                </label>
                <label className="filter-field">
                  <span>生效时间（Unix）</span>
                  <input
                    disabled={!canWrite}
                    onChange={event => updateDraft(index, { effective_from_unix: Number.parseInt(event.target.value || '0', 10) || 0 })}
                    value={draft.effective_from_unix ? String(draft.effective_from_unix) : ''}
                  />
                </label>
              </div>

              <label className="filter-field">
                <span>正文内容</span>
                <textarea
                  className="textarea-field textarea-field--lg"
                  disabled={!canWrite}
                  onChange={event => updateDraft(index, { content: event.target.value })}
                  value={draft.content ?? ''}
                />
              </label>
            </article>
          ))
        )}
      </div>
    </section>
  );
}
