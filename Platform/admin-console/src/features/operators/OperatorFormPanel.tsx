import { type FormEvent, useEffect, useState } from 'react';

import { InlineStatus } from '../../components/feedback/InlineStatus';
import type { AdminOperator } from '../../services/contracts';
import { adminRoleDefinitions, getAdminRoleDefinition } from './operatorRoles';

type OperatorFormPayload = {
  email: string;
  role: string;
  active: boolean;
};

type OperatorFormPanelProps = {
  selectedOperator: AdminOperator | null;
  draft: OperatorFormPayload;
  canWrite: boolean;
  isSubmitting?: boolean;
  onDraftChange: (patch: Partial<OperatorFormPayload>) => void;
  onReset: () => void;
  onSubmit: (payload: OperatorFormPayload) => Promise<void>;
};

const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function OperatorFormPanel({
  selectedOperator,
  draft,
  canWrite,
  isSubmitting = false,
  onDraftChange,
  onReset,
  onSubmit,
}: OperatorFormPanelProps) {
  const [errorMessage, setErrorMessage] = useState('');

  useEffect(() => {
    setErrorMessage('');
  }, [draft.active, draft.email, draft.role, selectedOperator]);

  const roleDefinition = getAdminRoleDefinition(draft.role);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const normalizedEmail = draft.email.trim().toLowerCase();
    if (!normalizedEmail) {
      setErrorMessage('管理员邮箱不能为空。');
      return;
    }

    if (!emailPattern.test(normalizedEmail)) {
      setErrorMessage('请输入有效的管理员邮箱。');
      return;
    }

    if (!draft.role.trim()) {
      setErrorMessage('请选择管理员角色。');
      return;
    }

    setErrorMessage('');
    await onSubmit({
      email: normalizedEmail,
      role: draft.role,
      active: draft.active,
    });
  }

  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>管理员编辑器</h2>
          <p>可从左侧表格选择管理员，也可直接输入邮箱以创建或更新后台角色。</p>
        </div>
      </div>

      {!canWrite ? <InlineStatus tone="warning">当前管理员只有只读权限，不能修改角色与启用状态。</InlineStatus> : null}

      <form className="form-stack" onSubmit={handleSubmit}>
        <label className="filter-field">
          <span>管理员邮箱</span>
          <input
            disabled={!canWrite || isSubmitting}
            onChange={event => onDraftChange({ email: event.target.value })}
            placeholder="reader@example.com"
            type="email"
            value={draft.email}
          />
        </label>

        <div className="form-grid">
          <label className="filter-field">
            <span>角色</span>
            <select disabled={!canWrite || isSubmitting} onChange={event => onDraftChange({ role: event.target.value })} value={draft.role}>
              {adminRoleDefinitions.map(item => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </select>
          </label>

          <label className="checkbox-field">
            <input checked={draft.active} disabled={!canWrite || isSubmitting} onChange={event => onDraftChange({ active: event.target.checked })} type="checkbox" />
            <span>管理员已启用</span>
          </label>
        </div>

        <div className="operator-form__status">
          <InlineStatus tone={roleDefinition.tone}>{roleDefinition.summary}</InlineStatus>
        </div>

        {errorMessage ? <p className="form-error">{errorMessage}</p> : null}

        <div className="form-actions">
          <button
            className="button button--ghost"
            disabled={isSubmitting}
            onClick={event => {
              event.preventDefault();
              onReset();
              setErrorMessage('');
            }}
            type="button"
          >
            重置表单
          </button>
          <button className="button button--primary" disabled={!canWrite || isSubmitting} type="submit">
            {isSubmitting ? '保存中…' : '保存管理员'}
          </button>
        </div>
      </form>
    </section>
  );
}
