import { InlineStatus } from '../../components/feedback/InlineStatus';
import { StatusBadge } from '../../components/display/StatusBadge';
import { getAdminRoleDefinition } from './operatorRoles';

export function RoleCapabilityPreview({ role }: { role: string }) {
  const definition = getAdminRoleDefinition(role);

  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>角色权限预览</h2>
          <p>用于确认该角色默认具备的 capability 范围，避免误授予后台高危权限。</p>
        </div>
        <StatusBadge tone={definition.tone}>{definition.label}</StatusBadge>
      </div>

      <InlineStatus tone={definition.tone}>{definition.summary}</InlineStatus>
      {definition.dangerNote ? <InlineStatus tone="warning">{definition.dangerNote}</InlineStatus> : null}

      <div className="capability-chip-grid" aria-label={`${definition.label}能力预览`}>
        {definition.capabilities.map(item => (
          <span className="capability-chip" key={item}>
            {item}
          </span>
        ))}
      </div>
    </section>
  );
}
