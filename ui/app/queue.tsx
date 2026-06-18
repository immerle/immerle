import { Pressable, Text, View } from 'react-native';
import { FlashList } from '@shopify/flash-list';
import { usePlayer } from '../src/audio/store';
import { CoverArt } from '../src/components/CoverArt';
import { IconButton, EmptyState } from '../src/components/ui';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

/**
 * Reorderable play queue. Tap a row to jump to it; use the arrows to reorder
 * (works on web and native) and the minus button to remove. Edits flow straight
 * to the engine via the player store, so the active queue updates live.
 */
export default function Queue() {
  const t = useT();
  const colors = useColors();
  const songs = usePlayer((s) => s.songs);
  const index = usePlayer((s) => s.index);
  const skipTo = usePlayer((s) => s.skipTo);
  const move = usePlayer((s) => s.move);
  const removeAt = usePlayer((s) => s.removeAt);

  if (songs.length === 0) {
    return <EmptyState icon="list" title={t('media.queue.empty')} subtitle={t('media.queue.emptySubtitle')} />;
  }

  return (
    <View className="flex-1 bg-background">
      <FlashList
        data={songs}
        keyExtractor={(s, i) => `${s.id}:${i}`}
        estimatedItemSize={64}
        extraData={index}
        renderItem={({ item, index: i }) => {
          const active = i === index;
          return (
            <View className={`flex-row items-center gap-2 px-3 py-2 ${active ? 'bg-primary/10' : ''}`}>
              <Pressable onPress={() => skipTo(i)} className="flex-1 flex-row items-center gap-3 active:opacity-70">
                <CoverArt coverArt={item.coverArt} size={44} rounded="rounded-md" />
                <View className="flex-1">
                  <Text numberOfLines={1} className={`text-base ${active ? 'font-semibold text-primary' : 'text-foreground'}`}>
                    {item.title}
                  </Text>
                  <Text numberOfLines={1} className="text-sm text-muted">
                    {item.artist}
                  </Text>
                </View>
              </Pressable>
              <IconButton name="chevron-up" size={20} color={colors.muted} onPress={() => i > 0 && move(i, i - 1)} />
              <IconButton name="chevron-down" size={20} color={colors.muted} onPress={() => i < songs.length - 1 && move(i, i + 1)} />
              <IconButton name="remove-circle" size={20} color={colors.danger} onPress={() => removeAt(i)} />
            </View>
          );
        }}
        contentContainerStyle={{ paddingBottom: 24 }}
      />
    </View>
  );
}
