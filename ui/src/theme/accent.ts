/**
 * User-customizable accent color.
 *
 * Only `primary`/`accent` follow the user's choice — the neutral palette and the
 * semantic colors (success/danger) stay fixed so meaning is preserved. The text
 * color shown *on* the accent (buttons, the play FAB) is computed for contrast.
 */

export interface AccentPreset {
  id: string;
  label: string;
  hex: string;
}

/** Default accent (the Spotify-style green) and the preset palette. */
export const DEFAULT_ACCENT = '#1ed760';

export const ACCENT_PRESETS: AccentPreset[] = [
  { id: 'green', label: 'Vert', hex: DEFAULT_ACCENT },
  { id: 'emerald', label: 'Émeraude', hex: '#10b981' },
  { id: 'teal', label: 'Sarcelle', hex: '#14b8a6' },
  { id: 'cyan', label: 'Cyan', hex: '#06b6d4' },
  { id: 'blue', label: 'Bleu', hex: '#3b82f6' },
  { id: 'indigo', label: 'Indigo', hex: '#6366f1' },
  { id: 'violet', label: 'Violet', hex: '#8b5cf6' },
  { id: 'fuchsia', label: 'Fuchsia', hex: '#d946ef' },
  { id: 'pink', label: 'Rose', hex: '#ec4899' },
  { id: 'red', label: 'Rouge', hex: '#ef4444' },
  { id: 'orange', label: 'Orange', hex: '#f97316' },
  { id: 'amber', label: 'Ambre', hex: '#f59e0b' },
];

/** Accepts `#rgb`, `#rrggbb`, or the same without the leading `#`. */
export function normalizeHex(input: string): string | null {
  let h = input.trim().replace(/^#/, '');
  if (/^[0-9a-fA-F]{3}$/.test(h)) {
    h = h
      .split('')
      .map((c) => c + c)
      .join('');
  }
  if (!/^[0-9a-fA-F]{6}$/.test(h)) return null;
  return `#${h.toLowerCase()}`;
}

function rgb(hex: string): [number, number, number] {
  const h = hex.replace(/^#/, '');
  return [
    parseInt(h.slice(0, 2), 16),
    parseInt(h.slice(2, 4), 16),
    parseInt(h.slice(4, 6), 16),
  ];
}

/** "R G B" channel string consumed by the CSS variables / NativeWind `vars()`. */
export function hexToChannels(hex: string): string {
  return rgb(hex).join(' ');
}

/** Pick black or white text for legibility on the given background color. */
export function contrastForeground(hex: string): '#000000' | '#ffffff' {
  const [r, g, b] = rgb(hex).map((c) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  });
  const luminance = 0.2126 * r + 0.7152 * g + 0.0722 * b;
  return luminance > 0.45 ? '#000000' : '#ffffff';
}
