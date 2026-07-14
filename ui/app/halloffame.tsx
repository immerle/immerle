import { useEffect, useState } from 'react';
import { Modal, Text, useWindowDimensions, View } from 'react-native';
import { Stack, router } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import DraggableFlatList, { RenderItemParams, ScaleDecorator } from 'react-native-draggable-flatlist';
import { useHallOfFame, useSetHallOfFameOrder, useSetHallOfFameNote } from '../src/query/hallOfFame';
import { HeroBackdrop } from '../src/components/HeroBackdrop';
import { HallOfFamePodium } from '../src/components/HallOfFamePodium';
import { TrackList } from '../src/components/TrackList';
import { DragHandle } from '../src/components/DragHandle';
import { Button, EmptyState, ErrorState, Field, IconButton, Loading } from '../src/components/ui';
import { PlayButton } from '../src/components/PlayButton';
import { usePlayer } from '../src/audio/store';
import { useAuth } from '../src/auth/store';
import { Song } from '../src/api/subsonic/types';
import { formatDuration } from '../src/utils/format';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';
import { useWebTitle } from '../src/utils/documentTitle';

/**
 * Hall of Fame: a user's personal top-tracks ranking — its own dedicated
 * entity (not a playlist). Top-3 podium in the header, the full ranked list
 * (with colored #1/#2/#3 badges) below, drag-reorder + per-track nostalgia
 * notes in edit mode. Tracks are added from any track's context menu ("Add to
 * Hall of Fame"), not from this screen.
 */
export default function HallOfFameScreen() {
  const t = useT();
  const colors = useColors();
  const { width } = useWindowDimensions();
  const wide = width >= 640;
  const insets = useSafeAreaInsets();
  const client = useAuth((s) => s.client);
  const q = useHallOfFame();
  const setOrder = useSetHallOfFameOrder();
  const setNote = useSetHallOfFameNote();
  const playSongs = usePlayer((s) => s.playSongs);

  const [editing, setEditing] = useState(false);
  const [ordered, setOrdered] = useState<Song[]>([]);
  const [noteFor, setNoteFor] = useState<Song | null>(null);
  const [noteDraft, setNoteDraft] = useState('');

  useEffect(() => {
    if (q.data) setOrdered(q.data.entries);
  }, [q.data]);
  useWebTitle(t('media.hallOfFame.title'));

  if (q.isLoading) {
    return (
      <>
        <Stack.Screen options={{ headerShown: false }} />
        <Loading />
      </>
    );
  }
  if (q.isError || !q.data) {
    return (
      <>
        <Stack.Screen options={{ headerShown: false }} />
        <ErrorState message={t('media.hallOfFame.loadError')} onRetry={q.refetch} />
      </>
    );
  }

  const songs = q.data.entries;
  const totalDuration = songs.reduce((n, s) => n + (s.duration ?? 0), 0);

  const removeSong = (id: string) => {
    setOrdered((prev) => prev.filter((s) => s.id !== id));
  };

  const openNote = (song: Song) => {
    setNoteFor(song);
    setNoteDraft(song.comment ?? '');
  };

  const saveNote = () => {
    if (!noteFor) return;
    const comment = noteDraft.trim();
    setNote.mutate(
      { trackId: noteFor.id, comment },
      {
        onSuccess: () => {
          setOrdered((prev) => prev.map((s) => (s.id === noteFor.id ? { ...s, comment } : s)));
          setNoteFor(null);
        },
      },
    );
  };

  const save = async () => {
    const changed = ordered.length !== songs.length || ordered.some((s, i) => s.id !== songs[i]?.id);
    if (changed) {
      await setOrder.mutateAsync(ordered.map((s) => s.id));
    }
    setEditing(false);
  };

  const coverUrl = client?.coverArtUrl(songs[0]?.coverArt, 700);

  const Header = (
    <View>
      <HeroBackdrop url={coverUrl} height={wide ? 260 : 300 + insets.top}>
        {!wide ? (
          <View className="absolute left-4 z-10" style={{ top: insets.top + 8 }}>
            <IconButton name="chevron-back" size={24} color="#fff" onPress={() => router.back()} accessibilityLabel={t('components.admin.back')} />
          </View>
        ) : null}
        <View className={`pb-5 ${wide ? 'flex-row items-end justify-between gap-6 pl-4 pr-10' : 'items-center gap-4 px-4'}`}>
          <View className={wide ? 'min-w-0' : 'items-center'}>
            <Text
              numberOfLines={2}
              className={`font-extrabold tracking-tight text-white ${wide ? 'text-5xl' : 'text-center text-3xl'}`}
            >
              {t('media.hallOfFame.title')}
            </Text>
            <Text className={`pt-3 text-sm text-white/90 ${wide ? '' : 'text-center'}`}>
              {t('media.hallOfFame.trackCount', { count: songs.length })} · {formatDuration(totalDuration)}
            </Text>
          </View>
          {songs.length ? <HallOfFamePodium top={songs.slice(0, 3)} onPress={(i) => playSongs(songs, i)} /> : null}
        </View>
      </HeroBackdrop>

      {/* Action bar over the page background. */}
      <View className="flex-row items-center justify-between gap-5 px-4 py-4">
        <PlayButton
          onPress={() => songs.length && playSongs(songs, 0)}
          size={56}
          accessibilityLabel={t('media.hallOfFame.play')}
        />
        <View className="flex-row items-center gap-4">
          {editing ? (
            <IconButton name="close" size={24} color={colors.muted} onPress={() => setEditing(false)} accessibilityLabel={t('media.hallOfFame.cancel')} />
          ) : null}
          <IconButton
            name={editing ? 'checkmark' : 'create-outline'}
            size={24}
            color={colors.primary}
            disabled={editing && setOrder.isPending}
            onPress={() => (editing ? save() : setEditing(true))}
            accessibilityLabel={t(editing ? 'media.hallOfFame.save' : 'media.hallOfFame.edit')}
          />
        </View>
      </View>
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <View className="flex-1 bg-background">
        {!songs.length && !editing ? (
          <>
            {Header}
            <EmptyState icon="trophy-outline" title={t('media.hallOfFame.empty')} subtitle={t('media.hallOfFame.emptySubtitle')} />
          </>
        ) : editing ? (
          <DraggableFlatList<Song>
            data={ordered}
            keyExtractor={(s) => s.id}
            onDragEnd={({ data }) => setOrdered(data)}
            ListHeaderComponent={Header}
            renderItem={({ item, drag, isActive }: RenderItemParams<Song>) => (
              <ScaleDecorator>
                <View className={`flex-row items-center gap-2 px-3 py-2 ${isActive ? 'bg-surface-alt' : ''}`}>
                  <DragHandle drag={drag} disabled={isActive} accessibilityLabel={t('media.hallOfFame.reorderHandle')} />
                  <View className="flex-1">
                    <Text numberOfLines={1} className="text-base text-foreground">
                      {item.title}
                    </Text>
                    <Text numberOfLines={1} className="text-sm text-muted">
                      {item.artist}
                    </Text>
                  </View>
                  <IconButton
                    name="chatbubble-ellipses-outline"
                    size={22}
                    color={item.comment ? colors.primary : colors.muted}
                    onPress={() => openNote(item)}
                    accessibilityLabel={t('media.hallOfFame.noteEdit')}
                  />
                  <IconButton name="remove-circle" size={22} color={colors.danger} onPress={() => removeSong(item.id)} />
                </View>
              </ScaleDecorator>
            )}
            contentContainerStyle={{ paddingBottom: 24 }}
          />
        ) : (
          <TrackList songs={songs} header={Header} showRank refreshing={q.isRefetching} onRefresh={q.refetch} />
        )}
      </View>

      <Modal visible={!!noteFor} transparent animationType="fade" onRequestClose={() => setNoteFor(null)}>
        <View className="flex-1 items-center justify-center bg-black/60 px-6">
          <View className="w-full max-w-sm gap-3 rounded-2xl bg-surface p-4">
            <Text className="text-base font-semibold text-foreground">{noteFor?.title}</Text>
            <Field value={noteDraft} onChangeText={setNoteDraft} placeholder={t('media.hallOfFame.notePlaceholder')} multiline />
            <View className="flex-row gap-2">
              <View className="flex-1">
                <Button title={t('media.hallOfFame.cancel')} variant="secondary" onPress={() => setNoteFor(null)} />
              </View>
              <View className="flex-1">
                <Button title={t('media.hallOfFame.save')} loading={setNote.isPending} onPress={saveNote} />
              </View>
            </View>
          </View>
        </View>
      </Modal>
    </>
  );
}
