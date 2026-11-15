import fs from 'node:fs';
import path from 'node:path';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  createDebouncedStorageWriter,
  createDeferredDestroyQueue,
  releaseVideoElementBuffers,
  safeStorageSet,
  setBoundedMapEntry
} from '../src/app/lifecycle';

const SRC_ROOT = path.resolve(__dirname, '../src');

describe('T-122 lifecycle, cleanup & hot-path storage primitives', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('coalesces high-frequency storage writes into a single trailing write and flushes deterministically', () => {
    const writes: number[] = [];
    const writer = createDebouncedStorageWriter<number>(1000, (v) => writes.push(v));

    // Simulate 20 progress ticks at 100ms intervals
    for (let i = 1; i <= 20; i++) {
      writer.schedule(i);
      vi.advanceTimersByTime(100);
    }

    expect(writes).toEqual([]);
    expect(writer.hasPending()).toBe(true);

    // Pause / unmount / pagehide flush must commit once immediately
    writer.flush();
    expect(writes).toEqual([20]);
    expect(writer.hasPending()).toBe(false);

    // Advancing past debounce delay after flush must not double-write
    vi.advanceTimersByTime(2000);
    expect(writes).toEqual([20]);
  });

  it('supports trailing timer write and clean cancellation', () => {
    const writes: string[] = [];
    const writer = createDebouncedStorageWriter<string>(300, (v) => writes.push(v));

    writer.schedule('ep-1');
    writer.schedule('ep-2');
    expect(writes).toEqual([]);

    vi.advanceTimersByTime(300);
    expect(writes).toEqual(['ep-2']);

    writer.schedule('cancelled');
    writer.cancel();
    expect(writer.hasPending()).toBe(false);
    vi.advanceTimersByTime(500);
    expect(writes).toEqual(['ep-2']);

    // Flushing when nothing is queued must be a safe no-op
    writer.flush();
    expect(writes).toEqual(['ep-2']);
  });

  it('flushes deferred teardown destructors immediately on unmount without double-calling', () => {
    const queue = createDeferredDestroyQueue();
    const destroyed: string[] = [];

    queue.schedule(() => destroyed.push('hls-1'), 380);
    queue.schedule(() => {
      throw new Error('already destroyed');
    }, 380);
    queue.schedule(() => destroyed.push('hls-2'), 380);

    expect(queue.size()).toBe(3);

    // Unmount flushes all pending delayed destructors synchronously
    queue.flushAll();
    expect(destroyed).toEqual(['hls-1', 'hls-2']);
    expect(queue.size()).toBe(0);

    // Pending timers must have been cleared so nothing fires again at 380ms
    vi.advanceTimersByTime(500);
    expect(destroyed).toEqual(['hls-1', 'hls-2']);
  });

  it('cancels deferred destructors cleanly via cancelAll()', () => {
    const queue = createDeferredDestroyQueue();
    const destroyed: string[] = [];
    queue.schedule(() => destroyed.push('hls-x'), 380);
    queue.cancelAll();
    expect(queue.size()).toBe(0);
    vi.advanceTimersByTime(500);
    expect(destroyed).toEqual([]);
  });

  it('releases HTMLMediaElement buffers cleanly and tolerates null/throwing elements', () => {
    const ops: string[] = [];
    releaseVideoElementBuffers({
      pause: () => ops.push('pause'),
      removeAttribute: (attr) => ops.push(`remove:${attr}`),
      load: () => ops.push('load')
    });
    expect(ops).toEqual(['pause', 'remove:src', 'load']);

    expect(() => releaseVideoElementBuffers(null)).not.toThrow();
    expect(() => releaseVideoElementBuffers(undefined)).not.toThrow();
    expect(() =>
      releaseVideoElementBuffers({
        pause: () => {
          throw new Error('detached');
        },
        removeAttribute: () => {
          throw new Error('detached');
        },
        load: () => {
          throw new Error('detached');
        }
      })
    ).not.toThrow();
  });

  it('caps in-memory Map caches with bounded FIFO/LRU eviction', () => {
    const cache = new Map<string, number>();
    for (let i = 1; i <= 25; i++) {
      setBoundedMapEntry(cache, `k${i}`, i, 20);
    }
    expect(cache.size).toBe(20);
    expect(cache.has('k1')).toBe(false);
    expect(cache.has('k5')).toBe(false);
    expect(cache.has('k6')).toBe(true);
    expect(cache.has('k25')).toBe(true);

    // Refresh existing key and insert another
    setBoundedMapEntry(cache, 'k6', 60, 20);
    setBoundedMapEntry(cache, 'k26', 26, 20);
    expect(cache.has('k6')).toBe(true);
    expect(cache.get('k6')).toBe(60);
    expect(cache.has('k7')).toBe(false);
  });

  it('safeStorageSet swallows storage quota errors cleanly', () => {
    vi.stubGlobal('localStorage', {
      setItem: () => {
        throw new Error('QuotaExceededError');
      }
    });
    expect(() => safeStorageSet('test.key', 'value')).not.toThrow();
  });

  it('debounces watchHistory localStorage writes on hot-path progress updates and flushes on demand', async () => {
    const storageMap = new Map<string, string>();
    const setItemSpy = vi.fn((k: string, v: string) => {
      storageMap.set(k, v);
    });
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => storageMap.get(k) ?? null,
      setItem: setItemSpy,
      removeItem: (k: string) => {
        storageMap.delete(k);
      }
    });

    vi.resetModules();
    const historyMod = await import('../src/app/history.svelte');
    setItemSpy.mockClear();

    // Simulate 15 progress reports during active playback
    for (let sec = 3; sec <= 45; sec += 3) {
      historyMod.recordHistoryEntry({
        shikiId: 54744,
        title: 'Alya',
        episodeOrdinal: 1,
        episodeIndex: 0,
        totalEpisodes: 12,
        currentTime: sec,
        duration: 1400
      });
    }

    // ZERO synchronous localStorage.setItem writes during the burst!
    expect(setItemSpy).not.toHaveBeenCalled();

    // Advance past 300ms (episodeProgress) & 1000ms (watchHistory) debounce windows
    vi.advanceTimersByTime(1000);
    expect(setItemSpy).toHaveBeenCalledTimes(2);
    expect(storageMap.get('mila.watch.history.v1')).toContain('"currentTime":45');

    // Explicit flush persists immediately without waiting for timer
    setItemSpy.mockClear();
    historyMod.recordHistoryEntry({
      shikiId: 54744,
      title: 'Alya',
      episodeOrdinal: 1,
      episodeIndex: 0,
      totalEpisodes: 12,
      currentTime: 90,
      duration: 1400
    });
    expect(setItemSpy).not.toHaveBeenCalled();
    historyMod.flushHistoryPersistence();
    expect(setItemSpy).toHaveBeenCalledTimes(2);
    expect(storageMap.get('mila.watch.history.v1')).toContain('"currentTime":90');

    // Clear history must cancel pending timers and write empty state immediately
    historyMod.clearWatchHistory();
    vi.advanceTimersByTime(2000);
    expect(storageMap.get('mila.watch.history.v1')).toBe('[]');
  });

  it('audits target components for mandatory teardown hooks (HlsPlayer, SearchPopup, EpisodeThumb, AnimePage, PlayerDock)', () => {
    const hlsPlayerSrc = fs.readFileSync(
      path.join(SRC_ROOT, 'marlakit/player/HlsPlayer.svelte'),
      'utf8'
    );
    expect(hlsPlayerSrc).toContain('onDestroy(');
    expect(hlsPlayerSrc).toContain('releaseVideoElementBuffers(videoEl)');
    expect(hlsPlayerSrc).toContain('releaseVideoElementBuffers(dubAudioEl)');
    expect(hlsPlayerSrc).toContain('pipClickTimer');
    expect(hlsPlayerSrc).toContain('pendingRestoreSeek');

    const searchPopupSrc = fs.readFileSync(
      path.join(SRC_ROOT, 'section/search/SearchPopup.svelte'),
      'utf8'
    );
    expect(searchPopupSrc).toContain('onDestroy(');
    expect(searchPopupSrc).toContain('openingDestroyQueue.flushAll()');
    expect(searchPopupSrc).toContain('releaseVideoElementBuffers');
    expect(searchPopupSrc).toContain('resultRefs.length');

    const episodeThumbSrc = fs.readFileSync(
      path.join(SRC_ROOT, 'pages/anime/EpisodeThumb.svelte'),
      'utf8'
    );
    expect(episodeThumbSrc).toContain('onDestroy(');
    expect(episodeThumbSrc).toContain('releaseVideoElementBuffers(videoEl)');
    expect(episodeThumbSrc).toContain('HlsCtor.Events.ERROR');

    const animePageSrc = fs.readFileSync(
      path.join(SRC_ROOT, 'pages/anime/AnimePage.svelte'),
      'utf8'
    );
    expect(animePageSrc).toContain('onDestroy(');
    expect(animePageSrc).toContain('ro.disconnect()');
    expect(animePageSrc).toContain('torrentReqToken');

    const playerDockSrc = fs.readFileSync(
      path.join(SRC_ROOT, 'app/PlayerDock.svelte'),
      'utf8'
    );
    expect(playerDockSrc).toContain('activeDragCleanup');
  });
});
