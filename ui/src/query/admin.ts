import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { ServerSettings, TranscodeProfile } from '../api/immerle/types';
import { RuntimeSettingsDTO } from '../api/immerleApi';
import { qk } from './keys';

// --- Library stats & scan --------------------------------------------------

export function useLibraryStats() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.libraryStats,
    enabled: !!client,
    staleTime: 60 * 1000,
    queryFn: ({ signal }) => client!.getLibraryStats(signal),
  });
}

/** Scan progress; polls every 2s while a scan is running. */
export function useScanProgress() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.scanProgress,
    enabled: !!client,
    queryFn: ({ signal }) => client!.getScanProgress(signal),
    refetchInterval: (q) => (q.state.data?.scanning ? 2000 : false),
  });
}

export function useStartScan() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (full: boolean) => client!.startScan(full),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.scanProgress }),
  });
}

// --- Users -----------------------------------------------------------------

export function useUsers() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: ['admin', 'users'] as const,
    enabled: !!client,
    queryFn: () => client!.getUsers(),
  });
}

export function useUserMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ['admin', 'users'] });

  const create = useMutation({
    mutationFn: (p: {
      username: string;
      password: string;
      displayName?: string;
      email?: string;
      adminRole?: boolean;
    }) => client!.createUser({ ...p, streamRole: true, playlistRole: true }),
    onSuccess: invalidate,
  });

  const update = useMutation({
    mutationFn: (p: { username: string; displayName?: string; email?: string; adminRole?: boolean }) =>
      client!.updateUser(p),
    onSuccess: invalidate,
  });

  const remove = useMutation({
    mutationFn: (username: string) => client!.deleteUser(username),
    onSuccess: invalidate,
  });

  const resetPassword = useMutation({
    mutationFn: (p: { username: string; password: string }) =>
      client!.changePassword(p.username, p.password),
  });

  return { create, update, remove, resetPassword };
}

// --- Dynamic providers (admin) ---------------------------------------------

export function useProviders() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.providers,
    enabled: !!client && !!client.has('dynamicProviders'),
    queryFn: ({ signal }) => client!.listProviders(signal),
  });
}

/** Create/update, enable/disable and delete mutations for dynamic providers. */
export function useProviderMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.providers });

  const upsert = useMutation({
    mutationFn: (p: {
      name: string;
      endpoint: string;
      config?: string;
      enabled?: boolean;
      kind?: string;
    }) => client!.upsertProvider(p),
    onSuccess: invalidate,
  });

  // Create a dynamic HTTP provider from its URL (server probes /capabilities).
  const create = useMutation({
    mutationFn: (endpoint: string) => client!.createProvider(endpoint),
    onSuccess: invalidate,
  });

  const setEnabled = useMutation({
    mutationFn: (p: { name: string; enabled: boolean }) =>
      client!.setProviderEnabled(p.name, p.enabled),
    onSuccess: invalidate,
  });

  const remove = useMutation({
    mutationFn: (name: string) => client!.deleteProvider(name),
    onSuccess: invalidate,
  });

  const reorder = useMutation({
    mutationFn: (order: string[]) => client!.reorderProviders(order),
    onSuccess: invalidate,
  });

  return { upsert, create, setEnabled, remove, reorder };
}

/** Recent warn/error events for one provider; only fetched while the popin is open. */
export function useProviderLogs(name: string | null) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.providerLogs(name ?? ''),
    enabled: !!client && !!name,
    queryFn: ({ signal }) => client!.getProviderLogs(name!, signal),
  });
}

export function useDownloadJobs() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.jobs,
    enabled: !!client && !!client.has('onDemandCatalog'),
    queryFn: ({ signal }) => client!.getDownloadJobs(signal),
    refetchInterval: (q) =>
      (q.state.data ?? []).some((j) => j.status === 'running' || j.status === 'queued')
        ? 1500
        : false,
  });
}

export function useJobMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.jobs });
  const retry = useMutation({
    mutationFn: (id: string) => client!.retryDownloadJob(id),
    onSuccess: invalidate,
  });
  const cancel = useMutation({
    mutationFn: (id: string) => client!.cancelDownloadJob(id),
    onSuccess: invalidate,
  });
  const purge = useMutation({ mutationFn: () => client!.purgeCache() });
  return { retry, cancel, purge };
}

// --- Runtime settings (admin) ----------------------------------------------

export function useSettings() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.settings,
    enabled: !!client && !!client.has('runtimeSettings'),
    queryFn: ({ signal }) => client!.getSettings(signal),
  });
}

export function useUpdateSettings() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: RuntimeSettingsDTO) => client!.updateSettings(patch),
    // The response carries the fresh full settings + pending-restart state.
    onSuccess: (res) => qc.setQueryData(qk.settings, res),
  });
}

// --- Downloads cleanup (admin) ---------------------------------------------

export function useCleanup() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.cleanup,
    enabled: !!client && !!client.has('runtimeSettings'),
    retry: false,
    queryFn: ({ signal }) => client!.getCleanup(signal),
  });
}

export function useCleanupMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const setEnabled = useMutation({
    mutationFn: (enabled: boolean) => client!.setCleanupEnabled(enabled),
    onSuccess: (status) => qc.setQueryData(qk.cleanup, status),
  });
  const run = useMutation({ mutationFn: () => client!.runCleanup() });
  return { setEnabled, run };
}

// --- Federation ------------------------------------------------------------

/** Register this instance with the hub. The response carries the refreshed
 * settings (with the hub-assigned id), so we prime the settings cache. */
export function useRegisterInstance() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => client!.registerInstance(),
    onSuccess: (res) => qc.setQueryData(qk.settings, res),
  });
}

/** Push a name / sqid change to the hub (server-side). Primes the settings
 * cache with the hub-canonical values on success. */
export function useUpdateFederationInstance() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: { name: string; sqid: string }) => client!.updateFederationInstance(p.name, p.sqid),
    onSuccess: (res) => qc.setQueryData(qk.settings, res),
  });
}

// --- Server / transcoding --------------------------------------------------

export function useTranscodeProfiles() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.transcodeProfiles,
    enabled: !!client && !!client.has('adminExtended'),
    queryFn: ({ signal }) => client!.getTranscodeProfiles(signal),
  });
}

export function useServerSettings() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.serverSettings,
    enabled: !!client && !!client.has('adminExtended'),
    queryFn: ({ signal }) => client!.getServerSettings(signal),
  });
}

export function useServerMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const updateSettings = useMutation({
    mutationFn: (settings: Partial<ServerSettings>) => client!.updateServerSettings(settings),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.serverSettings }),
  });
  const upsertProfile = useMutation({
    mutationFn: (profile: Partial<TranscodeProfile>) => client!.upsertTranscodeProfile(profile),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.transcodeProfiles }),
  });
  return { updateSettings, upsertProfile };
}
