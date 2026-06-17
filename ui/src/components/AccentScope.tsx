import { ReactNode } from 'react';
import { View } from 'react-native';
import { vars } from 'nativewind';
import { useTheme } from '../theme/store';
import { contrastForeground, hexToChannels } from '../theme/accent';

/**
 * Applies the user's custom accent to the NativeWind CSS variables for the whole
 * subtree (so `bg-primary`, `text-primary`, focus rings, etc. follow it). On web
 * this complements the document-root override (which also covers portals/modals).
 */
export function AccentScope({ children }: { children: ReactNode }) {
  const accent = useTheme((s) => s.accent);
  const style = accent
    ? vars({
        '--color-primary': hexToChannels(accent),
        '--color-accent': hexToChannels(accent),
        '--color-primary-foreground': hexToChannels(contrastForeground(accent)),
      })
    : undefined;
  return <View style={[{ flex: 1 }, style]}>{children}</View>;
}
