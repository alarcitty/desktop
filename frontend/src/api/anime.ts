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
  year?: number;
  status?: string;
  episodes?: number;
  episodesAired?: number;
  genres: string[];
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

export interface HomeSpotlightItem {
  shikiId: number;
  title: string;
  releaseAlias?: string;
  searchQueries: string[];
  bg: string;
  previews: string[];
  poster: string;
  score: number;
  metaTags: string[];
  description: string;
}

export interface HomeReleaseItem {
  id: string;
  shikiId?: number;
  releaseAlias: string;
  searchQueries: string[];
  title: string;
  titleLines: string[];
  description: string;
  poster: string;
  score?: number;
  tags: string[];
}

export interface HomeRecommendationItem {
  id: string;
  shikiId?: number;
  releaseAlias?: string;
  searchQueries: string[];
  title: string;
  poster: string;
  score?: number;
  tags: string[];
}

export interface HomeFeeds {
  spotlight: HomeSpotlightItem[];
  releases: HomeReleaseItem[];
  recommendations: HomeRecommendationItem[];
}

const shikiCache = new Map<number, ShikiArtwork>();
let batchPromise: Promise<void> | null = null;
let backendAvailable: boolean | null = null;
let homeFeedsCache: HomeFeeds | null = null;
let homeFeedsPromise: Promise<HomeFeeds> | null = null;

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

export function buildMetaTags(art?: {
  year?: number;
  episodes?: number;
  episodesAired?: number;
  genres?: readonly string[];
}): string[] {
  if (!art) return [];
  const tags: string[] = [];
  if (art.year && art.year > 1990) tags.push(String(art.year));
  const ep = art.episodes || art.episodesAired;
  if (ep && ep > 0) tags.push(`${ep} ep`);
  for (const g of (art.genres ?? []).slice(0, 3)) {
    if (g) tags.push(g);
  }
  return tags;
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
  status?: string;
  episodes?: number;
  episodesAired?: number;
  airedOn?: { year?: number | null } | null;
  genres?: Array<{ name?: string | null }> | null;
  poster?: {
    originalUrl?: string;
    main2xUrl?: string;
    mainUrl?: string;
  };
  screenshots?: Array<{ originalUrl?: string }>;
}

function mapShikiNode(item: ShikiGqlNode): ShikiArtwork | null {
  const numId = Number(item.id);
  if (!Number.isFinite(numId) || numId <= 0) return null;
  return {
    id: numId,
    name: item.name,
    russian: item.russian ?? '',
    english: item.english ?? '',
    description: cleanBBCode(item.description),
    score: item.score ?? 0,
    year: item.airedOn?.year ?? undefined,
    status: item.status ?? undefined,
    episodes: item.episodes ?? undefined,
    episodesAired: item.episodesAired ?? undefined,
    genres: (item.genres ?? [])
      .map((g) => g.name ?? '')
      .filter(Boolean),
    posterOriginal: resolveShikiUrl(item.poster?.originalUrl),
    posterMain: resolveShikiUrl(item.poster?.main2xUrl || item.poster?.mainUrl || item.poster?.originalUrl),
    screenshots: (item.screenshots ?? [])
      .map((s) => resolveShikiUrl(s.originalUrl))
      .filter(Boolean)
  };
}

// Fetches Shikimori posters, screenshots (banners), and cleaned descriptions in a single batch call.
export async function ensureShikimoriArtwork(ids: readonly number[]): Promise<Map<number, ShikiArtwork>> {
  while (true) {
    const missing = ids.filter((id) => id > 0 && !shikiCache.has(id));
    if (missing.length === 0) {
      return shikiCache;
    }

    if (batchPromise) {
      await batchPromise;
      continue;
    }

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
            status
            episodes
            episodesAired
            airedOn { year }
            genres { name }
            poster { originalUrl main2xUrl mainUrl }
            screenshots { originalUrl }
          }
        }`;
        const unique = Array.from(new Set(missing));
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
          const mapped = mapShikiNode(item);
          if (mapped) {
            shikiCache.set(mapped.id, mapped);
          }
        }
      } finally {
        batchPromise = null;
      }
    })();

    await batchPromise;
    return shikiCache;
  }
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
  if (target.shikiId) {
    await ensureShikimoriArtwork([target.shikiId]).catch(() => {});
  }
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

interface RawLibFeedItem {
  id: number;
  alias: string;
  year?: number | null;
  episodes_total?: number | null;
  description?: string | null;
  is_blocked_by_copyrights?: boolean;
  name?: { main?: string | null; english?: string | null } | null;
  shikimori?: { id?: number | null; rating?: number | null } | null;
  mal?: { rating?: number | null } | null;
  poster?: {
    src?: string | null;
    preview?: string | null;
    optimized?: { src?: string | null } | null;
  } | null;
  genres?: Array<{ name?: string | null }> | null;
  latest_episode?: { ordinal?: number | null } | null;
}

function franchiseKey(rawTitle: string): string {
  return rawTitle
    .toLowerCase()
    .replace(/\b(\d+(st|nd|rd|th)\s+season|season\s+\d+|part\s+\d+|cour\s+\d+|arc)\b/gi, '')
    .replace(/\b(ii|iii|iv|v)\b$/gi, '')
    .replace(/[^a-z0-9а-яё]+/gi, ' ')
    .trim();
}

function splitTitleLines(title: string): string[] {
  const words = title.trim().split(/\s+/).filter(Boolean);
  if (words.length <= 1 || title.length <= 13) return [title];

  const lines: string[] = [];
  let current = '';
  for (const word of words) {
    const candidate = current ? `${current} ${word}` : word;
    if (candidate.length <= 16 || current.length === 0) {
      current = candidate;
    } else {
      lines.push(current);
      current = word;
      if (lines.length === 2) {
        const restIndex = words.indexOf(word);
        const remaining = words.slice(restIndex).join(' ');
        lines.push(remaining);
        return lines;
      }
    }
  }
  if (current) lines.push(current);
  return lines.slice(0, 3);
}

function mapShikiToSpotlight(art: ShikiArtwork, releaseAlias?: string): HomeSpotlightItem {
  const shots = art.screenshots;
  const previews =
    shots.length >= 4
      ? shots.slice(1, 4)
      : shots.length >= 3
        ? shots.slice(0, 3)
        : [shots[0] ?? art.posterOriginal, shots[1] ?? shots[0] ?? art.posterOriginal, shots[2] ?? shots[0] ?? art.posterOriginal];

  return {
    shikiId: art.id,
    title: art.english || art.name || art.russian || 'Anime',
    releaseAlias,
    searchQueries: [art.name, art.english, art.russian].filter(Boolean),
    bg: shots[0] ?? art.posterOriginal ?? '',
    previews,
    poster: art.posterOriginal || art.posterMain || shots[0] || '',
    score: art.score > 0 ? Number(art.score.toFixed(1)) : 0,
    metaTags: buildMetaTags(art),
    description: truncateDescription(art.description, 330)
  };
}

export function getCachedHomeFeeds(): HomeFeeds | null {
  return homeFeedsCache;
}

// Loads live Spotlight, latest AniLiberty New Releases, and Recommendations without hardcoded lists.
export async function loadHomeFeeds(): Promise<HomeFeeds> {
  if (homeFeedsCache) {
    return homeFeedsCache;
  }
  if (homeFeedsPromise) {
    return homeFeedsPromise;
  }

  homeFeedsPromise = (async () => {
    const shikiQuery = `{
      ongoing: animes(order: popularity, status: "ongoing", kind: "tv", limit: 18, censored: false) {
        id name russian english description score status episodes episodesAired
        airedOn { year } genres { name }
        poster { originalUrl main2xUrl mainUrl }
        screenshots { originalUrl }
      }
      ranked: animes(order: ranked, kind: "tv", limit: 24, censored: false) {
        id name russian english description score status episodes episodesAired
        airedOn { year } genres { name }
        poster { originalUrl main2xUrl mainUrl }
        screenshots { originalUrl }
      }
    }`;

    const [latestRes, recRes, shikiRes] = await Promise.allSettled([
      fetchJsonWithTimeout<RawLibFeedItem[]>(
        `${ANILIBERTY_BASE}/anime/releases/latest?limit=14`,
        undefined,
        7000
      ),
      fetchJsonWithTimeout<RawLibFeedItem[]>(
        `${ANILIBERTY_BASE}/anime/releases/recommended?limit=12`,
        undefined,
        7000
      ),
      fetchJsonWithTimeout<{ data?: { ongoing?: ShikiGqlNode[]; ranked?: ShikiGqlNode[] } }>(
        SHIKI_GRAPHQL,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query: shikiQuery })
        },
        8500
      )
    ]);

    const latestRaw = latestRes.status === 'fulfilled' && Array.isArray(latestRes.value) ? latestRes.value : [];
    const recRaw = recRes.status === 'fulfilled' && Array.isArray(recRes.value) ? recRes.value : [];
    const ongoingNodes = shikiRes.status === 'fulfilled' ? shikiRes.value.data?.ongoing ?? [] : [];
    const rankedNodes = shikiRes.status === 'fulfilled' ? shikiRes.value.data?.ranked ?? [] : [];

    const ongoingArts: ShikiArtwork[] = [];
    for (const node of ongoingNodes) {
      const mapped = mapShikiNode(node);
      if (mapped) {
        shikiCache.set(mapped.id, mapped);
        ongoingArts.push(mapped);
      }
    }

    const rankedArts: ShikiArtwork[] = [];
    for (const node of rankedNodes) {
      const mapped = mapShikiNode(node);
      if (mapped) {
        shikiCache.set(mapped.id, mapped);
        rankedArts.push(mapped);
      }
    }

    const aliasByShikiId = new Map<number, string>();
    for (const item of [...latestRaw, ...recRaw]) {
      const sid = item.shikimori?.id;
      if (sid && item.alias && !aliasByShikiId.has(sid)) {
        aliasByShikiId.set(sid, item.alias);
      }
    }

    const libShikiIds = [...latestRaw.slice(0, 8), ...recRaw.slice(0, 8)]
      .map((item) => item.shikimori?.id ?? 0)
      .filter((id) => id > 0 && !shikiCache.has(id));
    if (libShikiIds.length > 0) {
      await ensureShikimoriArtwork(libShikiIds).catch(() => {});
    }

    const usedShikiIds = new Set<number>();
    const usedFranchises = new Set<string>();

    // 1. Spotlight (4 modern popular titles with rich 16:9 screenshots + poster)
    const spotlight: HomeSpotlightItem[] = [];
    const spotlightCandidates = [
      ...ongoingArts.filter((a) => (a.year ?? 0) >= 2022 && a.screenshots.length >= 3 && a.posterOriginal),
      ...rankedArts.filter((a) => a.screenshots.length >= 3 && a.posterOriginal)
    ];
    for (const art of spotlightCandidates) {
      if (spotlight.length >= 4) break;
      const key = franchiseKey(art.english || art.name);
      if (usedShikiIds.has(art.id) || (key && usedFranchises.has(key))) continue;
      usedShikiIds.add(art.id);
      if (key) usedFranchises.add(key);
      spotlight.push(mapShikiToSpotlight(art, aliasByShikiId.get(art.id)));
    }

    // 2. New Releases (4 live latest releases from AniLiberty, enriched with Shikimori metadata)
    const releases: HomeReleaseItem[] = [];
    const seenAliases = new Set<string>();
    for (const r of latestRaw) {
      if (releases.length >= 4) break;
      if (!r.alias || seenAliases.has(r.alias) || r.is_blocked_by_copyrights) continue;
      const shiki = r.shikimori?.id ? shikiCache.get(r.shikimori.id) : undefined;
      const poster = shiki?.posterOriginal || resolveLibUrl(r.poster?.src || r.poster?.optimized?.src || r.poster?.preview);
      if (!poster) continue;

      seenAliases.add(r.alias);
      const title = shiki?.english || r.name?.english || r.name?.main || 'Anime';
      const rawScore = shiki?.score && shiki.score > 0 ? shiki.score : (r.shikimori?.rating || r.mal?.rating || 0);
      const epCount = r.episodes_total || r.latest_episode?.ordinal || undefined;
      const libTags = [
        r.year ? String(r.year) : '',
        epCount ? `${epCount} ep` : '',
        ...(r.genres ?? []).slice(0, 3).map((g) => g.name ?? '').filter(Boolean)
      ].filter(Boolean);
      const tags = shiki ? buildMetaTags(shiki) : libTags;
      const descSource = shiki?.description || cleanBBCode(r.description);

      if (r.shikimori?.id) usedShikiIds.add(r.shikimori.id);
      const fKey = franchiseKey(title);
      if (fKey) usedFranchises.add(fKey);

      releases.push({
        id: r.alias,
        shikiId: r.shikimori?.id ?? undefined,
        releaseAlias: r.alias,
        searchQueries: [r.name?.english ?? '', r.name?.main ?? '', shiki?.name ?? ''].filter(Boolean),
        title,
        titleLines: splitTitleLines(title),
        description: truncateDescription(descSource, 155),
        poster,
        score: rawScore > 0 ? Number(rawScore.toFixed(1)) : undefined,
        tags: tags.length > 0 ? tags : libTags
      });
    }

    // 3. Recommendations (5 distinct high-rated posters from AniLiberty recommended + Shikimori ranked)
    const recommendations: HomeRecommendationItem[] = [];
    for (const r of recRaw) {
      if (recommendations.length >= 3) break;
      if (!r.alias || seenAliases.has(r.alias) || r.is_blocked_by_copyrights) continue;
      const sid = r.shikimori?.id ?? undefined;
      if (sid && usedShikiIds.has(sid)) continue;
      if ((r.year ?? 0) < 2018) continue;
      const shiki = sid ? shikiCache.get(sid) : undefined;
      const poster = shiki?.posterOriginal || resolveLibUrl(r.poster?.src || r.poster?.optimized?.src);
      if (!poster) continue;

      const title = shiki?.english || r.name?.english || r.name?.main || 'Anime';
      const fKey = franchiseKey(title);
      if (fKey && usedFranchises.has(fKey)) continue;

      seenAliases.add(r.alias);
      if (sid) usedShikiIds.add(sid);
      if (fKey) usedFranchises.add(fKey);
      const rawScore = shiki?.score && shiki.score > 0 ? shiki.score : (r.shikimori?.rating || r.mal?.rating || 0);

      recommendations.push({
        id: r.alias,
        shikiId: sid,
        releaseAlias: r.alias,
        searchQueries: [r.name?.english ?? '', r.name?.main ?? '', shiki?.name ?? ''].filter(Boolean),
        title,
        poster,
        score: rawScore > 0 ? Number(rawScore.toFixed(1)) : undefined,
        tags: shiki ? buildMetaTags(shiki) : []
      });
    }

    for (const art of rankedArts) {
      if (recommendations.length >= 5) break;
      if (!art.posterOriginal || usedShikiIds.has(art.id)) continue;
      const title = art.english || art.name || art.russian || 'Anime';
      const fKey = franchiseKey(title);
      if (fKey && usedFranchises.has(fKey)) continue;

      usedShikiIds.add(art.id);
      if (fKey) usedFranchises.add(fKey);
      recommendations.push({
        id: `shiki-${art.id}`,
        shikiId: art.id,
        releaseAlias: aliasByShikiId.get(art.id),
        searchQueries: [art.name, art.english, art.russian].filter(Boolean),
        title,
        poster: art.posterOriginal || art.posterMain,
        score: art.score > 0 ? Number(art.score.toFixed(1)) : undefined,
        tags: buildMetaTags(art)
      });
    }

    const result: HomeFeeds = { spotlight, releases, recommendations };
    if (spotlight.length > 0 || releases.length > 0 || recommendations.length > 0) {
      homeFeedsCache = result;
    }
    return result;
  })().finally(() => {
    homeFeedsPromise = null;
  });

  return homeFeedsPromise;
}
