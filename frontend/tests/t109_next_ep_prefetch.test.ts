import { afterEach, describe, expect, it, vi } from 'vitest';
import type { PlayableRelease } from '../src/api/anime';
import { NextEpisodePrefetcher, parseM3U8NextTarget } from '../src/app/prefetch';

function makeRelease(alias: string, provider = 'aniliberty'): PlayableRelease {
  return {
    id: 1,
    alias,
    title: 'Test Anime',
    provider,
    episodes: [
      {
        id: `${alias}-1`,
        ordinal: 1,
        duration: 1000,
        hls_1080: `https://cdn.example.com/${alias}/ep1/master.m3u8`
      },
      {
        id: `${alias}-2`,
        ordinal: 2,
        duration: 1000,
        hls_1080: `https://cdn.example.com/${alias}/ep2/master.m3u8`
      }
    ]
  };
}

describe('T-109 Next Ep Prefetch', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('parses master playlists to best variant and media playlists to first segment', () => {
    const master = `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=1500000,RESOLUTION=854x480
480/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=8000000,RESOLUTION=1920x1080
1080/index.m3u8`;
    expect(parseM3U8NextTarget(master, 'https://cdn.example.com/ep2/master.m3u8')).toEqual({
      kind: 'variant',
      url: 'https://cdn.example.com/ep2/1080/index.m3u8'
    });

    const variant = `#EXTM3U
#EXT-X-TARGETDURATION:6
#EXTINF:6.0,
seg-0001.ts
#EXTINF:6.0,
seg-0002.ts`;
    expect(parseM3U8NextTarget(variant, 'https://cdn.example.com/ep2/1080/index.m3u8')).toEqual({
      kind: 'segment',
      url: 'https://cdn.example.com/ep2/1080/seg-0001.ts'
    });
  });

  it('does not prefetch below 80% and triggers at most once at >= 80%', async () => {
    const calls: string[] = [];
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      calls.push(url);
      if (url.endsWith('/master.m3u8')) {
        return new Response('#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=8000000\n1080.m3u8');
      }
      if (url.endsWith('1080.m3u8')) {
        return new Response('#EXTM3U\n#EXTINF:4,\nseg0.ts');
      }
      return new Response(new Uint8Array([0x47, 0x40]));
    }) as unknown as typeof fetch;

    const aniSkipMock = vi.fn();
    const prefetcher = new NextEpisodePrefetcher({
      fetchFn: fetchMock,
      onAniSkipPrefetch: aniSkipMock
    });
    const rel = makeRelease('solo');

    expect(
      prefetcher.maybePrefetch({
        release: rel,
        episodeIndex: 0,
        currentTime: 799,
        duration: 1000,
        malId: 52299
      })
    ).toBe(false);
    expect(calls).toHaveLength(0);

    expect(
      prefetcher.maybePrefetch({
        release: rel,
        episodeIndex: 0,
        currentTime: 800,
        duration: 1000,
        malId: 52299
      })
    ).toBe(true);

    // Repeat call during the same episode is deduplicated.
    expect(
      prefetcher.maybePrefetch({
        release: rel,
        episodeIndex: 0,
        currentTime: 920,
        duration: 1000,
        malId: 52299
      })
    ).toBe(false);

    await vi.waitFor(() => {
      expect(calls).toContain('https://cdn.example.com/solo/ep2/master.m3u8');
      expect(calls).toContain('https://cdn.example.com/solo/ep2/1080.m3u8');
      expect(calls).toContain('https://cdn.example.com/solo/ep2/seg0.ts');
    });
    expect(aniSkipMock).toHaveBeenCalledTimes(1);
    expect(aniSkipMock.mock.calls[0][0]).toBe(52299);
    expect(aniSkipMock.mock.calls[0][1]).toBe(2);
  });

  it('skips torrent releases and dual-prefetches active Mix RU dub track', async () => {
    const calls: string[] = [];
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      calls.push(url);
      return new Response('#EXTM3U\n#EXTINF:4,\nfirst.ts');
    }) as unknown as typeof fetch;

    const prefetcher = new NextEpisodePrefetcher({ fetchFn: fetchMock });

    const torrentRel = makeRelease('tor', 'torrent');
    expect(
      prefetcher.maybePrefetch({
        release: torrentRel,
        episodeIndex: 0,
        currentTime: 900,
        duration: 1000
      })
    ).toBe(false);

    const subRel = makeRelease('sub');
    const dubRel = makeRelease('dub');
    expect(
      prefetcher.maybePrefetch({
        release: subRel,
        episodeIndex: 0,
        currentTime: 850,
        duration: 1000,
        dubRelease: dubRel,
        dubVoiceId: 'anilibria-dub'
      })
    ).toBe(true);

    await vi.waitFor(() => {
      expect(calls).toContain('https://cdn.example.com/sub/ep2/master.m3u8');
      expect(calls).toContain('https://cdn.example.com/dub/ep2/master.m3u8');
      expect(calls).toContain('https://cdn.example.com/dub/ep2/first.ts');
    });
  });

  it('aborts in-flight prefetch requests immediately on reset()', () => {
    const abortedStates: boolean[] = [];
    const fetchMock = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      return new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => {
          abortedStates.push(Boolean(init.signal?.aborted));
          reject(new DOMException('Aborted', 'AbortError'));
        });
      });
    }) as unknown as typeof fetch;

    const prefetcher = new NextEpisodePrefetcher({ fetchFn: fetchMock });
    const rel = makeRelease('abort-test');
    expect(
      prefetcher.maybePrefetch({
        release: rel,
        episodeIndex: 0,
        currentTime: 820,
        duration: 1000
      })
    ).toBe(true);
    expect(prefetcher.active).toBe(true);

    prefetcher.reset();
    expect(prefetcher.active).toBe(false);
    expect(abortedStates).toEqual([true]);
  });
});
