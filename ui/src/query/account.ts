import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import type { AccountLanguage } from '../api/immerle/client';

/** Hooks for the current user's account: connected devices + API tokens. */

const KEYS = {
  nowPlaying: ['account', 'nowPlaying'] as const,
  tokens: ['account', 'tokens'] as const,
  account: ['account', 'me'] as const,
  playbackTargets: ['account', 'playbackTargets'] as const,
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
    mutationFn: (patch: { displayName?: string; email?: string; language?: AccountLanguage }) =>
      client!.updateAccount(patch),
    onSuccess: (acc) => {
      qc.setQueryData(KEYS.account, acc);
      setDisplayName(acc.displayName || null); // keep greeting / top bar in sync
    },
  });
}

/** Who's playing what right now (across the account), via Subsonic `getNowPlaying`. */
export function useNowPlaying() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.nowPlaying,
    enabled: !!client,
    refetchInterval: 10000,
    queryFn: () => client!.getNowPlaying(),
  });
}

/** Recently-active app installs on this account, for the "cast to device" picker. Fetched fresh each time it opens. */
export function usePlaybackTargets(enabled: boolean) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.playbackTargets,
    enabled: enabled && !!client,
    queryFn: ({ signal }) => client!.listPlaybackTargets(signal),
  });
}

export function useTokens() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.tokens,
    enabled: !!client,
    refetchInterval: 10000,
    queryFn: ({ signal }) => client!.listTokens(signal),
  });
}

/**
 * The caller's device sessions (app logins, `isDevice`) — the "Connected
 * devices" screen. Shares the `/tokens` list with useTokens, filtered down;
 * revoke via useTokenMutations().revoke.
 */
export function useDevices() {
  const q = useTokens();
  return { ...q, data: q.data?.filter((t) => t.isDevice) };
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
