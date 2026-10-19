import * as Network from 'expo-network';
import { NetworkClass } from './quality';

/** Network Information API (Chromium/Android web only; absent elsewhere). */
interface WebConnection {
  type?: string;
  effectiveType?: string;
  saveData?: boolean;
  addEventListener?: (ev: 'change', cb: () => void) => void;
  removeEventListener?: (ev: 'change', cb: () => void) => void;
}

function webConnection(): WebConnection | undefined {
  return (globalThis.navigator as { connection?: WebConnection } | undefined)?.connection;
}

/** Classifies the current connection for adaptive streaming quality. */
export async function networkClass(): Promise<NetworkClass> {
  const c = webConnection();
  if (c?.saveData || /2g|3g/.test(c?.effectiveType ?? '')) return 'slow';
  if (c?.type === 'cellular') return 'cellular';
  try {
    const state = await Network.getNetworkStateAsync();
    if (state.type === Network.NetworkStateType.CELLULAR) return 'cellular';
  } catch {
    /* can't tell (e.g. web) → assume fast */
  }
  return 'fast';
}

/** Calls cb whenever the connection may have changed. Returns an unsubscribe. */
export function onNetworkChange(cb: () => void): () => void {
  let sub: { remove: () => void } | undefined;
  try {
    sub = Network.addNetworkStateListener(() => cb());
  } catch {
    /* no native module (tests/web fallback) */
  }
  const c = webConnection();
  c?.addEventListener?.('change', cb);
  return () => {
    sub?.remove();
    c?.removeEventListener?.('change', cb);
  };
}
