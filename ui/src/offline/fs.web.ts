// Web offline storage backed by IndexedDB. Audio blobs are stored under their
// offline file name (see paths.ts); playback gets a blob: object URL. Mirrors the
// fs.native.ts contract so the store/UI are platform-agnostic.

import { idbPut, idbGet, idbDelete, idbHas } from './idb';

export const isSupported = typeof indexedDB !== 'undefined';

// name -> object URL, so repeated queue builds reuse one URL per track instead of
// leaking a new one each time. Cleared entries are revoked.
const urlCache = new Map<string, string>();

// Ask the browser to make storage persistent (not evicted under pressure) once,
// before the first download. Best-effort: ignored if unsupported or denied.
let persistenceAsked = false;
async function requestPersistence(): Promise<void> {
  if (persistenceAsked) return;
  persistenceAsked = true;
  try {
    if (navigator.storage?.persist && !(await navigator.storage.persisted())) {
      await navigator.storage.persist();
    }
  } catch {
    /* best effort */
  }
}

// Native concept; unused on web (playback goes through playableUrl).
export function fileUri(_name: string): string {
  return '';
}

export async function ensureDir(): Promise<void> {
  /* no directories on web */
}

/**
 * Fetches url and stores the response as a blob under name, reporting 0..1
 * progress when the server sends Content-Length. Returns the byte size.
 */
export async function download(url: string, name: string, onProgress?: (p: number) => void): Promise<number> {
  await requestPersistence();
  const res = await fetch(url);
  if (!res.ok) throw new Error(`download failed: ${res.status}`);

  const total = Number(res.headers.get('Content-Length') ?? 0);
  let blob: Blob;
  if (onProgress && res.body && total > 0) {
    const reader = res.body.getReader();
    const chunks: BlobPart[] = [];
    let received = 0;
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      if (value) {
        chunks.push(value);
        received += value.length;
        onProgress(received / total);
      }
    }
    blob = new Blob(chunks, { type: res.headers.get('Content-Type') ?? 'audio/mpeg' });
  } else {
    blob = await res.blob();
  }

  await idbPut(name, blob);
  return blob.size;
}

export async function remove(name: string): Promise<void> {
  const cached = urlCache.get(name);
  if (cached) {
    URL.revokeObjectURL(cached);
    urlCache.delete(name);
  }
  await idbDelete(name);
}

export async function exists(name: string): Promise<boolean> {
  return idbHas(name);
}

/** A blob: URL the player can use as an audio source, or null if not stored. */
export async function playableUrl(name: string): Promise<string | null> {
  const cached = urlCache.get(name);
  if (cached) return cached;
  const blob = await idbGet(name);
  if (!blob) return null;
  const url = URL.createObjectURL(blob);
  urlCache.set(name, url);
  return url;
}
