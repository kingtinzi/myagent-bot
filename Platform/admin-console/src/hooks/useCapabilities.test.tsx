import { renderHook } from '@testing-library/react';

import { useCapabilities } from './useCapabilities';
import { useSessionStore } from '../stores/sessionStore';

describe('useCapabilities', () => {
  beforeEach(() => {
    useSessionStore.getState().clearSession();
  });

  it('derives read and write access from the current operator capabilities', () => {
    useSessionStore.getState().setSession({
      user: { id: 'ops-1', email: 'ops@example.com' },
      operator: {
        email: 'ops@example.com',
        role: 'finance',
        active: true,
        capabilities: ['wallet.write', 'orders.read'],
      },
    });

    const { result } = renderHook(() => useCapabilities());

    expect(result.current.hasCapability('wallet.write')).toBe(true);
    expect(result.current.canRead('wallet')).toBe(true);
    expect(result.current.canWrite('wallet')).toBe(true);
    expect(result.current.canRead('orders')).toBe(true);
    expect(result.current.canWrite('orders')).toBe(false);
    expect(result.current.canAccessModule('catalog')).toBe(false);
  });

  it('supports composite governance module access', () => {
    useSessionStore.getState().setSession({
      user: { id: 'gov-1', email: 'gov@example.com' },
      operator: {
        email: 'gov@example.com',
        role: 'governance',
        active: true,
        capabilities: ['agreements.read', 'notices.write'],
      },
    });

    const { result } = renderHook(() => useCapabilities());

    expect(result.current.canRead('governance')).toBe(true);
    expect(result.current.canWrite('governance')).toBe(true);
    expect(result.current.canAccessModule('wallet')).toBe(false);
  });
});
