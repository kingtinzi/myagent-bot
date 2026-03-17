import { useEffect, useState } from 'react';

import type { StatusTone } from '../display/StatusBadge';

type GlobalToastProps = {
  title?: string;
  message: string;
  tone?: StatusTone;
  durationMs?: number;
};

export function GlobalToast({ title, message, tone = 'info', durationMs = 4000 }: GlobalToastProps) {
  const [visible, setVisible] = useState(true);

  useEffect(() => {
    setVisible(true);

    if (durationMs <= 0) {
      return undefined;
    }

    const timeoutID = globalThis.setTimeout(() => {
      setVisible(false);
    }, durationMs);

    return () => {
      globalThis.clearTimeout(timeoutID);
    };
  }, [durationMs, message, title]);

  if (!message || !visible) {
    return null;
  }

  const className = ['global-toast', `is-${tone}`].join(' ');

  return (
    <div className={className} role="status" aria-live="polite">
      {title ? <strong>{title}</strong> : null}
      <span>{message}</span>
    </div>
  );
}
