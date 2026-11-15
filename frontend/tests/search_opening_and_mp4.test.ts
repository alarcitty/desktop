import { afterEach, describe, expect, it, vi } from 'vitest';
import { clampOpeningClipWindow, resolveHeroOpeningPreview } from '../src/api/anime';

describe('search opening preview & clip windowing', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('clamps opening window cleanly for short OVAs and keeps finite ranges intact', () => {
    // Unknown or non-finite duration keeps original window
    expect(clampOpeningClipWindow(96, 138, NaN)).toEqual({ startSec: 96, endSec: 138 });

    // Standard 24-minute TV episode leaves opening timing unchanged
    expect(clampOpeningClipWindow(84, 174, 1425)).toEqual({ startSec: 84, endSec: 174 });

    // Short 90s OVA / trailer clamps startSec before EOF instead of stalling at >= duration
    const shortClip = clampOpeningClipWindow(96, 138, 90);
    expect(shortClip.startSec).toBeGreaterThanOrEqual(0);
    expect(shortClip.startSec).toBeLessThan(86);
    expect(shortClip.endSec).toBeGreaterThan(shortClip.startSec);
    expect(shortClip.endSec).toBeLessThanOrEqual(89.75);
  });

  it('resolves direct MP4 fallback releases as isDirectVideo opening previews', async () => {
    vi.stubGlobal('window', globalThis);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('/ext/animevost/search')) {
          return new Response(
            JSON.stringify({
              data: [
                {
                  id: 777,
                  title: 'Test MP4 Release / Тест',
                  year: '2024',
                  series: '{"1 серия":"101"}'
                }
              ]
            }),
            { status: 200 }
          );
        }
        if (url.includes('/ext/animevost/playlist')) {
          return new Response(
            JSON.stringify([
              { name: '1 серия', hd: 'https://cdn.example.com/ep1_720.mp4', std: 'https://cdn.example.com/ep1_480.mp4' },
              { name: '2 серия', hd: 'https://cdn.example.com/ep2_720.mp4', std: 'https://cdn.example.com/ep2_480.mp4' }
            ]),
            { status: 200 }
          );
        }
        // Other providers return empty / 404
        return new Response(JSON.stringify({ data: [] }), { status: 404 });
      })
    );

    const clip = await resolveHeroOpeningPreview({
      title: 'Test MP4 Release',
      searchQueries: ['Test MP4 Release']
    });

    expect(clip).not.toBeNull();
    expect(clip?.isDirectVideo).toBe(true);
    expect(clip?.episodeOrdinal).toBe(2);
    expect(clip?.streamUrl).toBe('https://cdn.example.com/ep2_720.mp4');
    expect(clip?.streamUrlFast).toBe('https://cdn.example.com/ep2_480.mp4');
  });
});
