// Default/fallback offline storage contract. At runtime Metro resolves the
// platform variant instead — fs.native.ts (expo-file-system) or fs.web.ts
// (IndexedDB). This no-op file only loads where neither applies; it also serves
// as the shared type contract (tsc resolves `./fs` here).

export const isSupported = false;

export function fileUri(_name: string): string {
  return '';
}

export async function ensureDir(): Promise<void> {
  /* no-op on web */
}

export async function download(_url: string, _name: string, _onProgress?: (p: number) => void): Promise<number> {
  throw new Error('offline downloads are not supported on web');
}

export async function remove(_name: string): Promise<void> {
  /* no-op */
}

export async function exists(_name: string): Promise<boolean> {
  return false;
}

export async function playableUrl(_name: string): Promise<string | null> {
  return null;
}
