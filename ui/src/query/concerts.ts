import { useEffect, useRef } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { useToast } from '../stores/toast';
import { useT } from '../i18n/store';
import { qk } from './keys';

/** Your upcoming, non-dismissed concert matches (see account settings for the
 * city that drives matching), soonest first. Refreshed daily server-side, so
 * a slow poll is enough to pick up a fresh sync without a dedicated push
 * channel — same reasoning as the Jam-invites poll in query/social.ts.
 *
 * Toasts once for every match that wasn't already in the previous snapshot —
 * not on the first load (a page reload shouldn't re-announce concerts the
 * caller already knew about), same pattern as useJamInvites there.
 */
export function useConcerts() {
  const t = useT();
  const client = useAuth((s) => s.client);
  const query = useQuery({
    queryKey: qk.concerts,
    enabled: !!client && client.isFeatureEnabled('concertDiscovery'),
    refetchInterval: 10 * 60 * 1000,
    queryFn: ({ signal }) => client!.getConcerts(signal),
  });

  const seenIds = useRef<Set<string> | null>(null);
  useEffect(() => {
    if (!query.data) return;
    const ids = new Set(query.data.map((c) => c.id));
    if (seenIds.current === null) {
      seenIds.current = ids;
      return;
    }
    for (const c of query.data) {
      if (seenIds.current.has(c.id)) continue;
      useToast.getState().info(t('home.concerts.toastFound', { artist: c.artistName, event: c.eventName }));
    }
    seenIds.current = ids;
  }, [query.data, t]);

  return query;
}

export function useDismissConcert() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => client!.dismissConcert(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.concerts }),
  });
}

// --- Admin ---

export function useConcertsAdmin() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.concertsAdmin,
    enabled: !!client && client.has('concertDiscovery'),
    queryFn: ({ signal }) => client!.getConcertsStatus(signal),
  });
}

export function useUpdateConcertsConfig() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: { enabled?: boolean; ticketmasterApiKey?: string; skiddleApiKey?: string }) =>
      client!.updateConcertsConfig(patch),
    onSuccess: (status) => qc.setQueryData(qk.concertsAdmin, status),
  });
}

export function useConcertsSyncMutation() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => client!.runConcertsSync(),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.concerts }),
  });
}
