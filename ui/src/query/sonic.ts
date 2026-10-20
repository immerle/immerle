import { useState } from 'react';
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
