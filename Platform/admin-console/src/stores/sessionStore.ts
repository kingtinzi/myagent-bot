import { create } from 'zustand';

import type { AdminSession, AdminSessionStatus } from '../services/contracts';

type SessionStore = {
  status: AdminSessionStatus;
  session: AdminSession | null;
  errorMessage: string;
  setStatus: (status: AdminSessionStatus) => void;
  setSession: (session: AdminSession) => void;
  setAnonymous: (message?: string) => void;
  setError: (message: string) => void;
  clearSession: () => void;
};

export const useSessionStore = create<SessionStore>(set => ({
  status: 'idle',
  session: null,
  errorMessage: '',
  setStatus: status => set(state => ({ ...state, status })),
  setSession: session =>
    set({
      status: 'authenticated',
      session,
      errorMessage: '',
    }),
  setAnonymous: (message = '') =>
    set({
      status: 'anonymous',
      session: null,
      errorMessage: message,
    }),
  setError: message =>
    set(state => ({
      ...state,
      status: 'error',
      errorMessage: message,
    })),
  clearSession: () =>
    set({
      status: 'anonymous',
      session: null,
      errorMessage: '',
    }),
}));
