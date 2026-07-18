import { useEffect, useRef } from 'react';
import { ScrollView, Text, View } from 'react-native';
import { activeLineIndex, LyricLine } from '../lyrics/lyrics';

/**
 * Karaoke lyrics view for the full-screen player. For synced lyrics it
 * highlights the current line and keeps it scrolled into view; for unsynced
 * lyrics it's just scrollable text. Blank synced lines (instrumental breaks)
 * render as a ♪ so the timeline stays visible.
 */
export function Lyrics({
  lines,
  synced,
  positionMs,
}: {
  lines: LyricLine[];
  synced: boolean;
  positionMs: number;
}) {
  const scrollRef = useRef<ScrollView>(null);
  const offsets = useRef<number[]>([]);
  const active = synced ? activeLineIndex(lines, positionMs) : -1;

  useEffect(() => {
    if (active < 0) return;
    const y = offsets.current[active];
    if (y != null) scrollRef.current?.scrollTo({ y: Math.max(0, y - 120), animated: true });
  }, [active]);

  return (
    <ScrollView ref={scrollRef} className="flex-1" showsVerticalScrollIndicator={false}>
      <View className="py-4">
        {lines.map((line, i) => (
          <Text
            key={i}
            onLayout={(e) => {
              offsets.current[i] = e.nativeEvent.layout.y;
            }}
            className={`py-1.5 text-center text-lg ${i === active ? 'font-bold text-primary' : 'text-muted'}`}
          >
            {line.text || '♪'}
          </Text>
        ))}
      </View>
    </ScrollView>
  );
}
