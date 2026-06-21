import { useEffect, useRef, useState } from 'react';
import { Modal, Pressable, StyleSheet, View } from 'react-native';
import Animated, { Easing, runOnJS, useAnimatedStyle, useSharedValue, withTiming } from 'react-native-reanimated';
import { usePathname } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { LibrarySidebar } from './LibrarySidebar';
import { AdminSidebar } from './AdminSidebar';
import { useUI } from '../stores/ui';

// Slide far enough to clear the widest sidebar; over-translating just parks it
// further off-screen, so we don't need to measure the panel.
const OFFSCREEN = -320;

/**
 * Mobile slide-in drawer hosting the same sidebar as desktop: the admin page-nav
 * on /admin routes, the library/playlists sidebar everywhere else. Driven by a
 * transform (GPU-accelerated, smooth) rather than a layout animation, and kept
 * mounted while animating so it slides both in and out.
 */
export function MobileDrawer() {
  const open = useUI((s) => s.drawerOpen);
  const close = useUI((s) => s.closeDrawer);
  const insets = useSafeAreaInsets();
  const pathname = usePathname();
  const inAdmin = pathname === '/admin' || pathname.startsWith('/admin/');

  const tx = useSharedValue(OFFSCREEN);
  const dim = useSharedValue(0);
  const [mounted, setMounted] = useState(false);
  // Avoid closing on the initial mount; only react to genuine route changes.
  const lastPath = useRef(pathname);

  useEffect(() => {
    if (open) {
      setMounted(true);
      tx.value = withTiming(0, { duration: 240, easing: Easing.out(Easing.cubic) });
      dim.value = withTiming(1, { duration: 240 });
    } else if (mounted) {
      dim.value = withTiming(0, { duration: 190 });
      tx.value = withTiming(OFFSCREEN, { duration: 190, easing: Easing.in(Easing.cubic) }, (done) => {
        if (done) runOnJS(setMounted)(false);
      });
    }
  }, [open, mounted, tx, dim]);

  // Close when navigation happens (e.g. tapping a sidebar link).
  useEffect(() => {
    if (lastPath.current !== pathname) {
      lastPath.current = pathname;
      close();
    }
  }, [pathname, close]);

  const panelStyle = useAnimatedStyle(() => ({ transform: [{ translateX: tx.value }] }));
  const dimStyle = useAnimatedStyle(() => ({ opacity: dim.value }));

  if (!mounted) return null;

  return (
    <Modal transparent visible animationType="none" onRequestClose={close}>
      <View className="flex-1">
        <Animated.View style={[StyleSheet.absoluteFill, dimStyle]} className="bg-black/50">
          <Pressable className="flex-1" onPress={close} accessibilityLabel="" />
        </Animated.View>
        <Animated.View style={[panelStyle, { paddingTop: insets.top }]} className="absolute bottom-0 left-0 top-0 bg-surface">
          {inAdmin ? <AdminSidebar /> : <LibrarySidebar />}
        </Animated.View>
      </View>
    </Modal>
  );
}
