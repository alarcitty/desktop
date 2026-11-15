export interface DebouncedStorageWriter<T> {
  schedule(value: T): void;
  flush(): void;
  cancel(): void;
  hasPending(): boolean;
}

/**
 * Coalesces high-frequency state persistence into a single trailing write while
 * guaranteeing deterministic flush on pause, close, or teardown.
 */
export function createDebouncedStorageWriter<T>(
  delayMs: number,
  writeFn: (value: T) => void
): DebouncedStorageWriter<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let hasQueued = false;
  let queuedValue: T;

  function clearPendingTimer() {
    if (timer !== undefined) {
      clearTimeout(timer);
      timer = undefined;
    }
  }

  return {
    schedule(value: T) {
      queuedValue = value;
      hasQueued = true;
      clearPendingTimer();
      timer = setTimeout(() => {
        timer = undefined;
        if (!hasQueued) return;
        const val = queuedValue;
        hasQueued = false;
        writeFn(val);
      }, delayMs);
    },

    flush() {
      clearPendingTimer();
      if (!hasQueued) return;
      const val = queuedValue;
      hasQueued = false;
      writeFn(val);
    },

    cancel() {
      clearPendingTimer();
      hasQueued = false;
    },

    hasPending() {
      return hasQueued;
    }
  };
}

/** Safely writes to localStorage, ignoring SSR/private-mode/quota errors. */
export function safeStorageSet(key: string, value: string): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(key, value);
  } catch {
    // Ignore storage quota or private-mode errors.
  }
}

const beforeCloseFlushHandlers = new Set<() => void>();

/**
 * Registers a synchronous persistence flush callback executed on window close
 * (`beforeunload`, `pagehide`, and desktop `mila:before-close` events).
 */
export function registerBeforeCloseFlush(handler: () => void): () => void {
  beforeCloseFlushHandlers.add(handler);
  return () => {
    beforeCloseFlushHandlers.delete(handler);
  };
}

/** Executes every registered synchronous close-flush callback without throwing. */
export function flushBeforeClose(): void {
  for (const handler of Array.from(beforeCloseFlushHandlers)) {
    try {
      handler();
    } catch {
      // Never block window closing if a storage flush throws.
    }
  }
}

export interface DeferredDestroyQueue {
  schedule(fn: () => void, delayMs: number): void;
  flushAll(): void;
  cancelAll(): void;
  size(): number;
}

/**
 * Tracks deferred teardown callbacks (such as cross-fade delayed Hls.destroy calls)
 * so closing or unmounting a component executes every pending destructor immediately.
 */
export function createDeferredDestroyQueue(): DeferredDestroyQueue {
  const pending = new Set<{ timer: ReturnType<typeof setTimeout>; fn: () => void }>();

  function runSafely(fn: () => void) {
    try {
      fn();
    } catch {
      // Ignore already-destroyed media errors during teardown.
    }
  }

  return {
    schedule(fn: () => void, delayMs: number) {
      const entry: { timer: ReturnType<typeof setTimeout>; fn: () => void } = {
        timer: setTimeout(() => {
          pending.delete(entry);
          runSafely(fn);
        }, delayMs),
        fn
      };
      pending.add(entry);
    },

    flushAll() {
      if (pending.size === 0) return;
      const entries = Array.from(pending);
      pending.clear();
      for (const entry of entries) {
        clearTimeout(entry.timer);
        runSafely(entry.fn);
      }
    },

    cancelAll() {
      if (pending.size === 0) return;
      for (const entry of pending) {
        clearTimeout(entry.timer);
      }
      pending.clear();
    },

    size() {
      return pending.size;
    }
  };
}

export interface MediaElementLike {
  pause(): void;
  removeAttribute(qualifiedName: string): void;
  load(): void;
}

/**
 * Stops playback and detaches the active src attribute before calling load(),
 * forcing the media pipeline/MSE buffers to release decoded frames on teardown.
 */
export function releaseVideoElementBuffers(media: MediaElementLike | null | undefined): void {
  if (!media) return;
  try {
    media.pause();
  } catch {
    // Ignore pause failures during teardown.
  }
  try {
    media.removeAttribute('src');
    media.load();
  } catch {
    // Ignore detach failures on torn-down media elements.
  }
}

/** Stores a value in a capacity-bounded Map with FIFO/LRU insertion refresh. */
export function setBoundedMapEntry<K, V>(map: Map<K, V>, key: K, value: V, maxEntries: number): void {
  if (map.has(key)) {
    map.delete(key);
  }
  map.set(key, value);
  while (map.size > maxEntries) {
    const oldest = map.keys().next();
    if (oldest.done) break;
    map.delete(oldest.value);
  }
}
