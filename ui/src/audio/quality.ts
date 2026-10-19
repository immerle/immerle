/** Streaming quality presets that map to Subsonic transcoding params. */
export interface QualityPreset {
  id: string;
  labelKey: string;
  /** Server-side max bitrate in kbps; 0 means "original / no transcode". */
  maxBitRate: number;
  /** Target container, e.g. 'mp3', 'opus'. Undefined keeps the server default. */
  format?: string;
}

export const QUALITY_PRESETS: QualityPreset[] = [
  { id: 'original', labelKey: 'quality.original', maxBitRate: 0 },
  { id: 'high', labelKey: 'quality.high', maxBitRate: 320, format: 'mp3' },
  { id: 'medium', labelKey: 'quality.medium', maxBitRate: 192, format: 'mp3' },
  { id: 'low', labelKey: 'quality.low', maxBitRate: 96, format: 'opus' },
];

export const DEFAULT_QUALITY_ID = 'high';

export function presetById(id: string): QualityPreset {
  return QUALITY_PRESETS.find((p) => p.id === id) ?? QUALITY_PRESETS[1];
}

/** Coarse network class driving adaptive quality (see src/audio/network.ts). */
export type NetworkClass = 'fast' | 'cellular' | 'slow';

/** Best preset allowed per network class when adaptive quality is on. */
const NETWORK_CAP: Record<NetworkClass, string> = { fast: 'original', cellular: 'medium', slow: 'low' };

/**
 * The preset actually streamed: the user's choice, lowered to the network's cap
 * when adaptive quality is on. Never raises it (a data-saver pick stays low on Wi-Fi).
 */
export function effectiveQualityId(qualityId: string, adaptive: boolean, network: NetworkClass): string {
  const chosen = presetById(qualityId).id;
  if (!adaptive) return chosen;
  const rank = (id: string) => QUALITY_PRESETS.findIndex((p) => p.id === id);
  return rank(chosen) >= rank(NETWORK_CAP[network]) ? chosen : NETWORK_CAP[network];
}
