import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { qk } from './keys';

/** The caller's Last.fm connection state, gated by the `lastFm` capability. */
export function useLastFmStatus() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.lastFmStatus,
    enabled: !!client && client.isFeatureEnabled('lastFm'),
    queryFn: ({ signal }) => client!.getLastFmStatus(signal),
  });
}

/** Starts the desktop-auth handshake: mints a token + its approval URL. */
export function useStartLastFmConnect() {
  const client = useAuth((s) => s.client);
  return useMutation({ mutationFn: () => client!.startLastFmConnect() });
}

/** Finishes the handshake once the user has approved the token in their browser. */
export function useFinishLastFmConnect() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (token: string) => client!.finishLastFmConnect(token),
    onSuccess: (status) => qc.setQueryData(qk.lastFmStatus, status),
  });
}

export function useDisconnectLastFm() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => client!.disconnectLastFm(),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.lastFmStatus }),
  });
}

// --- Admin ---

export function useLastFmAdmin() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.lastFmAdmin,
    enabled: !!client && client.has('lastFm'),
    queryFn: ({ signal }) => client!.getLastFmAdminStatus(signal),
  });
}

export function useUpdateLastFmAdminConfig() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: { enabled?: boolean; apiKey?: string; apiSecret?: string }) => client!.updateLastFmAdminConfig(patch),
    onSuccess: (status) => qc.setQueryData(qk.lastFmAdmin, status),
  });
}
