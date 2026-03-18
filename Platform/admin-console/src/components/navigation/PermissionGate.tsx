import type { ReactNode } from 'react';

import { useCapabilities, type AdminModuleKey } from '../../hooks/useCapabilities';

type PermissionGateProps = {
  children: ReactNode;
  fallback?: ReactNode;
  capability?: string;
  moduleKey?: AdminModuleKey;
};

export function PermissionGate({ children, fallback = null, capability, moduleKey }: PermissionGateProps) {
  const capabilities = useCapabilities();

  const allowed = capability
    ? capabilities.hasCapability(capability)
    : moduleKey
      ? capabilities.canAccessModule(moduleKey)
      : true;

  return allowed ? <>{children}</> : <>{fallback}</>;
}
