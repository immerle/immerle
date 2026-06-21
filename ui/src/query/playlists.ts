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

/**
 * Reorder a playlist by rewriting its full entry list. Subsonic has no atomic
 * "set order" call, so we clear all entries then re-add them in the desired
 * order. The current and target lists are passed in to compute the removal
 * indices precisely.
 */
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
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: qk.publicPlaylists });
    qc.invalidateQueries({ queryKey: qk.playlists });
  };
  const subscribe = useMutation({
    mutationFn: (id: string) => client!.subscribePlaylist(id),
    onSuccess: invalidate,
  });
  const unsubscribe = useMutation({
    mutationFn: (id: string) => client!.unsubscribePlaylist(id),
    onSuccess: invalidate,
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
