import { useEffect } from 'react';
import { ScrollView, Text, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useQueryClient } from '@tanstack/react-query';
import { useJam } from '../../src/jam/store';
import { usePlayer } from '../../src/audio/store';
import { useAuth } from '../../src/auth/store';
import { CoverArt } from '../../src/components/CoverArt';
import { PlayButton } from '../../src/components/PlayButton';
import { Button, Card, IconButton } from '../../src/components/ui';
import { Ionicon } from '../../src/components/Ionicon';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';
import { useWebTitle } from '../../src/utils/documentTitle';
import { confirm } from '../../src/utils/confirm';

/**
 * Active Jam session. The host's playback drives everyone: the host controls
 * with the normal transport (which broadcasts), and followers' audio is kept in
 * sync by the Jam engine (see `src/jam/store.ts`). This screen shows who's in,
 * the now-playing track, and host controls.
 */
export default function Jam() {
  const t = useT();
  const colors = useColors();
  const { id } = useLocalSearchParams<{ id: string }>();
  const client = useAuth((s) => s.client);
  const qc = useQueryClient();

  const session = useJam((s) => s.session);
  const participants = useJam((s) => s.participants);
  const isHost = useJam((s) => s.isHost);
  const stop = useJam((s) => s.stop);
  const end = useJam((s) => s.end);

  const song = usePlayer((s) => (s.index >= 0 ? s.songs[s.index] : undefined));
  const status = usePlayer((s) => s.status);
  const toggle = usePlayer((s) => s.toggle);
  const next = usePlayer((s) => s.next);
  const previous = usePlayer((s) => s.previous);

  // Joining via a direct link / refresh: enrol as a follower if not already in.
  useEffect(() => {
    if (!id || !client) return;
    if (useJam.getState().sessionId !== id) {
      client
        .jamJoin(id)
        .then(() => useJam.getState().start(id, false))
        .catch(() => undefined);
    }
  }, [id, client]);

  useWebTitle(session?.name);

  const isPlaying = status === 'playing';

  // Navigating away from this screen must NOT disconnect anyone — the sync
  // engine (SSE/polling + the host's playback push) lives in the global
  // useJam store precisely so a Jam keeps running in the background while
  // browsing the rest of the app. Only the explicit "Leave"/"End" actions
  // below actually stop it.
  const onClose = () => router.back();

  const onLeave = async () => {
    await stop();
    router.back();
  };

  const onEnd = () =>
    confirm(
      t('social.jamScreen.endJamConfirm'),
      async () => {
        await end();
        void qc.invalidateQueries({ queryKey: ['jam', 'mine'] });
        router.back();
      },
      { cancel: t('social.common.cancel'), ok: t('social.jamScreen.endJam') },
    );

  return (
    <>
      <Stack.Screen
        options={{
          title: session?.name || t('social.jamScreen.title'),
          headerRight: () => (
            <IconButton name="close" color={colors.foreground} onPress={onClose} accessibilityLabel={t('social.common.close')} />
          ),
        }}
      />
      <ScrollView className="flex-1 bg-background" contentContainerStyle={{ padding: 16 }}>
        <Card className="items-center gap-3 py-6">
          <View className="flex-row items-center gap-2">
            <Ionicon name="radio" size={18} color={colors.primary} />
            <Text className="text-sm font-semibold text-primary">{isHost ? t('social.jamScreen.youAreHost') : t('social.jamScreen.synced')}</Text>
          </View>

          {song ? (
            <>
              <CoverArt coverArt={song.coverArt} size={180} rounded="rounded-2xl" />
              <Text numberOfLines={1} className="text-xl font-bold text-foreground">{song.title}</Text>
              <Text numberOfLines={1} className="text-sm text-muted">{song.artist}</Text>
            </>
          ) : (
            <View className="items-center gap-2 py-6">
              <View className="h-44 w-44 items-center justify-center rounded-2xl bg-surface-alt">
                <Ionicon name="musical-notes" size={48} color={colors.muted} />
              </View>
              <Text className="text-center text-sm text-muted">
                {isHost ? t('social.jamScreen.hostStartHint') : t('social.jamScreen.waitingHost')}
              </Text>
            </View>
          )}

          <View className="flex-row items-center gap-2">
            <View className={`h-2 w-2 rounded-full ${isPlaying ? 'bg-success' : 'bg-muted'}`} />
            <Text className="text-xs text-muted">{isPlaying ? t('social.jamScreen.playing') : t('social.jamScreen.paused')}</Text>
          </View>

          {isHost ? (
            <View className="flex-row items-center gap-6 pt-1">
              <IconButton name="play-skip-back" size={28} onPress={previous} accessibilityLabel={t('social.jamScreen.previous')} />
              <PlayButton playing={isPlaying} onPress={toggle} size={60} />
              <IconButton name="play-skip-forward" size={28} onPress={next} accessibilityLabel={t('social.jamScreen.next')} />
            </View>
          ) : (
            <Text className="pt-1 text-xs text-muted">{t('social.jamScreen.hostControls')}</Text>
          )}
        </Card>

        <Text className="px-1 pb-2 pt-5 text-lg font-bold text-foreground">
          {t('social.jamScreen.participants', { count: participants.length })}
        </Text>
        <View className="gap-2">
          {participants.map((p) => (
            <View key={p.userId ?? p.username} className="flex-row items-center gap-3 rounded-xl bg-surface p-3">
              <View className="h-9 w-9 items-center justify-center rounded-full bg-surface-alt">
                <Text className="font-bold text-foreground">{(p.username ?? '?').charAt(0).toUpperCase()}</Text>
              </View>
              <Text className="flex-1 text-base text-foreground">{p.username}</Text>
              {p.userId === session?.hostId ? (
                <View className="rounded-full bg-primary/15 px-2 py-0.5">
                  <Text className="text-xs font-medium text-primary">{t('social.jamScreen.hostBadge')}</Text>
                </View>
              ) : null}
            </View>
          ))}
        </View>

        <View className="gap-2 pt-6">
          <Button title={t('social.jamScreen.leaveJam')} variant="secondary" icon="exit-outline" onPress={onLeave} />
          {isHost ? (
            <Button title={t('social.jamScreen.endJam')} variant="danger" icon="stop-circle-outline" onPress={onEnd} />
          ) : null}
        </View>
      </ScrollView>
    </>
  );
}
