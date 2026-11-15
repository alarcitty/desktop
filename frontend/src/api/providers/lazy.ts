export interface LazyModuleLoader<T> {
  load: () => Promise<T>;
  getLoaded: () => T | null;
  resetForTest: () => void;
}

/**
 * Deduplicates concurrent dynamic imports and resets the pending promise on failure
 * so transient chunk-load errors can be retried cleanly.
 */
export function createLazyModuleLoader<T>(importer: () => Promise<T>): LazyModuleLoader<T> {
  let cached: T | null = null;
  let pending: Promise<T> | null = null;

  return {
    async load(): Promise<T> {
      if (cached) return cached;
      if (!pending) {
        pending = importer()
          .then((mod) => {
            cached = mod;
            return mod;
          })
          .catch((err) => {
            pending = null;
            throw err;
          });
      }
      return pending;
    },
    getLoaded(): T | null {
      return cached;
    },
    resetForTest(): void {
      cached = null;
      pending = null;
    }
  };
}
