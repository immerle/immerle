import { useEffect, useRef } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router } from 'expo-router';
import { useAuth } from '../auth/store';
import { useJam } from '../jam/store';
import { useToast } from '../stores/toast';
import { useT } from '../i18n/store';
import { qk } from './keys';

/** Social + Jam hooks, all gated by the relevant capability at the call site. */

const KEYS = {
  activity: ['social', 'activity'] as const,
  profile: (username: string) => ['social', 'profile', username] as const,
  jam: (id: string) => ['jam', id] as const,
  myJam: ['jam', 'mine'] as const,
  myJamInvites: ['jam', 'invites', 'mine'] as const,
};

/** A user's profile. Pass an empty string for the caller's own profile. */
export function useProfile(username: string) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.profile(username || '__me'),
    enabled: !!client && !!client.has('social'),
    queryFn: ({ signal }) => client!.getProfile(username || undefined, signal),
  });
}

export function useActivity() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.activity,
    enabled: !!client && !!client.has('social'),
    queryFn: ({ signal }) => client!.getActivity(signal),
  });
}

// --- Jam -------------------------------------------------------------------

export function useJamMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const create = useMutation({
    mutationFn: (p: { name?: string }) => client!.jamCreate(p.name),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEYS.myJam }),
  });
  const join = useMutation({
    mutationFn: (sessionId: string) => client!.jamJoin(sessionId),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEYS.myJamInvites }),
  });
  const leave = useMutation({
    mutationFn: (sessionId: string) => client!.jamLeave(sessionId),
  });
  const update = useMutation({
    mutationFn: (p: {
      sessionId: string;
      currentTrackId?: string;
      position?: number;
      state?: string;
    }) => client!.jamUpdate(p.sessionId, p),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: KEYS.jam(v.sessionId) }),
  });
  return { create, join, leave, update };
}

/** The session the caller is currently hosting, or `undefined` if none — the
 * header button's create-vs-invite state. */
export function useMyJam() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.myJam,
    enabled: !!client && !!client.has('jam'),
    queryFn: ({ signal }) => client!.jamMine(signal),
  });
}

/** Pending Jam invites addressed to the caller — pushed live via SSE on web;
 * falls back to polling where EventSource isn't available (native), mirroring
 * src/jam/store.ts's session-sync fallback. */
export function useJamInvites() {
  const t = useT();
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const jam = useJamMutations();
  const enabled = !!client && !!client.has('jam');
  const query = useQuery({
    queryKey: KEYS.myJamInvites,
    enabled,
    queryFn: ({ signal }) => client!.jamInvitesMine(signal),
  });

  // On web, updates arrive live over the shared play-queue SSE connection
  // (see audio/store.ts's connectPlayQueueLive) rather than a dedicated
  // stream here — every extra always-on SSE connection eats into the
  // browser's ~6-per-origin cap under HTTP/1.1. Native has no EventSource at
  // all (there or here), so it still needs its own poll.
  useEffect(() => {
    if (!enabled) return;
    if (typeof (globalThis as { EventSource?: unknown }).EventSource !== 'undefined') return;
    const timer = setInterval(() => qc.invalidateQueries({ queryKey: KEYS.myJamInvites }), 15000);
    return () => clearInterval(timer);
  }, [enabled, qc]);

  // Toast on every invite that wasn't already in the previous snapshot — not
  // on the first load (a page reload shouldn't re-announce invites the caller
  // already knew about).
  const seenIds = useRef<Set<string> | null>(null);
  useEffect(() => {
    if (!query.data) return;
    const ids = new Set(query.data.map((i) => i.id));
    if (seenIds.current === null) {
      seenIds.current = ids;
      return;
    }
    for (const inv of query.data) {
      if (seenIds.current.has(inv.id)) continue;
      useToast.getState().info(
        t('social.jam.invitedBy', { name: inv.inviterDisplayName || inv.inviterUsername, session: inv.sessionName }),
        {
          label: t('social.jam.join'),
          onPress: () => {
            jam.join.mutate(inv.sessionId, {
              onSuccess: () => {
                useJam.getState().start(inv.sessionId, false);
                router.push(`/jam/${inv.sessionId}` as never);
              },
            });
          },
        },
      );
    }
    seenIds.current = ids;
  }, [query.data, t, jam.join]);

  return query;
}

/** Invite a user to the caller's Jam session (host only), and dismiss a
 * pending invite (the invitee declining it). */
export function useJamInviteMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const invite = useMutation({
    mutationFn: (p: { sessionId: string; username: string }) => client!.jamInvite(p.sessionId, p.username),
  });
  const dismiss = useMutation({
    mutationFn: (id: string) => client!.jamInviteDismiss(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEYS.myJamInvites }),
  });
  return { invite, dismiss };
}

// --- Collaborative playlists ----------------------------------------------

export function useAddCollaborator() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { playlistId: string; username: string }) =>
      client!.addPlaylistCollaborator(p.playlistId, p.username),
    onSuccess: (_d, p) => qc.invalidateQueries({ queryKey: qk.playlist(p.playlistId) }),
  });
}
