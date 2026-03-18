import { create } from 'zustand';

import type { ConfirmActionConfig } from '../services/contracts';

type PendingConfirmDialog = ConfirmActionConfig & {
  open: boolean;
  inputValue: string;
  resolver: ((accepted: boolean) => void) | null;
};

type UIStore = {
  sidebarOpen: boolean;
  confirmDialog: PendingConfirmDialog;
  setSidebarOpen: (open: boolean) => void;
  toggleSidebar: () => void;
  setConfirmInput: (value: string) => void;
  requestConfirmation: (config: ConfirmActionConfig) => Promise<boolean>;
  resolveConfirmation: (accepted: boolean) => void;
};

function createClosedConfirmDialog(): PendingConfirmDialog {
  return {
    open: false,
    title: '',
    message: '',
    hint: '',
    confirmLabel: '确认',
    cancelLabel: '取消',
    tone: 'warning',
    requireText: '',
    inputValue: '',
    resolver: null,
  };
}

export const useUIStore = create<UIStore>((set, get) => ({
  sidebarOpen: false,
  confirmDialog: createClosedConfirmDialog(),
  setSidebarOpen: open => set({ sidebarOpen: open }),
  toggleSidebar: () => set(state => ({ sidebarOpen: !state.sidebarOpen })),
  setConfirmInput: value =>
    set(state => ({
      confirmDialog: {
        ...state.confirmDialog,
        inputValue: value,
      },
    })),
  requestConfirmation: config =>
    new Promise(resolve => {
      const activeResolver = get().confirmDialog.resolver;
      if (activeResolver) {
        activeResolver(false);
      }

      set({
        confirmDialog: {
          ...createClosedConfirmDialog(),
          ...config,
          open: true,
          resolver: resolve,
          confirmLabel: config.confirmLabel ?? '确认',
          cancelLabel: config.cancelLabel ?? '取消',
          tone: config.tone ?? 'warning',
          requireText: config.requireText ?? '',
        },
      });
    }),
  resolveConfirmation: accepted => {
    const { resolver } = get().confirmDialog;
    if (resolver) {
      resolver(accepted);
    }
    set({ confirmDialog: createClosedConfirmDialog() });
  },
}));
