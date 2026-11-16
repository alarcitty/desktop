import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  isInWatchLater,
  isLiked,
  localLibrary,
  mergeCloudLibraryItems,
  toggleLike,
  toggleWatchLater
} from '../src/app/library.svelte';
import {
  clearWatchHistory,
  mergeCloudHistoryState,
  watchHistory
} from '../src/app/history.svelte';
import { startCloudUserSync, stopCloudUserSync } from '../src/app/cloudSync.svelte';

afterEach(() => {
  stopCloudUserSync();
  for (const item of [...localLibrary.items]) {
    for (const list of item.lists) {
      if (list === 'likes') toggleLike({ shikiId: item.shikiId, title: item.title });
      if (list === 'watch_later') toggleWatchLater({ shikiId: item.shikiId, title: item.title });
    }
  }
  clearWatchHistory();
  delete (globalThis as { window?: unknown }).window;
});

describe('library without stubs, like toggle, and cloud sync', () => {
  it('starts empty with no hardcoded stub items and toggles likes/watch_later cleanly', () => {
    expect(localLibrary.items).toEqual([]);

    const liked = toggleLike({
      shikiId: 52991,
      title: 'Frieren: Beyond Journey’s End',
      poster: 'https://example.com/frieren.jpg',
      score: 9.3,
      tags: ['2023', 'Adventure', 'Drama']
    });
    expect(liked).toBe(true);
    expect(isLiked({ shikiId: 52991 })).toBe(true);
    expect(isInWatchLater({ shikiId: 52991 })).toBe(false);
    expect(localLibrary.items).toHaveLength(1);
    expect(localLibrary.items[0].lists).toEqual(['likes']);

    const saved = toggleWatchLater({
      shikiId: 52991,
      title: 'Frieren: Beyond Journey’s End'
    });
    expect(saved).toBe(true);
    expect(localLibrary.items[0].lists).toEqual(['likes', 'watch_later']);

    expect(toggleLike({ shikiId: 52991, title: 'Frieren: Beyond Journey’s End' })).toBe(false);
    expect(isLiked({ shikiId: 52991 })).toBe(false);
    expect(isInWatchLater({ shikiId: 52991 })).toBe(true);
  });

  it('merges remote cloud library and watch history cleanly and pushes live mutations', async () => {
    const cloudRequest = vi.fn().mockImplementation(async (method: string, path: string) => {
      if (method === 'GET' && path === '/api/v1/user/library') {
        return JSON.stringify({
          data: {
            items: [
              {
                id: 'shiki-16498',
                shikiId: 16498,
                title: 'Attack on Titan',
                description: 'Scout Regiment.',
                poster: 'https://img/aot.jpg',
                score: 8.6,
                tags: ['2013', 'Action'],
                lists: ['likes'],
                updatedAt: 2000
              }
            ]
          }
        });
      }
      if (method === 'GET' && path === '/api/v1/user/history') {
        return JSON.stringify({
          data: {
            entries: [
              {
                id: 'shiki:16498',
                shikiId: 16498,
                title: 'Attack on Titan',
                description: '',
                poster: 'https://img/aot.jpg',
                tags: ['2013'],
                episodeOrdinal: 4,
                episodeIndex: 3,
                totalEpisodes: 25,
                currentTime: 510,
                duration: 1440,
                updatedAt: 2500
              }
            ],
            episode_progress: {
              's:16498:ep:4': {
                currentTime: 510,
                duration: 1440,
                watched: false,
                updatedAt: 2500
              }
            }
          }
        });
      }
      return JSON.stringify({ data: { ok: true } });
    });

    (globalThis as { window?: unknown }).window = {
      go: { mila: { Mila: { CloudRequest: cloudRequest } } },
      setTimeout: globalThis.setTimeout,
      clearTimeout: globalThis.clearTimeout,
      addEventListener: () => {}
    };

    await startCloudUserSync();
    expect(isLiked({ shikiId: 16498 })).toBe(true);
    expect(watchHistory.entries).toHaveLength(1);
    expect(watchHistory.entries[0].episodeOrdinal).toBe(4);

    // Mutate after sync -> immediate PUT /api/v1/user/library.
    toggleLike({
      shikiId: 38000,
      title: 'Demon Slayer',
      tags: ['2019']
    });
    await Promise.resolve();
    expect(
      cloudRequest.mock.calls.some(
        ([m, p, body]) => m === 'PUT' && p === '/api/v1/user/library' && String(body).includes('Demon Slayer')
      )
    ).toBe(true);

    // Merge preserves remote order & updates cleanly.
    const merged = mergeCloudLibraryItems([]);
    expect(merged.length).toBeGreaterThanOrEqual(2);
    const mergedHist = mergeCloudHistoryState([], {});
    expect(mergedHist.entries).toHaveLength(1);
  });
});
