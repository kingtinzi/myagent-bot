import type { PropsWithChildren } from 'react';
import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';

import { useUIStore } from '../../stores/uiStore';
import { ConfirmDialog } from '../feedback/ConfirmDialog';
import { GlobalToast } from '../feedback/GlobalToast';
import { AdminSidebar, type AdminNavigationGroup } from './AdminSidebar';
import { AdminTopbar, type AdminOperatorSummary } from './AdminTopbar';

export type AdminShellOperator = AdminOperatorSummary;

export type AdminShellToast = {
  title?: string;
  message: string;
  tone?: 'neutral' | 'info' | 'success' | 'warning' | 'danger';
};

type AdminShellProps = PropsWithChildren<{
  navigationGroups: AdminNavigationGroup[];
  operator: AdminShellOperator;
  toast?: AdminShellToast;
  onRefresh?: () => void;
  onSignOut?: () => void;
}>;

export function AdminShell({
  navigationGroups,
  operator,
  toast,
  onRefresh,
  onSignOut,
  children,
}: AdminShellProps) {
  const location = useLocation();
  const sidebarOpen = useUIStore(state => state.sidebarOpen);
  const setSidebarOpen = useUIStore(state => state.setSidebarOpen);
  const toggleSidebar = useUIStore(state => state.toggleSidebar);

  useEffect(() => {
    setSidebarOpen(false);
  }, [location.pathname]);

  return (
    <div className="admin-app" data-testid="admin-shell-root">
      <a className="skip-link" href="#admin-main-content">
        跳到主内容
      </a>

      {toast ? <GlobalToast message={toast.message} tone={toast.tone} title={toast.title} /> : null}

      <AdminTopbar
        isSidebarOpen={sidebarOpen}
        onRefresh={onRefresh}
        onSignOut={onSignOut}
        onToggleSidebar={toggleSidebar}
        operator={operator}
      />

      <div className="admin-shell">
        <button
          aria-hidden={!sidebarOpen}
          className={['admin-shell__backdrop', sidebarOpen ? 'is-visible' : ''].filter(Boolean).join(' ')}
          onClick={() => setSidebarOpen(false)}
          tabIndex={sidebarOpen ? 0 : -1}
          type="button"
        />
        <AdminSidebar groups={navigationGroups} isOpen={sidebarOpen} onNavigate={() => setSidebarOpen(false)} />
        <main className="admin-main" id="admin-main-content" tabIndex={-1}>
          {children}
        </main>
      </div>
      <ConfirmDialog />
    </div>
  );
}
