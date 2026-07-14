import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { RadioStation } from '../api/immerle/types';
import { qk } from './keys';

type StationBody = { name: string; streamUrl: string; homepageUrl?: string; coverUrl?: string };

export function useRadioStations() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.radio,
    enabled: !!client && client.isFeatureEnabled('internetRadio'),
    queryFn: ({ signal }) => client!.listRadioStations(signal),
  });
}

export function useRadioMutations() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.radio });
  const create = useMutation({ mutationFn: (b: StationBody) => client!.createRadioStation(b), onSuccess: invalidate });
  const update = useMutation({
    mutationFn: (v: { id: string } & StationBody) => client!.updateRadioStation(v.id, v),
    onSuccess: invalidate,
  });
  const remove = useMutation({ mutationFn: (id: string) => client!.deleteRadioStation(id), onSuccess: invalidate });
  return { create, update, remove };
}

/** Like / unlike a station, optimistically flipping the cached list. */
export function useRadioLike() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: string; liked: boolean }) => client!.setRadioLiked(v.id, v.liked),
    onMutate: async (v) => {
      await qc.cancelQueries({ queryKey: qk.radio });
      const prev = qc.getQueryData<RadioStation[]>(qk.radio);
      qc.setQueryData<RadioStation[]>(qk.radio, (list) => (list ?? []).map((s) => (s.id === v.id ? { ...s, liked: v.liked } : s)));
      return { prev };
    },
    onError: (_e, _v, ctx) => {
      if (ctx?.prev) qc.setQueryData(qk.radio, ctx.prev);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: qk.radio }),
  });
}

// --- Admin toggle ---

export function useRadioAdmin() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.radioAdmin,
    enabled: !!client && client.has('internetRadio'),
    queryFn: ({ signal }) => client!.getRadioEnabled(signal),
  });
}

export function useSetRadio() {
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (enabled: boolean) => client!.setRadioEnabled(enabled),
    onSuccess: (enabled) => qc.setQueryData(qk.radioAdmin, enabled),
  });
}
