import type HlsType from 'hls.js';

let cachedHls: typeof HlsType | null = null;
let pendingHls: Promise<typeof HlsType> | null = null;

/**
 * Returns the already-loaded Hls constructor synchronously if available.
 */
export function getLoadedHls(): typeof HlsType | null {
  return cachedHls;
}

/**
 * Lazily imports `hls.js` once and caches the constructor across players/previews.
 * Clears the pending promise on rejection so transient chunk load errors can retry.
 */
export async function loadHls(): Promise<typeof HlsType> {
  if (cachedHls) return cachedHls;
  if (!pendingHls) {
    pendingHls = import('hls.js')
      .then((mod) => {
        cachedHls = mod.default;
        return mod.default;
      })
      .catch((err) => {
        pendingHls = null;
        throw err;
      });
  }
  return pendingHls;
}

/**
 * Clears cached module references (intended for deterministic unit testing).
 */
export function resetHlsLoaderForTest(): void {
  cachedHls = null;
  pendingHls = null;
}
