import { useCallback, useRef, useState } from 'react';
import {
  LayoutChangeEvent,
  NativeScrollEvent,
  NativeSyntheticEvent,
  Platform,
  Pressable,
  ScrollView,
  ScrollViewProps,
  View,
} from 'react-native';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

const SCROLL_STEP = 480; // px per arrow tap, roughly one row of tiles

/**
 * ponytail: horizontal ScrollView with edge arrows to scroll left/right on
 * web — touch already scrolls natively on mobile, untouched, arrows only
 * render on web. Mouse-drag scrolling on web was tried and abandoned:
 * blocking the click on whatever Pressable is under the cursor after a real
 * drag kept breaking in new ways (see this file's git history) — arrows
 * sidestep the problem entirely instead of fighting it.
 */
export function ScrollRow({ children, ...props }: ScrollViewProps) {
  const t = useT();
  const ref = useRef<ScrollView>(null);
  const scrollX = useRef(0);
  const contentWidth = useRef(0);
  const viewportWidth = useRef(0);
  const [canLeft, setCanLeft] = useState(false);
  const [canRight, setCanRight] = useState(false);

  const recompute = useCallback(() => {
    setCanLeft(scrollX.current > 1);
    setCanRight(scrollX.current < contentWidth.current - viewportWidth.current - 1);
  }, []);

  const onScroll = (e: NativeSyntheticEvent<NativeScrollEvent>) => {
    scrollX.current = e.nativeEvent.contentOffset.x;
    recompute();
  };
  const onContentSizeChange = (w: number) => {
    contentWidth.current = w;
    recompute();
  };
  const onLayout = (e: LayoutChangeEvent) => {
    viewportWidth.current = e.nativeEvent.layout.width;
    recompute();
  };
  const scrollBy = (dx: number) => {
    const target = Math.max(0, Math.min(scrollX.current + dx, contentWidth.current - viewportWidth.current));
    ref.current?.scrollTo({ x: target, animated: true });
  };

  return (
    <View className="relative">
      <ScrollView
        ref={ref}
        horizontal
        {...props}
        onScroll={onScroll}
        onContentSizeChange={onContentSizeChange}
        onLayout={onLayout}
        scrollEventThrottle={32}
      >
        {children}
      </ScrollView>
      {Platform.OS === 'web' && canLeft ? (
        <Arrow icon="chevron-back" side="left" label={t('components.scrollRow.scrollLeft')} onPress={() => scrollBy(-SCROLL_STEP)} />
      ) : null}
      {Platform.OS === 'web' && canRight ? (
        <Arrow icon="chevron-forward" side="right" label={t('components.scrollRow.scrollRight')} onPress={() => scrollBy(SCROLL_STEP)} />
      ) : null}
    </View>
  );
}

function Arrow({
  icon,
  side,
  label,
  onPress,
}: {
  icon: string;
  side: 'left' | 'right';
  label: string;
  onPress: () => void;
}) {
  const colors = useColors();
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      accessibilityLabel={label}
      style={{ top: '50%', marginTop: -18, [side]: 4 }}
      className="absolute h-9 w-9 items-center justify-center rounded-full bg-surface-alt shadow active:opacity-70"
    >
      <Ionicon name={icon} size={20} color={colors.foreground} />
    </Pressable>
  );
}
