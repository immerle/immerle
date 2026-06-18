import { ReactNode } from 'react';
import { Pressable, ScrollView, StyleSheet, Text, useWindowDimensions, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { LinearGradient } from 'expo-linear-gradient';
import { router } from 'expo-router';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';
import { WIDE_BREAKPOINT } from '../theme/layout';
import { useT } from '../i18n/store';

/**
 * Shared chrome for admin sub-pages: a centered, max-width scroll container with
 * a full-bleed gradient header on top and padded content below — keeping every
 * admin screen visually aligned with the Administration home.
 */
/** Shared max content width for every admin screen (home + sub-pages). */
export const ADMIN_MAX_WIDTH = 880;

export function AdminScroll({ header, children }: { header?: ReactNode; children: ReactNode }) {
  return (
    <SafeAreaView edges={['top']} className="flex-1 bg-background">
      {/* The header bleeds full-width; the body is capped + centered. */}
      <ScrollView contentContainerStyle={{ paddingBottom: 32 }}>
        {header}
        <View style={{ maxWidth: ADMIN_MAX_WIDTH, width: '100%', alignSelf: 'center', paddingHorizontal: 16, paddingTop: 12, gap: 14 }}>
          {children}
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

/**
 * Immersive page header for admin screens: a soft coloured gradient glow that
 * fades into the page background (echoing the blurred album/artist backdrops),
 * with a colour-chipped icon, a large title and an optional subtitle. A back
 * button shows on mobile; on desktop the global top bar already handles it.
 */
export function AdminHeader({
  color,
  title,
  subtitle,
  trailing,
  showBack = true,
}: {
  color: string;
  title: string;
  subtitle?: string;
  trailing?: ReactNode;
  showBack?: boolean;
}) {
  const t = useT();
  const colors = useColors();
  const { width } = useWindowDimensions();
  const wide = width >= WIDE_BREAKPOINT;
  return (
    // Full-bleed gradient; the content inside is capped to the page max-width.
    // Vertical, like the album/artist hero backdrops: colour on top, fading
    // down into the page background.
    <View className="overflow-hidden">
      <LinearGradient
        colors={[color + '66', color + '1f', 'transparent']}
        start={{ x: 0, y: 0 }}
        end={{ x: 0, y: 1 }}
        style={StyleSheet.absoluteFill}
      />
      <LinearGradient
        colors={['transparent', colors.background]}
        locations={[0.5, 1]}
        style={StyleSheet.absoluteFill}
      />
      <View
        style={{ maxWidth: ADMIN_MAX_WIDTH, width: '100%', alignSelf: 'center' }}
        className="flex-row items-center gap-3 px-4 pb-6 pt-5"
      >
        {showBack && !wide ? (
          <Pressable
            onPress={() => router.back()}
            accessibilityLabel={t('components.admin.back')}
            className="h-9 w-9 items-center justify-center rounded-full bg-surface-alt active:opacity-70"
          >
            <Ionicon name="chevron-back" size={20} color={colors.foreground} />
          </Pressable>
        ) : null}
        <View className="flex-1">
          <Text className="text-2xl font-bold tracking-tight text-foreground">{title}</Text>
          {subtitle ? (
            <Text className="text-sm text-muted" numberOfLines={1}>
              {subtitle}
            </Text>
          ) : null}
        </View>
        {trailing}
      </View>
    </View>
  );
}

export function CardTitle({
  icon,
  color,
  title,
  trailing,
}: {
  icon: string;
  color: string;
  title: string;
  trailing?: ReactNode;
}) {
  return (
    <View className="flex-row items-center gap-2.5">
      <View className="h-9 w-9 items-center justify-center rounded-xl" style={{ backgroundColor: color + '22' }}>
        <Ionicon name={icon} size={18} color={color} />
      </View>
      <Text className="flex-1 text-base font-semibold text-foreground">{title}</Text>
      {trailing}
    </View>
  );
}

/** Deterministic vivid colour from a string (stable per username/name). */
const HUES = ['#3b82f6', '#8b5cf6', '#ec4899', '#f59e0b', '#10b981', '#14b8a6', '#ef4444', '#6366f1'];
export function colorFor(seed: string): string {
  let h = 0;
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) >>> 0;
  return HUES[h % HUES.length];
}
