import { StatusBadge } from '../display/StatusBadge';
import type { TrendPanelItem } from '../../pages/dashboard/dashboard.types';

type SimpleTrendPanelProps = {
  title: string;
  description: string;
  items: TrendPanelItem[];
};

export function SimpleTrendPanel({ title, description, items }: SimpleTrendPanelProps) {
  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>{title}</h2>
          <p>{description}</p>
        </div>
      </div>
      <div className="list-grid">
        {items.map(item => (
          <article className="info-row" key={`${item.title}-${item.badge}`}>
            <div className="info-row__copy">
              <strong>{item.title}</strong>
              <p>{item.description}</p>
            </div>
            <div className="info-row__badge">
              <StatusBadge tone={item.tone}>{item.badge}</StatusBadge>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
