import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { InlineStatus } from '../../components/feedback/InlineStatus';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import type { PricingRule } from '../../services/contracts';

type PricingEditorPanelProps = {
  canWrite: boolean;
};

type PricingDraft = {
  model_id: string;
  version: string;
  effective_from_unix: string;
  input_price_micros_per_1k: string;
  output_price_micros_per_1k: string;
  fallback_price_fen: string;
};

function createEmptyPricingDraft(): PricingDraft {
  return {
    model_id: '',
    version: 'v1',
    effective_from_unix: '',
    input_price_micros_per_1k: '0',
    output_price_micros_per_1k: '0',
    fallback_price_fen: '0',
  };
}

function fromPricingRule(rule: PricingRule): PricingDraft {
  return {
    model_id: rule.model_id,
    version: rule.version ?? 'v1',
    effective_from_unix: rule.effective_from_unix ? String(rule.effective_from_unix) : '',
    input_price_micros_per_1k: String(rule.input_price_micros_per_1k),
    output_price_micros_per_1k: String(rule.output_price_micros_per_1k),
    fallback_price_fen: String(rule.fallback_price_fen),
  };
}

function toPricingRule(draft: PricingDraft): PricingRule {
  return {
    model_id: draft.model_id.trim(),
    version: draft.version.trim() || 'v1',
    effective_from_unix: draft.effective_from_unix.trim() ? Number.parseInt(draft.effective_from_unix.trim(), 10) : undefined,
    input_price_micros_per_1k: Number.parseInt(draft.input_price_micros_per_1k.trim() || '0', 10),
    output_price_micros_per_1k: Number.parseInt(draft.output_price_micros_per_1k.trim() || '0', 10),
    fallback_price_fen: Number.parseInt(draft.fallback_price_fen.trim() || '0', 10),
  };
}

export function PricingEditorPanel({ canWrite }: PricingEditorPanelProps) {
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const [drafts, setDrafts] = useState<PricingDraft[]>([]);
  const [statusMessage, setStatusMessage] = useState('维护官方模型的 token 计费与兜底价格规则。');

  const pricingQuery = useQuery({
    queryKey: ['admin', 'catalog', 'pricing'],
    queryFn: () => adminApi.getPricingRules(),
    retry: false,
  });

  useEffect(() => {
    if (!pricingQuery.data) {
      return;
    }
    setDrafts(pricingQuery.data.data.map(fromPricingRule));
  }, [pricingQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!pricingQuery.data?.revision) {
        throw new Error('定价规则缺少 revision，请重新加载后再保存。');
      }

      const confirmed = await confirmAction({
        title: '保存定价规则',
        message: `即将保存 ${drafts.length} 条定价规则。`,
        hint: '定价变更会直接影响官方模型计费结果。',
        confirmLabel: '确认保存',
        tone: 'danger',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.savePricingRules(drafts.map(toPricingRule), pricingQuery.data.revision);
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('已取消定价规则保存。');
        return;
      }

      setDrafts(result.data.map(fromPricingRule));
      setStatusMessage('定价规则已保存。');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'catalog', 'pricing'] });
    },
  });

  const statusTone = pricingQuery.isError || saveMutation.isError ? 'danger' : pricingQuery.isFetching ? 'info' : 'success';
  const statusText = pricingQuery.isError
    ? pricingQuery.error instanceof Error
      ? pricingQuery.error.message
      : '加载定价规则失败，请稍后重试。'
    : saveMutation.isError
      ? saveMutation.error instanceof Error
        ? saveMutation.error.message
        : '保存定价规则失败，请稍后重试。'
      : pricingQuery.isFetching
        ? '正在加载定价规则…'
        : statusMessage;

  function updateDraft(index: number, patch: Partial<PricingDraft>) {
    setDrafts(current => current.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item)));
  }

  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>定价规则</h2>
          <p>支持按模型 ID / 版本维护输入、输出 token 单价与兜底人民币分价。</p>
        </div>
      </div>

      <InlineStatus tone={statusTone}>{statusText}</InlineStatus>

      <div className="resource-toolbar">
        <button className="button button--ghost" onClick={() => setDrafts(pricingQuery.data?.data.map(fromPricingRule) ?? [])} type="button">
          重置为服务端版本
        </button>
        <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => [...current, createEmptyPricingDraft()])} type="button">
          新增规则
        </button>
        <button className="button button--primary" disabled={!canWrite || saveMutation.isPending} onClick={() => void saveMutation.mutateAsync()} type="button">
          {saveMutation.isPending ? '保存中…' : '保存定价规则'}
        </button>
      </div>

      <div className="list-grid">
        {drafts.length === 0 ? (
          <InlineStatus tone="warning">当前还没有定价规则，请至少配置一个生效中的价格版本。</InlineStatus>
        ) : (
          drafts.map((draft, index) => (
            <article className="collection-card" key={`${draft.model_id || 'pricing'}-${index}`}>
              <div className="collection-card__header">
                <strong>{draft.model_id || `规则 ${index + 1}`}</strong>
                <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => current.filter((_, itemIndex) => itemIndex !== index))} type="button">
                  删除
                </button>
              </div>

              <div className="form-grid">
                <label className="filter-field">
                  <span>模型 ID</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { model_id: event.target.value })} value={draft.model_id} />
                </label>
                <label className="filter-field">
                  <span>版本</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { version: event.target.value })} value={draft.version} />
                </label>
                <label className="filter-field">
                  <span>生效时间（Unix）</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { effective_from_unix: event.target.value })} value={draft.effective_from_unix} />
                </label>
                <label className="filter-field">
                  <span>输入价（微元 / 1K）</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { input_price_micros_per_1k: event.target.value })} value={draft.input_price_micros_per_1k} />
                </label>
                <label className="filter-field">
                  <span>输出价（微元 / 1K）</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { output_price_micros_per_1k: event.target.value })} value={draft.output_price_micros_per_1k} />
                </label>
                <label className="filter-field">
                  <span>兜底价格（分）</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { fallback_price_fen: event.target.value })} value={draft.fallback_price_fen} />
                </label>
              </div>
            </article>
          ))
        )}
      </div>
    </section>
  );
}
