import type { FormEventHandler, ReactNode } from 'react';

type FilterBarProps = {
  actions?: ReactNode;
  children: ReactNode;
  onSubmit?: FormEventHandler<HTMLFormElement>;
};

export function FilterBar({ actions, children, onSubmit }: FilterBarProps) {
  return (
    <form className="filter-bar-card" onSubmit={onSubmit}>
      <div className="filter-bar-card__fields">{children}</div>
      {actions ? <div className="filter-bar-card__actions">{actions}</div> : null}
    </form>
  );
}
