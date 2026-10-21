import { SongSonicDTO } from '../api/immerleApi';

/** "128 BPM · Am (8A) · Energy 82%", the short form shown in the track menu. */
export function sonicSummary(t: (key: string, params?: Record<string, unknown>) => string, s: SongSonicDTO): string {
  const key = s.key ? `${s.key}${s.mode === 'minor' ? 'm' : ''} (${s.camelot})` : '';
  return [s.bpm ? `${Math.round(s.bpm)} BPM` : '', key, t('components.trackMenu.energy', { percent: Math.round((s.energy ?? 0) * 100) })]
    .filter(Boolean)
    .join(' · ');
}
