import { useState } from 'react';
import { Text, View } from 'react-native';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useLocalSongs, useUploadTracks } from '../src/query/local';
import { TrackList } from '../src/components/TrackList';
import { LocalCover } from '../src/components/LocalCover';
import { UploadDropzone } from '../src/components/UploadDropzone';
import { LocalTrackEditModal } from '../src/components/LocalTrackEditModal';
import { PlayButton } from '../src/components/PlayButton';
import { EmptyState, ErrorState, Loading } from '../src/components/ui';
import { usePlayer } from '../src/audio/store';
import { Song } from '../src/api/subsonic/types';
import { formatDuration } from '../src/utils/format';
import { useT } from '../src/i18n/store';

/**
 * "Musiques locales" — a virtual playlist of the tracks the user uploaded from
 * the web (drag-and-drop). Tracks can be played, added to playlists (via the
 * row menu), and edited (rename / cover) by their owner.
 */
export default function Local() {
  const t = useT();
  const { title, artist } = useLocalSearchParams<{ title?: string; artist?: string }>();
  const q = useLocalSongs();
  const upload = useUploadTracks();
  const playSongs = usePlayer((s) => s.playSongs);
  const [editing, setEditing] = useState<Song | null>(null);

  const songs = q.data ?? [];
  const totalDuration = songs.reduce((n, s) => n + (s.duration ?? 0), 0);

  const Header = (
    <View className="w-full max-w-2xl items-center gap-3 self-center px-4 pb-2 pt-2">
      <LocalCover size={200} rounded={16} />
      <Text className="text-2xl font-bold tracking-tight text-foreground">{t('media.local.title')}</Text>
      <Text className="text-xs text-muted">
        {t('media.local.trackCount', { count: songs.length })} · {formatDuration(totalDuration)}
      </Text>
      {title ? (
        <Text className="text-center text-sm text-muted">
          {t('media.local.lookingFor', { name: artist ? `${title} — ${artist}` : title })}
        </Text>
      ) : null}
      <View className="w-full py-2">
        <UploadDropzone onFiles={(files) => upload.mutate(files)} busy={upload.isPending} />
      </View>
      {songs.length > 0 ? (
        <View className="w-full flex-row items-center justify-end py-1">
          <PlayButton
            onPress={() => playSongs(songs, 0)}
            size={56}
            accessibilityLabel={t('media.local.play')}
          />
        </View>
      ) : null}
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ title: t('media.local.title') }} />
      <View className="flex-1 bg-background">
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message={t('media.local.loadError')} onRetry={q.refetch} />
        ) : songs.length === 0 ? (
          <View className="flex-1">
            {Header}
            <EmptyState icon="cloud-upload-outline" title={t('media.local.empty')} subtitle={t('media.local.emptySubtitle')} />
          </View>
        ) : (
          <TrackList
            songs={songs}
            header={Header}
            refreshing={q.isRefetching}
            onRefresh={q.refetch}
            onEditTrack={setEditing}
          />
        )}
      </View>
      <LocalTrackEditModal song={editing} onClose={() => setEditing(null)} />
    </>
  );
}
