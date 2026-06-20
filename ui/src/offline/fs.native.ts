// Native offline filesystem (iOS/Android) backed by expo-file-system. Files live
// under <documentDirectory>/offline/. Callers pass a stable basename (see
// paths.ts) and we rebuild the absolute URI here, so a changed container path
// after a reinstall doesn't strand the registry.

import * as FileSystem from 'expo-file-system';

export const isSupported = true;

const DIR = `${FileSystem.documentDirectory ?? ''}offline/`;

export function fileUri(name: string): string {
  return DIR + name;
}

export async function ensureDir(): Promise<void> {
  const info = await FileSystem.getInfoAsync(DIR);
  if (!info.exists) {
    await FileSystem.makeDirectoryAsync(DIR, { intermediates: true });
  }
}

/**
 * Downloads url into <offline>/name, reporting 0..1 progress, and returns the
 * resulting file size in bytes. Throws if the download fails.
 */
export async function download(url: string, name: string, onProgress?: (p: number) => void): Promise<number> {
  await ensureDir();
  const dest = fileUri(name);
  const task = FileSystem.createDownloadResumable(url, dest, {}, (d) => {
    if (onProgress && d.totalBytesExpectedToWrite > 0) {
      onProgress(d.totalBytesWritten / d.totalBytesExpectedToWrite);
    }
  });
  await task.downloadAsync();
  const info = await FileSystem.getInfoAsync(dest);
  if (!info.exists) throw new Error('download produced no file');
  return info.size ?? 0;
}

/** The local file:// URI the player uses as an audio source (native is direct). */
export async function playableUrl(name: string): Promise<string | null> {
  return fileUri(name);
}

export async function remove(name: string): Promise<void> {
  await FileSystem.deleteAsync(fileUri(name), { idempotent: true });
}

export async function exists(name: string): Promise<boolean> {
  const info = await FileSystem.getInfoAsync(fileUri(name));
  return info.exists;
}
