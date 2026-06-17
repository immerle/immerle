import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';

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
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: KEYS.friends });
    qc.invalidateQueries({ queryKey: KEYS.pending });
  };
  const request = useMutation({
    mutationFn: (username: string) => client!.requestFriend(username),
    onSuccess: invalidate,
  });
  const accept = useMutation({
    mutationFn: (username: string) => client!.acceptFriend(username),
    onSuccess: invalidate,
  });
  return { request, accept };
}

// --- Jam -------------------------------------------------------------------

export function useJamState(sessionId: string) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.jam(sessionId),
    enabled: !!client && !!sessionId,
    // Poll for cross-device sync (works on web + native without an SSE lib).
    refetchInterval: 2000,
    queryFn: ({ signal }) => client!.jamState(sessionId, signal),
  });
}

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
  return useMutation({
    mutationFn: (p: { playlistId: string; username: string }) =>
      client!.addPlaylistCollaborator(p.playlistId, p.username),
  });
}
