import { useColorScheme } from 'nativewind';
import { contrastForeground } from './accent';
import { useTheme } from './store';

/**
 * Raw color values mirroring the CSS variables in `global.css`. Used where a
 * Tailwind class can't reach — native props like `tabBarActiveTintColor`,
 * `ActivityIndicator` color, status bar, gradients.
 */
export const palette = {
  light: {
    background: '#ffffff',
    surface: '#f6f6f6',
    surfaceAlt: '#ededed',
    foreground: '#000000',
    muted: '#6b6b6b',
    border: '#e5e5e5',
    primary: '#1db954',
    primaryForeground: '#000000',
    accent: '#1db954',
    danger: '#d72020',
    success: '#1db954',
  },
  dark: {
    background: '#121212',
    surface: '#181818',
    surfaceAlt: '#282828',
    foreground: '#ffffff',
    muted: '#b3b3b3',
    border: '#2a2a2a',
    primary: '#1ed760',
    primaryForeground: '#000000',
    accent: '#1ed760',
    danger: '#e22134',
    success: '#1ed760',
  },
} as const;

export type ColorTokens = { [K in keyof typeof palette.light]: string };

/** Current resolved color tokens, reactive to the color scheme and custom accent. */
export function useColors(): ColorTokens {
  const { colorScheme } = useColorScheme();
  const accent = useTheme((s) => s.accent);
  const base = colorScheme === 'dark' ? palette.dark : palette.light;
  if (!accent) return base;
  // Only the accent follows the user's choice; neutrals/semantics stay fixed.
  return { ...base, primary: accent, accent, primaryForeground: contrastForeground(accent) };
}
