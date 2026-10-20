import { Song } from '../api/subsonic/types';

/** Extend the queue once fewer than this many tracks are left after the current one. */
export const AUTOPLAY_THRESHOLD = 3;
/** Tracks added per extension. */
export const AUTOPLAY_BATCH = 20;
/** The radio blends the last tracks played (current one included). */
const SEED_COUNT = 5;
// ponytail: only the queue's tail is excluded, to keep the URL short; an
// older track can come back in a very long session.
const MAX_EXCLUDE = 200;

/**
 * What to ask the sonic radio for to extend the queue, or null while enough
 * tracks are left (or there is nothing to seed from).
 */
export function autoplayRequest(songs: Song[], index: number): { seeds: string[]; exclude: string[] } | null {
  if (index < 0 || index >= songs.length || songs.length - 1 - index >= AUTOPLAY_THRESHOLD) return null;
  const seeds = songs
    .slice(Math.max(0, index - SEED_COUNT + 1), index + 1)
    .filter((s) => !s.unresolved)
    .map((s) => s.id);
  if (!seeds.length) return null;
  return { seeds, exclude: songs.slice(-MAX_EXCLUDE).map((s) => s.id) };
}
