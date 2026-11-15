import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  buildTorrentMediaUrl,
  buildTorrentSubtitleUrl,
  ensureTorrentMediaBaseUrl,
  openTorrentMagnet,
  setTorrentMediaBaseUrl
} from '../src/api/providers/torrent';

describe('torrent loopback streaming URL builder', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('uses loopback mediaBaseUrl when configured and forwards codec/transcode flags', () => {
    setTorrentMediaBaseUrl('https://evil.example.com');
    // Non-loopback URLs must be rejected.
    expect(buildTorrentMediaUrl('deadbeef', 0, true, 1, 15.5)).toMatch(/^\/media\/stream\.mp4\?/);

    setTorrentMediaBaseUrl('http://127.0.0.1:51234/');
    const remuxUrl = buildTorrentMediaUrl('deadbeef', 2, true, 1, 42.5, {
      videoCodec: 'hevc',
      transcode: true
    });
    expect(remuxUrl.startsWith('http://127.0.0.1:51234/media/stream.mp4?')).toBe(true);
    expect(remuxUrl).toContain('vcodec=hevc');
    expect(remuxUrl).toContain('transcode=1');
    expect(remuxUrl).toContain('t=42.50');

    const directMp4Url = buildTorrentMediaUrl('deadbeef', 0, false);
    expect(directMp4Url).toBe('http://127.0.0.1:51234/torrent/stream/deadbeef/0');

    const subUrl = buildTorrentSubtitleUrl('deadbeef', 2, 0);
    expect(subUrl).toBe('http://127.0.0.1:51234/media/subtitles.vtt?hash=deadbeef&file=2&s=0');
  });

  it('captures mediaBaseUrl from Wails TorrentOpen and GetTorrentMediaBase', async () => {
    vi.stubGlobal('window', {
      go: {
        mila: {
          Mila: {
            TorrentOpen: vi.fn().mockResolvedValue(
              JSON.stringify({
                infoHash: 'abcd1234',
                name: 'Test Release',
                mediaBaseUrl: 'http://127.0.0.1:61000',
                files: []
              })
            ),
            GetTorrentMediaBase: vi.fn().mockResolvedValue('http://127.0.0.1:61000')
          }
        }
      }
    });

    const info = await openTorrentMagnet('magnet:?xt=urn:btih:abcd1234');
    expect(info.mediaBaseUrl).toBe('http://127.0.0.1:61000');
    const base = await ensureTorrentMediaBaseUrl();
    expect(base).toBe('http://127.0.0.1:61000');
  });
});
