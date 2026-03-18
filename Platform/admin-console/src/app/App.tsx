import { useState } from 'react';
import { RouterProvider } from 'react-router-dom';

import { AppProviders } from './providers';
import { createAppRouter } from './router';
import { InlineStatus } from '../components/feedback/InlineStatus';
import { useAdminSession } from '../hooks/useAdminSession';
import { AdminLoginPage } from '../pages/auth/AdminLoginPage';

function AppBootScreen() {
  return (
    <div className="admin-auth-shell">
      <section aria-live="polite" className="admin-auth-loading" role="status">
        <div className="admin-auth-loading__panel">
          <strong>正在恢复管理员会话</strong>
          <InlineStatus tone="info">PinchBot 管理后台正在校验登录态与权限能力，请稍候…</InlineStatus>
        </div>
      </section>
    </div>
  );
}

function AppViewport() {
  const [router] = useState(() => createAppRouter());
  const { session, status, errorMessage, sessionQuery } = useAdminSession({ enabled: true });

  if (!session && (status === 'idle' || status === 'loading' || sessionQuery.isPending || sessionQuery.isFetching)) {
    return <AppBootScreen />;
  }

  if (!session) {
    return <AdminLoginPage initialMessage={errorMessage} />;
  }

  return <RouterProvider router={router} />;
}

export default function App() {
  return (
    <AppProviders>
      <AppViewport />
    </AppProviders>
  );
}
