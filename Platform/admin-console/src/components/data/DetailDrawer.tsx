import type { PropsWithChildren, ReactNode } from 'react';

type DetailDrawerProps = PropsWithChildren<{
  actions?: ReactNode;
  description?: string;
  open: boolean;
  onClose: () => void;
  title: string;
}>;

export function DetailDrawer({ actions, children, description, open, onClose, title }: DetailDrawerProps) {
  if (!open) {
    return null;
  }

  return (
    <div className="detail-drawer-backdrop" onClick={onClose} role="presentation">
      <aside
        aria-label={title}
        className="detail-drawer"
        onClick={event => event.stopPropagation()}
        role="dialog"
      >
        <header className="detail-drawer__header">
          <div className="detail-drawer__copy">
            <h2>{title}</h2>
            {description ? <p>{description}</p> : null}
          </div>
          <div className="detail-drawer__toolbar">
            {actions}
            <button className="button button--ghost" onClick={onClose} type="button">
              关闭
            </button>
          </div>
        </header>
        <div className="detail-drawer__body">{children}</div>
      </aside>
    </div>
  );
}
