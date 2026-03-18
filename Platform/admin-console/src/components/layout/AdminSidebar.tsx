import { NavLink } from 'react-router-dom';

import type { StatusTone } from '../display/StatusBadge';
import { StatusBadge } from '../display/StatusBadge';

export type AdminNavigationItem = {
  id: string;
  to: string;
  label: string;
  description: string;
  badge?: string;
  badgeTone?: StatusTone;
};

export type AdminNavigationGroup = {
  id: string;
  label: string;
  items: AdminNavigationItem[];
};

type AdminSidebarProps = {
  groups: AdminNavigationGroup[];
  isOpen: boolean;
  onNavigate?: () => void;
};

export function AdminSidebar({ groups, isOpen, onNavigate }: AdminSidebarProps) {
  return (
    <aside className={['admin-sidebar', isOpen ? 'is-open' : ''].filter(Boolean).join(' ')} id="admin-sidebar">
      <nav aria-label="后台模块">
        {groups.map(group => (
          <section className="admin-sidebar__group" key={group.id}>
            <h2>{group.label}</h2>
            <div className="admin-sidebar__group-links">
              {group.items.map(item => (
                <NavLink
                  className={({ isActive }) =>
                    ['admin-sidebar__link', isActive ? 'is-active' : ''].filter(Boolean).join(' ')
                  }
                  key={item.id}
                  onClick={onNavigate}
                  to={item.to}
                >
                  <span className="admin-sidebar__link-copy">
                    <strong>{item.label}</strong>
                    <small>{item.description}</small>
                  </span>
                  {item.badge ? <StatusBadge tone={item.badgeTone ?? 'info'}>{item.badge}</StatusBadge> : null}
                </NavLink>
              ))}
            </div>
          </section>
        ))}
      </nav>
    </aside>
  );
}
