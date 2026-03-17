import { type FormEvent, useMemo, useState } from 'react';

import { InlineStatus } from '../../components/feedback/InlineStatus';
import { StatusBadge } from '../../components/display/StatusBadge';
import { useAdminSession } from '../../hooks/useAdminSession';

type AdminLoginPageProps = {
  initialMessage?: string;
};

const adminHighlights = [
  '按角色收口用户、钱包、订单与治理权限',
  '所有高风险财务与配置动作统一确认',
  '支持中文运营工作流与响应式终端访问',
];

export function AdminLoginPage({ initialMessage = '' }: AdminLoginPageProps) {
  const { login, isAuthenticating, errorMessage } = useAdminSession();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

  const visibleMessage = useMemo(() => {
    const message = errorMessage.trim() || initialMessage.trim();
    if (!message) {
      return '';
    }

    return message;
  }, [errorMessage, initialMessage]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      await login({
        email: email.trim(),
        password,
      });
    } catch {
      // 错误文案由 useAdminSession 写入全局状态，这里不重复抛出。
    }
  }

  return (
    <div className="admin-auth-shell">
      <div className="admin-auth-card">
        <section className="admin-auth-hero">
          <StatusBadge tone="info">PinchBot 平台控制台</StatusBadge>
          <div className="admin-auth-hero__copy">
            <h1>PinchBot 管理后台</h1>
            <p>统一管理官方模型、用户钱包、充值订单、治理规则与高风险后台操作。</p>
          </div>
          <div className="admin-auth-highlight-list">
            {adminHighlights.map(item => (
              <article className="admin-auth-highlight" key={item}>
                <strong>{item}</strong>
                <span>基于真实管理员 session 与 capability 动态收口页面与操作范围。</span>
              </article>
            ))}
          </div>
        </section>

        <section className="admin-auth-panel">
          <div className="admin-auth-panel__header">
            <div>
              <h2>管理员登录</h2>
              <p>使用已授予后台权限的邮箱账号登录。未分配管理员角色的账号无法进入后台。</p>
            </div>
          </div>

          {visibleMessage ? <InlineStatus tone="warning">{visibleMessage}</InlineStatus> : null}

          <form className="admin-auth-form" onSubmit={handleSubmit}>
            <label className="admin-auth-field">
              <span>邮箱</span>
              <input
                autoComplete="email"
                disabled={isAuthenticating}
                name="email"
                onChange={event => setEmail(event.target.value)}
                placeholder="admin@example.com"
                type="email"
                value={email}
              />
            </label>

            <label className="admin-auth-field">
              <span>密码</span>
              <input
                autoComplete="current-password"
                disabled={isAuthenticating}
                name="password"
                onChange={event => setPassword(event.target.value)}
                placeholder="请输入后台密码"
                type="password"
                value={password}
              />
            </label>

            <div className="admin-auth-actions">
              <button className="button button--primary admin-auth-submit" disabled={isAuthenticating} type="submit">
                {isAuthenticating ? '登录中…' : '登录后台'}
              </button>
            </div>
          </form>
        </section>
      </div>
    </div>
  );
}
