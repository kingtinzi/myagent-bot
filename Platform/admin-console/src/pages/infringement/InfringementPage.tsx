import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { DataTable } from '../../components/data/DataTable';
import { StatusBadge } from '../../components/display/StatusBadge';
import { InlineStatus } from '../../components/feedback/InlineStatus';
import { PageHeader } from '../../components/layout/PageHeader';
import { PermissionGate } from '../../components/navigation/PermissionGate';
import { useCapabilities } from '../../hooks/useCapabilities';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import type { InfringementReport } from '../../services/contracts';

type InfringementFilterDraft = {
  userId: string;
  keyword: string;
  status: string;
  reviewedBy: string;
};

function createFilterDraft(): InfringementFilterDraft {
  return {
    userId: '',
    keyword: '',
    status: '',
    reviewedBy: '',
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

function safeExternalURL(value: string) {
  const text = String(value ?? '').trim();
  if (!text) {
    return '';
  }

  try {
    const parsed = new URL(text);
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.href : '';
  } catch {
    return '';
  }
}

function statusTone(status: string | undefined): 'neutral' | 'info' | 'success' | 'warning' | 'danger' {
  switch ((status ?? '').trim()) {
    case 'resolved':
    case 'closed':
      return 'success';
    case 'under_review':
      return 'warning';
    case 'rejected':
      return 'info';
    case 'pending':
      return 'danger';
    default:
      return 'neutral';
  }
}

function statusLabel(status: string | undefined) {
  switch ((status ?? '').trim()) {
    case 'pending':
      return '待处理';
    case 'under_review':
      return '处理中';
    case 'resolved':
      return '已处置';
    case 'closed':
      return '已关闭';
    case 'rejected':
      return '已驳回';
    default:
      return status?.trim() || '未知状态';
  }
}

function toQuery(draft: InfringementFilterDraft) {
  return {
    userId: draft.userId.trim() || undefined,
    keyword: draft.keyword.trim() || undefined,
    status: draft.status.trim() || undefined,
    reviewedBy: draft.reviewedBy.trim() || undefined,
    limit: 50,
  };
}

export function InfringementPage() {
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const capabilities = useCapabilities();
  const [filterDraft, setFilterDraft] = useState<InfringementFilterDraft>(() => createFilterDraft());
  const [appliedFilters, setAppliedFilters] = useState(() => toQuery(createFilterDraft()));
  const [selectedReportID, setSelectedReportID] = useState('');
  const [nextStatus, setNextStatus] = useState('under_review');
  const [resolution, setResolution] = useState('');
  const [statusMessage, setStatusMessage] = useState('侵权处理台聚合案件、证据和结论，保证审核留痕。');

  const reportsQuery = useQuery({
    queryKey: ['admin', 'infringement', appliedFilters],
    queryFn: () => adminApi.listInfringementReports(appliedFilters),
    retry: false,
  });

  const selectedReport = useMemo(
    () => reportsQuery.data?.find(item => item.id === selectedReportID) ?? null,
    [reportsQuery.data, selectedReportID],
  );

  useEffect(() => {
    const items = reportsQuery.data ?? [];
    if (items.length === 0) {
      setSelectedReportID('');
      return;
    }

    const next = items.find(item => item.id === selectedReportID) ?? items[0];
    setSelectedReportID(next.id);
  }, [reportsQuery.data, selectedReportID]);

  useEffect(() => {
    if (!selectedReport) {
      setNextStatus('under_review');
      setResolution('');
      return;
    }
    setNextStatus(selectedReport.status === 'pending' ? 'under_review' : selectedReport.status);
    setResolution(selectedReport.resolution ?? '');
  }, [selectedReport?.id]);

  const updateMutation = useMutation({
    mutationFn: async (report: InfringementReport) => {
      const confirmed = await confirmAction({
        title: `更新侵权报告：${report.id}`,
        message: `即将把案件「${report.subject}」更新为「${statusLabel(nextStatus)}」。`,
        hint: '该操作会写入案件处理结论与 reviewed_by，请输入 CONFIRM 继续。',
        confirmLabel: '确认更新',
        tone: 'danger',
        requireText: 'CONFIRM',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.updateInfringementReport(report.id, {
        status: nextStatus.trim(),
        resolution: resolution.trim() || undefined,
      });
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('侵权处理更新已取消。');
        return;
      }
      setSelectedReportID(result.id);
      setStatusMessage(`侵权报告 ${result.id} 已更新为「${statusLabel(result.status)}」。`);
      void queryClient.invalidateQueries({ queryKey: ['admin', 'infringement'] });
    },
  });

  const columns = useMemo(
    () => [
      {
        key: 'subject',
        header: '案件',
        cell: (row: InfringementReport) => (
          <div className="user-identity-cell">
            <strong>{row.subject}</strong>
            <small>{row.id}</small>
          </div>
        ),
      },
      {
        key: 'user',
        header: '用户',
        cell: (row: InfringementReport) => (
          <div className="user-identity-cell">
            <strong>{row.username || row.user_no || row.user_id}</strong>
            <small>{row.user_id}</small>
          </div>
        ),
      },
      {
        key: 'status',
        header: '状态',
        cell: (row: InfringementReport) => <StatusBadge tone={statusTone(row.status)}>{statusLabel(row.status)}</StatusBadge>,
      },
      {
        key: 'reviewedBy',
        header: '处理人',
        cell: (row: InfringementReport) => row.reviewed_by || '—',
      },
      {
        key: 'updated',
        header: '最近更新',
        cell: (row: InfringementReport) => formatTimestamp(row.updated_unix),
      },
    ],
    [],
  );

  const statusToneValue = reportsQuery.isError || updateMutation.isError ? 'danger' : reportsQuery.isFetching ? 'info' : 'success';
  const statusText = reportsQuery.isError
    ? reportsQuery.error instanceof Error
      ? reportsQuery.error.message
      : '加载侵权报告失败，请稍后重试。'
    : updateMutation.isError
      ? updateMutation.error instanceof Error
        ? updateMutation.error.message
        : '更新侵权报告失败，请稍后重试。'
      : reportsQuery.isFetching
        ? '正在加载侵权报告…'
        : statusMessage;

  function applyFilters() {
    setAppliedFilters(toQuery(filterDraft));
    setStatusMessage('已应用侵权筛选条件。');
  }

  function resetFilters() {
    const next = createFilterDraft();
    setFilterDraft(next);
    setAppliedFilters(toQuery(next));
    setStatusMessage('已重置侵权筛选条件。');
  }

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="侵权处理"
        title="案件、证据与处理结论"
        description="围绕侵权举报建立完整处理台，帮助治理管理员快速查看证据、统一更新状态并保留结论。"
        meta={
          <>
            <StatusBadge tone="warning">{`当前展示 ${reportsQuery.data?.length ?? 0} 条`}</StatusBadge>
            <StatusBadge tone={capabilities.hasCapability('infringement.review') ? 'success' : 'info'}>
              {capabilities.hasCapability('infringement.review') ? '可处理' : '只读巡检'}
            </StatusBadge>
          </>
        }
      />

      <section className="filter-bar-card">
        <div className="filter-bar-card__fields">
          <label className="filter-field">
            <span>用户 ID</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, userId: event.target.value }))} value={filterDraft.userId} />
          </label>
          <label className="filter-field">
            <span>用户关键词</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, keyword: event.target.value }))} value={filterDraft.keyword} />
          </label>
          <label className="filter-field">
            <span>案件状态</span>
            <select onChange={event => setFilterDraft(current => ({ ...current, status: event.target.value }))} value={filterDraft.status}>
              <option value="">全部</option>
              <option value="pending">待处理</option>
              <option value="under_review">处理中</option>
              <option value="resolved">已处置</option>
              <option value="closed">已关闭</option>
              <option value="rejected">已驳回</option>
            </select>
          </label>
          <label className="filter-field">
            <span>处理人</span>
            <input onChange={event => setFilterDraft(current => ({ ...current, reviewedBy: event.target.value }))} value={filterDraft.reviewedBy} />
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

      <InlineStatus tone={statusToneValue}>{statusText}</InlineStatus>

      <div className="panel-grid panel-grid--balanced">
        <section className="panel">
          <div className="panel__header">
            <div>
              <h2>侵权案件列表</h2>
              <p>点击任意案件，在右侧查看证据、处理进展与结论编辑区。</p>
            </div>
          </div>

          <DataTable
            caption="侵权案件列表"
            columns={columns}
            emptyMessage="当前筛选条件下未找到侵权报告。"
            getRowKey={row => row.id}
            onRowClick={row => {
              setSelectedReportID(row.id);
              setStatusMessage(`已聚焦侵权报告 ${row.id}。`);
            }}
            rows={reportsQuery.data ?? []}
          />
        </section>

        <section className="panel">
          <div className="panel__header">
            <div>
              <h2>案件处理工作台</h2>
              <p>证据阅读、状态更新和处理结论统一在这里完成，避免多处留痕不一致。</p>
            </div>
          </div>

          {!selectedReport ? (
            <InlineStatus tone="warning">请选择一条侵权报告查看详情。</InlineStatus>
          ) : (
            <div className="form-stack">
              <div className="detail-summary-card">
                <strong>{selectedReport.subject}</strong>
                <p>{selectedReport.description}</p>
              </div>

              <dl className="detail-meta-list">
                <div>
                  <dt>案件 ID</dt>
                  <dd>{selectedReport.id}</dd>
                </div>
                <div>
                  <dt>用户</dt>
                  <dd>{selectedReport.username || selectedReport.user_no || selectedReport.user_id}</dd>
                </div>
                <div>
                  <dt>当前状态</dt>
                  <dd>{statusLabel(selectedReport.status)}</dd>
                </div>
                <div>
                  <dt>处理人</dt>
                  <dd>{selectedReport.reviewed_by || '—'}</dd>
                </div>
                <div>
                  <dt>最近更新</dt>
                  <dd>{formatTimestamp(selectedReport.updated_unix)}</dd>
                </div>
              </dl>

              <section className="panel">
                <div className="panel__header">
                  <div>
                    <h2>证据链接</h2>
                    <p>仅允许 http / https 外链；不安全链接会被阻止展示。</p>
                  </div>
                </div>

                <div className="list-grid">
                  {(selectedReport.evidence_urls ?? []).length === 0 ? (
                    <InlineStatus tone="warning">当前案件未附带证据链接。</InlineStatus>
                  ) : (
                    selectedReport.evidence_urls?.map(link => {
                      const safeLink = safeExternalURL(link);
                      return safeLink ? (
                        <article className="collection-card" key={link}>
                          <strong>{safeLink}</strong>
                          <a href={safeLink} rel="noreferrer noopener" target="_blank">
                            打开证据链接
                          </a>
                        </article>
                      ) : (
                        <article className="collection-card" key={link}>
                          <strong>已拦截不安全链接</strong>
                          <p className="collection-card__hint">{link}</p>
                        </article>
                      );
                    })
                  )}
                </div>
              </section>

              <PermissionGate
                capability="infringement.review"
                fallback={<InlineStatus tone="info">当前账号只能查看侵权详情，不能提交处理结论。</InlineStatus>}
              >
                <div className="form-grid">
                  <label className="filter-field">
                    <span>下一状态</span>
                    <select onChange={event => setNextStatus(event.target.value)} value={nextStatus}>
                      <option value="under_review">处理中</option>
                      <option value="resolved">已处置</option>
                      <option value="closed">已关闭</option>
                      <option value="rejected">已驳回</option>
                    </select>
                  </label>
                </div>

                <label className="filter-field">
                  <span>处理结论</span>
                  <textarea className="textarea-field textarea-field--lg" onChange={event => setResolution(event.target.value)} value={resolution} />
                </label>

                <div className="form-actions">
                  <button className="button button--primary" disabled={updateMutation.isPending} onClick={() => void updateMutation.mutateAsync(selectedReport)} type="button">
                    更新案件状态
                  </button>
                </div>
              </PermissionGate>
            </div>
          )}
        </section>
      </div>
    </section>
  );
}
