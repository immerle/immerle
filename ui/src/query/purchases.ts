import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { BandcampCollectionItem, BandcampJob } from '../api/immerle/types';
import { qk } from './keys';
type CollectionQueryData = BandcampCollectionItem[] | undefined;

/** Bandcamp purchase-import hooks, gated by the `bandcampImport` capability. */

const RUNNING = new Set(['queued', 'running']);

export function useBandcampStatus() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.bandcampStatus,
    enabled: !!client && client.isFeatureEnabled('bandcampImport'),
    queryFn: ({ signal }) => client!.getBandcampStatus(signal),
  });
}

export function useConnectBandcamp() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (cookie: string) => client!.connectBandcamp(cookie),
    onSuccess: (status) => qc.setQueryData(qk.bandcampStatus, status),
  });
}

export function useDisconnectBandcamp() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => client!.disconnectBandcamp(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.bandcampStatus });
      qc.invalidateQueries({ queryKey: qk.bandcampCollection });
    },
  });
}

/** The caller's Bandcamp purchase collection — only fetched once connected.
 * Polls while any item's import job is still queued/running. */
export function useBandcampCollection(connected: boolean) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.bandcampCollection,
    enabled: !!client && client.isFeatureEnabled('bandcampImport') && connected,
    queryFn: ({ signal }) => client!.getBandcampCollection(signal),
    refetchInterval: (q) => ((q.state.data as CollectionQueryData) ?? []).some((i) => RUNNING.has(i.jobStatus ?? '')) ? 2000 : false,
  });
}

export function useImportBandcampItem() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (item: BandcampCollectionItem) => client!.importBandcampItem(item),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.bandcampJobs });
      qc.invalidateQueries({ queryKey: qk.bandcampCollection });
    },
  });
}

/** Polls while any job is still queued/running. */
export function useBandcampJobs() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.bandcampJobs,
    enabled: !!client && client.isFeatureEnabled('bandcampImport'),
    queryFn: ({ signal }) => client!.getBandcampJobs(signal),
    refetchInterval: (q) => ((q.state.data as BandcampJob[] | undefined) ?? []).some((j) => RUNNING.has(j.status)) ? 2000 : false,
  });
}
