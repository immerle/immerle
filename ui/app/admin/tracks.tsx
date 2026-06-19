import { useState } from 'react';
import { Alert, Platform, Pressable, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import * as ImagePicker from 'expo-image-picker';
import { useAdminTracks, useTrackMutations } from '../../src/query/tracks';
import { Song } from '../../src/api/subsonic/types';
import { Button, ErrorState, Field, Loading } from '../../src/components/ui';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { CoverArt } from '../../src/components/CoverArt';
import { Ionicon } from '../../src/components/Ionicon';
import { formatDuration } from '../../src/utils/format';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';

const PAGE_SIZE = 50;

/** Admin library: list downloaded tracks, edit metadata/cover, delete. */
export default function AdminTracks() {
  const t = useT();
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(0);
  const q = useAdminTracks(query, PAGE_SIZE, page * PAGE_SIZE);
  const [expanded, setExpanded] = useState<string | null>(null);

  const tracks = q.data?.tracks ?? [];
  const total = q.data?.total ?? 0;
  const from = total === 0 ? 0 : page * PAGE_SIZE + 1;
  const to = Math.min((page + 1) * PAGE_SIZE, total);

  const onSearch = (v: string) => {
    setQuery(v);
    setPage(0);
  };

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll
        header={
          <AdminHeader
            color="#1ed760"
            title={t('admin.tracks.title')}
            subtitle={t('admin.tracks.trackCount', { count: total })}
          />
        }
      >
        <Field
          label={t('admin.tracks.searchLabel')}
          placeholder={t('admin.tracks.searchPlaceholder')}
          autoCapitalize="none"
          value={query}
          onChangeText={onSearch}
        />

        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message={t('admin.tracks.loadError')} onRetry={q.refetch} />
        ) : tracks.length === 0 ? (
          <Text className="py-8 text-center text-muted">{t('admin.tracks.empty')}</Text>
        ) : (
          <View className="overflow-hidden rounded-2xl bg-surface">
            {tracks.map((track, i) => (
              <TrackRow
                key={track.id}
                track={track}
                first={i === 0}
                expanded={expanded === track.id}
                onToggle={() => setExpanded((cur) => (cur === track.id ? null : track.id))}
              />
            ))}
          </View>
        )}

        {total > PAGE_SIZE ? (
          <View className="flex-row items-center justify-between pt-1">
            <Button
              title={t('admin.tracks.prev')}
              size="sm"
              variant="secondary"
              icon="chevron-back"
              disabled={page === 0}
              onPress={() => setPage((p) => Math.max(0, p - 1))}
            />
            <Text className="text-xs text-muted">
              {t('admin.tracks.range', { from, to, total })}
            </Text>
            <Button
              title={t('admin.tracks.next')}
              size="sm"
              variant="secondary"
              icon="chevron-forward"
              disabled={to >= total}
              onPress={() => setPage((p) => p + 1)}
            />
          </View>
        ) : null}
      </AdminScroll>
    </>
  );
}

function TrackRow({
  track,
  first,
  expanded,
  onToggle,
}: {
  track: Song;
  first: boolean;
  expanded: boolean;
  onToggle: () => void;
}) {
  const t = useT();
  const colors = useColors();
  const { update, uploadCover, remove } = useTrackMutations();

  const [title, setTitle] = useState(track.title);
  const [genre, setGenre] = useState(track.genre ?? '');
  const [year, setYear] = useState(track.year ? String(track.year) : '');
  const [trackNo, setTrackNo] = useState(track.track ? String(track.track) : '');

  const save = () =>
    update.mutate({
      id: track.id,
      edit: {
        title: title.trim(),
        genre: genre.trim(),
        year: Number(year) || 0,
        trackNo: Number(trackNo) || 0,
      },
    });

  const pickCover = async () => {
    if (Platform.OS !== 'web') {
      const perm = await ImagePicker.requestMediaLibraryPermissionsAsync();
      if (!perm.granted) return;
    }
    const res = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ['images'],
      quality: 0.9,
    });
    if (res.canceled || !res.assets?.length) return;
    const asset = res.assets[0];
    uploadCover.mutate({ id: track.id, uri: asset.uri, mime: asset.mimeType ?? 'image/jpeg' });
  };

  const confirmDelete = () => {
    const doDelete = () => remove.mutate(track.id);
    if (Platform.OS === 'web') doDelete();
    else
      Alert.alert(t('admin.tracks.deleteConfirmTitle'), track.title, [
        { text: t('admin.tracks.cancel'), style: 'cancel' },
        { text: t('admin.tracks.delete'), style: 'destructive', onPress: doDelete },
      ]);
  };

  return (
    <View className={first ? '' : 'border-t border-border'}>
      <Pressable onPress={onToggle} className="flex-row items-center gap-3 px-3 py-2.5 active:bg-surface-alt">
        <CoverArt coverArt={track.coverArt} size={44} />
        <View className="flex-1">
          <Text className="text-base font-semibold text-foreground" numberOfLines={1}>
            {track.title}
          </Text>
          <Text className="text-xs text-muted" numberOfLines={1}>
            {track.artist} · {track.album}
          </Text>
        </View>
        <Text className="text-xs text-muted">{formatDuration(track.duration)}</Text>
        <Ionicon name={expanded ? 'chevron-up' : 'chevron-down'} size={18} color={colors.muted} />
      </Pressable>

      {expanded ? (
        <View className="gap-2 px-3 pb-3">
          <View className="flex-row items-center gap-3">
            <CoverArt coverArt={track.coverArt} size={64} />
            <View className="flex-1">
              <Button
                title={t('admin.tracks.changeCover')}
                size="sm"
                variant="secondary"
                icon="image-outline"
                loading={uploadCover.isPending}
                onPress={pickCover}
              />
            </View>
          </View>

          <Field label={t('admin.tracks.titleLabel')} value={title} onChangeText={setTitle} />
          <Field label={t('admin.tracks.genreLabel')} value={genre} onChangeText={setGenre} />
          <View className="flex-row gap-2">
            <View className="flex-1">
              <Field label={t('admin.tracks.yearLabel')} keyboardType="number-pad" value={year} onChangeText={setYear} />
            </View>
            <View className="flex-1">
              <Field label={t('admin.tracks.trackNoLabel')} keyboardType="number-pad" value={trackNo} onChangeText={setTrackNo} />
            </View>
          </View>

          <View className="flex-row gap-2">
            <View className="flex-1">
              <Button title={t('admin.tracks.save')} size="sm" icon="checkmark" loading={update.isPending} onPress={save} />
            </View>
            <Button title={t('admin.tracks.delete')} size="sm" icon="trash-outline" variant="danger" loading={remove.isPending} onPress={confirmDelete} />
          </View>
        </View>
      ) : null}
    </View>
  );
}
