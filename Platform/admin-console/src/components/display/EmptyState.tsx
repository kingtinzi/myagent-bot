import type { ReactNode } from 'react';

type EmptyStateProps = {
  eyebrow?: string;
  title: string;
  description: string;
  action?: ReactNode;
};

export function EmptyState({ eyebrow, title, description, action }: EmptyStateProps) {
  return (
    <section className="empty-state">
      {eyebrow ? <span className="empty-state__eyebrow">{eyebrow}</span> : null}
      <div className="empty-state__copy">
        <h3>{title}</h3>
        <p>{description}</p>
      </div>
      {action ? <div className="empty-state__actions">{action}</div> : null}
    </section>
  );
}
