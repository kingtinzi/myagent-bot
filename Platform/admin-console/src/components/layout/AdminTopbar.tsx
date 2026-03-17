import { StatusBadge, type StatusTone } from '../display/StatusBadge';

export type AdminOperatorSummary = {
  displayName: string;
  meta: string;
  roleLabel: string;
  roleTone?: StatusTone;
};

type AdminTopbarProps = {
  operator: AdminOperatorSummary;
  isSidebarOpen: boolean;
  onRefresh?: () => void;
  onSignOut?: () => void;
  onToggleSidebar: () => void;
};

function BrandGlyph() {
  return (
    <svg aria-hidden="true" className="admin-topbar__brand-glyph" viewBox="0 0 24 24">
      <path d="M5 6.5a2.5 2.5 0 0 1 2.5-2.5h3.2a4.3 4.3 0 0 1 3.04 1.26l4 4A4.3 4.3 0 0 1 19 12.3v4.2A3.5 3.5 0 0 1 15.5 20H8.5A3.5 3.5 0 0 1 5 16.5v-10Z" />
      <path d="M9 8h4.2a1.8 1.8 0 0 1 1.27.53l1.99 1.99A1.8 1.8 0 0 1 17 11.79V15" />
    </svg>
  );
}

export function AdminTopbar({
  operator,
  isSidebarOpen,
  onRefresh,
  onSignOut,
  onToggleSidebar,
}: AdminTopbarProps) {
  return (
    <header className="admin-topbar">
      <div className="admin-topbar__lead">
        <button
          aria-controls="admin-sidebar"
          aria-expanded={isSidebarOpen}
          aria-label={isSidebarOpen ? '收起导航' : '展开导航'}
          className="button button--ghost admin-topbar__menu"
          onClick={onToggleSidebar}
          type="button"
        >
          <span />
          <span />
          <span />
        </button>

        <div className="admin-topbar__brand">
          <span className="admin-topbar__brand-mark">
            <BrandGlyph />
          </span>
          <div className="admin-topbar__brand-copy">
            <strong>PinchBot 管理后台</strong>
            <span>极简企业 SaaS 后台壳层</span>
          </div>
        </div>
      </div>

      <div className="admin-topbar__actions">
        <div className="admin-topbar__identity">
          <strong>{operator.displayName}</strong>
          <span>{operator.meta}</span>
        </div>
        <StatusBadge tone={operator.roleTone ?? 'info'}>{operator.roleLabel}</StatusBadge>
        <button className="button button--ghost" onClick={onRefresh} type="button">
          刷新后台
        </button>
        <button className="button button--primary" onClick={onSignOut} type="button">
          退出登录
        </button>
      </div>
    </header>
  );
}
