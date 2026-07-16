import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { Song } from '../api/subsonic/types';
import { PlaylistCoverSpec } from '../api/immerle/client';
import { qk } from './keys';

export function usePlaylists() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.playlists,
    enabled: !!client,
    queryFn: () => client!.getPlaylists(),
  });
}

/** "Made to measure" playlists (Top du mois/On Repeat/Favoris oubliés) for Home. */
export function useCustomPlaylists() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.customPlaylists,
    enabled: !!client,
    queryFn: () => client!.getCustomPlaylists(),
  });
}

export function usePlaylist(id: string) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.playlist(id),
    enabled: !!client && !!id,
    queryFn: () => client!.getPlaylist(id),
  });
}

export function useCreatePlaylist() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ name, songIds }: { name: string; songIds?: string[] }) =>
      client!.createPlaylist(name, songIds ?? []),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.playlists }),
  });
}

export function useDeletePlaylist() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => client!.deletePlaylist(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.playlists }),
  });
}

export function useRenamePlaylist() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name, comment }: { id: string; name?: string; comment?: string }) =>
      client!.updatePlaylist(id, { name, comment }),
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: qk.playlists });
      qc.invalidateQueries({ queryKey: qk.playlist(v.id) });
    },
  });
}

export function useAddToPlaylist() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, songIds }: { id: string; songIds: string[] }) =>
      client!.updatePlaylist(id, { songIdToAdd: songIds }),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: qk.playlist(v.id) }),
  });
}

export function useRemoveFromPlaylist() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, indices }: { id: string; indices: number[] }) =>
      client!.updatePlaylist(id, { songIndexToRemove: indices }),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: qk.playlist(v.id) }),
  });
}

// --- Public playlists: discovery + subscription ---------------------------

export function usePublicPlaylists() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.publicPlaylists,
    enabled: !!client && !!client.has('publicPlaylists'),
    queryFn: ({ signal }) => client!.getPublicPlaylists(signal),
  });
}

export function useSubscriptionMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  // Also invalidate the single-playlist query (qk.playlist(id) — a distinct
  // root from qk.playlists, so it doesn't get swept by the list invalidation
  // below): the detail screen's `subscribed` flag must refresh right after
  // subscribing/unsubscribing from there, not just the list/discover views.
  const invalidate = (id: string) => {
    qc.invalidateQueries({ queryKey: qk.publicPlaylists });
    qc.invalidateQueries({ queryKey: qk.playlists });
    qc.invalidateQueries({ queryKey: qk.playlist(id) });
  };
  const subscribe = useMutation({
    mutationFn: (id: string) => client!.subscribePlaylist(id),
    onSuccess: (_data, id) => invalidate(id),
  });
  const unsubscribe = useMutation({
    mutationFn: (id: string) => client!.unsubscribePlaylist(id),
    onSuccess: (_data, id) => invalidate(id),
  });
  return { subscribe, unsubscribe };
}

/** Toggle a playlist's public visibility (owner only). */
export function useSetPlaylistPublic() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, isPublic }: { id: string; isPublic: boolean }) =>
      client!.updatePlaylist(id, { public: isPublic }),
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: qk.playlist(v.id) });
      qc.invalidateQueries({ queryKey: qk.publicPlaylists });
    },
  });
}

/** Set a playlist's cover from a picked image (owner only). */
export function useSetPlaylistCover() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, uri, mime }: { id: string; uri: string; mime?: string }) =>
      client!.setPlaylistCover(id, uri, mime),
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: qk.playlist(v.id) });
      qc.invalidateQueries({ queryKey: qk.playlists });
    },
  });
}

/** Generate and set a playlist's cover from a design spec (owner only). */
export function useGeneratePlaylistCover() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, spec, bgUri }: { id: string; spec: PlaylistCoverSpec; bgUri?: string }) =>
      client!.generatePlaylistCover(id, spec, bgUri),
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: qk.playlist(v.id) });
      qc.invalidateQueries({ queryKey: qk.playlists });
    },
  });
}

/** Subsonic has no atomic "set order" call, so reordering clears all entries then re-adds them in the desired order. */
export function useReorderPlaylist() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, ordered }: { id: string; ordered: Song[] }) => {
      const indices = ordered.map((_, i) => i);
      await client!.updatePlaylist(id, { songIndexToRemove: indices });
      await client!.updatePlaylist(id, {
        songIdToAdd: ordered.map((s) => s.id),
      });
    },
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: qk.playlist(v.id) }),
  });
}
