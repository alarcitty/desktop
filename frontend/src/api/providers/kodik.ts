import type { VoiceTrackOption } from '../../marlakit/player/types';
import type { PlayableEpisode } from '../anime';
import { dedupeSearchQueries, fetchJsonWithTimeout, scoreTitleSimilarity } from './shared';

const YANI_PROXY_BASE = '/ext/yani';
const YANI_REFERER = 'https://yani.tv/';

interface RawYaniAnimeItem {
  anime_id: number;
  title?: string;
  year?: number;
  poster?: {
    fullsize?: string;
    big?: string;
    medium?: string;
  };
  remote_ids?: {
    shikimori_id?: number;
  };
}

interface RawYaniVideoItem {
  video_id?: number;
  number?: string;
  iframe_url?: string;
  data?: {
    player?: string;
    dubbing?: string;
  };
}

interface RawYaniResponse<T> {
  response?: T;
}

export interface KodikDubOption {
  studioKey: string;
  voice: VoiceTrackOption;
  episodes: PlayableEpisode[];
}

export interface KodikMatch {
  animeId: number;
  title: string;
  poster?: string;
  year?: number;
  dubs: KodikDubOption[];
}

const searchCache = new Map<string, RawYaniAnimeItem[]>();
const dubsCache = new Map<number, KodikDubOption[]>();
const matchCache = new Map<string, KodikMatch | null>();

const STUDIO_ALIAS_MAP: Record<string, string> = {
  studioband: 'студийная банда',
  'studio band': 'студийная банда',
  'студийная банда': 'студийная банда',
  'anilibria tv': 'anilibria',
  anilibriatv: 'anilibria',
  anilibria: 'anilibria',
  aniliberty: 'anilibria',
  'dream cast': 'dream cast',
  dreamcast: 'dream cast',
  animevost: 'animevost',
  'anime vost': 'animevost',
  anidub: 'anidub',
  'shiza project': 'shiza project',
  shiza: 'shiza project',
  crunchyroll: 'crunchyroll',
  '2x2': '2x2'
};

const POPULAR_STUDIO_ORDER: string[] = [
  'anidub',
  'студийная банда',
  'anilibria',
  'dream cast',
  'shiza project',
  'crunchyroll',
  'flarrow films',
  '2x2',
  'jam',
  'anistar',
  'animevost',
  'aniplay',
  'animaunt',
  'kansai',
  'sovetromantica',
  'комната диди',
  'anifilm',
  'anirise'
];

export function buildKodikMasterUrl(embedUrl: string, donorReferer = YANI_REFERER): string {
  const cleanEmbed = embedUrl.trim().startsWith('//') ? `https:${embedUrl.trim()}` : embedUrl.trim();
  const params = new URLSearchParams();
  params.set('embed', cleanEmbed);
  params.set('ref', donorReferer);
  return `/ext/kodik/master.m3u8?${params.toString()}`;
}

export function normalizeCanonicalStudioKey(rawLabel: string, isSub = false): string {
  let clean = rawLabel
    .toLowerCase()
    .replace(/ё/g, 'е')
    .replace(/^озвучка\s+/i, '')
    .replace(/^субтитры\s+/i, '')
    .replace(/^sub\s*·\s*/i, '')
    .replace(/[^\p{L}\p{N}\s]+/gu, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  if (isSub && (!clean || clean === 'original' || clean === 'original sub' || clean === 'субтитры')) {
    return 'sub:__generic__';
  }
  const canonical = STUDIO_ALIAS_MAP[clean] ?? clean;
  return isSub ? `sub:${canonical}` : canonical;
}

function parseKodikDubbingLabel(rawDubbing?: string): { label: string; isSub: boolean } {
  const raw = (rawDubbing ?? '').trim() || 'Kodik';
  const lower = raw.toLowerCase();
  if (lower.startsWith('субтитры') || lower.includes('subtitles')) {
    const stripped = raw.replace(/^субтитры\s*/i, '').trim();
    if (!stripped || stripped.toLowerCase() === 'субтитры') {
      return { label: 'Original (Sub)', isSub: true };
    }
    return { label: `Sub · ${stripped}`, isSub: true };
  }
  const cleaned = raw.replace(/^озвучка\s+/i, '').trim() || raw;
  if (cleaned.toLowerCase() === 'anilibriatv' || cleaned.toLowerCase() === 'anilibria.tv') {
    return { label: 'AniLibria', isSub: false };
  }
  return { label: cleaned, isSub: false };
}

function parseEpisodeOrdinal(rawNumber?: string, fallbackOrdinal = 1): number {
  const clean = (rawNumber ?? '').trim().replace(',', '.');
  const m = clean.match(/\d+(?:\.\d+)?/);
  if (!m) return fallbackOrdinal;
  const n = Number(m[0]);
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : fallbackOrdinal;
}

function studioPopularityRank(canonicalKey: string): number {
  const bare = canonicalKey.replace(/^sub:/, '');
  const idx = POPULAR_STUDIO_ORDER.indexOf(bare);
  return idx >= 0 ? idx : POPULAR_STUDIO_ORDER.length + 100;
}

function normalizeYaniPoster(raw?: string): string | undefined {
  const trimmed = raw?.trim();
  if (!trimmed) return undefined;
  return trimmed.startsWith('//') ? `https:${trimmed}` : trimmed;
}

export async function searchYaniAnime(query: string): Promise<RawYaniAnimeItem[]> {
  const clean = query.trim();
  const key = clean.toLowerCase();
  if (key.length < 2) return [];
  const cached = searchCache.get(key);
  if (cached) return cached;

  try {
    const payload = await fetchJsonWithTimeout<RawYaniResponse<RawYaniAnimeItem[]>>(
      `${YANI_PROXY_BASE}/anime?q=${encodeURIComponent(clean)}&limit=12`,
      undefined,
      7500
    );
    const items = Array.isArray(payload?.response) ? payload.response : [];
    searchCache.set(key, items);
    return items;
  } catch {
    return [];
  }
}

export async function fetchYaniKodikDubs(yaniAnimeId: number): Promise<KodikDubOption[]> {
  if (!Number.isFinite(yaniAnimeId) || yaniAnimeId <= 0) return [];
  const cached = dubsCache.get(yaniAnimeId);
  if (cached) return cached;

  let rawVideos: RawYaniVideoItem[] = [];
  try {
    const payload = await fetchJsonWithTimeout<RawYaniResponse<RawYaniVideoItem[]>>(
      `${YANI_PROXY_BASE}/anime/${encodeURIComponent(String(yaniAnimeId))}/videos`,
      undefined,
      9000
    );
    rawVideos = Array.isArray(payload?.response) ? payload.response : [];
  } catch {
    return [];
  }

  const grouped = new Map<
    string,
    {
      label: string;
      isSub: boolean;
      byOrdinal: Map<number, string>;
    }
  >();

  for (const item of rawVideos) {
    const playerName = (item.data?.player ?? '').toLowerCase();
    const iframe = item.iframe_url?.trim() ?? '';
    if (!playerName.includes('kodik') || !iframe) continue;

    const { label, isSub } = parseKodikDubbingLabel(item.data?.dubbing);
    const studioKey = normalizeCanonicalStudioKey(label, isSub);
    if (!studioKey) continue;

    let bucket = grouped.get(studioKey);
    if (!bucket) {
      bucket = { label, isSub, byOrdinal: new Map() };
      grouped.set(studioKey, bucket);
    }

    const ordinal = parseEpisodeOrdinal(item.number, bucket.byOrdinal.size + 1);
    if (!bucket.byOrdinal.has(ordinal)) {
      bucket.byOrdinal.set(ordinal, iframe);
    }
  }

  let maxEpCount = 0;
  for (const bucket of grouped.values()) {
    if (bucket.byOrdinal.size > maxEpCount) {
      maxEpCount = bucket.byOrdinal.size;
    }
  }
  const fullThreshold = Math.max(1, Math.floor(maxEpCount * 0.75));

  const dubs: KodikDubOption[] = [];
  for (const [studioKey, bucket] of grouped.entries()) {
    const ordinals = Array.from(bucket.byOrdinal.keys()).sort((a, b) => a - b);
    if (ordinals.length === 0) continue;

    const episodes: PlayableEpisode[] = ordinals.map((ordinal) => {
      const iframe = bucket.byOrdinal.get(ordinal)!;
      return {
        id: `kodik:${yaniAnimeId}:${studioKey}:${ordinal}`,
        ordinal,
        name: `Episode ${ordinal}`,
        duration: 0,
        hls_720: buildKodikMasterUrl(iframe, YANI_REFERER)
      };
    });

    dubs.push({
      studioKey,
      voice: {
        id: `kodik:${yaniAnimeId}:${studioKey}`,
        label: bucket.label,
        sublabel: `${episodes.length} ep · ${bucket.isSub ? 'Kodik 720p Sub' : 'Kodik 720p'}`,
        provider: 'kodik',
        providerId: yaniAnimeId,
        sourceId: studioKey,
        kind: 'hls',
        isSub: bucket.isSub
      },
      episodes
    });
  }

  // Sort: full/near-full dubs first, then popular studios, then full subs, then incomplete dubs/subs.
  dubs.sort((a, b) => {
    const aFull = a.episodes.length >= fullThreshold ? 0 : 1;
    const bFull = b.episodes.length >= fullThreshold ? 0 : 1;
    const aTier = (a.voice.isSub ? 2 : 0) + aFull;
    const bTier = (b.voice.isSub ? 2 : 0) + bFull;
    if (aTier !== bTier) return aTier - bTier;

    const aRank = studioPopularityRank(a.studioKey);
    const bRank = studioPopularityRank(b.studioKey);
    if (aRank !== bRank) return aRank - bRank;

    if (b.episodes.length !== a.episodes.length) {
      return b.episodes.length - a.episodes.length;
    }
    return a.voice.label.localeCompare(b.voice.label, 'ru');
  });

  if (dubs.length > 0) {
    dubsCache.set(yaniAnimeId, dubs);
  }
  return dubs;
}

export async function fetchKodikEpisodes(yaniAnimeId: number, studioKey: string): Promise<PlayableEpisode[]> {
  const dubs = await fetchYaniKodikDubs(yaniAnimeId);
  return dubs.find((d) => d.studioKey === studioKey)?.episodes ?? [];
}

export async function findKodikMatch(
  targetTitles: string[],
  targetYear?: number | null,
  shikiId?: number
): Promise<KodikMatch | null> {
  const queries = dedupeSearchQueries(targetTitles).slice(0, 4);
  if (queries.length === 0) return null;

  const cacheKey = `${shikiId ?? 0}:${targetYear ?? 0}:${queries.join('|').toLowerCase()}`;
  if (matchCache.has(cacheKey)) {
    return matchCache.get(cacheKey) ?? null;
  }

  let bestItem: RawYaniAnimeItem | null = null;
  let bestScore = -1;

  for (const q of queries) {
    const candidates = await searchYaniAnime(q);
    for (const cand of candidates) {
      if (!cand?.anime_id) continue;
      if (shikiId && cand.remote_ids?.shikimori_id === shikiId) {
        bestItem = cand;
        bestScore = 100_000;
        break;
      }
      const titleList = cand.title ? [cand.title] : [];
      const score = scoreTitleSimilarity(titleList, targetTitles, cand.year ?? null, targetYear);
      if (score > bestScore) {
        bestScore = score;
        bestItem = cand;
      }
    }
    if (bestScore >= 100_000 || bestScore >= 850) break;
  }

  if (!bestItem || bestScore < 140) {
    matchCache.set(cacheKey, null);
    return null;
  }

  const dubs = await fetchYaniKodikDubs(bestItem.anime_id);
  if (dubs.length === 0) {
    matchCache.set(cacheKey, null);
    return null;
  }

  const match: KodikMatch = {
    animeId: bestItem.anime_id,
    title: bestItem.title?.trim() || queries[0],
    poster: normalizeYaniPoster(bestItem.poster?.fullsize || bestItem.poster?.big || bestItem.poster?.medium),
    year: bestItem.year || targetYear || undefined,
    dubs
  };
  matchCache.set(cacheKey, match);
  return match;
}
