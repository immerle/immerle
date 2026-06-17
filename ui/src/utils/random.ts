/**
 * Generate a random lowercase-hex string of `length` characters.
 *
 * Prefers a CSPRNG (`crypto.getRandomValues`, available on web and on native
 * via React Native's runtime) and falls back to `Math.random` only if no
 * crypto source exists. Used for the per-session Subsonic auth salt.
 */
export function randomHex(length = 16): string {
  const bytes = Math.ceil(length / 2);
  const buf = new Uint8Array(bytes);
  const g = globalThis as { crypto?: { getRandomValues?: (a: Uint8Array) => void } };
  if (g.crypto?.getRandomValues) {
    g.crypto.getRandomValues(buf);
  } else {
    for (let i = 0; i < bytes; i += 1) buf[i] = Math.floor(Math.random() * 256);
  }
  let out = '';
  for (let i = 0; i < bytes; i += 1) out += buf[i].toString(16).padStart(2, '0');
  return out.slice(0, length);
}
