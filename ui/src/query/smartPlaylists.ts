import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { SmartRules } from '../api/immerle/types';
import { qk } from './keys';

export function useSmartPlaylists() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.smartPlaylists,
    enabled: !!client && client.has('smartPlaylists'),
    queryFn: ({ signal }) => client!.listSmartPlaylists(signal),
  });
}

export function useSmartPlaylistMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.smartPlaylists });
  const create = useMutation({
    mutationFn: (v: { name: string; rules: SmartRules }) => client!.createSmartPlaylist(v.name, v.rules),
    onSuccess: invalidate,
  });
  const update = useMutation({
    mutationFn: (v: { id: string; name: string; rules: SmartRules }) => client!.updateSmartPlaylist(v.id, v.name, v.rules),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: (id: string) => client!.deleteSmartPlaylist(id),
    onSuccess: invalidate,
  });
  return { create, update, remove };
}

// --- Admin toggle ---

export function useSmartPlaylistsAdmin() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.smartPlaylistsAdmin,
    enabled: !!client && client.has('smartPlaylists'),
    queryFn: ({ signal }) => client!.getSmartPlaylistsEnabled(signal),
  });
}

export function useSetSmartPlaylists() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (enabled: boolean) => client!.setSmartPlaylistsEnabled(enabled),
    onSuccess: (enabled) => qc.setQueryData(qk.smartPlaylistsAdmin, enabled),
  });
}
