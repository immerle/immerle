import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { qk } from './keys';

/** Social + Jam hooks, all gated by the relevant capability at the call site. */

const KEYS = {
  friends: ['social', 'friends'] as const,
  pending: ['social', 'pending'] as const,
  activity: ['social', 'activity'] as const,
  profile: (username: string) => ['social', 'profile', username] as const,
  jam: (id: string) => ['jam', id] as const,
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

export function useFriends() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.friends,
    enabled: !!client && !!client.has('social'),
    queryFn: ({ signal }) => client!.getFriends(signal),
  });
}

export function usePendingFriends() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.pending,
    enabled: !!client && !!client.has('social'),
    queryFn: ({ signal }) => client!.getPendingFriends(signal),
    refetchInterval: 15000,
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

export function useFriendMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  // Also refresh that user's profile (its `isFriend` flag) — not just the
  // friends/pending lists — since a profile page may already be cached from
  // before the request/accept.
  const invalidate = (username: string) => {
    qc.invalidateQueries({ queryKey: KEYS.friends });
    qc.invalidateQueries({ queryKey: KEYS.pending });
    qc.invalidateQueries({ queryKey: KEYS.profile(username) });
  };
  const request = useMutation({
    mutationFn: (username: string) => client!.requestFriend(username),
    onSuccess: (_d, username) => invalidate(username),
  });
  const accept = useMutation({
    mutationFn: (username: string) => client!.acceptFriend(username),
    onSuccess: (_d, username) => invalidate(username),
  });
  return { request, accept };
}

// --- Jam -------------------------------------------------------------------

export function useJamMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const create = useMutation({
    mutationFn: (p: { name?: string }) => client!.jamCreate(p.name),
  });
  const join = useMutation({
    mutationFn: (sessionId: string) => client!.jamJoin(sessionId),
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
