import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { DataTable } from '../../components/data/DataTable';
import { InlineStatus } from '../../components/feedback/InlineStatus';
import { StatusBadge } from '../../components/display/StatusBadge';
import { PageHeader } from '../../components/layout/PageHeader';
import { PermissionGate } from '../../components/navigation/PermissionGate';
import { OperatorFormPanel } from '../../features/operators/OperatorFormPanel';
import { RoleCapabilityPreview } from '../../features/operators/RoleCapabilityPreview';
import { getAdminRoleDefinition } from '../../features/operators/operatorRoles';
import { useCapabilities } from '../../hooks/useCapabilities';
import { useConfirmAction } from '../../hooks/useConfirmAction';
import { adminApi } from '../../services/adminApi';
import type { AdminOperator } from '../../services/contracts';

type OperatorDraft = {
  email: string;
  role: string;
  active: boolean;
};

function createDraft(operator?: AdminOperator | null): OperatorDraft {
  return {
    email: operator?.email ?? '',
    role: operator?.role ?? 'super_admin',
    active: operator?.active ?? true,
  };
}

function formatUpdatedAt(value?: number) {
  if (!value) {
    return '—';
  }

  return new Date(value * 1000).toLocaleString('zh-CN', {
    hour12: false,
  });
}

export function OperatorsPage() {
  const queryClient = useQueryClient();
  const confirmAction = useConfirmAction();
  const capabilities = useCapabilities();
  const [selectedEmail, setSelectedEmail] = useState('');
  const [draft, setDraft] = useState<OperatorDraft>(() => createDraft());
  const [statusMessage, setStatusMessage] = useState('选择管理员后可查看角色与权限范围。');

  const operatorsQuery = useQuery({
    queryKey: ['admin', 'operators'],
    queryFn: () => adminApi.listOperators(),
    retry: false,
  });

  const selectedOperator = useMemo(
    () => operatorsQuery.data?.find(item => item.email === selectedEmail) ?? null,
    [operatorsQuery.data, selectedEmail],
  );

  useEffect(() => {
    const operators = operatorsQuery.data ?? [];
    if (operators.length === 0) {
      setSelectedEmail('');
      setDraft(createDraft());
      return;
    }

    const hasSelected = operators.some(item => item.email === selectedEmail);
    const nextOperator = hasSelected ? operators.find(item => item.email === selectedEmail) ?? operators[0] : operators[0];

    setSelectedEmail(nextOperator.email);
    setDraft(previous => {
      if (previous.email === nextOperator.email && previous.role === nextOperator.role && previous.active === nextOperator.active) {
        return previous;
      }
      return createDraft(nextOperator);
    });
  }, [operatorsQuery.data, selectedEmail]);

  const saveMutation = useMutation({
    mutationFn: async (payload: OperatorDraft) => {
      const roleDefinition = getAdminRoleDefinition(payload.role);
      const current = operatorsQuery.data?.find(item => item.email === payload.email) ?? null;
      const confirmed = await confirmAction({
        title: '管理员角色变更',
        message: `即将保存管理员 ${payload.email}。`,
        hint: [
          current ? `当前角色：${getAdminRoleDefinition(current.role).label}。` : '这将创建一条新的管理员记录。',
          `变更后角色：${roleDefinition.label}。`,
          payload.active ? '管理员将继续保持启用状态。' : '管理员访问权限将被停用。',
          roleDefinition.dangerNote ?? '',
        ]
          .filter(Boolean)
          .join(' '),
        confirmLabel: '确认保存',
        tone: payload.role === 'super_admin' || !payload.active ? 'danger' : 'warning',
        requireText: payload.role === 'super_admin' ? 'SUPERADMIN' : '',
      });

      if (!confirmed) {
        return null;
      }

      return adminApi.saveOperator(payload.email, {
        role: payload.role,
        active: payload.active,
      });
    },
    onSuccess: result => {
      if (!result) {
        setStatusMessage('管理员更新已取消。');
        return;
      }
      setSelectedEmail(result.email);
      setDraft(createDraft(result));
      setStatusMessage('管理员已保存。');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'operators'] });
    },
  });

  const columns = useMemo(
    () => [
      {
        key: 'email',
        header: '管理员',
        cell: (row: AdminOperator) => (
          <div className="user-identity-cell">
            <strong>{row.email}</strong>
            <small>{row.user_id || '尚未绑定用户 ID'}</small>
          </div>
        ),
      },
      {
        key: 'role',
        header: '角色',
        cell: (row: AdminOperator) => <StatusBadge tone={getAdminRoleDefinition(row.role).tone}>{getAdminRoleDefinition(row.role).label}</StatusBadge>,
      },
      {
        key: 'active',
        header: '状态',
        cell: (row: AdminOperator) => <StatusBadge tone={row.active ? 'success' : 'danger'}>{row.active ? '启用' : '停用'}</StatusBadge>,
      },
      {
        key: 'capabilities',
        header: '权限数',
        cell: (row: AdminOperator) => `${row.capabilities.length} 项`,
      },
      {
        key: 'updated',
        header: '最近更新',
        cell: (row: AdminOperator) => formatUpdatedAt(row.updated_unix),
      },
    ],
    [],
  );

  if (!capabilities.canAccessModule('operators')) {
    return <InlineStatus tone="warning">当前管理员没有查看管理员与权限模块的权限。</InlineStatus>;
  }

  return (
    <section className="page-stack">
      <PageHeader
        eyebrow="权限治理"
        title="管理员与权限"
        description="统一管理后台账号角色、能力边界与启用状态，所有高风险变更都需显式确认。"
        meta={<StatusBadge tone={capabilities.hasCapability('operators.write') ? 'success' : 'warning'}>{capabilities.hasCapability('operators.write') ? '可编辑' : '只读模式'}</StatusBadge>}
      />

      <InlineStatus tone={operatorsQuery.isFetching ? 'info' : 'success'}>
        {operatorsQuery.isFetching ? '正在加载管理员目录…' : statusMessage}
      </InlineStatus>

      <div className="panel-grid panel-grid--balanced">
        <section className="panel">
          <div className="panel__header">
            <div>
              <h2>管理员目录</h2>
              <p>点击任意管理员可切换右侧编辑器与角色能力预览。</p>
            </div>
          </div>

          <DataTable
            caption="管理员列表"
            columns={columns}
            emptyMessage="当前没有管理员记录。"
            getRowKey={row => row.email}
            onRowClick={row => {
              setSelectedEmail(row.email);
              setDraft(createDraft(row));
              setStatusMessage(`已切换到管理员 ${row.email}。`);
            }}
            rows={operatorsQuery.data ?? []}
          />
        </section>

        <div className="page-stack">
          <PermissionGate
            capability="operators.write"
            fallback={
              <section className="panel">
                <div className="panel__header">
                  <div>
                    <h2>管理员编辑器</h2>
                    <p>当前账号仅可巡检管理员目录，不能修改角色与启用状态。</p>
                  </div>
                </div>
                <InlineStatus tone="warning">需要 operators.write 权限才能保存管理员角色与状态。</InlineStatus>
              </section>
            }
          >
            <OperatorFormPanel
              canWrite={capabilities.hasCapability('operators.write')}
              draft={draft}
              isSubmitting={saveMutation.isPending}
              onDraftChange={patch => setDraft(previous => ({ ...previous, ...patch }))}
              onReset={() => setDraft(createDraft(selectedOperator))}
              onSubmit={async payload => {
                await saveMutation.mutateAsync(payload);
              }}
              selectedOperator={selectedOperator}
            />
          </PermissionGate>

          <RoleCapabilityPreview role={draft.role} />
        </div>
      </div>
    </section>
  );
}
