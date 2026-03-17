import { useMemo } from 'react';

import type { AdminOperator } from '../services/contracts';
import { useSessionStore } from '../stores/sessionStore';

export type AdminModuleKey =
  | 'dashboard'
  | 'users'
  | 'operators'
  | 'orders'
  | 'wallet'
  | 'catalog'
  | 'audits'
  | 'refunds'
  | 'infringement'
  | 'governance';

type ModuleCapabilityMap = Record<AdminModuleKey, { read: string[]; write: string[] }>;

const moduleCapabilities: ModuleCapabilityMap = {
  dashboard: { read: ['dashboard.read'], write: [] },
  users: { read: ['users.read'], write: ['users.write'] },
  operators: { read: ['operators.read'], write: ['operators.write'] },
  orders: { read: ['orders.read'], write: ['orders.write'] },
  wallet: { read: ['wallet.read'], write: ['wallet.write'] },
  catalog: {
    read: ['models.read', 'routes.read', 'pricing.read', 'agreements.read', 'runtime.read'],
    write: ['models.write', 'routes.write', 'pricing.write', 'agreements.write', 'runtime.write'],
  },
  audits: { read: ['audit.read'], write: [] },
  refunds: { read: ['refunds.read'], write: ['refunds.review'] },
  infringement: { read: ['infringement.read'], write: ['infringement.review'] },
  governance: {
    read: ['agreements.read', 'notices.read', 'risk.read', 'retention.read'],
    write: ['agreements.write', 'notices.write', 'risk.write', 'retention.write'],
  },
};

function capabilitySet(operator: AdminOperator | null | undefined) {
  return new Set((operator?.capabilities ?? []).map(item => item.trim()).filter(Boolean));
}

function hasAnyCapability(capabilities: Set<string>, items: string[]) {
  return items.some(item => capabilities.has(item));
}

function canReadModule(capabilities: Set<string>, moduleKey: AdminModuleKey) {
  const definition = moduleCapabilities[moduleKey];
  return hasAnyCapability(capabilities, [...definition.read, ...definition.write]);
}

function canWriteModule(capabilities: Set<string>, moduleKey: AdminModuleKey) {
  return hasAnyCapability(capabilities, moduleCapabilities[moduleKey].write);
}

export function useCapabilities(operatorOverride?: AdminOperator | null) {
  const sessionOperator = useSessionStore(state => state.session?.operator ?? null);
  const operator = operatorOverride ?? sessionOperator;

  return useMemo(() => {
    const capabilities = capabilitySet(operator);

    return {
      operator,
      hasCapability: (capability: string) => capabilities.has(capability.trim()),
      canAccessModule: (moduleKey: AdminModuleKey) => canReadModule(capabilities, moduleKey),
      canRead: (moduleKey: AdminModuleKey) => canReadModule(capabilities, moduleKey),
      canWrite: (moduleKey: AdminModuleKey) => canWriteModule(capabilities, moduleKey),
      allCapabilities: Array.from(capabilities).sort(),
    };
  }, [operator]);
}
