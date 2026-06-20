// Tiny IndexedDB wrapper for offline audio blobs (web). One object store keyed by
// the offline file name (see paths.ts); the value is the raw audio Blob. Kept
// dependency-free and small so it's unit-testable with fake-indexeddb.

const DB_NAME = 'immerle-offline';
const STORE = 'tracks';

function open(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 1);
    req.onupgradeneeded = () => req.result.createObjectStore(STORE);
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

// Runs fn in a transaction and resolves with the request result *after* the
// transaction commits (so a write is durable before we resolve).
function run<T>(mode: IDBTransactionMode, fn: (store: IDBObjectStore) => IDBRequest): Promise<T> {
  return open().then(
    (db) =>
      new Promise<T>((resolve, reject) => {
        const tx = db.transaction(STORE, mode);
        let result: T;
        const req = fn(tx.objectStore(STORE));
        req.onsuccess = () => {
          result = req.result as T;
        };
        tx.oncomplete = () => {
          db.close();
          resolve(result);
        };
        tx.onerror = () => {
          db.close();
          reject(tx.error);
        };
        tx.onabort = () => {
          db.close();
          reject(tx.error);
        };
      }),
  );
}

export function idbPut(key: string, blob: Blob): Promise<void> {
  return run<unknown>('readwrite', (s) => s.put(blob, key)).then(() => undefined);
}

export function idbGet(key: string): Promise<Blob | undefined> {
  return run<Blob | undefined>('readonly', (s) => s.get(key));
}

export function idbDelete(key: string): Promise<void> {
  return run<unknown>('readwrite', (s) => s.delete(key)).then(() => undefined);
}

export async function idbHas(key: string): Promise<boolean> {
  const k = await run<IDBValidKey | undefined>('readonly', (s) => s.getKey(key));
  return k !== undefined;
}
