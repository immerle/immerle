import { create } from 'zustand';
import { useAuth } from '../auth/store';
import { usePlayer } from '../audio/store';
import { JamParticipantDTO, JamSessionDTO } from '../api/immerleApi';

type Session = JamSessionDTO & { updatedAt?: string };

/**
 * Real-time Jam sync. Host is the source of truth: pushes `{currentTrackId,
 * position(ms), state, trackIds}` on change or every few seconds (`jamUpdate`).
 * Followers get the session (SSE on web, polling on native) and drive their
 * audio engine to the drift-corrected position. Runs globally (create/join to
 * leave) so sync continues while browsing other screens.
 */
interface JamStore {
  sessionId: string | null;
  isHost: boolean;
  session: Session | null;
  participants: JamParticipantDTO[];
  active: boolean;
  start: (sessionId: string, isHost: boolean) => void;
  stop: () => Promise<void>;
  end: () => Promise<void>;
}

// Module-scoped sync resources (not reactive state).
let sse: { close: () => void } | null = null;
let pollTimer: ReturnType<typeof setInterval> | null = null;
let hostTimer: ReturnType<typeof setInterval> | null = null;
let unsubPlayer: (() => void) | null = null;
let applying = false;

function api() {
  return useAuth.getState().client;
}

function cleanup() {
  sse?.close();
  sse = null;
  if (pollTimer) clearInterval(pollTimer);
  if (hostTimer) clearInterval(hostTimer);
  pollTimer = null;
  hostTimer = null;
  unsubPlayer?.();
  unsubPlayer = null;
}

/** Drift-corrected live position (seconds) for a session snapshot. */
function livePosition(session: Session): number {
  const base = (session.position ?? 0) / 1000;
  if (session.state !== 'playing' || !session.updatedAt) return base;
  const elapsed = (Date.now() - Date.parse(session.updatedAt)) / 1000;
  return base + Math.max(0, elapsed);
}

/** Follower: drive the local audio engine to match the host's session. */
async function applySession(session: Session | null) {
  if (!session || applying) return;
  const player = usePlayer.getState();
  const want = session.currentTrackId;
  if (!want) return; // host hasn't started playing yet
  const target = livePosition(session);
  const curId = player.current()?.id;

  applying = true;
  try {
    if (want !== curId) {
      await player.playTrackById(want, target, session.state === 'playing');
    } else {
      if (session.state === 'playing' && player.status !== 'playing') await player.toggle();
      else if (session.state === 'paused' && player.status === 'playing') await player.toggle();
      if (session.state === 'playing' && Math.abs(player.position - target) > 2.5) {
        await player.seekTo(target);
      }
    }
  } finally {
    applying = false;
  }
}

function handleSnapshot(session: Session | null, participants: JamParticipantDTO[], isHost: boolean) {
  if (!useJam.getState().active) return;
  useJam.setState({ session: session ?? null, participants: participants ?? [] });
  if (!isHost) void applySession(session);
}

/** Host: push the local player's state to the session. */
async function pushHost(sessionId: string) {
  const client = api();
  const player = usePlayer.getState();
  const song = player.index >= 0 ? player.songs[player.index] : undefined;
  if (!client || !song) return;
  await client
    .jamUpdate(sessionId, {
      currentTrackId: song.id,
      position: Math.floor(player.position * 1000),
      state: player.status === 'playing' ? 'playing' : 'paused',
      trackIds: player.songs.map((s) => s.id),
    })
    .catch(() => undefined);
}

function startHostPush(sessionId: string) {
  let lastKey = '';
  unsubPlayer = usePlayer.subscribe((s) => {
    if (!useJam.getState().active) return;
    const key = `${s.index >= 0 ? s.songs[s.index]?.id : ''}:${s.status}`;
    if (key !== lastKey) {
      lastKey = key;
      void pushHost(sessionId);
    }
  });
  // Periodic resync so seeks and drift propagate even without track/state changes.
  hostTimer = setInterval(() => void pushHost(sessionId), 8000);
}

function connect(sessionId: string, isHost: boolean) {
  const client = api();
  if (!client) return;
  const ES = (globalThis as { EventSource?: new (url: string) => EventSourceLike }).EventSource;

  if (ES) {
    const es = new ES(client.jamEventsUrl(sessionId));
    es.addEventListener('state', (e: { data: string }) => {
      try {
        const d = JSON.parse(e.data) as { session?: Session; participants?: JamParticipantDTO[] };
        handleSnapshot(d.session ?? null, d.participants ?? [], isHost);
      } catch {
        /* ignore malformed events */
      }
    });
    sse = es;
  } else {
    const poll = () =>
      client
        .jamState(sessionId)
        .then((r) => handleSnapshot(r.session ?? null, r.participants, isHost))
        .catch(() => undefined);
    void poll();
    pollTimer = setInterval(poll, 1500);
  }
}

interface EventSourceLike {
  addEventListener: (type: string, listener: (e: { data: string }) => void) => void;
  close: () => void;
}

export const useJam = create<JamStore>((set, get) => ({
  sessionId: null,
  isHost: false,
  session: null,
  participants: [],
  active: false,

  start: (sessionId, isHost) => {
    cleanup();
    set({ sessionId, isHost, session: null, participants: [], active: true });
    connect(sessionId, isHost);
    if (isHost) startHostPush(sessionId);
  },

  stop: async () => {
    const client = api();
    const sid = get().sessionId;
    cleanup();
    set({ sessionId: null, isHost: false, session: null, participants: [], active: false });
    if (client && sid) await client.jamLeave(sid).catch(() => undefined);
  },

  end: async () => {
    const client = api();
    const sid = get().sessionId;
    cleanup();
    set({ sessionId: null, isHost: false, session: null, participants: [], active: false });
    if (client && sid) await client.jamEnd(sid).catch(() => undefined);
  },
}));
