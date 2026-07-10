import TrackPlayer, { type BackgroundEvent, Event } from '@rntp/player';

/**
 * Single dispatch point for remote-control events (lockscreen, notification,
 * headset, CarPlay/Android Auto), shared by both delivery paths @rntp/player
 * uses: `addEventListener` (iOS foreground + audio background) and
 * `registerBackgroundEventHandler` (Android background, JS may not be running).
 */
async function handleRemoteEvent(event: BackgroundEvent): Promise<void> {
  switch (event.type) {
    case Event.RemotePlay:
      TrackPlayer.play();
      break;
    case Event.RemotePause:
      TrackPlayer.pause();
      break;
    case Event.RemoteNext:
      TrackPlayer.skipToNext();
      break;
    case Event.RemotePrevious:
      TrackPlayer.skipToPrevious();
      break;
    case Event.RemoteStop:
      TrackPlayer.stop();
      break;
    case Event.RemoteSeek:
      TrackPlayer.seekTo(event.position);
      break;
    case Event.RemoteSkipForward:
      TrackPlayer.seekBy(event.interval ?? 15);
      break;
    case Event.RemoteSkipBackward:
      TrackPlayer.seekBy(-(event.interval ?? 15));
      break;
    default:
      break;
  }
}

/** iOS foreground + audio-background dispatch (also covers Android UI-foreground). */
export function wireRemoteEvents(): void {
  TrackPlayer.addEventListener(Event.RemotePlay, () =>
    handleRemoteEvent({ type: Event.RemotePlay }),
  );
  TrackPlayer.addEventListener(Event.RemotePause, () =>
    handleRemoteEvent({ type: Event.RemotePause }),
  );
  TrackPlayer.addEventListener(Event.RemoteNext, () =>
    handleRemoteEvent({ type: Event.RemoteNext }),
  );
  TrackPlayer.addEventListener(Event.RemotePrevious, () =>
    handleRemoteEvent({ type: Event.RemotePrevious }),
  );
  TrackPlayer.addEventListener(Event.RemoteStop, () =>
    handleRemoteEvent({ type: Event.RemoteStop }),
  );
  TrackPlayer.addEventListener(Event.RemoteSeek, (e) =>
    handleRemoteEvent({ type: Event.RemoteSeek, ...e }),
  );
  TrackPlayer.addEventListener(Event.RemoteSkipForward, (e) =>
    handleRemoteEvent({ type: Event.RemoteSkipForward, ...e }),
  );
  TrackPlayer.addEventListener(Event.RemoteSkipBackward, (e) =>
    handleRemoteEvent({ type: Event.RemoteSkipBackward, ...e }),
  );
}

/** Android-only: dispatch while the app is backgrounded and JS isn't running. */
export function registerBackgroundHandler(): void {
  TrackPlayer.registerBackgroundEventHandler(() => handleRemoteEvent);
}
