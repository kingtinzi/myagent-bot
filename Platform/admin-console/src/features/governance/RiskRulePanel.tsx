import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { InlineStatus } from '../../components/feedback/InlineStatus';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import type { RiskRule } from '../../services/contracts';

type RiskRulePanelProps = {
  canWrite: boolean;
};

function createEmptyRiskRule(): RiskRule {
  return {
    key: '',
    name: '',
    description: '',
    enabled: false,
  };
}

export function RiskRulePanel({ canWrite }: RiskRulePanelProps) {
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const [drafts, setDrafts] = useState<RiskRule[]>([]);
  const [statusMessage, setStatusMessage] = useState('集中维护风控检查规则、人工审核开关和风险描述文案。');

  const riskQuery = useQuery({
    queryKey: ['admin', 'governance', 'risk'],
    queryFn: () => adminApi.getRiskRules(),
    retry: false,
  });

  useEffect(() => {
    if (!riskQuery.data) {
      return;
    }
    setDrafts(riskQuery.data.data);
  }, [riskQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!riskQuery.data?.revision) {
        throw new Error('风控规则缺少 revision，请重新加载后再保存。');
      }

      const confirmed = await confirmAction({
        title: '保存风控规则',
        message: `即将保存 ${drafts.length} 条风控规则。`,
        hint: '风控规则变更会影响侵权、充值或模型调用链路中的审核策略。',
        confirmLabel: '确认保存',
        tone: 'danger',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.saveRiskRules(drafts, riskQuery.data.revision);
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('已取消风控规则保存。');
        return;
      }
      setDrafts(result.data);
      setStatusMessage('风控规则已保存。');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'governance', 'risk'] });
    },
  });

  const statusTone = riskQuery.isError || saveMutation.isError ? 'danger' : riskQuery.isFetching ? 'info' : 'success';
  const statusText = riskQuery.isError
    ? riskQuery.error instanceof Error
      ? riskQuery.error.message
      : '加载风控规则失败，请稍后重试。'
    : saveMutation.isError
      ? saveMutation.error instanceof Error
        ? saveMutation.error.message
        : '保存风控规则失败，请稍后重试。'
      : riskQuery.isFetching
        ? '正在加载风控规则…'
        : statusMessage;

  function updateDraft(index: number, patch: Partial<RiskRule>) {
    setDrafts(current => current.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item)));
  }

  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>风控规则</h2>
          <p>维护用于模型调用、支付提醒、侵权审查等场景的风险策略目录。</p>
        </div>
      </div>

      <InlineStatus tone={statusTone}>{statusText}</InlineStatus>

      <div className="resource-toolbar">
        <button className="button button--ghost" onClick={() => setDrafts(riskQuery.data?.data ?? [])} type="button">
          重置为服务端版本
        </button>
        <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => [...current, createEmptyRiskRule()])} type="button">
          新增规则
        </button>
        <button className="button button--primary" disabled={!canWrite || saveMutation.isPending} onClick={() => void saveMutation.mutateAsync()} type="button">
          {saveMutation.isPending ? '保存中…' : '保存风控规则'}
        </button>
      </div>

      <div className="list-grid">
        {drafts.length === 0 ? (
          <InlineStatus tone="warning">当前还没有风控规则，请至少维护一条可审计的风险策略。</InlineStatus>
        ) : (
          drafts.map((draft, index) => (
            <article className="collection-card" key={`${draft.key || 'risk'}-${index}`}>
              <div className="collection-card__header">
                <strong>{draft.name || draft.key || `规则 ${index + 1}`}</strong>
                <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => current.filter((_, itemIndex) => itemIndex !== index))} type="button">
                  删除
                </button>
              </div>

              <div className="form-grid">
                <label className="filter-field">
                  <span>规则 Key</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { key: event.target.value })} value={draft.key} />
                </label>
                <label className="filter-field">
                  <span>规则名称</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { name: event.target.value })} value={draft.name} />
                </label>
                <label className="checkbox-field">
                  <input checked={draft.enabled} disabled={!canWrite} onChange={event => updateDraft(index, { enabled: event.target.checked })} type="checkbox" />
                  <span>启用规则</span>
                </label>
              </div>

              <label className="filter-field">
                <span>规则说明</span>
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
