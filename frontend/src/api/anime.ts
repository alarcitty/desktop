import type { HlsSources, SkipRange, VoiceTrackOption } from '../marlakit/player/types';
import { fetchAnimeVostEpisodes, findAnimeVostMatch } from './providers/animevost';
import { fetchAnixartEpisodes, findAnixartDiscovery } from './providers/anixart';
import {
  dedupeSearchQueries,
  fetchJsonWithTimeout,
  normalizeTitleText,
  SPECIAL_TITLE_RE,
  titleTokens
} from './providers/shared';

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
  mp4_1080?: string | null;
  mp4_720?: string | null;
  mp4_480?: string | null;
  embed_url?: string | null;
}

export interface PlayableRelease {
  id: number;
  alias: string;
  title: string;
  titleEnglish?: string;
  year?: number;
  typeLabel?: string;
  provider?: 'aniliberty' | 'animevost' | 'anixart' | string;
  voiceLabel?: string;
  voiceActors?: string[];
  poster?: string;
  banner?: string;
  episodes: PlayableEpisode[];
  fallbackNote?: string;
}

export interface HeroOpeningOption {
  number: number;
  label: string;
  title: string;
  alias?: string;
  episode: number;
  startOffsetSec?: number;
}

export interface HomeSpotlightItem {
  shikiId: number;
  title: string;
  releaseAlias?: string;
  openingAlias?: string;
  openingEpisode?: number;
  openingStartOffsetSec?: number;
  customPreviewUrl?: string;
  customPreviewAspect?: '16:9' | 'fill';
  openings?: HeroOpeningOption[];
  defaultOpeningNumber?: number;
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
  fallbackPoster?: string;
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
  fallbackPoster?: string;
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

function inferReleaseTypeValue(
  rawType?: string | null,
  ...titles: Array<string | null | undefined>
): string {
  const normalized = (rawType ?? '').trim().toUpperCase();
  if (normalized) return normalized;
  const combined = titles.filter(Boolean).join(' ');
  if (/(?:^|[^\p{L}\p{N}])(special|сп[еэ]шл|спецвыпуск)(?:$|[^\p{L}\p{N}])/iu.test(combined)) return 'SPECIAL';
  if (/(?:^|[^\p{L}\p{N}])ova(?:$|[^\p{L}\p{N}])/iu.test(combined)) return 'OVA';
  if (/(?:^|[^\p{L}\p{N}])(movie|film|фильм)(?:$|[^\p{L}\p{N}])/iu.test(combined)) return 'MOVIE';
  return 'TV';
}

interface RawLibRelease {
  id: number;
  alias: string;
  year?: number | null;
  type?: { value?: string | null; description?: string | null } | null;
  shikimori?: { id?: number | null } | null;
  name?: { main?: string; english?: string; alternative?: string | null };
  poster?: { src?: string; preview?: string };
  members?: Array<{
    role?: { value?: string | null; description?: string | null } | null;
    nickname?: string | null;
  }> | null;
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

  const voiceActors = Array.from(
    new Set(
      (r.members ?? [])
        .filter((m) => m.role?.value === 'voicing' && m.nickname?.trim())
        .map((m) => m.nickname!.trim())
    )
  );

  return {
    id: r.id,
    alias: r.alias,
    title: r.name?.main || shikiArt?.english || shikiArt?.name || 'Anime',
    titleEnglish: r.name?.english || shikiArt?.english,
    year: r.year ?? undefined,
    typeLabel: inferReleaseTypeValue(r.type?.value, r.name?.main, r.name?.english, r.alias),
    voiceLabel: 'AniLiberty',
    voiceActors,
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
    year?: number;
    type?: string;
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
          year: env.data.year,
          typeLabel: inferReleaseTypeValue(
            env.data.type,
            env.data.title,
            env.data.title_english,
            env.data.alias
          ),
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

  const candIsSpecial = candTitles.some((t) => SPECIAL_TITLE_RE.test(t));
  const targetIsSpecial = targetTitles.some((t) => SPECIAL_TITLE_RE.test(t));
  if (candIsSpecial && !targetIsSpecial) {
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
        if (shikiArt?.year && cand.year && Math.abs(shikiArt.year - cand.year) <= 1) {
          score += 20;
        }
        best = Math.max(best, score);
      }
    }
  }

  return best;
}

// Resolves a playable release matching the exact target (AniLiberty first, then AnimeVost MP4 / Anixart).
export async function resolvePlayableRelease(target: WatchTarget): Promise<PlayableRelease> {
  if (target.shikiId) {
    await ensureShikimoriArtwork([target.shikiId]).catch(() => {});
  }
  const shikiArt = target.shikiId ? shikiCache.get(target.shikiId) : undefined;

  if (target.releaseAlias) {
    const direct = await fetchReleaseByIdOrAlias(target.releaseAlias, shikiArt, target.shikiId);
    if (direct) return { ...direct, provider: direct.provider ?? 'aniliberty' };
  }

  const queries = dedupeSearchQueries([
    ...(target.searchQueries ?? []),
    shikiArt?.russian ?? '',
    shikiArt?.english ?? '',
    shikiArt?.name ?? '',
    target.title
  ]);

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
    if (rel) return { ...rel, provider: 'aniliberty' };
  }

  const providerTitles = dedupeSearchQueries([
    shikiArt?.russian,
    target.title,
    shikiArt?.english,
    shikiArt?.name,
    ...(target.searchQueries ?? [])
  ]);

  const avMatch = await findAnimeVostMatch(providerTitles, shikiArt?.year ?? null).catch(() => null);
  if (avMatch && avMatch.episodes.length > 0) {
    return {
      id: avMatch.releaseId,
      alias: avMatch.voice.id,
      title: shikiArt?.english || shikiArt?.name || avMatch.title || target.title,
      titleEnglish: shikiArt?.english || shikiArt?.name,
      year: shikiArt?.year ?? avMatch.year,
      provider: 'animevost',
      voiceLabel: avMatch.voice.label,
      poster: shikiArt?.posterOriginal || shikiArt?.posterMain || avMatch.poster,
      banner: shikiArt?.screenshots?.[0] || avMatch.poster,
      episodes: avMatch.episodes
    };
  }

  const axMatch = await findAnixartDiscovery(providerTitles, shikiArt?.year ?? null).catch(() => null);
  if (axMatch && axMatch.voices.length > 0) {
    for (const voice of axMatch.voices.slice(0, 3)) {
      const eps = await fetchAnixartEpisodes(axMatch.releaseId, Number(voice.sourceId)).catch(() => []);
      if (eps.length > 0) {
        return {
          id: axMatch.releaseId,
          alias: voice.id,
          title: shikiArt?.english || shikiArt?.name || axMatch.title || target.title,
          titleEnglish: shikiArt?.english || shikiArt?.name,
          year: shikiArt?.year ?? axMatch.year,
          provider: 'anixart',
          voiceLabel: voice.label,
          poster: shikiArt?.posterOriginal || shikiArt?.posterMain || axMatch.poster,
          banner: shikiArt?.screenshots?.[0] || axMatch.poster,
          episodes: eps
        };
      }
    }
  }

  throw new Error(`«${target.title}» сейчас недоступно для воспроизведения ни на одном из источников`);
}

export function episodeSources(ep: PlayableEpisode): HlsSources {
  return {
    hls_1080: ep.hls_1080,
    hls_720: ep.hls_720,
    hls_480: ep.hls_480,
    mp4_1080: ep.mp4_1080,
    mp4_720: ep.mp4_720,
    mp4_480: ep.mp4_480,
    embed_url: ep.embed_url
  };
}

interface RawLibFeedItem {
  id: number;
  alias: string;
  year?: number | null;
  is_ongoing?: boolean;
  type?: { value?: string | null; description?: string | null } | null;
  episodes_total?: number | null;
  description?: string | null;
  is_blocked_by_copyrights?: boolean;
  name?: { main?: string | null; english?: string | null } | null;
  shikimori?: { id?: number | null; rating?: number | null } | null;
  mal?: { rating?: number | null } | null;
  rating?: { average?: number | null } | null;
  poster?: {
    src?: string | null;
    preview?: string | null;
    optimized?: { src?: string | null } | null;
  } | null;
  genres?: Array<{ id?: number | null; name?: string | null }> | null;
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

function pickHighlightPreviews(shots: readonly string[], posterOriginal: string): string[] {
  const n = shots.length;
  if (n >= 8) {
    const indices = [Math.floor(n * 0.25), Math.floor(n * 0.55), Math.floor(n * 0.82)];
    return indices.map((idx) => shots[idx] ?? shots[0] ?? posterOriginal);
  }
  if (n >= 4) {
    return shots.slice(1, 4);
  }
  if (n >= 3) {
    return shots.slice(0, 3);
  }
  return [shots[0] ?? posterOriginal, shots[1] ?? shots[0] ?? posterOriginal, shots[2] ?? shots[0] ?? posterOriginal];
}

const HERO_SPOTLIGHT_LIMIT = 6;

function mapShikiToSpotlight(art: ShikiArtwork, releaseAlias?: string): HomeSpotlightItem {
  const shots = art.screenshots;
  const previews = pickHighlightPreviews(shots, art.posterOriginal);

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

// Loads curated top-5 Spotlight plus live AniLiberty New Releases and Recommendations.
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
    const libPosterByShikiId = new Map<number, string>();
    for (const item of [...latestRaw, ...recRaw, ...topCatRaw]) {
      if (item.is_blocked_by_copyrights) continue;
      const sid = item.shikimori?.id;
      if (sid && item.alias && !aliasByShikiId.has(sid)) {
        aliasByShikiId.set(sid, item.alias);
      }
      const libPoster = resolveLibUrl(item.poster?.src || item.poster?.optimized?.src || item.poster?.preview);
      if (sid && libPoster && !libPosterByShikiId.has(sid)) {
        libPosterByShikiId.set(sid, libPoster);
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

    // 1. Spotlight (fully dynamic live titles from Shikimori ongoing/ranked & AniLiberty top catalog)
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
      if (spotlight.length >= HERO_SPOTLIGHT_LIMIT) break;
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
      const libPoster = resolveLibUrl(r.poster?.src || r.poster?.optimized?.src || r.poster?.preview);
      const shikiPoster = shiki?.posterOriginal || shiki?.posterMain || '';
      const poster = libPoster || shikiPoster;
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
        fallbackPoster: shikiPoster && shikiPoster !== poster ? shikiPoster : undefined,
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
      const libPoster = resolveLibUrl(r.poster?.src || r.poster?.optimized?.src || r.poster?.preview);
      const shikiPoster = shiki?.posterOriginal || shiki?.posterMain || '';
      const poster = libPoster || shikiPoster;
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
        fallbackPoster: shikiPoster && shikiPoster !== poster ? shikiPoster : undefined,
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

      const libPoster = libPosterByShikiId.get(art.id) ?? '';
      const shikiPoster = art.posterOriginal || art.posterMain;
      const poster = libPoster || shikiPoster;

      usedShikiIds.add(art.id);
      if (fKey) usedFranchises.add(fKey);
      recommendations.push({
        id: `shiki-${art.id}`,
        shikiId: art.id,
        releaseAlias: aliasByShikiId.get(art.id),
        searchQueries: [art.name, art.english, art.russian].filter(Boolean),
        title,
        poster,
        fallbackPoster: shikiPoster && shikiPoster !== poster ? shikiPoster : undefined,
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

export type SearchFilterMode = 'all' | 'library' | 'ongoing';

export interface CatalogGenreOption {
  id: number;
  name: string;
  totalReleases?: number;
}

export interface SearchQueryOptions {
  signal?: AbortSignal;
  filter?: SearchFilterMode;
  fromYear?: number;
  toYear?: number;
  minEpisodes?: number;
  maxEpisodes?: number;
  minScore?: number;
  type?: '' | 'TV' | 'MOVIE' | 'OVA';
  genreIds?: number[];
  sorting?: '' | 'RATING_DESC' | 'FRESH_AT_DESC';
}

export interface SearchGroupItem {
  id: string;
  title: string;
  subTitle?: string;
  year?: number;
  score?: number;
  typeLabel?: string;
  isOngoing?: boolean;
  episodesTotal?: number;
  genres?: string[];
  genreIds?: number[];
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
    const typeVal = inferReleaseTypeValue(
      r.type?.value,
      r.name?.main,
      r.name?.english,
      r.alias
    );
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
  const isSingleSpecialOrOva =
    seasons.length === 1 &&
    (rootSeason?.shortLabel === 'Special' || rootSeason?.shortLabel === 'OVA');

  const baseTitle = isSingleSpecialOrOva
    ? rootSeason?.titleEnglish || rootSeason?.title || r.name?.english || r.name?.main || 'Anime'
    : franchise?.nameEnglish ||
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

  const shikiId = targetSeason?.shikiId ?? r.shikimori?.id ?? undefined;
  const shiki = shikiId ? shikiCache.get(shikiId) : undefined;

  const rawScore =
    (shiki?.score && shiki.score > 0 ? shiki.score : 0) ||
    r.shikimori?.rating ||
    r.mal?.rating ||
    r.rating?.average ||
    0;
  const libGenres = (r.genres ?? [])
    .map((g) => g?.name?.trim())
    .filter((g): g is string => Boolean(g));
  const genres = libGenres.length > 0 ? libGenres : (shiki?.genres ?? []);
  const genreIds = (r.genres ?? [])
    .map((g) => g?.id)
    .filter((id): id is number => typeof id === 'number' && id > 0);
  const epCount =
    (typeof targetSeason?.episodesTotal === 'number' && targetSeason.episodesTotal > 0
      ? targetSeason.episodesTotal
      : undefined) ??
    (typeof r.episodes_total === 'number' && r.episodes_total > 0 ? r.episodes_total : undefined) ??
    (typeof r.latest_episode?.ordinal === 'number' && r.latest_episode.ordinal > 0
      ? r.latest_episode.ordinal
      : undefined) ??
    (shiki?.episodes && shiki.episodes > 0 ? shiki.episodes : undefined) ??
    (shiki?.episodesAired && shiki.episodesAired > 0 ? shiki.episodesAired : undefined);

  return {
    id: franchise ? `fr-${franchise.id}` : `rel-${r.id}`,
    title: cleanTitle,
    subTitle: franchise?.name && franchise.name !== cleanTitle ? franchise.name : undefined,
    year: targetSeason?.year ?? r.year ?? shiki?.year ?? undefined,
    score: rawScore > 0 ? Number(rawScore.toFixed(1)) : undefined,
    typeLabel: targetSeason?.typeLabel ?? r.type?.value ?? r.type?.description ?? 'TV',
    isOngoing: r.is_ongoing,
    episodesTotal: epCount,
    genres: genres.length > 0 ? genres : undefined,
    genreIds: genreIds.length > 0 ? genreIds : undefined,
    poster,
    shikiId,
    releaseAlias: targetSeason?.alias ?? r.alias,
    searchQueries: Array.from(new Set(queries)),
    seasonCount: Math.max(seasons.length, extraSeasonCount)
  };
}

let catalogGenresCache: CatalogGenreOption[] | null = null;
let catalogGenresPending: Promise<CatalogGenreOption[]> | null = null;

export async function loadCatalogGenres(signal?: AbortSignal): Promise<CatalogGenreOption[]> {
  if (catalogGenresCache) return catalogGenresCache;
  if (catalogGenresPending) return catalogGenresPending;

  catalogGenresPending = (async () => {
    try {
      const raw = await fetchJsonWithTimeout<
        Array<{ id?: number; name?: string | null; total_releases?: number | null }>
      >(`${ANILIBERTY_BASE}/anime/catalog/references/genres`, { signal }, 4500);
      if (!Array.isArray(raw)) return [];
      const mapped = raw
        .filter((g): g is { id: number; name: string; total_releases?: number | null } =>
          Boolean(g && typeof g.id === 'number' && g.name?.trim())
        )
        .map((g) => ({
          id: g.id,
          name: g.name.trim(),
          totalReleases: typeof g.total_releases === 'number' ? g.total_releases : undefined
        }))
        .sort((a, b) => (b.totalReleases ?? 0) - (a.totalReleases ?? 0));
      if (mapped.length > 0) catalogGenresCache = mapped;
      return mapped;
    } catch {
      return [];
    } finally {
      catalogGenresPending = null;
    }
  })();

  return catalogGenresPending;
}

export async function searchGroupedAnime(
  rawQuery: string,
  opts?: AbortSignal | SearchQueryOptions
): Promise<SearchGroupItem[]> {
  const signal = opts instanceof AbortSignal ? opts : opts?.signal;
  const o: SearchQueryOptions = opts && !(opts instanceof AbortSignal) ? opts : {};
  const filter: SearchFilterMode = o.filter ?? 'all';
  const query = rawQuery.trim();
  const hasAdvancedFilter = Boolean(
    o.fromYear ||
      o.toYear ||
      o.minScore ||
      o.minEpisodes ||
      o.maxEpisodes ||
      o.type ||
      (o.genreIds && o.genreIds.length > 0) ||
      o.sorting
  );
  if (filter === 'library' || signal?.aborted) return [];

  const sortedGenres = o.genreIds?.length ? [...o.genreIds].sort((a, b) => a - b).join(',') : '';
  const cacheKey = [
    filter,
    query.toLowerCase(),
    o.fromYear ?? '',
    o.toYear ?? '',
    o.minEpisodes ?? '',
    o.maxEpisodes ?? '',
    o.minScore ?? '',
    o.type ?? '',
    sortedGenres,
    o.sorting ?? ''
  ].join('|');

  const cached = groupedSearchCache.get(cacheKey);
  if (cached) return cached;

  const catalogParams = new URLSearchParams({ limit: '50', page: '1' });
  if (query.length >= 2) catalogParams.set('f[search]', query);
  if (filter === 'ongoing') {
    catalogParams.set('f[publish_statuses]', 'IS_ONGOING');
  }
  if (o.fromYear) catalogParams.set('f[years][from_year]', String(o.fromYear));
  if (o.toYear) catalogParams.set('f[years][to_year]', String(o.toYear));
  if (o.type) catalogParams.set('f[types]', o.type);
  if (sortedGenres) catalogParams.set('f[genres]', sortedGenres);

  const effectiveSort =
    o.sorting ||
    (o.minScore && o.minScore > 0
      ? 'RATING_DESC'
      : filter === 'ongoing'
        ? 'FRESH_AT_DESC'
        : query.length === 0
          ? 'RATING_DESC'
          : '');
  if (effectiveSort) catalogParams.set('f[sorting]', effectiveSort);

  const catalogParamsPage2 = new URLSearchParams(catalogParams);
  catalogParamsPage2.set('page', '2');

  const catalogUrl1 = `${ANILIBERTY_BASE}/anime/catalog/releases?${catalogParams.toString()}`;
  const catalogUrl2 = `${ANILIBERTY_BASE}/anime/catalog/releases?${catalogParamsPage2.toString()}`;
  const useAppSearch = filter === 'all' && !hasAdvancedFilter && query.length >= 2;
  const needPage2 = query.length < 2 || hasAdvancedFilter || filter === 'ongoing';

  const [appRes, catRes1, catRes2] = await Promise.allSettled([
    useAppSearch
      ? fetchJsonWithTimeout<RawLibFeedItem[]>(
          `${ANILIBERTY_BASE}/app/search/releases?query=${encodeURIComponent(query)}`,
          { signal },
          4500
        )
      : Promise.resolve([] as RawLibFeedItem[]),
    fetchJsonWithTimeout<{ data?: RawLibFeedItem[] }>(catalogUrl1, { signal }, 5000),
    needPage2
      ? fetchJsonWithTimeout<{ data?: RawLibFeedItem[] }>(catalogUrl2, { signal }, 5000)
      : Promise.resolve({ data: [] as RawLibFeedItem[] })
  ]);
  if (signal?.aborted) return [];

  const rawHits: RawLibFeedItem[] = [];
  const seenIds = new Set<number>();
  for (const item of [
    ...(catRes1.status === 'fulfilled' && Array.isArray(catRes1.value?.data) ? catRes1.value.data : []),
    ...(appRes.status === 'fulfilled' && Array.isArray(appRes.value) ? appRes.value : []),
    ...(catRes2.status === 'fulfilled' && Array.isArray(catRes2.value?.data) ? catRes2.value.data : [])
  ]) {
    if (!item?.id || !item.alias || item.is_blocked_by_copyrights || seenIds.has(item.id)) continue;
    seenIds.add(item.id);
    rawHits.push(item);
  }

  const candidates = rawHits.slice(0, 90);

  // Batch-enrich any candidates missing ratings that have a Shikimori ID
  const missingScoreShikiIds = candidates
    .filter((hit) => !(hit.shikimori?.rating || hit.mal?.rating || hit.rating?.average) && hit.shikimori?.id)
    .map((hit) => hit.shikimori!.id!)
    .filter((id) => id > 0 && !shikiCache.has(id))
    .slice(0, 40);
  if (missingScoreShikiIds.length > 0) {
    await ensureShikimoriArtwork(missingScoreShikiIds).catch(() => {});
  }
  if (signal?.aborted) return [];

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
    const franchise = franchiseByReleaseCache.get(hit.id) ?? null;
    const fallbackTitle = hit.name?.english || hit.name?.main || '';
    const fKey = franchiseKey(fallbackTitle);
    const groupKey = franchise
      ? `fr:${franchise.id}`
      : fKey
        ? `title:${fKey}`
        : `rel:${hit.id}`;

    const candidateItem = releaseCandidateToSearchItem(hit, franchise, 1);
    const existing = groups.get(groupKey);
    if (!existing) {
      orderedKeys.push(groupKey);
      groups.set(groupKey, {
        item: candidateItem,
        earliestYear: hit.year ?? 9999,
        count: 1,
        hasFranchise: Boolean(franchise)
      });
      continue;
    }

    existing.count += 1;
    if (!existing.item.score && candidateItem.score) {
      existing.item.score = candidateItem.score;
    }
    if (!existing.item.episodesTotal && candidateItem.episodesTotal) {
      existing.item.episodesTotal = candidateItem.episodesTotal;
    }
    if ((!existing.item.genres || existing.item.genres.length === 0) && candidateItem.genres) {
      existing.item.genres = candidateItem.genres;
    }
    if (!existing.hasFranchise) {
      const hitYear = hit.year ?? 9999;
      if (hitYear < existing.earliestYear) {
        const prevScore = existing.item.score;
        const prevEp = existing.item.episodesTotal;
        const prevGenres = existing.item.genres;
        existing.earliestYear = hitYear;
        existing.item = releaseCandidateToSearchItem(hit, franchise, existing.count);
        if (!existing.item.score && prevScore) existing.item.score = prevScore;
        if (!existing.item.episodesTotal && prevEp) existing.item.episodesTotal = prevEp;
        if ((!existing.item.genres || existing.item.genres.length === 0) && prevGenres) {
          existing.item.genres = prevGenres;
        }
      } else {
        existing.item.seasonCount = Math.max(existing.item.seasonCount, existing.count);
      }
    }
  }

  const grouped = orderedKeys
    .map((k) => groups.get(k)!.item)
    .filter((item) => {
      if (o.fromYear && item.year && item.year < o.fromYear) return false;
      if (o.toYear && item.year && item.year > o.toYear) return false;
      if (o.minScore && o.minScore > 0) {
        if (!item.score || item.score < o.minScore) return false;
      }
      if (o.minEpisodes && o.minEpisodes > 1) {
        if (item.episodesTotal != null && item.episodesTotal < o.minEpisodes) return false;
      }
      if (o.maxEpisodes && o.maxEpisodes > 0) {
        if (item.episodesTotal != null && item.episodesTotal > o.maxEpisodes) return false;
      }
      return true;
    })
    .slice(0, 50);
  if (grouped.length > 0) {
    groupedSearchCache.set(cacheKey, grouped);
  }
  return grouped;
}

export function buildPrimaryVoiceOption(rel: PlayableRelease): VoiceTrackOption {
  const provider = rel.provider ?? 'aniliberty';
  const cast = rel.voiceActors?.slice(0, 3).join(', ');
  const epPart = rel.episodes.length > 0 ? `${rel.episodes.length} ep` : '';
  const kindTag = provider === 'animevost' ? 'MP4' : provider === 'anixart' ? 'Mirror' : '';
  const sublabel = cast || [epPart, kindTag].filter(Boolean).join(' · ') || undefined;
  return {
    id: rel.alias,
    label: rel.voiceLabel || (provider === 'animevost' ? 'AnimeVost' : 'AniLiberty'),
    sublabel,
    releaseAlias: provider === 'aniliberty' ? rel.alias : undefined,
    provider,
    providerId: rel.id,
    kind: provider === 'animevost' ? 'mp4' : provider === 'anixart' ? 'embed' : 'hls'
  };
}

export async function resolveVoiceTrackRelease(
  voice: VoiceTrackOption,
  baseRelease: PlayableRelease | null,
  target: WatchTarget
): Promise<PlayableRelease> {
  const shikiArt = target.shikiId ? shikiCache.get(target.shikiId) : undefined;
  if (voice.provider === 'animevost') {
    const releaseId = Number(voice.providerId);
    const episodes = await fetchAnimeVostEpisodes(releaseId);
    if (episodes.length === 0) {
      throw new Error(`AnimeVost: нет доступных серий для ${target.title}`);
    }
    return {
      ...(baseRelease ?? {
        id: releaseId,
        title: shikiArt?.english || shikiArt?.name || target.title,
        titleEnglish: shikiArt?.english || shikiArt?.name,
        year: shikiArt?.year,
        poster: shikiArt?.posterOriginal || shikiArt?.posterMain,
        banner: shikiArt?.screenshots?.[0]
      }),
      id: releaseId,
      alias: voice.id,
      provider: 'animevost',
      voiceLabel: voice.label,
      voiceActors: undefined,
      episodes
    };
  }

  if (voice.provider === 'anixart') {
    const releaseId = Number(voice.providerId);
    const typeId = Number(voice.sourceId);
    const episodes = await fetchAnixartEpisodes(releaseId, typeId);
    if (episodes.length === 0) {
      throw new Error(`${voice.label}: нет доступных серий`);
    }
    return {
      ...(baseRelease ?? {
        id: releaseId,
        title: shikiArt?.english || shikiArt?.name || target.title,
        titleEnglish: shikiArt?.english || shikiArt?.name,
        year: shikiArt?.year,
        poster: shikiArt?.posterOriginal || shikiArt?.posterMain,
        banner: shikiArt?.screenshots?.[0]
      }),
      id: releaseId,
      alias: voice.id,
      provider: 'anixart',
      voiceLabel: voice.label,
      voiceActors: undefined,
      episodes
    };
  }

  return resolvePlayableRelease({
    ...target,
    releaseAlias: voice.releaseAlias || voice.id
  });
}

export async function findVoiceOptions(
  primary: PlayableRelease,
  shikiId?: number,
  extraTitles?: string[]
): Promise<VoiceTrackOption[]> {
  const base = buildPrimaryVoiceOption(primary);
  const options: VoiceTrackOption[] = [base];
  const seenIds = new Set<string>([base.id]);
  const shikiArt = shikiId ? shikiCache.get(shikiId) : undefined;
  const searchTitles = dedupeSearchQueries([
    shikiArt?.russian,
    primary.title,
    primary.titleEnglish,
    shikiArt?.english,
    shikiArt?.name,
    ...(extraTitles ?? [])
  ]);
  const primaryProvider = primary.provider ?? 'aniliberty';
  const libSearchTerm = (primary.titleEnglish || primary.title || '').trim();

  const libPromise =
    primaryProvider === 'aniliberty' && shikiId && libSearchTerm.length >= 2
      ? Promise.allSettled([
          fetchJsonWithTimeout<LibSearchCandidate[]>(
            `${ANILIBERTY_BASE}/app/search/releases?query=${encodeURIComponent(libSearchTerm)}`,
            undefined,
            4500
          ),
          fetchJsonWithTimeout<{ data?: LibSearchCandidate[] }>(
            `${ANILIBERTY_BASE}/anime/catalog/releases?f[search]=${encodeURIComponent(libSearchTerm)}&limit=12`,
            undefined,
            4500
          )
        ])
      : Promise.resolve(null);

  const avPromise =
    primaryProvider !== 'animevost'
      ? findAnimeVostMatch(searchTitles, shikiArt?.year ?? primary.year ?? null)
      : Promise.resolve(null);

  const axPromise = findAnixartDiscovery(searchTitles, shikiArt?.year ?? primary.year ?? null, {
    hasNativeAniLiberty: primaryProvider === 'aniliberty',
    hasNativeAnimeVost: primaryProvider === 'animevost'
  });

  const [libSettled, avSettled, axSettled] = await Promise.allSettled([
    libPromise,
    avPromise,
    axPromise
  ]);

  if (libSettled.status === 'fulfilled' && libSettled.value) {
    const [appRes, catRes] = libSettled.value;
    const seenAliases = new Set<string>([primary.alias]);
    const matching: LibSearchCandidate[] = [];
    for (const cand of [
      ...(appRes.status === 'fulfilled' && Array.isArray(appRes.value) ? appRes.value : []),
      ...(catRes.status === 'fulfilled' && Array.isArray(catRes.value?.data) ? catRes.value.data : [])
    ]) {
      if (!cand?.alias || seenAliases.has(cand.alias)) continue;
      if (cand.shikimori?.id !== shikiId) continue;
      seenAliases.add(cand.alias);
      matching.push(cand);
    }

    const extraReleases = await Promise.all(
      matching.slice(0, 3).map((cand) => fetchReleaseByIdOrAlias(cand.alias, undefined, shikiId))
    ).catch(() => []);

    for (const rel of extraReleases) {
      if (!rel || rel.alias === primary.alias || rel.episodes.length === 0 || seenIds.has(rel.alias)) continue;
      seenIds.add(rel.alias);
      const altCast = rel.voiceActors?.slice(0, 3).join(', ');
      options.push({
        id: rel.alias,
        label: rel.title && rel.title !== primary.title ? `AniLiberty · ${rel.title}` : `AniLiberty · ${rel.alias}`,
        sublabel: altCast || `${rel.episodes.length} ep`,
        releaseAlias: rel.alias,
        provider: 'aniliberty',
        providerId: rel.id,
        kind: 'hls'
      });
    }
  }

  let hasNativeAnimeVost = primaryProvider === 'animevost';
  if (avSettled.status === 'fulfilled' && avSettled.value && !seenIds.has(avSettled.value.voice.id)) {
    seenIds.add(avSettled.value.voice.id);
    hasNativeAnimeVost = true;
    options.push(avSettled.value.voice);
  }

  if (axSettled.status === 'fulfilled' && axSettled.value) {
    for (const axVoice of axSettled.value.voices) {
      if (hasNativeAnimeVost && axVoice.label.trim().toLowerCase() === 'animevost') continue;
      if (seenIds.has(axVoice.id)) continue;
      seenIds.add(axVoice.id);
      options.push(axVoice);
    }
  }

  return options;
}

export interface HeroOpeningPreview {
  streamUrl: string;
  streamUrlFast?: string;
  startSec: number;
  endSec: number;
  episodeOrdinal: number;
  isDirectVideo?: boolean;
  aspectRatio?: '16:9' | 'fill';
}

export type HeroOpeningTarget = WatchTarget & {
  openingAlias?: string;
  openingEpisode?: number;
  openingStartOffsetSec?: number;
  customPreviewUrl?: string;
  customPreviewAspect?: '16:9' | 'fill';
};

const heroOpeningCache = new Map<string, HeroOpeningPreview | null>();
const heroOpeningPending = new Map<string, Promise<HeroOpeningPreview | null>>();
const warmedPlaylists = new Set<string>();

function buildOpeningCacheKey(target: HeroOpeningTarget): string {
  const previewAlias = target.openingAlias ?? target.releaseAlias;
  const offsetTag =
    target.openingStartOffsetSec != null ? String(Math.max(0, target.openingStartOffsetSec)) : 'mid';
  return `${previewAlias || (target.shikiId ? `shiki:${target.shikiId}` : target.title.toLowerCase())}#${target.openingEpisode ?? 'auto'}+${offsetTag}`;
}

export function getCachedHeroOpeningPreview(
  target: HeroOpeningTarget
): HeroOpeningPreview | null | undefined {
  if (target.customPreviewUrl) {
    return {
      streamUrl: target.customPreviewUrl,
      streamUrlFast: target.customPreviewUrl,
      startSec: 0,
      endSec: 48,
      episodeOrdinal: 0,
      isDirectVideo: true,
      aspectRatio: target.customPreviewAspect ?? 'fill'
    };
  }
  const key = buildOpeningCacheKey(target);
  if (!heroOpeningCache.has(key)) return undefined;
  return heroOpeningCache.get(key) ?? null;
}

async function warmOpeningHlsCache(clip: HeroOpeningPreview): Promise<void> {
  const url = clip.streamUrlFast || clip.streamUrl;
  if (!url || clip.isDirectVideo) return;
  const cacheKey = `${url}#${Math.floor(clip.startSec)}`;
  if (warmedPlaylists.has(cacheKey)) return;
  warmedPlaylists.add(cacheKey);

  try {
    const res = await fetch(url);
    if (!res.ok) return;
    const text = await res.text();
    const lines = text.split(/\r?\n/);
    let accSec = 0;
    let pendingDur = 0;
    let warmedSegments = 0;

    for (const rawLine of lines) {
      const line = rawLine.trim();
      if (!line) continue;
      if (line.startsWith('#EXTINF:')) {
        const durMatch = line.match(/#EXTINF:([\d.]+)/);
        pendingDur = durMatch ? parseFloat(durMatch[1]) || 0 : 0;
        continue;
      }
      if (line.startsWith('#')) continue;

      const segEnd = accSec + pendingDur;
      accSec = segEnd;
      pendingDur = 0;

      if (segEnd >= clip.startSec) {
        const segUrl = new URL(line, url).href;
        void fetch(segUrl).catch(() => {});
        warmedSegments += 1;
        if (warmedSegments >= 2) break;
      }
    }
  } catch {
    // Ignore background cache warming errors
  }
}

export function warmHeroOpeningPreview(target: HeroOpeningTarget): void {
  const cached = getCachedHeroOpeningPreview(target);
  if (cached) {
    void warmOpeningHlsCache(cached);
    return;
  }
  if (cached === null) return;
  void resolveHeroOpeningPreview(target)
    .then((clip) => {
      if (clip) {
        void warmOpeningHlsCache(clip);
      }
    })
    .catch(() => {});
}

export async function resolveHeroOpeningPreview(
  target: HeroOpeningTarget
): Promise<HeroOpeningPreview | null> {
  if (target.customPreviewUrl) {
    return {
      streamUrl: target.customPreviewUrl,
      streamUrlFast: target.customPreviewUrl,
      startSec: 0,
      endSec: 48,
      episodeOrdinal: 0,
      isDirectVideo: true,
      aspectRatio: target.customPreviewAspect ?? 'fill'
    };
  }
  const previewAlias = target.openingAlias ?? target.releaseAlias;
  const key = buildOpeningCacheKey(target);
  if (heroOpeningCache.has(key)) {
    return heroOpeningCache.get(key) ?? null;
  }
  const existing = heroOpeningPending.get(key);
  if (existing) {
    return existing;
  }

  const req = (async (): Promise<HeroOpeningPreview | null> => {
    try {
      const rel = await resolvePlayableRelease({
        ...target,
        releaseAlias: previewAlias,
        shikiId: target.openingAlias ? undefined : target.shikiId
      });
      const hasValidOpening = (ep: (typeof rel.episodes)[number], minDuration: number) =>
        ep.opening?.start != null &&
        ep.opening?.stop != null &&
        ep.opening.stop > ep.opening.start + minDuration &&
        Boolean(ep.hls_720 || ep.hls_480 || ep.hls_1080);

      // Prefer episodes 2+ with full openings (since Episode 1 often omits OP or places it over ending credits).
      const opEp =
        (target.openingEpisode != null
          ? rel.episodes.find((ep) => ep.ordinal === target.openingEpisode && hasValidOpening(ep, 6))
          : undefined) ??
        rel.episodes.find((ep) => ep.ordinal >= 2 && hasValidOpening(ep, 45)) ??
        rel.episodes.find((ep) => ep.ordinal >= 2 && hasValidOpening(ep, 18)) ??
        rel.episodes.find((ep) => ep.ordinal >= 2 && hasValidOpening(ep, 6)) ??
        rel.episodes.find((ep) => hasValidOpening(ep, 18)) ??
        rel.episodes.find((ep) => hasValidOpening(ep, 6));

      if (opEp?.opening?.start != null && opEp.opening.stop != null) {
        const streamUrl = opEp.hls_720 || opEp.hls_480 || opEp.hls_1080 || '';
        const streamUrlFast = opEp.hls_480 || opEp.hls_720 || opEp.hls_1080 || '';
        if (streamUrl) {
          const opLen = Math.max(0, opEp.opening.stop - opEp.opening.start);
          // Start around the middle (~46%) of the opening by default to skip intro sponsor/staff credits and drop straight into the chorus.
          const defaultMidOffset =
            opLen >= 50
              ? Math.round(opLen * 0.46)
              : opLen >= 24
                ? Math.round(opLen * 0.4)
                : Math.max(0, Math.round(opLen * 0.25));
          const effectiveOffset =
            target.openingStartOffsetSec != null
              ? Math.max(0, target.openingStartOffsetSec)
              : defaultMidOffset;
          const trimmedStart = Math.min(
            opEp.opening.start + effectiveOffset,
            Math.max(opEp.opening.start, opEp.opening.stop - 14)
          );
          const result: HeroOpeningPreview = {
            streamUrl,
            streamUrlFast: streamUrlFast || streamUrl,
            startSec: trimmedStart,
            endSec: opEp.opening.stop,
            episodeOrdinal: opEp.ordinal
          };
          heroOpeningCache.set(key, result);
          if (previewAlias && rel.alias && rel.alias !== previewAlias) {
            heroOpeningCache.set(buildOpeningCacheKey({ ...target, releaseAlias: rel.alias }), result);
          }
          return result;
        }
      }

      // If no opening markers exist on the release, prefer Episode 2 (where regular TV OP sits around 1:35) over Episode 1.
      const fallbackEp =
        rel.episodes.find((ep) => ep.ordinal >= 2 && Boolean(ep.hls_720 || ep.hls_480 || ep.hls_1080)) ??
        rel.episodes.find((ep) => Boolean(ep.hls_720 || ep.hls_480 || ep.hls_1080));
      const fallbackUrl = fallbackEp?.hls_720 || fallbackEp?.hls_480 || fallbackEp?.hls_1080;
      const fallbackFast = fallbackEp?.hls_480 || fallbackEp?.hls_720 || fallbackEp?.hls_1080;
      if (fallbackEp && fallbackUrl && (fallbackEp.duration === 0 || fallbackEp.duration >= 150)) {
        const result: HeroOpeningPreview = {
          streamUrl: fallbackUrl,
          streamUrlFast: fallbackFast || fallbackUrl,
          startSec: 96,
          endSec: 138,
          episodeOrdinal: fallbackEp.ordinal
        };
        heroOpeningCache.set(key, result);
        return result;
      }

      heroOpeningCache.set(key, null);
      return null;
    } catch {
      return null;
    } finally {
      heroOpeningPending.delete(key);
    }
  })();

  heroOpeningPending.set(key, req);
  return req;
}
