import type { ComponentProps } from 'react';
import { Pressable, Text, View } from 'react-native';
import { Tabs } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';

// expo-router forks react-navigation's bottom-tabs types (SDK 56+), so import the
// tab bar props from wherever expo-router's <Tabs tabBar> actually expects them
// instead of pinning to @react-navigation/bottom-tabs, which drifts out of sync.
type BottomTabBarProps = Parameters<NonNullable<ComponentProps<typeof Tabs>['tabBar']>>[0];

// Filled icon when active, outline when idle — per tab route name.
const ICONS: Record<string, { on: string; off: string }> = {
  index: { on: 'home', off: 'home-outline' },
  search: { on: 'search', off: 'search-outline' },
  playlists: { on: 'list', off: 'list-outline' },
  social: { on: 'people', off: 'people-outline' },
  admin: { on: 'shield-checkmark', off: 'shield-checkmark-outline' },
  settings: { on: 'settings', off: 'settings-outline' },
};

/**
 * Custom bottom navigation for mobile — replaces the default React-Navigation
 * tab bar with the app's own look: filled/outline icons, accent-coloured active
 * state and a small active pill, on a flat background the player floats above.
 */
export function MobileTabBar({ state, descriptors, navigation }: BottomTabBarProps) {
  const insets = useSafeAreaInsets();
  const colors = useColors();

  return (
    <View
      className="flex-row border-t border-border/60 bg-background px-1 pt-1.5"
      style={{ paddingBottom: Math.max(insets.bottom, 10) }}
    >
      {state.routes.map((route, index) => {
        const { options } = descriptors[route.key];
        // expo-router hides href:null tabs via tabBarItemStyle.display — honour it.
        if ((options.tabBarItemStyle as { display?: string } | undefined)?.display === 'none') return null;

        const focused = state.index === index;
        const label = (options.title as string) ?? route.name;
        const icon = ICONS[route.name] ?? { on: 'ellipse', off: 'ellipse-outline' };
        const color = focused ? colors.primary : colors.muted;

        const onPress = () => {
          const event = navigation.emit({ type: 'tabPress', target: route.key, canPreventDefault: true });
          if (!focused && !event.defaultPrevented) navigation.navigate(route.name as never);
        };

        return (
          <Pressable
            key={route.key}
            onPress={onPress}
            accessibilityRole="tab"
            accessibilityState={{ selected: focused }}
            accessibilityLabel={label}
            className="flex-1 items-center gap-1 active:opacity-60"
          >
            <View
              className={`items-center justify-center rounded-full px-5 py-1 ${focused ? 'bg-primary/12' : ''}`}
            >
              <Ionicon name={focused ? icon.on : icon.off} size={22} color={color} />
            </View>
            <Text numberOfLines={1} style={{ color }} className="text-[10px] font-semibold">
              {label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}
