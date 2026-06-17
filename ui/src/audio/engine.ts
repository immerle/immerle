import type { AudioEngine } from './types';

/**
 * Platform resolution fallback.
 *
 * Metro resolves `./engine` to `engine.web.ts` (web) or `engine.native.ts`
 * (iOS/Android). This base file is only ever picked up by TypeScript for types
 * and as a last-resort fallback; it never runs in a real bundle.
 */
export function createEngine(): AudioEngine {
  throw new Error('No audio engine available for this platform');
}
