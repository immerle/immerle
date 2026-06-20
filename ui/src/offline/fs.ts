// Web fallback for the offline filesystem. Offline downloads are native-only for
// now (W6); on web every call is a no-op and isSupported is false, so the store
// short-circuits. The real implementation lives in fs.native.ts.

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
  /* no-op on web */
}

export async function exists(_name: string): Promise<boolean> {
  return false;
}
