import { useState } from 'react';
import { Pressable, Text, View } from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import Slider from '@react-native-community/slider';
import { CoverArt } from '../src/components/CoverArt';
import { IconButton } from '../src/components/ui';
import { PlayButton } from '../src/components/PlayButton';
import { Lyrics } from '../src/components/Lyrics';
import { usePlayer } from '../src/audio/store';
import { useLyrics } from '../src/query/lyrics';
import { formatDuration } from '../src/utils/format';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

/**
 * Full-screen "now playing". Presented modally. Shows large artwork, a seekable
 * progress bar, primary transport controls, and repeat mode. Track/queue state
 * comes entirely from the player store, so it mirrors whatever the OS controls
 * (lockscreen/MediaSession) do too.
 */
export default function Player() {
  const t = useT();
  const colors = useColors();
  const song = usePlayer((s) => (s.index >= 0 ? s.songs[s.index] : undefined));
  const status = usePlayer((s) => s.status);
  const position = usePlayer((s) => s.position);
  const duration = usePlayer((s) => s.duration);
  const repeat = usePlayer((s) => s.repeat);
  const toggle = usePlayer((s) => s.toggle);
  const next = usePlayer((s) => s.next);
  const previous = usePlayer((s) => s.previous);
  const seekTo = usePlayer((s) => s.seekTo);
  const cycleRepeat = usePlayer((s) => s.cycleRepeat);
  const shuffle = usePlayer((s) => s.shuffle);
  const toggleShuffle = usePlayer((s) => s.toggleShuffle);

  const [scrubbing, setScrubbing] = useState<number | null>(null);
  const [showLyrics, setShowLyrics] = useState(false);

  const { data: lyrics } = useLyrics(song?.id);
  const hasLyrics = !!lyrics && lyrics.lines.length > 0;
  const lyricsVisible = showLyrics && hasLyrics;

  if (!song) {
    return (
      <SafeAreaView className="flex-1 items-center justify-center bg-background">
        <Text className="text-muted">{t('media.player.nothingPlaying')}</Text>
        <Pressable onPress={() => router.back()} className="mt-4">
          <Text className="text-primary">{t('media.player.close')}</Text>
        </Pressable>
      </SafeAreaView>
    );
  }

  const isPlaying = status === 'playing';
  const shownPosition = scrubbing ?? position;
  const repeatIcon = repeat === 'track' ? 'repeat' : 'repeat';
  const repeatActive = repeat !== 'off';

  return (
    <SafeAreaView className="flex-1 bg-background">
      <View className="flex-row items-center justify-between px-4 pt-2">
        <IconButton name="chevron-down" size={28} onPress={() => router.back()} accessibilityLabel={t('media.player.close')} />
        <Text className="text-sm font-medium text-muted">{t('media.player.nowPlaying')}</Text>
        <View className="flex-row items-center gap-1">
          {hasLyrics ? (
            <IconButton
              name="mic"
              size={22}
              color={lyricsVisible ? colors.primary : colors.muted}
              onPress={() => setShowLyrics((v) => !v)}
              accessibilityLabel={t('media.player.lyrics')}
            />
          ) : null}
          <IconButton name="list" size={24} onPress={() => router.push('/queue')} accessibilityLabel={t('media.player.queue')} />
        </View>
      </View>

      <View className="flex-1 justify-center px-6">
        {lyricsVisible && lyrics ? (
          <View className="flex-1">
            <Lyrics lines={lyrics.lines} synced={lyrics.synced} positionMs={shownPosition * 1000} />
          </View>
        ) : (
          <View className="items-center">
            <CoverArt coverArt={song.coverArt} url={song.coverUrl} size={300} rounded="rounded-3xl" />
          </View>
        )}

        <View className="pt-8">
          <Text numberOfLines={1} className="text-2xl font-bold text-foreground">
            {song.title}
          </Text>
          <Text numberOfLines={1} className="pt-1 text-lg text-muted">
            {song.artist}
          </Text>
        </View>

        <View className="pt-6">
          <Slider
            style={{ opacity: song.remote ? 0.4 : 1 }}
            minimumValue={0}
            maximumValue={duration > 0 ? duration : 1}
            value={shownPosition}
            minimumTrackTintColor={colors.primary}
            maximumTrackTintColor={colors.border}
            thumbTintColor={colors.primary}
            onValueChange={(v) => setScrubbing(v)}
            onSlidingComplete={(v) => {
              setScrubbing(null);
              void seekTo(v);
            }}
          />
          <View className="flex-row justify-between">
            <Text className="text-xs text-muted">{formatDuration(shownPosition)}</Text>
            <Text className="text-xs text-muted">{formatDuration(duration)}</Text>
          </View>
          {song.remote ? (
            <Text className="pt-1 text-center text-xs text-muted">{t('media.player.seekUnavailableRemote')}</Text>
          ) : null}
        </View>

        <View className="flex-row items-center justify-between px-2 pt-6">
          <IconButton
            name={repeatIcon}
            size={24}
            color={repeatActive ? colors.primary : colors.muted}
            onPress={cycleRepeat}
            accessibilityLabel={t('media.player.repeat')}
          />
          <IconButton name="play-skip-back" size={34} onPress={previous} accessibilityLabel={t('media.player.previous')} />
          <PlayButton playing={isPlaying} onPress={toggle} size={72} />
          <IconButton name="play-skip-forward" size={34} onPress={next} accessibilityLabel={t('media.player.next')} />
          <IconButton
            name="shuffle"
            size={24}
            color={shuffle ? colors.primary : colors.muted}
            onPress={toggleShuffle}
            accessibilityLabel={t('media.player.shuffle')}
          />
        </View>
      </View>
    </SafeAreaView>
  );
}
