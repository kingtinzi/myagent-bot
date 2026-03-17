import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import type { AdminLoginInput } from '../services/contracts';
import { adminApi } from '../services/adminApi';
import { AdminApiError } from '../services/http';
import { useSessionStore } from '../stores/sessionStore';

export const adminSessionQueryKey = ['admin', 'session'] as const;

type UseAdminSessionOptions = {
  enabled?: boolean;
};

export function useAdminSession(options: UseAdminSessionOptions = {}) {
  const queryClient = useQueryClient();
  const session = useSessionStore(state => state.session);
  const status = useSessionStore(state => state.status);
  const errorMessage = useSessionStore(state => state.errorMessage);
  const setStatus = useSessionStore(state => state.setStatus);
  const setSession = useSessionStore(state => state.setSession);
  const setAnonymous = useSessionStore(state => state.setAnonymous);
  const setError = useSessionStore(state => state.setError);
  const clearSession = useSessionStore(state => state.clearSession);

  const sessionQuery = useQuery({
    queryKey: adminSessionQueryKey,
    queryFn: async () => {
      setStatus('loading');
      return adminApi.getSession();
    },
    enabled: options.enabled ?? false,
    retry: false,
  });

  useEffect(() => {
    if (sessionQuery.data) {
      setSession(sessionQuery.data);
      return;
    }

    if (!sessionQuery.error) {
      return;
    }

    if (sessionQuery.error instanceof AdminApiError && sessionQuery.error.status === 401) {
      setAnonymous(sessionQuery.error.message);
      return;
    }

    setError(sessionQuery.error instanceof Error ? sessionQuery.error.message : '加载管理员会话失败。');
  }, [sessionQuery.data, sessionQuery.error, setAnonymous, setError, setSession]);

  const loginMutation = useMutation({
    mutationFn: async (input: AdminLoginInput) => {
      setStatus('loading');
      return adminApi.login(input);
    },
    onSuccess: data => {
      setSession(data);
      queryClient.setQueryData(adminSessionQueryKey, data);
    },
    onError: error => {
      setError(error instanceof Error ? error.message : '管理员登录失败。');
    },
  });

  const logoutMutation = useMutation({
    mutationFn: () => adminApi.logout(),
    onSuccess: () => {
      clearSession();
      queryClient.removeQueries({ queryKey: adminSessionQueryKey });
    },
    onError: () => {
      clearSession();
    },
  });

  return {
    session,
    status,
    errorMessage,
    sessionQuery,
    login: loginMutation.mutateAsync,
    logout: logoutMutation.mutateAsync,
    refresh: () => queryClient.invalidateQueries({ queryKey: adminSessionQueryKey }),
    isAuthenticating: sessionQuery.isFetching || loginMutation.isPending || logoutMutation.isPending,
  };
}
