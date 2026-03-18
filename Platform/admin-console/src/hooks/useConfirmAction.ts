import { useCallback } from 'react';

import type { ConfirmActionConfig } from '../services/contracts';
import { useUIStore } from '../stores/uiStore';

export function useConfirmAction() {
  const requestConfirmation = useUIStore(state => state.requestConfirmation);

  return useCallback(
    (config: ConfirmActionConfig) => {
      return requestConfirmation(config);
    },
    [requestConfirmation],
  );
}
