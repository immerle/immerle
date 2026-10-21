import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Song } from '../api/subsonic/types';
import { useAuth } from '../auth/store';
import { usePlayer } from '../audio/store';
import { useToast } from '../stores/toast';
import { tError } from '../i18n';
import { useT } from '../i18n/store';

/**
 * Starts a sonic radio: tracks that sound like the seeds (track, album or
 * artist ids), played right away after `lead` (e.g. the seed track itself).
 * `available` is false while the admin has the sonic radio switched off.
 */
export function useStartRadio() {
  const t = useT();
  const client = useAuth((s) => s.client);
  const available = useAuth((s) => s.client?.isFeatureEnabled('sonicRadio') ?? false);
  const [busy, setBusy] = useState(false);

  const start = async (seeds: string[], lead: Song[] = []) => {
    if (!client || busy) return;
    setBusy(true);
    try {
      const songs = await client.getSonicRadio(seeds, { exclude: lead.map((s) => s.id) });
      if (!songs.length) {
        useToast.getState().error(t('components.sonicRadio.notAnalyzed'));
        return;
      }
      await usePlayer.getState().playSongs([...lead, ...songs], 0);
    } catch (e) {
      useToast.getState().error(tError(e));
    } finally {
      setBusy(false);
    }
  };

  return { available, busy, start };
}

/** Orders songs by id the way the server returned them (Auto-DJ). */
function byOrder(songs: Song[], ids: string[]): Song[] {
  const pool = new Map<string, Song[]>();
  for (const s of songs) pool.set(s.id, [...(pool.get(s.id) ?? []), s]);
  return ids.map((id) => pool.get(id)?.shift()).filter((s): s is Song => !!s);
}

/**
 * Auto-DJ: play a list ordered for smooth transitions, or reorder the queue
 * from the current track on. `available` is false while the admin has it
 * switched off.
 */
export function useAutoDj() {
  const client = useAuth((s) => s.client);
  const available = useAuth((s) => s.client?.isFeatureEnabled('sonicAutoDj') ?? false);
  const [busy, setBusy] = useState(false);

  const run = async (fn: () => Promise<void>) => {
    if (!client || busy) return;
    setBusy(true);
    try {
      await fn();
    } catch (e) {
      useToast.getState().error(tError(e));
    } finally {
      setBusy(false);
    }
  };

  const play = (songs: Song[]) =>
    run(async () => {
      const ids = await client!.autoDjOrder(songs.map((s) => s.id), false);
      await usePlayer.getState().playSongs(byOrder(songs, ids), 0);
    });

  const reorderQueue = () =>
    run(async () => {
      const { songs, index } = usePlayer.getState();
      const ids = await client!.autoDjOrder(songs.slice(index).map((s) => s.id), true);
      await usePlayer.getState().reorderUpcoming(ids);
    });

  return { available, busy, play, reorderQueue };
}

/** A track's audio analysis (tempo, key, energy), null when not analyzed. */
export function useSongSonic(song: Song | null) {
  const client = useAuth((s) => s.client);
  const enabled = !!song && !song.remote && !song.unresolved && !!client?.isFeatureEnabled('sonicAnalysis');
  return useQuery({
    queryKey: ['songs', song?.id, 'sonic'] as const,
    enabled,
    staleTime: 60 * 60 * 1000, // only changes when the file is re-analyzed
    queryFn: ({ signal }) => client!.getSongSonic(song!.id, signal),
  });
}
