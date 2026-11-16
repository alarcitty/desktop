import { afterEach, describe, expect, it, vi } from 'vitest';
import { findVoiceOptions, resolvePlayableRelease } from '../src/api/anime';
import { buildHentaiQueries, findAllHentaiTracks, findHentaiMatch } from '../src/api/providers/hentai';

describe('hentai provider matching and fallback playback', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('builds prioritized Romaji/ASCII queries with suffix stripping', () => {
    const queries = buildHentaiQueries([
      'Девушка только для меня',
      'Boku dake no Hentai Kanojo The Animation'
    ]);
    expect(queries).toEqual([
      'Boku dake no Hentai Kanojo The Animation',
      'Boku dake no Hentai Kanojo',
      'Девушка только для меня'
    ]);
  });

  it('ranks HentaiMama 1080p HLS primary while keeping HentaiGasm Uncensored & Subbed as voice options', async () => {
    vi.stubGlobal('window', globalThis);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const u = String(input);
        if (u.includes('/ext/hentai/search')) {
          return new Response(
            JSON.stringify([
              {
                provider: 'hentaigasm',
                target: 'https://hentaigasm.com/ep-1/|https://hentaigasm.com/ep-2/',
                title: 'Boku Dake No Hentai Kanojo The Animation',
                variant: 'Uncensored',
                episodeCount: 2
              },
              {
                provider: 'hentaimama',
                target: 'https://hentaimama.io/tvshows/boku-dake-no-hentai-kanojo-the-animation/',
                title: 'Boku Dake no Hentai Kanojo The Animation',
                year: 2017,
                variant: 'Subbed'
              }
            ]),
            { status: 200 }
          );
        }
        if (u.includes('/ext/hentai/episodes') && u.includes('provider=hentaimama')) {
          return new Response(
            JSON.stringify([
              {
                ordinal: 1,
                name: 'Episode 1',
                duration: 1056,
                hls_1080: '/ext/stream?ref=https%3A%2F%2Fhentaimama.io%2F&url=https%3A%2F%2Fhentaidoge.org%2Fs%2Fmaster.m3u8',
                mp4_1080: '/ext/stream?ref=https%3A%2F%2Fhentaimama.io%2F&url=https%3A%2F%2Fgdvid.info%2F1080p.mp4'
              }
            ]),
            { status: 200 }
          );
        }
        return new Response(JSON.stringify({ data: [] }), { status: 404 });
      })
    );

    const match = await findHentaiMatch(['Boku dake no Hentai Kanojo The Animation'], 2017);
    expect(match).not.toBeNull();
    expect(match?.provider).toBe('hentaimama');
    expect(match?.episodes).toHaveLength(1);
    expect(match?.episodes[0].hls_1080).toContain('/ext/stream?');

    const tracks = await findAllHentaiTracks(['Boku dake no Hentai Kanojo The Animation'], 2017);
    expect(tracks.map((t) => t.voice.label)).toEqual(['HentaiMama', 'HentaiGasm · Uncensored']);

    const release = await resolvePlayableRelease({
      title: 'Boku dake no Hentai Kanojo The Animation',
      searchQueries: ['Boku dake no Hentai Kanojo The Animation']
    });
    expect(release.provider).toBe('hentaimama');
    expect(release.episodes[0].ordinal).toBe(1);

    const voices = await findVoiceOptions(release);
    expect(voices.map((v) => v.label)).toContain('HentaiGasm · Uncensored');
  });
});
