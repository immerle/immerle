import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { ImportDTO } from '../api/immerleApi';
import { qk } from './keys';

/** Playlist-import hooks, gated by the `playlistImport` capability. */

const KEYS = {
  sources: ['imports', 'sources'] as const,
  list: ['imports', 'list'] as const,
};

const RUNNING = new Set(['pending', 'queued', 'running']);

export function useImportSources() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.sources,
    enabled: !!client && !!client.has('playlistImport'),
    queryFn: ({ signal }) => client!.listImportSources(signal),
  });
}

export function useImports() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: KEYS.list,
    enabled: !!client && !!client.has('playlistImport'),
    queryFn: ({ signal }) => client!.listImports(signal),
    // Poll while an import is still running.
    refetchInterval: (q) =>
      ((q.state.data as ImportDTO[] | undefined) ?? []).some((i) => RUNNING.has(i.status ?? '')) ? 2000 : false,
  });
}

/** One import with its per-track items (polls while still running). */
export function useImportStatus(id: string) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: ['imports', 'status', id] as const,
    enabled: !!client && !!id && !!client.has('playlistImport'),
    queryFn: ({ signal }) => client!.getImportStatus(id, signal),
    refetchInterval: (q) => (RUNNING.has((q.state.data as ImportDTO | undefined)?.status ?? '') ? 2000 : false),
  });
}

export function useStartImport() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ source, ref }: { source: string; ref: string }) => client!.startImport(source, ref),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEYS.list });
      qc.invalidateQueries({ queryKey: qk.playlists });
    },
  });
}

/** Validate (no query) or modify (corrected query) a flagged import item. */
export function useResolveImportItem() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ itemId, query }: { itemId: string; query?: string }) => client!.resolveImportItem(itemId, query),
    onSuccess: () => {
      // Refresh the import detail/list and the (now updated) playlist.
      qc.invalidateQueries({ queryKey: ['imports'] });
      qc.invalidateQueries({ queryKey: qk.playlists });
    },
  });
}
