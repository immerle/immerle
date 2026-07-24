import { useEffect, useRef } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { useToast } from '../stores/toast';
import { useT } from '../i18n/store';
import { qk } from './keys';

/** Your upcoming, non-dismissed concert matches (see admin settings for the
 * country that drives matching), soonest first. Refreshed daily server-side,
 * so a slow poll is enough to pick up a fresh sync without a dedicated push
 * channel — same reasoning as the Jam-invites poll in query/social.ts. */
export function useConcerts() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.concerts,
    enabled: !!client && client.isFeatureEnabled('concertDiscovery'),
    refetchInterval: 10 * 60 * 1000,
    queryFn: ({ signal }) => client!.getConcerts(signal),
  });
}

/** Toasts once for every concert match that wasn't already in the previous
 * snapshot — not on the first load (a page reload shouldn't re-announce
 * concerts the caller already knew about), same pattern as useJamInvites.
 *
 * Call this once from a persistently-mounted component (NotificationsBell,
 * same as the Jam-invite equivalent) — calling it from a tab screen (the
 * Home banner used to) resets the "seen" baseline every time the user
 * navigates away and back, silently swallowing the toast for anything that
 * synced while they were elsewhere. */
export function useConcertNotifications() {
  const t = useT();
  const query = useConcerts();
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
    mutationFn: (patch: { enabled?: boolean; country?: string; ticketmasterApiKey?: string; skiddleApiKey?: string }) =>
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
