import { useEffect } from 'react';
import {
  Modal,
  Pressable,
  StyleSheet,
  Text,
  useWindowDimensions,
  View,
} from 'react-native';
import { usePathname } from 'expo-router';
import { SafeAreaView, useSafeAreaInsets } from 'react-native-safe-area-context';
import { searchNav, useSearchUI } from '../search/store';
import { SearchResults } from './SearchResults';
import { Field } from './ui';
import { WIDE_BREAKPOINT } from '../theme/layout';

/**
 * Global search surface. There is no dedicated search page anymore:
 * - **Wide (web)**: a popover anchored just under the header search input.
 * - **Narrow (mobile)**: a full-screen modal with its own search field.
 *
 * Both are driven by the shared search store. The overlay auto-dismisses when a
 * result navigates the app (pathname change).
 */
export function SearchOverlay() {
  const { width } = useWindowDimensions();
  const insets = useSafeAreaInsets();
  const pathname = usePathname();
  const open = useSearchUI((s) => s.open);
  const query = useSearchUI((s) => s.query);
  const recents = useSearchUI((s) => s.recents);
  const setQuery = useSearchUI((s) => s.setQuery);
  const close = useSearchUI((s) => s.close);
  const activeIndex = useSearchUI((s) => s.activeIndex);
  const setActiveIndex = useSearchUI((s) => s.setActiveIndex);

  const wide = width >= WIDE_BREAKPOINT;

  // Dismiss when navigation happens (e.g. tapping an artist/album result).
  useEffect(() => {
    if (open) close();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname]);

  // Desktop keyboard navigation: ↑/↓ move the highlight, Enter selects, Esc closes.
  useEffect(() => {
    if (!wide || !open || typeof document === 'undefined') return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setActiveIndex(Math.min(activeIndex + 1, Math.max(0, searchNav.count - 1)));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setActiveIndex(Math.max(0, activeIndex - 1));
      } else if (e.key === 'Enter') {
        if (searchNav.count > 0) {
          e.preventDefault();
          searchNav.selectAt(activeIndex);
        }
      } else if (e.key === 'Escape') {
        e.preventDefault();
        close();
      }
    };
    // Capture phase: React Native Web's event system swallows keydown before it
    // reaches a bubble-phase document listener, so we must intercept earlier.
    document.addEventListener('keydown', onKey, true);
    return () => document.removeEventListener('keydown', onKey, true);
  }, [wide, open, activeIndex, setActiveIndex, close]);

  // --- Mobile: full-screen modal -----------------------------------------
  if (!wide) {
    return (
      <Modal visible={open} animationType="slide" onRequestClose={close} transparent={false}>
        <SafeAreaView className="flex-1 bg-background">
          <View className="flex-row items-center gap-2 px-4 py-2">
            <View className="flex-1">
              <Field
                icon="search"
                placeholder="Artistes, albums, titres…"
                value={query}
                onChangeText={setQuery}
                autoFocus
                autoCapitalize="none"
                autoCorrect={false}
                returnKeyType="search"
                clearButtonMode="while-editing"
              />
            </View>
            <Pressable onPress={close} hitSlop={8} className="active:opacity-60">
              <Text className="text-base font-semibold text-primary">Annuler</Text>
            </Pressable>
          </View>
          <SearchResults onClose={close} />
        </SafeAreaView>
      </Modal>
    );
  }

  // --- Desktop: popover under the header search input ---------------------
  // Shown while focused with a query, or while focused-empty if there are
  // recent searches to offer.
  if (!open || (query.trim().length === 0 && recents.length === 0)) return null;
  const top = insets.top + 56; // header height

  return (
    <View style={[StyleSheet.absoluteFill, { top }]} pointerEvents="box-none">
      <Pressable style={StyleSheet.absoluteFill} onPress={close} />
      <View pointerEvents="box-none" className="items-center">
        <View
          pointerEvents="auto"
          className="mt-2 w-full max-w-[600px] overflow-hidden rounded-2xl border border-border bg-surface"
          style={{
            maxHeight: '80%',
            shadowColor: '#000',
            shadowOpacity: 0.4,
            shadowRadius: 24,
            shadowOffset: { width: 0, height: 12 },
            elevation: 12,
          }}
        >
          <SearchResults onClose={close} />
        </View>
      </View>
    </View>
  );
}
