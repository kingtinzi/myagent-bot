import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';

import { DataTable } from '../../components/data/DataTable';
import { StatusBadge } from '../../components/display/StatusBadge';
import { InlineStatus } from '../../components/feedback/InlineStatus';
import { PageHeader } from '../../components/layout/PageHeader';
import { adminApi } from '../../services/adminApi';
import type { AdminAuditLog, AdminAuditLogsQuery } from '../../services/contracts';

type AuditFilterDraft = {
  action: string;
  targetType: string;
  targetId: string;
  actorUserId: string;
  riskLevel: string;
  sinceUnix: string;
  untilUnix: string;
};

function createFilterDraft(): AuditFilterDraft {
  return {
    action: '',
    targetType: '',
    targetId: '',
    actorUserId: '',
    riskLevel: '',
    sinceUnix: '',
    untilUnix: '',
  };
}

function toQuery(draft: AuditFilterDraft): AdminAuditLogsQuery {
  const parseOptionalNumber = (value: string) => {
    const trimmed = value.trim();
    if (!trimmed) {
      return undefined;
    }
    const parsed = Number.parseInt(trimmed, 10);
    return Number.isFinite(parsed) ? parsed : undefined;
  };

  return {
    action: draft.action.trim() || undefined,
    targetType: draft.targetType.trim() || undefined,
    targetId: draft.targetId.trim() || undefined,
    actorUserId: draft.actorUserId.trim() || undefined,
    riskLevel: draft.riskLevel.trim() || undefined,
    sinceUnix: parseOptionalNumber(draft.sinceUnix),
    untilUnix: parseOptionalNumber(draft.untilUnix),
    limit: 50,
  };
}

function formatTimestamp(value?: number) {
  if (!value) {
    return '—';
  }

  return new Date(value * 1000).toLocaleString('zh-CN', {
    hour12: false,
  });
}

function riskTone(value?: string): 'neutral' | 'info' | 'success' | 'warning' | 'danger' {
  switch ((value ?? '').trim()) {
    case 'high':
      return 'danger';
    case 'medium':
      return 'warning';
    case 'low':
      return 'info';
    default:
      return 'neutral';
  }
}

function riskLabel(value?: string) {
  switch ((value ?? '').trim()) {
    case 'high':
      return '高风险';
    case 'medium':
      return '中风险';
    case 'low':
      return '低风险';
    default:
      return '未标记';
  }
}

export function AuditsPage() {
  const [filterDraft, setFilterDraft] = useState<AuditFilterDraft>(() => createFilterDraft());
  const [appliedFilters, setAppliedFilters] = useState<AdminAuditLogsQuery>(() => toQuery(createFilterDraft()));
  const [selectedAuditID, setSelectedAuditID] = useState('');
  const [statusMessage, setStatusMessage] = useState('支持按动作、目标、操作者和风险等级筛选高权限操作轨迹。');

  const auditsQuery = useQuery({
    queryKey: ['admin', 'audits', appliedFilters],
    queryFn: () => adminApi.listAuditLogs(appliedFilters),
    retry: false,
  });

  useEffect(() => {
    const items = auditsQuery.data ?? [];

    if (items.length === 0) {
      setSelectedAuditID('');
      return;
    }

    if (!items.some(item => item.id === selectedAuditID)) {
      setSelectedAuditID(items[0].id);
    }
  }, [auditsQuery.data, selectedAuditID]);

  const selectedAudit = useMemo(
    () => auditsQuery.data?.find(item => item.id === selectedAuditID) ?? null,
    [auditsQuery.data, selectedAuditID],
  );

  const columns = useMemo(
    () => [
      {
        key: 'created',
        header: '时间',
        cell: (row: AdminAuditLog) => formatTimestamp(row.created_unix),
      },
      {
        key: 'actor',
        header: '操作者',
        cell: (row: AdminAuditLog) => (
          <div className="user-identity-cell">
            <strong>{row.actor_email || '未知管理员'}</strong>
            <small>{row.actor_user_id || '未记录用户 ID'}</small>
          </div>
        ),
      },
      {
        key: 'action',
        header: '动作',
        cell: (row: AdminAuditLog) => row.action,
      },
      {
        key: 'target',
        header: '目标对象',
        cell: (row: AdminAuditLog) => `${row.target_type || '—'} / ${row.target_id || '—'}`,
      },
      {
        key: 'risk',
        header: '风险级别',
        cell: (row: AdminAuditLog) => <StatusBadge tone={riskTone(row.risk_level)}>{riskLabel(row.risk_level)}</StatusBadge>,
      },
    ],
    [],
  );

  const statusTone = auditsQuery.isError ? 'danger' : auditsQuery.isFetching ? 'info' : 'success';
  const statusText = auditsQuery.isError
    ? auditsQuery.error instanceof Error
      ? auditsQuery.error.message
      : '加载审计日志失败，请稍后重试。'
    : auditsQuery.isFetching
      ? '正在加载审计日志…'
      : statusMessage;

  function applyFilters() {
    setAppliedFilters(toQuery(filterDraft));
    setStatusMessage('已应用审计筛选条件。');
  }

  function resetFilters() {
    const nextDraft = createFilterDraft();
    setFilterDraft(nextDraft);
    setAppliedFilters(toQuery(nextDraft));
    setStatusMessage('已重置审计筛选条件。');
  }

  function exportCSV() {
    const url = adminApi.buildAuditLogsExportURL(appliedFilters);
    globalThis.open?.(url, '_blank', 'noopener,noreferrer');
    setStatusMessage('已发起审计日志 CSV 导出。');
  }

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="安全审计"
        title="高权限操作与风险轨迹"
        description="聚合管理员关键动作、目标对象和风险等级，方便审计巡检、对账复核与问题回溯。"
        meta={
          <>
            <StatusBadge tone="info">只读审计</StatusBadge>
            <StatusBadge tone="warning">{`当前展示 ${auditsQuery.data?.length ?? 0} 条`}</StatusBadge>
          </>
        }
        actions={
          <div className="form-actions">
            <button className="button button--ghost" onClick={exportCSV} type="button">
              导出 CSV
            </button>
          </div>
        }
      />

      <section className="filter-bar-card">
        <div className="filter-bar-card__fields">
          <label className="filter-field">
            <span>动作</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, action: event.target.value }))} value={filterDraft.action} />
          </label>
          <label className="filter-field">
            <span>目标类型</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, targetType: event.target.value }))} value={filterDraft.targetType} />
          </label>
          <label className="filter-field">
            <span>目标 ID</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, targetId: event.target.value }))} value={filterDraft.targetId} />
          </label>
          <label className="filter-field">
            <span>操作者 ID</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, actorUserId: event.target.value }))} value={filterDraft.actorUserId} />
          </label>
          <label className="filter-field">
            <span>风险等级</span>
            <select onChange={event => setFilterDraft(current => ({ ...current, riskLevel: event.target.value }))} value={filterDraft.riskLevel}>
              <option value="">全部</option>
              <option value="high">高风险</option>
              <option value="medium">中风险</option>
              <option value="low">低风险</option>
            </select>
          </label>
          <label className="filter-field">
            <span>起始时间（Unix）</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, sinceUnix: event.target.value }))} value={filterDraft.sinceUnix} />
          </label>
          <label className="filter-field">
            <span>结束时间（Unix）</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, untilUnix: event.target.value }))} value={filterDraft.untilUnix} />
          </label>
        </div>

        <div className="filter-bar-card__actions">
          <button className="button button--ghost" onClick={resetFilters} type="button">
            重置筛选
          </button>
          <button className="button button--primary" onClick={applyFilters} type="button">
            应用筛选
          </button>
        </div>
      </section>

      <InlineStatus tone={statusTone}>{statusText}</InlineStatus>

      <div className="panel-grid panel-grid--balanced">
        <section className="panel">
          <div className="panel__header">
            <div>
              <h2>审计日志列表</h2>
              <p>点击任意日志可在右侧查看动作细节、目标对象与风险描述。</p>
            </div>
          </div>

          <DataTable
            caption="审计日志列表"
            columns={columns}
            emptyMessage="当前筛选条件下未找到审计日志。"
            getRowKey={row => row.id}
            onRowClick={row => {
              setSelectedAuditID(row.id);
              setStatusMessage(`已聚焦审计日志 ${row.id}。`);
            }}
            rows={auditsQuery.data ?? []}
          />
        </section>

        <section className="panel">
          <div className="panel__header">
            <div>
              <h2>审计详情</h2>
              <p>用于回看动作上下文、风险等级与具体说明，辅助导出前的人工复核。</p>
            </div>
          </div>

          {!selectedAudit ? (
            <InlineStatus tone="warning">请选择一条审计日志查看详情。</InlineStatus>
          ) : (
            <>
              <div className="detail-summary-card">
                <strong>{selectedAudit.action}</strong>
                <p>{selectedAudit.detail || '当前日志未填写补充说明。'}</p>
              </div>

              <dl className="detail-meta-list">
                <div>
                  <dt>日志 ID</dt>
                  <dd>{selectedAudit.id}</dd>
                </div>
                <div>
                  <dt>操作者</dt>
                  <dd>{selectedAudit.actor_email || selectedAudit.actor_user_id || '未知管理员'}</dd>
                </div>
                <div>
                  <dt>目标对象</dt>
                  <dd>{`${selectedAudit.target_type || '—'} / ${selectedAudit.target_id || '—'}`}</dd>
                </div>
                <div>
                  <dt>风险级别</dt>
                  <dd>{riskLabel(selectedAudit.risk_level)}</dd>
                </div>
                <div>
                  <dt>发生时间</dt>
                  <dd>{formatTimestamp(selectedAudit.created_unix)}</dd>
                </div>
              </dl>
            </>
          )}
        </section>
      </div>
    </section>
  );
}
