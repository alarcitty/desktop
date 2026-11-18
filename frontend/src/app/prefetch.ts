import { episodeSources, type PlayableEpisode, type PlayableRelease } from '../api/anime';
import { normalizeMp4Sources, normalizeQualitySources } from '../marlakit/player/hlsConfig';

export const PREFETCH_PROGRESS_THRESHOLD = 0.8;

export interface PrefetchContext {
  release: PlayableRelease | null;
  episodeIndex: number;
  currentTime: number;
  duration: number;
  dubRelease?: PlayableRelease | null;
  dubVoiceId?: string;
  malId?: number;
}

export interface PrefetchDependencies {
  fetchFn?: typeof fetch;
  onAniSkipPrefetch?: (malId: number, episodeOrdinal: number, signal: AbortSignal) => Promise<void> | void;
}

export function resolveEpisodePrefetchUrl(ep?: PlayableEpisode | null): string | null {
  if (!ep || ep.remux) return null;
  const sources = episodeSources(ep);
  const hls = normalizeQualitySources(sources);
  if (hls.length > 0 && hls[0].url) {
    return hls[0].url;
  }
  const mp4 = normalizeMp4Sources(sources);
  if (mp4.length > 0 && mp4[0].url) {
    return mp4[0].url;
  }
  return null;
}

function resolvePlaylistUrl(rawLine: string, baseUrl: string): string | null {
  try {
    return new URL(rawLine, baseUrl).toString();
  } catch {
    return null;
  }
}

/**
 * Extracts either the highest-quality child variant `.m3u8` URL (master playlist) or the first media segment URL.
 */
export function parseM3U8NextTarget(
  playlistText: string,
  baseUrl: string
): { kind: 'variant' | 'segment'; url: string } | null {
  if (!playlistText) return null;
  const lines = playlistText.split(/\r?\n/);

  let isMaster = false;
  let bestVariantBandwidth = -1;
  let bestVariantUrl: string | null = null;
  let pendingBandwidth = -1;
  let initSegmentUrl: string | null = null;

  for (const raw of lines) {
    const line = raw.trim();
    if (!line) continue;

    if (line.startsWith('#')) {
      if (line.startsWith('#EXT-X-STREAM-INF')) {
        isMaster = true;
        const bwMatch = line.match(/BANDWIDTH=(\d+)/i);
        pendingBandwidth = bwMatch ? Number(bwMatch[1]) : 0;
      } else if (!initSegmentUrl && line.startsWith('#EXT-X-MAP:')) {
        const uriMatch = line.match(/URI="([^"]+)"/i);
        if (uriMatch?.[1]) {
          initSegmentUrl = resolvePlaylistUrl(uriMatch[1], baseUrl);
        }
      }
      continue;
    }

    const resolved = resolvePlaylistUrl(line, baseUrl);
    if (!resolved) continue;

    if (isMaster || /\.m3u8($|[?#])/i.test(resolved)) {
      const score = pendingBandwidth >= 0 ? pendingBandwidth : 0;
      if (score >= bestVariantBandwidth) {
        bestVariantBandwidth = score;
        bestVariantUrl = resolved;
      }
      pendingBandwidth = -1;
      continue;
    }

    return { kind: 'segment', url: initSegmentUrl || resolved };
  }

  if (bestVariantUrl) {
    return { kind: 'variant', url: bestVariantUrl };
  }
  if (initSegmentUrl) {
    return { kind: 'segment', url: initSegmentUrl };
  }
  return null;
}

async function warmStreamUrl(
  rawUrl: string,
  signal: AbortSignal,
  fetchFn: typeof fetch
): Promise<void> {
  if (!rawUrl || signal.aborted) return;

  const isHls = /\.m3u8($|[?#])/i.test(rawUrl) || rawUrl.includes('master.m3u8');
  if (!isHls) {
    // For direct MP4 streams, warm connection + initial bytes only without pulling full media.
    await fetchFn(rawUrl, {
      method: 'GET',
      headers: { Range: 'bytes=0-262143' },
      signal
    }).catch(() => {});
    return;
  }

  let manifestUrl = rawUrl;
  for (let depth = 0; depth < 2; depth++) {
    if (signal.aborted) return;
    const resp = await fetchFn(manifestUrl, { method: 'GET', signal });
    if (!resp.ok || signal.aborted) return;
    const text = await resp.text();
    if (signal.aborted) return;
    const parsed = parseM3U8NextTarget(text, manifestUrl);
    if (!parsed) return;
    if (parsed.kind === 'variant') {
      manifestUrl = parsed.url;
      continue;
    }
    // Warm first media segment (small range/prefix request).
    await fetchFn(parsed.url, {
      method: 'GET',
      headers: { Range: 'bytes=0-524287' },
      signal
    }).catch(() => {});
    return;
  }
}

export class NextEpisodePrefetcher {
  private controller: AbortController | null = null;
  private triggeredKey = '';

  constructor(private readonly deps: PrefetchDependencies = {}) {}

  private getFetchFn(): typeof fetch | null {
    if (this.deps.fetchFn) return this.deps.fetchFn;
    if (typeof fetch === 'function') return fetch;
    return null;
  }

  cancel(): void {
    if (this.controller) {
      this.controller.abort();
      this.controller = null;
    }
  }

  reset(): void {
    this.cancel();
    this.triggeredKey = '';
  }

  get active(): boolean {
    return Boolean(this.controller && !this.controller.signal.aborted);
  }

  maybePrefetch(ctx: PrefetchContext): boolean {
    const { release, episodeIndex, currentTime, duration, dubRelease, dubVoiceId = '', malId = 0 } = ctx;
    if (!release || release.provider === 'torrent') return false;
    if (!Number.isFinite(duration) || duration <= 0) return false;
    if (!Number.isFinite(currentTime) || currentTime < 0) return false;
    if (currentTime / duration < PREFETCH_PROGRESS_THRESHOLD) return false;

    const currentEp = release.episodes[episodeIndex];
    const nextEp = release.episodes[episodeIndex + 1];
    if (!currentEp || !nextEp || currentEp.remux || nextEp.remux) return false;

    const key = `${release.alias}:${nextEp.ordinal}:${dubVoiceId}`;
    if (this.triggeredKey === key) return false;

    const primaryUrl = resolveEpisodePrefetchUrl(nextEp);
    const dubNextEp =
      dubRelease && dubRelease.provider !== 'torrent'
        ? dubRelease.episodes.find((ep) => ep.ordinal === nextEp.ordinal) ?? null
        : null;
    const dubUrl = resolveEpisodePrefetchUrl(dubNextEp);

    if (!primaryUrl && !dubUrl && !(malId > 0 && !nextEp.opening && !nextEp.ending)) {
      return false;
    }

    this.cancel();
    this.triggeredKey = key;
    const controller = new AbortController();
    this.controller = controller;
    const fetchFn = this.getFetchFn();

    if (fetchFn) {
      if (primaryUrl) {
        void warmStreamUrl(primaryUrl, controller.signal, fetchFn).catch(() => {});
      }
      if (dubUrl) {
        void warmStreamUrl(dubUrl, controller.signal, fetchFn).catch(() => {});
      }
    }

    if (malId > 0 && !nextEp.opening && !nextEp.ending && this.deps.onAniSkipPrefetch) {
      void Promise.resolve(
        this.deps.onAniSkipPrefetch(malId, nextEp.ordinal, controller.signal)
      ).catch(() => {});
    }

    return true;
  }
}
