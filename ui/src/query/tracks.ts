import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { TrackEdit } from '../api/immerle/types';
import { invalidateCatalog, qk } from './keys';

/** Admin: paginated, searchable list of downloaded tracks. */
export function useAdminTracks(query: string, limit = 50, offset = 0) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: [...qk.adminTracks(query), limit, offset] as const,
    enabled: !!client,
    queryFn: ({ signal }) => client!.adminListTracks({ query, limit, offset }, signal),
  });
}

/**
 * Admin: edit metadata, replace cover, delete. All three invalidate the admin
 * track list plus the wider catalog — a track is cached independently
 * everywhere it's shown, so e.g. delete would otherwise leave it playable elsewhere.
 */
export function useTrackMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['admin', 'tracks'] });
    invalidateCatalog(qc);
  };

  const update = useMutation({
    mutationFn: (p: { id: string; edit: TrackEdit }) => client!.adminUpdateTrack(p.id, p.edit),
    onSuccess: invalidate,
  });

  const uploadCover = useMutation({
    mutationFn: (p: { id: string; uri: string; mime?: string }) =>
      client!.adminSetTrackCover(p.id, p.uri, p.mime),
    onSuccess: invalidate,
  });

  const remove = useMutation({
    mutationFn: (id: string) => client!.adminDeleteTrack(id),
    onSuccess: () => {
      invalidate();
      qc.invalidateQueries({ queryKey: qk.libraryStats });
    },
  });

  return { update, uploadCover, remove };
}
