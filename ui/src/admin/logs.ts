import { useEffect, useState } from 'react';
import { useAuth } from '../auth/store';

/** One structured JSON log line as emitted by the server's slog JSON handler. */
export interface LogLine {
  time?: string;
  level?: string;
  msg?: string;
  [key: string]: unknown;
}

/** How many recent lines the viewer keeps in memory (oldest dropped first). */
const MAX_LINES = 500;

interface EventSourceLike {
  addEventListener: (type: string, listener: (e: { data?: string }) => void) => void;
  close: () => void;
}

/**
 * Live server log stream for the admin log viewer — Server-Sent Events, web
 * only (native has no EventSource; this is a diagnostics screen, not worth a
 * poll fallback like the play-queue/jam live channels have).
 */
export function useLogStream(): { lines: LogLine[]; connected: boolean; supported: boolean; clear: () => void } {
  const client = useAuth((s) => s.client);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [connected, setConnected] = useState(false);
  const ES = (globalThis as { EventSource?: new (url: string) => EventSourceLike }).EventSource;

  useEffect(() => {
    if (!client || !ES) return;
    const es = new ES(client.logsStreamUrl());
    es.addEventListener('open', () => setConnected(true));
    es.addEventListener('error', () => setConnected(false));
    es.addEventListener('log', (e) => {
      if (!e.data) return;
      try {
        const line = JSON.parse(e.data) as LogLine;
        setLines((prev) => (prev.length >= MAX_LINES ? [...prev.slice(1), line] : [...prev, line]));
      } catch {
        // ignore a malformed line rather than breaking the whole stream
      }
    });
    return () => es.close();
  }, [client, ES]);

  return { lines, connected, supported: !!ES, clear: () => setLines([]) };
}
