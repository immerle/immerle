import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { qk } from './keys';

/** The caller's year-in-review for the given calendar year. */
export function useWrapped(year: number) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.wrapped(year),
    enabled: !!client && client.isFeatureEnabled('wrapped'),
    queryFn: ({ signal }) => client!.getWrapped(year, signal),
  });
}

/** Admin: current Wrapped feature toggle state. */
export function useWrappedAdmin() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.wrappedAdmin,
    enabled: !!client && client.has('wrapped'),
    queryFn: ({ signal }) => client!.getWrappedEnabled(signal),
  });
}

/** Admin: toggle the Wrapped feature. */
export function useSetWrapped() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (enabled: boolean) => client!.setWrappedEnabled(enabled),
    onSuccess: (enabled) => qc.setQueryData(qk.wrappedAdmin, enabled),
  });
}
