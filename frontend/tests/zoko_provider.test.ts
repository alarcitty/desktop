import { afterEach, describe, expect, it, vi } from 'vitest';
import { findVoiceOptions, resolvePlayableRelease, resolveVoiceTrackRelease } from '../src/api/anime';
import { buildZokoMasterUrl, findZokoMatch, pickAsciiQuery } from '../src/api/providers/zoko';

describe('zoko english dub provider', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('picks ASCII/Romaji query and builds proxied master.m3u8 URLs', () => {
    expect(pickAsciiQuery(['Провожающая в последний путь Фрирен', 'Sousou no Frieren'])).toBe(
      'Sousou no Frieren'
    );
    expect(buildZokoMasterUrl(52991, 3, 'dub')).toBe('/ext/zoko/master.m3u8?mal=52991&ep=3&mode=dub');
  });

  it('resolves English Dub 1080p HLS release, voice options, and track switching', async () => {
    vi.stubGlobal('window', globalThis);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const u = String(input);
        if (u.includes('/ext/zoko/probe')) {
          return new Response(
            JSON.stringify({
              malId: 52991,
              hasDub: true,
              hasSub: true,
              episodes: [1, 2, 3]
            }),
            { status: 200 }
          );
        }
        return new Response(JSON.stringify({ data: [] }), { status: 404 });
      })
    );

    const match = await findZokoMatch(['Sousou no Frieren'], 52991, 3);
    expect(match).not.toBeNull();
    expect(match?.dubs.map((d) => d.voice.label)).toEqual(['English Dub', 'HiAnime · Eng Sub']);
    expect(match?.dubs[0].episodes[0].hls_1080).toBe('/ext/zoko/master.m3u8?mal=52991&ep=1&mode=dub');

    const release = await resolvePlayableRelease({
      shikiId: 52991,
      title: 'Sousou no Frieren',
      searchQueries: ['Sousou no Frieren']
    });
    expect(release.provider).toBe('zoko');
    expect(release.voiceLabel).toBe('English Dub');
    expect(release.episodes).toHaveLength(3);

    const voices = await findVoiceOptions(release, 52991);
    const subVoice = voices.find((v) => v.sourceId === 'sub');
    expect(subVoice).toBeDefined();

    const switched = await resolveVoiceTrackRelease(subVoice!, release, {
      shikiId: 52991,
      title: 'Sousou no Frieren'
    });
    expect(switched.provider).toBe('zoko');
    expect(switched.episodes[0].hls_1080).toBe('/ext/zoko/master.m3u8?mal=52991&ep=1&mode=sub');
  });
});
