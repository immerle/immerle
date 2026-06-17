import { useEffect } from 'react';
import { ScrollView, Text, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useJam } from '../../src/jam/store';
import { usePlayer } from '../../src/audio/store';
import { useAuth } from '../../src/auth/store';
import { CoverArt } from '../../src/components/CoverArt';
import { PlayButton } from '../../src/components/PlayButton';
import { Button, Card, IconButton } from '../../src/components/ui';
import { Ionicon } from '../../src/components/Ionicon';
import { useColors } from '../../src/theme/colors';

/**
 * Active Jam session. The host's playback drives everyone: the host controls
 * with the normal transport (which broadcasts), and followers' audio is kept in
 * sync by the Jam engine (see `src/jam/store.ts`). This screen shows who's in,
 * the now-playing track, and host controls.
 */
export default function Jam() {
  const colors = useColors();
  const { id } = useLocalSearchParams<{ id: string }>();
  const client = useAuth((s) => s.client);

  const session = useJam((s) => s.session);
  const participants = useJam((s) => s.participants);
  const isHost = useJam((s) => s.isHost);
  const stop = useJam((s) => s.stop);

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

  const isPlaying = status === 'playing';

  const onLeave = async () => {
    await stop();
    router.back();
  };

  return (
    <>
      <Stack.Screen
        options={{
          title: session?.name || 'Jam',
          headerRight: () => (
            <IconButton name="exit-outline" color={colors.danger} onPress={onLeave} accessibilityLabel="Quitter" />
          ),
        }}
      />
      <ScrollView className="flex-1 bg-background" contentContainerStyle={{ padding: 16 }}>
        <Card className="items-center gap-3 py-6">
          <View className="flex-row items-center gap-2">
            <Ionicon name="radio" size={18} color={colors.primary} />
            <Text className="text-sm font-semibold text-primary">{isHost ? 'Vous êtes l’hôte' : 'Écoute synchronisée'}</Text>
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
                {isHost ? 'Lancez une lecture (album, recherche…) pour démarrer le Jam.' : "En attente de l'hôte…"}
              </Text>
            </View>
          )}

          <View className="flex-row items-center gap-2">
            <View className={`h-2 w-2 rounded-full ${isPlaying ? 'bg-success' : 'bg-muted'}`} />
            <Text className="text-xs text-muted">{isPlaying ? 'En lecture' : 'En pause'}</Text>
          </View>

          {isHost ? (
            <View className="flex-row items-center gap-6 pt-1">
              <IconButton name="play-skip-back" size={28} onPress={previous} accessibilityLabel="Précédent" />
              <PlayButton playing={isPlaying} onPress={toggle} size={60} />
              <IconButton name="play-skip-forward" size={28} onPress={next} accessibilityLabel="Suivant" />
            </View>
          ) : (
            <Text className="pt-1 text-xs text-muted">L'hôte contrôle la lecture.</Text>
          )}
        </Card>

        <Text className="px-1 pb-2 pt-5 text-lg font-bold text-foreground">
          Participants ({participants.length})
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
                  <Text className="text-xs font-medium text-primary">Hôte</Text>
                </View>
              ) : null}
            </View>
          ))}
        </View>

        <View className="pt-6">
          <Button title="Quitter le Jam" variant="secondary" icon="exit-outline" onPress={onLeave} />
        </View>
      </ScrollView>
    </>
  );
}
