import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { qk } from './keys';

/** The caller's Hall of Fame (auto-created on first access). */
export function useHallOfFame() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.hallOfFame,
    enabled: !!client && client.isFeatureEnabled('hallOfFame'),
    queryFn: ({ signal }) => client!.getHallOfFame(signal),
  });
}

/** A user's full Hall of Fame (read-only unless it's the caller's own — see
 * profile's "see all" link). */
export function useUserHallOfFame(username: string) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.hallOfFameByUser(username),
    enabled: !!client && !!username && client.isFeatureEnabled('hallOfFame'),
    queryFn: ({ signal }) => client!.getUserHallOfFame(username, signal),
  });
}

/** Replace the full ranked track list (reorder, add and remove all go through this). */
export function useSetHallOfFameOrder() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ids: string[]) => client!.setHallOfFameOrder(ids),
    onSuccess: (data) => qc.setQueryData(qk.hallOfFame, data),
  });
}

/** Append one track (the "Add to Hall of Fame" track-menu action). */
export function useAddToHallOfFame() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (trackId: string) => client!.addToHallOfFame(trackId),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.hallOfFame }),
  });
}

/** Set (or clear) a track's nostalgia note. */
export function useSetHallOfFameNote() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ trackId, comment }: { trackId: string; comment: string }) =>
      client!.setHallOfFameNote(trackId, comment),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.hallOfFame }),
  });
}

/** Admin: current Hall of Fame feature toggle state. */
export function useHallOfFameAdmin() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.hallOfFameAdmin,
    enabled: !!client && client.has('hallOfFame'),
    queryFn: ({ signal }) => client!.getHallOfFameEnabled(signal),
  });
}

/** Admin: toggle the Hall of Fame feature. */
export function useSetHallOfFame() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (enabled: boolean) => client!.setHallOfFameEnabled(enabled),
    onSuccess: (enabled) => qc.setQueryData(qk.hallOfFameAdmin, enabled),
  });
}
