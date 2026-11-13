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
  shikimori?: { id?: number | null } | null;
  name?: { main?: string; english?: string; alternative?: string | null };
  poster?: { src?: string; preview?: string };
  episodes?: Array<{
    id: string;
    ordinal: number;
    name?: string | null;
    name_english?: string | null;
    duration?: number;
    preview?: {
      src?: string | null;
      preview?: string | null;
      thumbnail?: string | null;
      optimized?: {
        src?: string | null;
        preview?: string | null;
        thumbnail?: string | null;
      } | null;
    } | null;
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
      preview: resolveLibUrl(
        ep.preview?.optimized?.preview ||
          ep.preview?.optimized?.src ||
          ep.preview?.preview ||
          ep.preview?.src ||
          ep.preview?.optimized?.thumbnail ||
          ep.preview?.thumbnail
      ),
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
    shikimori_id?: number;
    title: string;
    title_english?: string;
    poster?: { original?: string; main?: string };
    banner?: string;
    episodes?: PlayableEpisode[];
  };
}

async function fetchReleaseByIdOrAlias(
  idOrAlias: string,
  shikiArt?: ShikiArtwork,
  expectedShikiId?: number
): Promise<PlayableRelease | null> {
  if (await isBackendUp()) {
    try {
      const env = await fetchJsonWithTimeout<BackendDetailEnvelope>(
        `${BACKEND_BASE}/anime/releases/${encodeURIComponent(idOrAlias)}`,
        undefined,
        7000
      );
      if (env.data && (env.data.episodes?.length ?? 0) > 0) {
        if (
          expectedShikiId &&
          env.data.shikimori_id &&
          env.data.shikimori_id !== expectedShikiId
        ) {
          return null;
        }
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
    if (expectedShikiId && raw.shikimori?.id && raw.shikimori.id !== expectedShikiId) {
      return null;
    }
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

interface LibSearchCandidate {
  id: number;
  alias: string;
  year?: number | null;
  name?: { main?: string | null; english?: string | null; alternative?: string | null } | null;
  shikimori?: { id?: number | null } | null;
}

const TITLE_STOP_WORDS = new Set([
  'the',
  'a',
  'an',
  'of',
  'in',
  'on',
  'to',
  'and',
  'for',
  'no',
  'wa',
  'ga',
  'ni',
  'wo',
  'season',
  'part'
]);

function normalizeTitleText(raw: string): string {
  return raw
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim();
}

function titleTokens(raw: string): string[] {
  const all = normalizeTitleText(raw).split(' ').filter(Boolean);
  const meaningful = all.filter((w) => !TITLE_STOP_WORDS.has(w) && (w.length >= 2 || /^\d+$/.test(w)));
  return meaningful.length > 0 ? meaningful : all;
}

function scoreReleaseCandidate(
  cand: LibSearchCandidate,
  target: WatchTarget,
  shikiArt?: ShikiArtwork
): number {
  if (target.shikiId && cand.shikimori?.id) {
    return cand.shikimori.id === target.shikiId ? 10_000 : -1;
  }

  const candTitles = [cand.name?.main, cand.name?.english, cand.name?.alternative]
    .map((v) => v?.trim() ?? '')
    .filter(Boolean);
  const targetTitles = [
    target.title,
    shikiArt?.english,
    shikiArt?.name,
    shikiArt?.russian,
    ...(target.searchQueries ?? [])
  ]
    .map((v) => v?.trim() ?? '')
    .filter(Boolean);

  if (candTitles.length === 0 || targetTitles.length === 0) {
    return -1;
  }

  let best = -1;
  for (const cTitle of candTitles) {
    const cNorm = normalizeTitleText(cTitle);
    const cToks = titleTokens(cTitle);
    if (!cNorm || cToks.length === 0) continue;

    for (const tTitle of targetTitles) {
      const tNorm = normalizeTitleText(tTitle);
      const tToks = titleTokens(tTitle);
      if (!tNorm || tToks.length === 0) continue;

      if (cNorm === tNorm) {
        best = Math.max(best, 900);
        continue;
      }

      const tSet = new Set(tToks);
      let matches = 0;
      for (const tok of cToks) {
        if (tSet.has(tok)) matches += 1;
      }
      if (matches === 0) continue;

      const minLen = Math.min(cToks.length, tToks.length);
      const maxLen = Math.max(cToks.length, tToks.length);
      const containment = matches / minLen;
      const jaccard = matches / maxLen;

      if (containment >= 0.8 && jaccard >= 0.6) {
        let score = Math.round(500 * jaccard + 100 * containment);
        const cSpecial = /\b(special|спэшл|ova|recap)\b/i.test(cTitle);
        const tSpecial = /\b(special|спэшл|ova|recap)\b/i.test(tTitle);
        if (cSpecial && !tSpecial) score -= 120;
        if (shikiArt?.year && cand.year && Math.abs(shikiArt.year - cand.year) <= 1) {
          score += 20;
        }
        best = Math.max(best, score);
      }
    }
  }

  return best;
}

// Resolves a playable HLS release matching the exact target (prefers Go backend, falls back to AniLiberty).
export async function resolvePlayableRelease(target: WatchTarget): Promise<PlayableRelease> {
  if (target.shikiId) {
    await ensureShikimoriArtwork([target.shikiId]).catch(() => {});
  }
  const shikiArt = target.shikiId ? shikiCache.get(target.shikiId) : undefined;

  if (target.releaseAlias) {
    const direct = await fetchReleaseByIdOrAlias(target.releaseAlias, shikiArt, target.shikiId);
    if (direct) return direct;
  }

  const rawQueries = [
    ...(target.searchQueries ?? []),
    shikiArt?.russian ?? '',
    shikiArt?.english ?? '',
    shikiArt?.name ?? '',
    target.title
  ];
  const seenQueries = new Set<string>();
  const queries: string[] = [];
  for (const q of rawQueries) {
    const trimmed = q.trim();
    const key = trimmed.toLowerCase();
    if (trimmed.length < 2 || seenQueries.has(key)) continue;
    seenQueries.add(key);
    queries.push(trimmed);
  }

  const candidatesById = new Map<number, { cand: LibSearchCandidate; score: number }>();

  for (const q of queries) {
    const [appRes, catRes] = await Promise.allSettled([
      fetchJsonWithTimeout<LibSearchCandidate[]>(
        `${ANILIBERTY_BASE}/app/search/releases?query=${encodeURIComponent(q)}`,
        undefined,
        6000
      ),
      fetchJsonWithTimeout<{ data?: LibSearchCandidate[] }>(
        `${ANILIBERTY_BASE}/anime/catalog/releases?f[search]=${encodeURIComponent(q)}&limit=10`,
        undefined,
        6000
      )
    ]);

    const batch: LibSearchCandidate[] = [
      ...(appRes.status === 'fulfilled' && Array.isArray(appRes.value) ? appRes.value : []),
      ...(catRes.status === 'fulfilled' && Array.isArray(catRes.value?.data) ? catRes.value.data : [])
    ];

    let foundExactShiki = false;
    for (const cand of batch) {
      if (!cand?.id) continue;
      const score = scoreReleaseCandidate(cand, target, shikiArt);
      if (score <= 0) continue;
      const prev = candidatesById.get(cand.id);
      if (!prev || score > prev.score) {
        candidatesById.set(cand.id, { cand, score });
      }
      if (score >= 10_000) {
        foundExactShiki = true;
      }
    }

    if (foundExactShiki) break;
  }

  const rankedCandidates = Array.from(candidatesById.values()).sort((a, b) => b.score - a.score);
  for (const { cand } of rankedCandidates.slice(0, 5)) {
    const rel = await fetchReleaseByIdOrAlias(String(cand.id), shikiArt, target.shikiId);
    if (rel) return rel;
  }

  throw new Error(`«${target.title}» сейчас недоступно для воспроизведения на AniLiberty`);
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

    const [latestRes, recRes, topCatRes, shikiRes] = await Promise.allSettled([
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
      fetchJsonWithTimeout<{ data?: RawLibFeedItem[] }>(
        `${ANILIBERTY_BASE}/anime/catalog/releases?f[sorting]=RATING_DESC&limit=25`,
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
    const topCatRaw =
      topCatRes.status === 'fulfilled' && Array.isArray(topCatRes.value?.data) ? topCatRes.value.data : [];
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
    for (const item of [...latestRaw, ...recRaw, ...topCatRaw]) {
      if (item.is_blocked_by_copyrights) continue;
      const sid = item.shikimori?.id;
      if (sid && item.alias && !aliasByShikiId.has(sid)) {
        aliasByShikiId.set(sid, item.alias);
      }
    }

    const libShikiIds = [
      ...topCatRaw.slice(0, 16),
      ...latestRaw.slice(0, 8),
      ...recRaw.slice(0, 8)
    ]
      .filter((item) => !item.is_blocked_by_copyrights)
      .map((item) => item.shikimori?.id ?? 0)
      .filter((id) => id > 0 && !shikiCache.has(id));
    if (libShikiIds.length > 0) {
      await ensureShikimoriArtwork(libShikiIds).catch(() => {});
    }

    const topLibArts: ShikiArtwork[] = [];
    for (const item of topCatRaw) {
      if (item.is_blocked_by_copyrights || !item.shikimori?.id) continue;
      const art = shikiCache.get(item.shikimori.id);
      if (art) topLibArts.push(art);
    }

    const usedShikiIds = new Set<number>();
    const usedFranchises = new Set<string>();

    // 1. Spotlight (4 popular playable titles with rich 16:9 screenshots + poster)
    const spotlight: HomeSpotlightItem[] = [];
    const spotlightCandidates = [
      ...ongoingArts.filter(
        (a) => aliasByShikiId.has(a.id) && (a.year ?? 0) >= 2022 && a.screenshots.length >= 3 && a.posterOriginal
      ),
      ...topLibArts.filter((a) => a.screenshots.length >= 3 && a.posterOriginal),
      ...rankedArts.filter((a) => aliasByShikiId.has(a.id) && a.screenshots.length >= 3 && a.posterOriginal),
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

    for (const art of [...topLibArts, ...rankedArts]) {
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

export interface AnimeSeasonEntry {
  id: number;
  alias: string;
  shikiId?: number;
  title: string;
  titleEnglish?: string;
  shortLabel: string;
  year?: number;
  typeLabel?: string;
  episodesTotal?: number;
  poster: string;
}

export interface SearchGroupItem {
  id: string;
  title: string;
  subTitle?: string;
  year?: number;
  poster: string;
  posterPosition?: string;
  shikiId?: number;
  releaseAlias?: string;
  searchQueries: string[];
  seasonCount: number;
}

interface RawFranchiseReleaseNode {
  sort_order?: number;
  release_id?: number;
  release?: (RawLibFeedItem & {
    type?: { value?: string | null; description?: string | null } | null;
    name?: { main?: string | null; english?: string | null; alternative?: string | null } | null;
    poster?: {
      src?: string | null;
      preview?: string | null;
      optimized?: { src?: string | null; preview?: string | null } | null;
    } | null;
  }) | null;
}

interface RawFranchise {
  id: string;
  name?: string | null;
  name_english?: string | null;
  first_year?: number | null;
  last_year?: number | null;
  total_releases?: number | null;
  franchise_releases?: RawFranchiseReleaseNode[] | null;
}

interface ResolvedFranchise {
  id: string;
  name?: string;
  nameEnglish?: string;
  seasons: AnimeSeasonEntry[];
}

const franchiseByReleaseCache = new Map<number, ResolvedFranchise | null>();
const franchisePending = new Map<number, Promise<ResolvedFranchise | null>>();
const groupedSearchCache = new Map<string, SearchGroupItem[]>();

function mapFranchiseSeasons(rawList: RawFranchiseReleaseNode[]): AnimeSeasonEntry[] {
  const valid = rawList
    .filter((node) => node.release && !node.release.is_blocked_by_copyrights)
    .sort((a, b) => {
      const sortDiff = (a.sort_order ?? 9999) - (b.sort_order ?? 9999);
      if (sortDiff !== 0) return sortDiff;
      return (a.release?.year ?? 9999) - (b.release?.year ?? 9999);
    });

  let seasonOrdinal = 0;
  let movieOrdinal = 0;
  const movieCount = valid.filter((n) => n.release?.type?.value === 'MOVIE').length;

  return valid.map((node, index) => {
    const r = node.release!;
    const typeVal = (r.type?.value ?? 'TV').toUpperCase();
    let shortLabel: string;

    if (typeVal === 'MOVIE') {
      movieOrdinal += 1;
      shortLabel = movieCount > 1 ? `Movie ${movieOrdinal}` : 'Movie';
    } else if (typeVal === 'OVA' || typeVal === 'SPECIAL') {
      shortLabel = typeVal === 'OVA' ? 'OVA' : 'Special';
    } else if (typeVal === 'TV' || typeVal === 'ONA' || typeVal === 'WEB') {
      seasonOrdinal += 1;
      shortLabel = `Season ${seasonOrdinal}`;
    } else {
      shortLabel = `Part ${index + 1}`;
    }

    const poster = resolveLibUrl(
      r.poster?.optimized?.src || r.poster?.src || r.poster?.optimized?.preview || r.poster?.preview
    );

    return {
      id: r.id,
      alias: r.alias,
      shikiId: r.shikimori?.id ?? undefined,
      title: r.name?.main || r.name?.english || 'Anime',
      titleEnglish: r.name?.english || undefined,
      shortLabel,
      year: r.year ?? undefined,
      typeLabel: r.type?.description || r.type?.value || undefined,
      episodesTotal: r.episodes_total ?? undefined,
      poster
    };
  });
}

async function resolveFranchiseByReleaseId(releaseId: number): Promise<ResolvedFranchise | null> {
  if (franchiseByReleaseCache.has(releaseId)) {
    return franchiseByReleaseCache.get(releaseId) ?? null;
  }
  const existing = franchisePending.get(releaseId);
  if (existing) {
    return existing;
  }

  const req = (async (): Promise<ResolvedFranchise | null> => {
    try {
      const payload = await fetchJsonWithTimeout<RawFranchise | RawFranchise[] | null>(
        `${ANILIBERTY_BASE}/anime/franchises/release/${releaseId}`,
        undefined,
        3500
      );
      const raw = Array.isArray(payload) ? payload[0] : payload;
      if (!raw || !Array.isArray(raw.franchise_releases) || raw.franchise_releases.length === 0) {
        franchiseByReleaseCache.set(releaseId, null);
        return null;
      }

      const seasons = mapFranchiseSeasons(raw.franchise_releases);
      if (seasons.length === 0) {
        franchiseByReleaseCache.set(releaseId, null);
        return null;
      }

      const resolved: ResolvedFranchise = {
        id: raw.id,
        name: raw.name ?? undefined,
        nameEnglish: raw.name_english ?? undefined,
        seasons
      };

      franchiseByReleaseCache.set(releaseId, resolved);
      for (const s of seasons) {
        franchiseByReleaseCache.set(s.id, resolved);
      }
      return resolved;
    } catch {
      return null;
    } finally {
      franchisePending.delete(releaseId);
    }
  })();

  franchisePending.set(releaseId, req);
  return req;
}

export async function loadFranchiseSeasons(releaseId: number): Promise<AnimeSeasonEntry[]> {
  const resolved = await resolveFranchiseByReleaseId(releaseId);
  return resolved?.seasons ?? [];
}

function stripSeasonSuffix(title: string): string {
  return title
    .replace(/\s*[:\-–]?\s*(\d+(?:st|nd|rd|th)\s+season|season\s+\d+|tv\s+\d+)\b.*$/i, '')
    .replace(/\s+\b(II|III|IV|V)\b$/i, '')
    .trim();
}

function releaseCandidateToSearchItem(
  r: RawLibFeedItem,
  franchise: ResolvedFranchise | null,
  extraSeasonCount = 1
): SearchGroupItem {
  const seasons = franchise?.seasons ?? [];
  const rootSeason = seasons.find((s) => s.shortLabel.startsWith('Season')) ?? seasons[0];

  const baseTitle =
    franchise?.nameEnglish ||
    rootSeason?.titleEnglish ||
    rootSeason?.title ||
    r.name?.english ||
    r.name?.main ||
    'Anime';
  const cleanTitle = rootSeason ? stripSeasonSuffix(baseTitle) || baseTitle : baseTitle;
  const targetSeason = rootSeason ?? null;

  const poster =
    targetSeason?.poster ||
    resolveLibUrl(r.poster?.optimized?.src || r.poster?.src || r.poster?.preview);

  const queries = [
    targetSeason?.titleEnglish ?? '',
    targetSeason?.title ?? '',
    franchise?.nameEnglish ?? '',
    franchise?.name ?? '',
    r.name?.english ?? '',
    r.name?.main ?? ''
  ].filter(Boolean);

  return {
    id: franchise ? `fr-${franchise.id}` : `rel-${r.id}`,
    title: cleanTitle,
    subTitle: franchise?.name && franchise.name !== cleanTitle ? franchise.name : undefined,
    year: targetSeason?.year ?? r.year ?? undefined,
    poster,
    shikiId: targetSeason?.shikiId ?? r.shikimori?.id ?? undefined,
    releaseAlias: targetSeason?.alias ?? r.alias,
    searchQueries: Array.from(new Set(queries)),
    seasonCount: Math.max(seasons.length, extraSeasonCount)
  };
}

export async function searchGroupedAnime(rawQuery: string): Promise<SearchGroupItem[]> {
  const query = rawQuery.trim();
  if (query.length < 2) return [];

  const cacheKey = query.toLowerCase();
  const cached = groupedSearchCache.get(cacheKey);
  if (cached) return cached;

  const [appRes, catRes] = await Promise.allSettled([
    fetchJsonWithTimeout<RawLibFeedItem[]>(
      `${ANILIBERTY_BASE}/app/search/releases?query=${encodeURIComponent(query)}`,
      undefined,
      4500
    ),
    fetchJsonWithTimeout<{ data?: RawLibFeedItem[] }>(
      `${ANILIBERTY_BASE}/anime/catalog/releases?f[search]=${encodeURIComponent(query)}&limit=15`,
      undefined,
      4500
    )
  ]);

  const rawHits: RawLibFeedItem[] = [];
  const seenIds = new Set<number>();
  for (const item of [
    ...(catRes.status === 'fulfilled' && Array.isArray(catRes.value?.data) ? catRes.value.data : []),
    ...(appRes.status === 'fulfilled' && Array.isArray(appRes.value) ? appRes.value : [])
  ]) {
    if (!item?.id || !item.alias || item.is_blocked_by_copyrights || seenIds.has(item.id)) continue;
    seenIds.add(item.id);
    rawHits.push(item);
  }

  const candidates = rawHits.slice(0, 12);
  const resolvedList = await Promise.all(candidates.map((hit) => resolveFranchiseByReleaseId(hit.id)));

  interface PendingGroup {
    item: SearchGroupItem;
    earliestYear: number;
    count: number;
    hasFranchise: boolean;
  }

  const groups = new Map<string, PendingGroup>();
  const orderedKeys: string[] = [];

  for (let i = 0; i < candidates.length; i += 1) {
    const hit = candidates[i];
    const franchise = resolvedList[i];
    const fallbackTitle = hit.name?.english || hit.name?.main || '';
    const fKey = franchiseKey(fallbackTitle);
    const groupKey = franchise
      ? `fr:${franchise.id}`
      : fKey
        ? `title:${fKey}`
        : `rel:${hit.id}`;

    const existing = groups.get(groupKey);
    if (!existing) {
      orderedKeys.push(groupKey);
      groups.set(groupKey, {
        item: releaseCandidateToSearchItem(hit, franchise, 1),
        earliestYear: hit.year ?? 9999,
        count: 1,
        hasFranchise: Boolean(franchise)
      });
      continue;
    }

    existing.count += 1;
    if (!existing.hasFranchise) {
      const hitYear = hit.year ?? 9999;
      if (hitYear < existing.earliestYear) {
        existing.earliestYear = hitYear;
        existing.item = releaseCandidateToSearchItem(hit, franchise, existing.count);
      } else {
        existing.item.seasonCount = Math.max(existing.item.seasonCount, existing.count);
      }
    }
  }

  const grouped = orderedKeys.map((k) => groups.get(k)!.item).slice(0, 10);
  if (grouped.length > 0) {
    groupedSearchCache.set(cacheKey, grouped);
  }
  return grouped;
}
