import { callUnifiedContract } from './bridge';
import type { PlayableEpisode } from './anime';
import type { SkipRange } from '../marlakit/player/types';

export interface AniSkipResponse {
  mal_id: number;
  episode: number;
  found: boolean;
  opening?: SkipRange | null;
  ending?: SkipRange | null;
}

const MAX_ANISKIP_CACHE = 512;
const aniSkipMemoryCache = new Map<string, AniSkipResponse>();
const aniSkipPending = new Map<string, Promise<AniSkipResponse | null>>();

function cacheKey(malId: number, episode: number): string {
  return `${malId}:${episode}`;
}

function setBounded(key: string, val: AniSkipResponse): void {
  if (aniSkipMemoryCache.size >= MAX_ANISKIP_CACHE && !aniSkipMemoryCache.has(key)) {
    const oldest = aniSkipMemoryCache.keys().next().value;
    if (oldest !== undefined) {
      aniSkipMemoryCache.delete(oldest);
    }
  }
  aniSkipMemoryCache.set(key, val);
}

export function clearAniSkipCacheForTest(): void {
  aniSkipMemoryCache.clear();
  aniSkipPending.clear();
}

export function hasSkipInterval(range?: SkipRange | null): boolean {
  if (!range) return false;
  const start = Number(range.start ?? 0);
  const stop = Number(range.stop ?? 0);
  return Number.isFinite(start) && Number.isFinite(stop) && stop > start + 1;
}

export function mergeEpisodeSkipFallback(
  ep: PlayableEpisode,
  skip?: AniSkipResponse | null
): PlayableEpisode {
  if (!skip || !skip.found) return ep;
  const nextOpening = hasSkipInterval(ep.opening)
    ? ep.opening
    : hasSkipInterval(skip.opening)
      ? skip.opening
      : ep.opening;
  const nextEnding = hasSkipInterval(ep.ending)
    ? ep.ending
    : hasSkipInterval(skip.ending)
      ? skip.ending
      : ep.ending;

  if (nextOpening === ep.opening && nextEnding === ep.ending) {
    return ep;
  }
  return {
    ...ep,
    opening: nextOpening,
    ending: nextEnding
  };
}

function isAniSkipPayload(value: unknown): value is AniSkipResponse {
  return Boolean(
    value &&
      typeof value === 'object' &&
      typeof (value as AniSkipResponse).found === 'boolean'
  );
}

/**
 * Fetches OP/ED skip ranges from Go AniSkip fallback service (Wails IPC or `/ext/aniskip/{malId}/{ep}`).
 */
export async function fetchAniSkipTimes(
  malId: number,
  episode: number,
  signal?: AbortSignal
): Promise<AniSkipResponse | null> {
  if (!Number.isFinite(malId) || malId <= 0 || !Number.isFinite(episode) || episode <= 0) {
    return null;
  }
  if (signal?.aborted) return null;

  const cleanMal = Math.trunc(malId);
  const cleanEp = Math.trunc(episode);
  const key = cacheKey(cleanMal, cleanEp);

  const cached = aniSkipMemoryCache.get(key);
  if (cached) return cached;

  const existing = aniSkipPending.get(key);
  if (existing) {
    return existing;
  }

  const req = callUnifiedContract<AniSkipResponse>({
    wailsCall: (bridge) =>
      bridge.GetAniSkipTimes
        ? bridge.GetAniSkipTimes(cleanMal, cleanEp)
        : Promise.reject(new Error('missing GetAniSkipTimes')),
    httpPath: `/ext/aniskip/${cleanMal}/${cleanEp}`,
    httpInit: signal ? { signal } : undefined,
    timeoutMs: 6000,
    validate: isAniSkipPayload
  })
    .then((res) => {
      if (signal?.aborted) return null;
      setBounded(key, res);
      return res;
    })
    .catch(() => null)
    .finally(() => {
      if (aniSkipPending.get(key) === req) {
        aniSkipPending.delete(key);
      }
    });

  aniSkipPending.set(key, req);
  return req;
}
