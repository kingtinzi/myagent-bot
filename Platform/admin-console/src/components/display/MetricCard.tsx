import type { StatusTone } from './StatusBadge';
import { StatusBadge } from './StatusBadge';

type MetricCardProps = {
  label: string;
  value: string;
  caption: string;
  trend?: string;
  tone?: Exclude<StatusTone, 'neutral'>;
};

export function MetricCard({ label, value, caption, trend, tone = 'info' }: MetricCardProps) {
  const className = ['metric-card', `is-${tone}`].join(' ');

  return (
    <article className={className}>
      <div className="metric-card__header">
        <span>{label}</span>
        {trend ? <StatusBadge tone={tone}>{trend}</StatusBadge> : null}
      </div>
      <strong>{value}</strong>
      <p>{caption}</p>
    </article>
  );
}
