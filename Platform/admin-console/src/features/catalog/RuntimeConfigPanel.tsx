import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { InlineStatus } from '../../components/feedback/InlineStatus';
import { StatusBadge } from '../../components/display/StatusBadge';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import type { RuntimeConfigState } from '../../services/contracts';
import { JsonAdvancedEditor } from './JsonAdvancedEditor';

type RuntimeConfigPanelProps = {
  canWrite: boolean;
};

function stringifyRuntimeConfig(state: RuntimeConfigState) {
  return JSON.stringify(state, null, 2);
}

function parseRuntimeConfig(jsonText: string): RuntimeConfigState {
  return JSON.parse(jsonText) as RuntimeConfigState;
}

export function RuntimeConfigPanel({ canWrite }: RuntimeConfigPanelProps) {
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const [draft, setDraft] = useState('{}');
  const [statusMessage, setStatusMessage] = useState('高级运行时快照适合用于审计、迁移与紧急修复，请谨慎操作。');

  const runtimeQuery = useQuery({
    queryKey: ['admin', 'catalog', 'runtime'],
    queryFn: () => adminApi.getRuntimeConfig(),
    retry: false,
  });

  useEffect(() => {
    if (!runtimeQuery.data) {
      return;
    }
    setDraft(stringifyRuntimeConfig(runtimeQuery.data.data));
  }, [runtimeQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!runtimeQuery.data?.revision) {
        throw new Error('运行时快照缺少 revision，请重新加载后再保存。');
      }

      let payload: RuntimeConfigState;
      try {
        payload = parseRuntimeConfig(draft);
      } catch (error) {
        throw new Error(error instanceof Error ? `运行时快照 JSON 无效：${error.message}` : '运行时快照 JSON 无效，请修正后重试。');
      }

      const confirmed = await confirmAction({
        title: '保存运行时快照',
        message: '即将直接覆盖运行时快照中的模型、路由、定价与协议配置。',
        hint: '这是最高风险的目录操作，建议仅在结构化面板无法完成迁移时使用。保存前请输入 RUNTIME 确认。',
        confirmLabel: '确认覆盖',
        tone: 'danger',
        requireText: 'RUNTIME',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.saveRuntimeConfig(payload, runtimeQuery.data.revision);
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('已取消运行时快照保存。');
        return;
      }

      setDraft(stringifyRuntimeConfig(result.data));
      setStatusMessage('运行时快照已保存。');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'catalog', 'runtime'] });
    },
  });

  const statusTone = runtimeQuery.isError || saveMutation.isError ? 'danger' : runtimeQuery.isFetching ? 'info' : 'warning';
  const statusText = runtimeQuery.isError
    ? runtimeQuery.error instanceof Error
      ? runtimeQuery.error.message
      : '加载运行时快照失败，请稍后重试。'
    : saveMutation.isError
      ? saveMutation.error instanceof Error
        ? saveMutation.error.message
        : '保存运行时快照失败，请稍后重试。'
      : runtimeQuery.isFetching
        ? '正在加载运行时快照…'
        : statusMessage;

  const summary = useMemo(() => {
    const state = runtimeQuery.data?.data;
    if (!state) {
      return [];
    }

    return [
      { label: '官方模型', value: `${state.official_models.length} 项`, tone: 'info' as const },
      { label: '官方路由', value: `${state.official_routes.length} 条`, tone: 'warning' as const },
      { label: '定价规则', value: `${state.pricing_rules.length} 条`, tone: 'success' as const },
      { label: '协议版本', value: `${state.agreements.length} 份`, tone: 'neutral' as const },
    ];
  }, [runtimeQuery.data]);

  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>运行时快照</h2>
          <p>保留完整的原始配置视图，适合做迁移核对、批量导入导出与应急恢复。</p>
        </div>
        <StatusBadge tone="danger">高级模式</StatusBadge>
      </div>

      <InlineStatus tone={statusTone}>{statusText}</InlineStatus>

      <div className="resource-toolbar">
        <button className="button button--ghost" onClick={() => setDraft(runtimeQuery.data ? stringifyRuntimeConfig(runtimeQuery.data.data) : '{}')} type="button">
          重置为服务端版本
        </button>
        <button className="button button--primary" disabled={!canWrite || saveMutation.isPending} onClick={() => void saveMutation.mutateAsync()} type="button">
          {saveMutation.isPending ? '保存中…' : '保存运行时快照'}
        </button>
      </div>

      <div className="catalog-summary-grid">
        {summary.map(item => (
          <article className="catalog-summary-card" key={item.label}>
            <span>{item.label}</span>
            <strong>{item.value}</strong>
            <StatusBadge tone={item.tone}>已纳入快照</StatusBadge>
          </article>
        ))}
      </div>

      <JsonAdvancedEditor
        disabled={!canWrite}
        label="完整运行时 JSON"
        onChange={setDraft}
        value={draft}
      />
    </section>
  );
}
