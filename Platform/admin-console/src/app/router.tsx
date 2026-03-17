import { useQueryClient } from '@tanstack/react-query';
import { createBrowserRouter } from 'react-router-dom';

function QueryProviderProbe() {
  const queryClient = useQueryClient();

  return (
    <output hidden data-testid="query-provider-ready">
      {queryClient ? 'ready' : 'missing'}
    </output>
  );
}

function AdminConsoleScaffold() {
  return (
    <main className="admin-shell" data-testid="admin-shell-root">
      <QueryProviderProbe />
      <section className="admin-shell__hero" aria-labelledby="admin-console-heading">
        <span className="admin-shell__eyebrow">Admin Console Alpha</span>
        <div className="admin-shell__headline">
          <h1 id="admin-console-heading">PinchBot 管理后台（重构中）</h1>
          <p>
            Wave 1：前端工程骨架搭建中。后续会在这个壳层中接入用户、钱包、订单、模型与治理模块。
          </p>
        </div>
        <div className="admin-shell__grid">
          <article className="admin-shell__card">
            <strong>统一后台壳层</strong>
            <span>React + TypeScript + Vite</span>
          </article>
          <article className="admin-shell__card">
            <strong>权限驱动导航</strong>
            <span>后续接入 capability gating</span>
          </article>
          <article className="admin-shell__card">
            <strong>模块化业务域</strong>
            <p>用户中心、钱包与订单、模型目录、审核与治理将按业务域重建。</p>
          </article>
        </div>
      </section>
    </main>
  );
}

export function createAppRouter() {
  return createBrowserRouter([
    {
      path: '/',
      element: <AdminConsoleScaffold />,
    },
  ]);
}
