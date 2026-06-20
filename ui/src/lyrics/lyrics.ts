/** A single lyrics line. `startMs` is the playback offset for synced lyrics, or
 * null for plain (unsynced) lyrics. */
export interface LyricLine {
  startMs: number | null;
  text: string;
}

/** A lyrics document for one track. `synced` means every line carries a
 * timestamp, so the player can highlight the current line (karaoke). */
export interface Lyrics {
  synced: boolean;
  lines: LyricLine[];
}

/**
 * Index of the line that should be highlighted at `positionMs`: the last line
 * whose start is at or before the position. Returns -1 before the first
 * timestamp (or when the lyrics aren't synced). Assumes lines are in order.
 */
export function activeLineIndex(lines: LyricLine[], positionMs: number): number {
  let active = -1;
  for (let i = 0; i < lines.length; i++) {
    const start = lines[i].startMs;
    if (start == null) continue;
    if (start <= positionMs) active = i;
    else break;
  }
  return active;
}
