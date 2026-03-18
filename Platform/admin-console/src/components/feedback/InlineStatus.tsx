import type { PropsWithChildren } from 'react';

import type { StatusTone } from '../display/StatusBadge';

type InlineStatusProps = PropsWithChildren<{
  tone?: StatusTone;
}>;

export function InlineStatus({ tone = 'info', children }: InlineStatusProps) {
  const className = ['inline-status', `is-${tone}`].join(' ');

  return (
    <div className={className} role="status" aria-live="polite">
      {children}
    </div>
  );
}
