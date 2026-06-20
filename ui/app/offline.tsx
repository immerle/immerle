import { Pressable, Switch, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { Button, Card, EmptyState } from '../src/components/ui';
import { AdminHeader, AdminScroll } from '../src/components/AdminUI';
import { CoverArt } from '../src/components/CoverArt';
import { Ionicon } from '../src/components/Ionicon';
import { useDownloads, OfflineEntry } from '../src/offline/store';
import { usePlayer } from '../src/audio/store';
import { Song } from '../src/api/subsonic/types';
import { formatBytes } from '../src/utils/format';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

/** An offline entry carries enough to play it (the URL is resolved locally). */
function toSong(e: OfflineEntry): Song {
  return { id: e.id, title: e.title, artist: e.artist, album: e.album, coverArt: e.coverArt, duration: e.duration };
}

/**
 * Manage tracks downloaded for offline playback: list them, see total size,
 * remove one or all. Tapping a row plays the whole downloaded set from there.
 */
export default function Offline() {
  const t = useT();
  const colors = useColors();
  const entries = useDownloads((s) => s.entries);
  const remove = useDownloads((s) => s.remove);
  const clearAll = useDownloads((s) => s.clearAll);
  const wifiOnly = useDownloads((s) => s.wifiOnly);
  const setWifiOnly = useDownloads((s) => s.setWifiOnly);
  const lastError = useDownloads((s) => s.lastError);
  const clearError = useDownloads((s) => s.clearError);

  const list = Object.values(entries).sort((a, b) => b.downloadedAt - a.downloadedAt);
  const totalSize = list.reduce((n, e) => n + (e.size ?? 0), 0);

  const play = (entry: OfflineEntry) => {
    const idx = list.findIndex((e) => e.id === entry.id);
    void usePlayer.getState().playSongs(list.map(toSong), Math.max(0, idx));
  };

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll
        header={
          <AdminHeader
            color={colors.primary}
            title={t('offline.title')}
            subtitle={list.length ? t('offline.summary', { count: list.length, size: formatBytes(totalSize) }) : t('offline.subtitle')}
          />
        }
      >
        {lastError === 'quota' ? (
          <Pressable onPress={clearError}>
            <Card className="flex-row items-center gap-3 border border-danger/40 bg-danger/10">
              <Ionicon name="warning-outline" size={20} color={colors.danger} />
              <Text className="flex-1 text-sm text-foreground">{t('offline.quotaError')}</Text>
              <Text className="text-xs font-semibold text-danger">{t('offline.dismiss')}</Text>
            </Card>
          </Pressable>
        ) : null}

        <Card className="flex-row items-center justify-between">
          <Text className="flex-1 pr-2 text-base text-foreground">{t('offline.wifiOnly')}</Text>
          <Switch value={wifiOnly} onValueChange={setWifiOnly} trackColor={{ true: colors.primary, false: colors.border }} />
        </Card>

        {!list.length ? (
          <EmptyState icon="cloud-offline-outline" title={t('offline.emptyTitle')} subtitle={t('offline.emptySubtitle')} />
        ) : (
          <>
            {list.map((e) => (
              <Card key={e.id} className="flex-row items-center gap-3">
                <Pressable className="flex-1 flex-row items-center gap-3 active:opacity-70" onPress={() => play(e)}>
                  <CoverArt coverArt={e.coverArt} size={48} rounded="rounded-md" />
                  <View className="flex-1">
                    <Text numberOfLines={1} className="text-base font-semibold text-foreground">
                      {e.title}
                    </Text>
                    <Text numberOfLines={1} className="text-sm text-muted">
                      {e.artist}
                      {e.size ? ` · ${formatBytes(e.size)}` : ''}
                    </Text>
                  </View>
                </Pressable>
                <Pressable
                  accessibilityLabel={t('offline.remove')}
                  className="h-9 w-9 items-center justify-center rounded-lg active:bg-surface-alt"
                  onPress={() => void remove(e.id)}
                >
                  <Ionicon name="trash-outline" size={20} color={colors.danger} />
                </Pressable>
              </Card>
            ))}
            <View className="pt-2">
              <Button title={t('offline.clearAll')} variant="danger" icon="trash-outline" onPress={() => void clearAll()} />
            </View>
          </>
        )}
      </AdminScroll>
    </>
  );
}
