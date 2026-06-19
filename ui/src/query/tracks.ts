import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { TrackEdit } from '../api/immerle/types';
import { qk } from './keys';

/** Admin: paginated, searchable list of downloaded tracks. */
export function useAdminTracks(query: string, limit = 50, offset = 0) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: [...qk.adminTracks(query), limit, offset] as const,
    enabled: !!client,
    queryFn: ({ signal }) => client!.adminListTracks({ query, limit, offset }, signal),
  });
}

/** Admin: edit metadata, replace cover, delete — all invalidate the track list. */
export function useTrackMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin', 'tracks'] });

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
    onSuccess: invalidate,
  });

  return { update, uploadCover, remove };
}
