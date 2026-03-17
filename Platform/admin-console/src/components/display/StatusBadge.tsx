import type { PropsWithChildren } from 'react';

export type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger';

type StatusBadgeProps = PropsWithChildren<{
  tone?: StatusTone;
}>;

export function StatusBadge({ tone = 'neutral', children }: StatusBadgeProps) {
  const className = ['status-badge', tone !== 'neutral' ? `is-${tone}` : ''].filter(Boolean).join(' ');

  return <span className={className}>{children}</span>;
}
