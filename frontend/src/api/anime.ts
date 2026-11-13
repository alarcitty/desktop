import type { HlsSources, SkipRange } from '../marlakit/player/types';

const BACKEND_BASE = 'http://localhost:8080/api/v1';
const SHIKI_GRAPHQL = 'https://shikimori.io/api/graphql';
const ANILIBERTY_BASE = 'https://aniliberty.top/api/v1';

export interface ShikiArtwork {
  id: number;
  name: string;
  russian: string;
  english: string;
  description: string;
  score: number;
  posterOriginal: string;
  posterMain: string;
  screenshots: string[];
}

export interface PlayableEpisode {
  id: string;
  ordinal: number;
  name?: string;
  duration: number;
  preview?: string;
  opening?: SkipRange | null;
  ending?: SkipRange | null;
  hls_1080?: string | null;
  hls_720?: string | null;
  hls_480?: string | null;
}

export interface PlayableRelease {
  id: number;
  alias: string;
  title: string;
  titleEnglish?: string;
  poster?: string;
  banner?: string;
  episodes: PlayableEpisode[];
  fallbackNote?: string;
}

// Track known titles used across Home, Releases, Recommendations, and Library.
export const FEATURED_SHIKI_IDS = [
  54492, // The Apothecary Diaries
  39535, // Mushoku Tensei
  34572, // Black Clover
  52991, // Frieren: Beyond Journey's End
  43523, // Tsukimichi: Moonlit Fantasy
  54744, // Alya Sometimes Hides Her Feelings in Russian
  52198, // Kaguya-sama: Love Is War - First Kiss
  37999, // Kaguya-sama S1
  57334, // Dan Da Dan
  48736, // My Dress-Up Darling
  52299  // Solo Leveling
] as const;

const shikiCache = new Map<number, ShikiArtwork>();
let batchPromise: Promise<void> | null = null;
let backendAvailable: boolean | null = null;

function cleanBBCode(raw: string | null | undefined): string {
  if (!raw) return '';
  return raw
    .replace(/\r\n/g, '\n')
    .replace(/\[br\]/g, '\n')
    .replace(/\[\/?[a-zA-Z0-9_*]+(?:=[^\]]*)?\]/g, '')
    .replace(/[ \t]{2,}/g, ' ')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

export function truncateDescription(raw: string | null | undefined, maxChars: number): string {
  const normalized = (raw ?? '').replace(/\s+/g, ' ').trim();
  if (!normalized || normalized.length <= maxChars) return normalized;
  const slice = normalized.slice(0, maxChars);
  const lastSpace = slice.lastIndexOf(' ');
  const cut = lastSpace > maxChars * 0.6 ? slice.slice(0, lastSpace) : slice;
  return `${cut.replace(/[.,;:!?—-]+$/, '').trimEnd()}...`;
}

function resolveShikiUrl(raw?: string | null): string {
  if (!raw) return '';
  if (raw.startsWith('http://') || raw.startsWith('https://')) return raw;
  if (raw.startsWith('//')) return `https:${raw}`;
  if (raw.startsWith('/')) return `https://shikimori.io${raw}`;
  return raw;
}

function resolveLibUrl(raw?: string | null): string {
  if (!raw) return '';
  if (raw.startsWith('http://') || raw.startsWith('https://')) return raw;
  if (raw.startsWith('/')) return `https://aniliberty.top${raw}`;
  return raw;
}

async function fetchJsonWithTimeout<T>(url: string, init?: RequestInit, timeoutMs = 4500): Promise<T> {
  const ctrl = new AbortController();
  const timer = window.setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const res = await fetch(url, { ...init, signal: ctrl.signal });
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`);
    }
    return (await res.json()) as T;
  } finally {
    window.clearTimeout(timer);
  }
}

async function isBackendUp(): Promise<boolean> {
  if (backendAvailable !== null) return backendAvailable;
  try {
    await fetchJsonWithTimeout('http://localhost:8080/health', undefined, 700);
    backendAvailable = true;
  } catch {
    backendAvailable = false;
  }
  // Re-probe periodically so starting the Go backend later picks up automatically.
  window.setTimeout(() => {
    backendAvailable = null;
  }, 15_000);
  return backendAvailable;
}

interface ShikiGqlNode {
  id: string;
  name: string;
  russian?: string;
  english?: string;
  description?: string;
  score?: number;
  poster?: {
    originalUrl?: string;
    main2xUrl?: string;
    mainUrl?: string;
  };
  screenshots?: Array<{ originalUrl?: string }>;
}

// Fetches Shikimori posters, screenshots (banners), and cleaned descriptions in a single batch call.
export async function ensureShikimoriArtwork(ids: readonly number[] = FEATURED_SHIKI_IDS): Promise<Map<number, ShikiArtwork>> {
  const missing = ids.filter((id) => id > 0 && !shikiCache.has(id));
  if (missing.length === 0) {
    return shikiCache;
  }

  if (!batchPromise) {
    batchPromise = (async () => {
      try {
        const query = `query($ids: String, $limit: PositiveInt) {
          animes(ids: $ids, limit: $limit, censored: false) {
            id
            name
            russian
            english
            description
            score
            poster { originalUrl main2xUrl mainUrl }
            screenshots { originalUrl }
          }
        }`;
        const unique = Array.from(new Set([...FEATURED_SHIKI_IDS, ...missing]));
        const body = JSON.stringify({
          query,
          variables: { ids: unique.join(','), limit: unique.length }
        });
        const payload = await fetchJsonWithTimeout<{ data?: { animes?: ShikiGqlNode[] } }>(
          SHIKI_GRAPHQL,
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body
          },
          8000
        );
        for (const item of payload.data?.animes ?? []) {
          const numId = Number(item.id);
          if (!Number.isFinite(numId) || numId <= 0) continue;
          shikiCache.set(numId, {
            id: numId,
            name: item.name,
            russian: item.russian ?? '',
            english: item.english ?? '',
            description: cleanBBCode(item.description),
            score: item.score ?? 0,
            posterOriginal: resolveShikiUrl(item.poster?.originalUrl),
            posterMain: resolveShikiUrl(item.poster?.main2xUrl || item.poster?.mainUrl || item.poster?.originalUrl),
            screenshots: (item.screenshots ?? [])
              .map((s) => resolveShikiUrl(s.originalUrl))
              .filter(Boolean)
          });
        }
      } finally {
        batchPromise = null;
      }
    })();
  }

  await batchPromise;
  return shikiCache;
}

export function getCachedArtwork(id: number): ShikiArtwork | undefined {
  return shikiCache.get(id);
}

interface RawLibRelease {
  id: number;
  alias: string;
  name?: { main?: string; english?: string };
  poster?: { src?: string; preview?: string };
  episodes?: Array<{
    id: string;
    ordinal: number;
    name?: string | null;
    name_english?: string | null;
    duration?: number;
    preview?: { src?: string; preview?: string; optimized?: { src?: string } };
    opening?: SkipRange | null;
    ending?: SkipRange | null;
    hls_1080?: string | null;
    hls_720?: string | null;
    hls_480?: string | null;
  }>;
}

function mapLibRelease(r: RawLibRelease, shikiArt?: ShikiArtwork): PlayableRelease {
  const episodes: PlayableEpisode[] = (r.episodes ?? [])
    .filter((ep) => Boolean(ep.hls_1080 || ep.hls_720 || ep.hls_480))
    .map((ep) => ({
      id: ep.id,
      ordinal: ep.ordinal,
      name: ep.name || ep.name_english || undefined,
      duration: ep.duration ?? 0,
      preview: resolveLibUrl(ep.preview?.optimized?.src || ep.preview?.src || ep.preview?.preview),
      opening: ep.opening,
      ending: ep.ending,
      hls_1080: ep.hls_1080,
      hls_720: ep.hls_720,
      hls_480: ep.hls_480
    }));

  return {
    id: r.id,
    alias: r.alias,
    title: r.name?.main || shikiArt?.english || shikiArt?.name || 'Anime',
    titleEnglish: r.name?.english || shikiArt?.english,
    poster: shikiArt?.posterOriginal || resolveLibUrl(r.poster?.src || r.poster?.preview),
    banner: shikiArt?.screenshots[0] || shikiArt?.posterOriginal || resolveLibUrl(r.poster?.src),
    episodes
  };
}

interface BackendDetailEnvelope {
  data?: {
    id: number;
    alias: string;
    title: string;
    title_english?: string;
    poster?: { original?: string; main?: string };
    banner?: string;
    episodes?: PlayableEpisode[];
  };
}

async function fetchReleaseByIdOrAlias(idOrAlias: string, shikiArt?: ShikiArtwork): Promise<PlayableRelease | null> {
  if (await isBackendUp()) {
    try {
      const env = await fetchJsonWithTimeout<BackendDetailEnvelope>(
        `${BACKEND_BASE}/anime/releases/${encodeURIComponent(idOrAlias)}`,
        undefined,
        7000
      );
      if (env.data && (env.data.episodes?.length ?? 0) > 0) {
        return {
          id: env.data.id,
          alias: env.data.alias,
          title: env.data.title,
          titleEnglish: env.data.title_english,
          poster: env.data.poster?.original || env.data.poster?.main || shikiArt?.posterOriginal,
          banner: env.data.banner || shikiArt?.screenshots[0],
          episodes: env.data.episodes ?? []
        };
      }
    } catch {
      // Fall through to direct AniLiberty request
    }
  }

  try {
    const raw = await fetchJsonWithTimeout<RawLibRelease>(
      `${ANILIBERTY_BASE}/anime/releases/${encodeURIComponent(idOrAlias)}`,
      undefined,
      7000
    );
    const mapped = mapLibRelease(raw, shikiArt);
    return mapped.episodes.length > 0 ? mapped : null;
  } catch {
    return null;
  }
}

export interface WatchTarget {
  title: string;
  releaseAlias?: string;
  shikiId?: number;
  searchQueries?: string[];
}

// Resolves a playable HLS release (prefers Go backend when running, falls back to AniLiberty + Shikimori).
export async function resolvePlayableRelease(target: WatchTarget): Promise<PlayableRelease> {
  await ensureShikimoriArtwork(target.shikiId ? [target.shikiId] : FEATURED_SHIKI_IDS).catch(() => {});
  const shikiArt = target.shikiId ? shikiCache.get(target.shikiId) : undefined;

  if (target.releaseAlias) {
    const direct = await fetchReleaseByIdOrAlias(target.releaseAlias, shikiArt);
    if (direct) return direct;
  }

  const queries = target.searchQueries ?? [target.title];
  for (const q of queries) {
    const trimmed = q.trim();
    if (!trimmed) continue;
    try {
      const found = await fetchJsonWithTimeout<Array<{ id: number; alias: string }>>(
        `${ANILIBERTY_BASE}/app/search/releases?query=${encodeURIComponent(trimmed)}`,
        undefined,
        6000
      );
      for (const candidate of found.slice(0, 3)) {
        const rel = await fetchReleaseByIdOrAlias(String(candidate.id), shikiArt);
        if (rel) return rel;
      }
    } catch {
      // Try next query
    }
  }

  // Fallback to a live playable release on AniLiberty so playback always works for titles unavailable there.
  const latest = await fetchJsonWithTimeout<Array<{ id: number; alias: string }>>(
    `${ANILIBERTY_BASE}/anime/releases/latest?limit=5`,
    undefined,
    6000
  );
  for (const item of latest) {
    const rel = await fetchReleaseByIdOrAlias(String(item.id), shikiArt);
    if (rel) {
      return {
        ...rel,
        fallbackNote: `«${target.title}» недоступно на AniLiberty — показан актуальный релиз: ${rel.title}`
      };
    }
  }

  throw new Error('No playable HLS streams found');
}

export function episodeSources(ep: PlayableEpisode): HlsSources {
  return {
    hls_1080: ep.hls_1080,
    hls_720: ep.hls_720,
    hls_480: ep.hls_480
  };
}
