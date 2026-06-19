import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { qk } from './keys';

/** Hooks for the "local" library: the tracks the user uploaded from the web UI. */

export function useLocalSongs() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.local,
    enabled: !!client,
    queryFn: ({ signal }) => client!.listLocalSongs(signal),
  });
}

/** Upload one or more audio files; refreshes the local list when done. */
export function useUploadTracks() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (files: File[]) => {
      for (const file of files) await client!.uploadTrack(file);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.local }),
  });
}

export function useRenameTrack() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, title }: { id: string; title: string }) => client!.renameTrack(id, title),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.local }),
  });
}

export function useSetTrackCover() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, image }: { id: string; image: File }) => client!.setTrackCover(id, image),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.local }),
  });
}

export function useDeleteTrack() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => client!.deleteTrack(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.local }),
  });
}
