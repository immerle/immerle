import TrackPlayer, { Event } from 'react-native-track-player';

/**
 * Playback service for react-native-track-player. Registered once at app start
 * (see `engine.native.ts`). It maps OS/remote transport events (lockscreen,
 * notification, headset, CarPlay/Android Auto) onto player actions so playback
 * keeps responding while the app is backgrounded or the screen is locked.
 */
export async function PlaybackService(): Promise<void> {
  TrackPlayer.addEventListener(Event.RemotePlay, () => TrackPlayer.play());
  TrackPlayer.addEventListener(Event.RemotePause, () => TrackPlayer.pause());
  TrackPlayer.addEventListener(Event.RemoteNext, () => TrackPlayer.skipToNext());
  TrackPlayer.addEventListener(Event.RemotePrevious, () => TrackPlayer.skipToPrevious());
  TrackPlayer.addEventListener(Event.RemoteSeek, ({ position }) =>
    TrackPlayer.seekTo(position),
  );
  TrackPlayer.addEventListener(Event.RemoteStop, () => TrackPlayer.stop());
  TrackPlayer.addEventListener(Event.RemoteJumpForward, async ({ interval }) => {
    const pos = (await TrackPlayer.getProgress()).position;
    await TrackPlayer.seekTo(pos + (interval ?? 15));
  });
  TrackPlayer.addEventListener(Event.RemoteJumpBackward, async ({ interval }) => {
    const pos = (await TrackPlayer.getProgress()).position;
    await TrackPlayer.seekTo(Math.max(0, pos - (interval ?? 15)));
  });
}
