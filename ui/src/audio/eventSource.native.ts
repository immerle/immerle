import { AppState } from 'react-native';
import { fetch } from 'expo/fetch';
import type { EventSourceLike } from './eventSource';

export type { EventSourceLike } from './eventSource';

type Listener = (e: { data?: string }) => void;

const RECONNECT_MS = 3000;
// The server pings every 20s (see handleStreamPlayQueue): this long without
// a byte means the connection died silently (network switch, app resumed
// after iOS suspended it) and a read would otherwise hang forever.
const SILENCE_TIMEOUT_MS = 45000;

/**
 * React Native has no EventSource: a minimal one over expo/fetch's streaming
 * body, so native gets the same instant play-queue push as web instead of a
 * 5s poll (remote commands sent to a phone used to land seconds late).
 * Reconnects forever, like the browser's, and right away when the app comes
 * back to the foreground (the server re-sends the full state on connect).
 */
class NativeEventSource implements EventSourceLike {
  private listeners: Record<string, Listener[]> = {};
  private controller: AbortController | null = null;
  private silenceTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(private readonly url: string) {
    AppState.addEventListener('change', (state) => {
      if (state === 'active') this.reconnect();
    });
    void this.run();
  }

  /** Drop the current connection; run() opens a new one immediately. */
  private reconnect(): void {
    this.controller?.abort();
  }

  private armSilenceTimer(): void {
    if (this.silenceTimer) clearTimeout(this.silenceTimer);
    this.silenceTimer = setTimeout(() => this.reconnect(), SILENCE_TIMEOUT_MS);
  }

  addEventListener(type: string, listener: Listener): void {
    (this.listeners[type] ??= []).push(listener);
  }

  private emit(type: string, data?: string): void {
    this.listeners[type]?.forEach((l) => l({ data }));
  }

  private async run(): Promise<void> {
    for (;;) {
      const controller = new AbortController();
      this.controller = controller;
      this.armSilenceTimer();
      try {
        const res = await fetch(this.url, { headers: { Accept: 'text/event-stream' }, signal: controller.signal });
        if (!res.ok || !res.body) throw new Error(`HTTP ${res.status}`);
        this.emit('open');
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buf = '';
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          this.armSilenceTimer();
          buf += decoder.decode(value, { stream: true });
          let sep = buf.indexOf('\n\n');
          while (sep >= 0) {
            this.dispatch(buf.slice(0, sep));
            buf = buf.slice(sep + 2);
            sep = buf.indexOf('\n\n');
          }
        }
      } catch {
        /* reconnect below */
      }
      if (this.silenceTimer) clearTimeout(this.silenceTimer);
      this.emit('error');
      // A deliberate reconnect (foreground, silence) goes again at once; a
      // failure backs off so a down server isn't hammered.
      if (!controller.signal.aborted) await new Promise((r) => setTimeout(r, RECONNECT_MS));
    }
  }

  /** One SSE block: `event:` + `data:` lines; comments (`: ping`) ignored. */
  private dispatch(block: string): void {
    let event = 'message';
    const data: string[] = [];
    for (const line of block.split('\n')) {
      if (line.startsWith('event:')) event = line.slice(6).trim();
      else if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''));
    }
    if (data.length) this.emit(event, data.join('\n'));
  }
}

export const EventSourceImpl: (new (url: string) => EventSourceLike) | undefined = NativeEventSource;
