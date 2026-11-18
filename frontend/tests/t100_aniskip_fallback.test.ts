import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  clearAniSkipCacheForTest,
  fetchAniSkipTimes,
  mergeEpisodeSkipFallback
} from '../src/api/aniskip';
import type { PlayableEpisode } from '../src/api/anime';

describe('T-100 AniSkip OP/ED fallback', () => {
  afterEach(() => {
    clearAniSkipCacheForTest();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('calls Wails GetAniSkipTimes when available and caches subsequent lookups', async () => {
    const ipcSpy = vi.fn().mockResolvedValue(
      JSON.stringify({
        data: {
          mal_id: 21,
          episode: 5,
          found: true,
          opening: { start: 85, stop: 175 },
          ending: { start: 1290, stop: 1380 }
        }
      })
    );
    vi.stubGlobal('window', {
      go: { mila: { Mila: { GetAniSkipTimes: ipcSpy } } }
    });

    const first = await fetchAniSkipTimes(21, 5);
    const second = await fetchAniSkipTimes(21, 5);

    expect(ipcSpy).toHaveBeenCalledTimes(1);
    expect(ipcSpy).toHaveBeenCalledWith(21, 5);
    expect(first?.found).toBe(true);
    expect(first?.opening).toEqual({ start: 85, stop: 175 });
    expect(second?.ending).toEqual({ start: 1290, stop: 1380 });
  });

  it('falls back to /ext/aniskip/{malId}/{episode} and deduplicates concurrent requests', async () => {
    const fetchSpy = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          data: {
            mal_id: 16498,
            episode: 1,
            found: true,
            opening: { start: 40, stop: 130 }
          }
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    );
    vi.stubGlobal('fetch', fetchSpy);

    const [a, b] = await Promise.all([fetchAniSkipTimes(16498, 1), fetchAniSkipTimes(16498, 1)]);
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    expect(String(fetchSpy.mock.calls[0][0])).toContain('/ext/aniskip/16498/1');
    expect(a?.opening).toEqual({ start: 40, stop: 130 });
    expect(b?.opening).toEqual({ start: 40, stop: 130 });
  });

  it('fills only missing opening/ending ranges without overwriting native provider timings', () => {
    const baseEp: PlayableEpisode = {
      id: 'ep-1',
      ordinal: 1,
      duration: 1420,
      opening: { start: 10, stop: 100 },
      ending: null
    };

    const merged = mergeEpisodeSkipFallback(baseEp, {
      mal_id: 100,
      episode: 1,
      found: true,
      opening: { start: 50, stop: 140 },
      ending: { start: 1300, stop: 1390 }
    });

    expect(merged.opening).toEqual({ start: 10, stop: 100 });
    expect(merged.ending).toEqual({ start: 1300, stop: 1390 });
  });
});
