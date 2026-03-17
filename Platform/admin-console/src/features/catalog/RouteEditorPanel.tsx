import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { InlineStatus } from '../../components/feedback/InlineStatus';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import { JsonAdvancedEditor } from './JsonAdvancedEditor';
import {
  createEmptyRouteDraft,
  normalizeRouteDrafts,
  redactedSecretPlaceholder,
  routeProtocolMeta,
  routeProtocolOptions,
  serializeRouteDrafts,
  type RouteDraft,
} from './routeProtocol';

type RouteEditorPanelProps = {
  canWrite: boolean;
};

export function RouteEditorPanel({ canWrite }: RouteEditorPanelProps) {
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const [drafts, setDrafts] = useState<RouteDraft[]>([]);
  const [statusMessage, setStatusMessage] = useState('显式维护协议类型 / 调用方式、模型映射与上游配置。');

  const routesQuery = useQuery({
    queryKey: ['admin', 'catalog', 'routes'],
    queryFn: () => adminApi.getOfficialRoutes(),
    retry: false,
  });

  useEffect(() => {
    if (!routesQuery.data) {
      return;
    }
    setDrafts(normalizeRouteDrafts(routesQuery.data.data));
  }, [routesQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!routesQuery.data?.revision) {
        throw new Error('官方路由缺少 revision，请重新加载后再保存。');
      }

      const serialized = serializeRouteDrafts(drafts);
      const confirmed = await confirmAction({
        title: '保存官方路由',
        message: `即将保存 ${serialized.length} 条官方路由。`,
        hint: '路由更新会直接影响官方模型实际调用的上游协议与接口地址。',
        confirmLabel: '确认保存',
        tone: 'danger',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.saveOfficialRoutes(serialized, routesQuery.data.revision);
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('已取消官方路由保存。');
        return;
      }

      setDrafts(normalizeRouteDrafts(result.data));
      setStatusMessage('官方路由已保存。');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'catalog', 'routes'] });
    },
  });

  const statusTone = routesQuery.isError || saveMutation.isError ? 'danger' : routesQuery.isFetching ? 'info' : 'success';
  const statusText = routesQuery.isError
    ? routesQuery.error instanceof Error
      ? routesQuery.error.message
      : '加载官方路由失败，请稍后重试。'
    : saveMutation.isError
      ? saveMutation.error instanceof Error
        ? saveMutation.error.message
        : '保存官方路由失败，请稍后重试。'
      : routesQuery.isFetching
        ? '正在加载官方路由…'
        : statusMessage;

  function updateDraft(index: number, patch: Partial<RouteDraft>) {
    setDrafts(current => current.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item)));
  }

  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>官方路由</h2>
          <p>将前台 public model 映射到具体上游模型与协议，同时保留密钥与 endpoint 的安全占位机制。</p>
        </div>
      </div>

      <InlineStatus tone={statusTone}>{statusText}</InlineStatus>

      <div className="resource-toolbar">
        <button className="button button--ghost" onClick={() => setDrafts(normalizeRouteDrafts(routesQuery.data?.data ?? []))} type="button">
          重置为服务端版本
        </button>
        <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => [...current, createEmptyRouteDraft()])} type="button">
          新增路由
        </button>
        <button className="button button--primary" disabled={!canWrite || saveMutation.isPending} onClick={() => void saveMutation.mutateAsync()} type="button">
          {saveMutation.isPending ? '保存中…' : '保存官方路由'}
        </button>
      </div>

      <div className="list-grid">
        {drafts.length === 0 ? (
          <InlineStatus tone="warning">当前还没有官方路由，请先新增至少一个 public model 映射。</InlineStatus>
        ) : (
          drafts.map((draft, index) => (
            <article className="collection-card" key={`${draft.publicModelID || 'route'}-${index}`}>
              <div className="collection-card__header">
                <strong>{draft.publicModelID || `路由 ${index + 1}`}</strong>
                <button className="button button--ghost" disabled={!canWrite} onClick={() => setDrafts(current => current.filter((_, itemIndex) => itemIndex !== index))} type="button">
                  删除
                </button>
              </div>

              <div className="form-grid">
                <label className="filter-field">
                  <span>对外模型 ID</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { publicModelID: event.target.value })} value={draft.publicModelID} />
                </label>
                <label className="filter-field">
                  <span>显示名称</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { modelName: event.target.value })} value={draft.modelName} />
                </label>
                <label className="filter-field">
                  <span>协议类型 / 调用方式</span>
                  <select disabled={!canWrite} onChange={event => updateDraft(index, { protocol: event.target.value })} value={draft.protocol}>
                    {routeProtocolOptions().map(option => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="filter-field">
                  <span>模型值 / 上游模型 ID</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { modelValue: event.target.value })} value={draft.modelValue} />
                </label>
                <label className="filter-field">
                  <span>API Base</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { apiBase: event.target.value })} value={draft.apiBase} />
                </label>
                <label className="filter-field">
                  <span>代理地址</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { proxy: event.target.value })} value={draft.proxy} />
                </label>
                <label className="filter-field">
                  <span>鉴权方式</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { authMethod: event.target.value })} value={draft.authMethod} />
                </label>
                <label className="filter-field">
                  <span>连接模式</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { connectMode: event.target.value })} value={draft.connectMode} />
                </label>
                <label className="filter-field">
                  <span>工作区</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { workspace: event.target.value })} value={draft.workspace} />
                </label>
                <label className="filter-field">
                  <span>RPM</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { rpm: event.target.value })} value={draft.rpm} />
                </label>
                <label className="filter-field">
                  <span>最大 Tokens 字段名</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { maxTokensField: event.target.value })} value={draft.maxTokensField} />
                </label>
                <label className="filter-field">
                  <span>请求超时（秒）</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { requestTimeout: event.target.value })} value={draft.requestTimeout} />
                </label>
                <label className="filter-field">
                  <span>思考强度</span>
                  <input disabled={!canWrite} onChange={event => updateDraft(index, { thinkingLevel: event.target.value })} value={draft.thinkingLevel} />
                </label>
              </div>

              <p className="collection-card__hint">{routeProtocolMeta(draft.protocol).hint}</p>

              <div className="form-grid">
                <label className="filter-field">
                  <span>新的 API Key</span>
                  <input
                    disabled={!canWrite}
                    onChange={event => updateDraft(index, { apiKeyDraft: event.target.value })}
                    placeholder={draft.keepExistingSecret ? '留空则保留现有密钥' : ''}
                    value={draft.apiKeyDraft}
                  />
                </label>
                <label className="checkbox-field">
                  <input
                    checked={draft.keepExistingSecret}
                    disabled={!canWrite || draft.persistedApiKey.trim() === ''}
                    onChange={event => updateDraft(index, { keepExistingSecret: event.target.checked })}
                    type="checkbox"
                  />
                  <span>{draft.persistedApiKey.trim() === redactedSecretPlaceholder ? '保留现有密钥' : '无现有密钥可保留'}</span>
                </label>
              </div>

              <JsonAdvancedEditor
                disabled={!canWrite}
                label="额外 model_config 字段（JSON）"
                onChange={value => updateDraft(index, { extraConfigJSON: value })}
                value={draft.extraConfigJSON}
              />
            </article>
          ))
        )}
      </div>
    </section>
  );
}
