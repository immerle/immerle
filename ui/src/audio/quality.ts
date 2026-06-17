/** Streaming quality presets that map to Subsonic transcoding params. */
export interface QualityPreset {
  id: string;
  label: string;
  /** Server-side max bitrate in kbps; 0 means "original / no transcode". */
  maxBitRate: number;
  /** Target container, e.g. 'mp3', 'opus'. Undefined keeps the server default. */
  format?: string;
}

export const QUALITY_PRESETS: QualityPreset[] = [
  { id: 'original', label: 'Original (sans transcodage)', maxBitRate: 0 },
  { id: 'high', label: 'Haute — 320 kbps', maxBitRate: 320, format: 'mp3' },
  { id: 'medium', label: 'Moyenne — 192 kbps', maxBitRate: 192, format: 'mp3' },
  { id: 'low', label: 'Économie de données — 96 kbps', maxBitRate: 96, format: 'opus' },
];

export const DEFAULT_QUALITY_ID = 'high';

export function presetById(id: string): QualityPreset {
  return QUALITY_PRESETS.find((p) => p.id === id) ?? QUALITY_PRESETS[1];
}
