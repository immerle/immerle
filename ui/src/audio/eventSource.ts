/** Minimal subset of EventSource used by the play-queue live channel. */
export interface EventSourceLike {
  addEventListener: (type: string, listener: (e: { data?: string }) => void) => void;
}

/** The browser's own EventSource (see eventSource.native.ts for native). */
export const EventSourceImpl = (globalThis as { EventSource?: new (url: string) => EventSourceLike }).EventSource;
