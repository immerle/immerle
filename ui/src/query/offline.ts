import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { qk } from './keys';

/** Admin: current offline-downloads feature toggle state. */
export function useOfflineAdmin() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.offlineAdmin,
    enabled: !!client && client.has('offlineDownloads'),
    queryFn: ({ signal }) => client!.getOfflineEnabled(signal),
  });
}

/** Admin: toggle offline downloads. */
export function useSetOffline() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (enabled: boolean) => client!.setOfflineEnabled(enabled),
    onSuccess: (enabled) => qc.setQueryData(qk.offlineAdmin, enabled),
  });
}
