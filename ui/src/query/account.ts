import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';

/** Hooks for the current user's account: connected devices + API tokens. */

const KEYS = {
  nowPlaying: ['account', 'nowPlaying'] as const,
  tokens: ['account', 'tokens'] as const,
  account: ['account', 'me'] as const,
};

/** The caller's own account (display name + email), editable via `/account`. */
export function useAccount() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.account,
    enabled: !!client,
    queryFn: ({ signal }) => client!.getAccount(signal),
  });
}

export function useUpdateAccount() {
  const client = useAuth((s) => s.client);
  const setDisplayName = useAuth((s) => s.setDisplayName);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: { displayName?: string; email?: string }) => client!.updateAccount(patch),
    onSuccess: (acc) => {
      qc.setQueryData(KEYS.account, acc);
      setDisplayName(acc.displayName || null); // keep greeting / top bar in sync
    },
  });
}

/** Connected devices / active sessions, via Subsonic `getNowPlaying`. */
export function useNowPlaying() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.nowPlaying,
    enabled: !!client,
    refetchInterval: 10000,
    queryFn: () => client!.subsonic.getNowPlaying(),
  });
}

export function useTokens() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.tokens,
    enabled: !!client,
    queryFn: ({ signal }) => client!.listTokens(signal),
  });
}

export function useTokenMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: KEYS.tokens });
  const create = useMutation({
    mutationFn: (p: { name?: string; expires?: number }) =>
      client!.createToken(p.name, p.expires ? new Date(p.expires).toISOString() : undefined),
    onSuccess: invalidate,
  });
  const revoke = useMutation({
    mutationFn: (id: string) => client!.revokeToken(id),
    onSuccess: invalidate,
  });
  return { create, revoke };
}
