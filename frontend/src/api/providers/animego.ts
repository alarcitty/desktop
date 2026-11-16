import { getWailsMilaBridge } from '../bridge';
import type { VoiceTrackOption } from '../../marlakit/player/types';
import type { PlayableEpisode } from '../anime';
import { dedupeSearchQueries, fetchJsonWithTimeout, scoreTitleSimilarity } from './shared';

export interface RawAnimeGOSearchItem {
  id: number;
  title: string;
  originalTitle?: string;
  year?: number;
  poster?: string;
  slug: string;
  url: string;
}

export interface RawAnimeGODub {
  translationId: string;
  title: string;
  providerSlug: string;
  embedUrl: string;
}

export interface RawAnimeGOPlayer {
  animeId: number;
  episodes: Array<{ ordinal: number; episodeId: number }>;
  dubs: RawAnimeGODub[];
}

export interface AnimeGODubOption {
  translationId: string;
  embedUrl: string;
  voice: VoiceTrackOption;
  episodes: PlayableEpisode[];
}

export interface AnimeGOMatch {
  animeId: number;
  title: string;
  poster?: string;
  year?: number;
  dubs: AnimeGODubOption[];
}

const searchCache = new Map<string, RawAnimeGOSearchItem[]>();
const playerCache = new Map<number, RawAnimeGOPlayer>();
const episodesByKey = new Map<string, PlayableEpisode[]>();
const embedByKey = new Map<string, string>();

function getWailsAnimeGO() {
  return getWailsMilaBridge();
}

export function buildAniboomMasterUrl(embedUrl: string, ordinal: number): string {
  const params = new URLSearchParams();
  params.set('embed', embedUrl);
  params.set('ep', String(ordinal));
  return `/ext/aniboom/master.m3u8?${params.toString()}`;
}

export function buildAnimeGOKodikMasterUrl(
  animeId: number,
  episodeId: number,
  translationId: string,
  embedUrl: string
): string {
  const params = new URLSearchParams();
  if (episodeId > 0 && translationId) {
    params.set('animego_id', String(animeId));
    params.set('ep_id', String(episodeId));
    params.set('tr', translationId);
  }
  if (embedUrl) {
    params.set('embed', embedUrl);
  }
  params.set('ref', 'https://animego.me/');
  return `/ext/kodik/master.m3u8?${params.toString()}`;
}

function buildAnimeGOEpisodeStream(
  animeId: number,
  ep: { ordinal: number; episodeId: number },
  trId: string,
  embedUrl: string,
  providerSlug: string
): Pick<PlayableEpisode, 'hls_1080' | 'hls_720'> {
  if (providerSlug === 'kodik') {
    return {
      hls_720: buildAnimeGOKodikMasterUrl(animeId, ep.episodeId, trId, embedUrl)
    };
  }
  return {
    hls_1080: buildAniboomMasterUrl(embedUrl, ep.ordinal)
  };
}

export function normalizeAnimeGODubLabel(rawTitle: string): { label: string; isSub: boolean } {
  const trimmed = rawTitle.trim() || 'AniBoom';
  const lower = trimmed.toLowerCase();
  if (lower.includes('субтитр') || lower === 'sub' || lower === 'subs' || lower.includes('озвучка отсутствует')) {
    return { label: 'Original (Sub)', isSub: true };
  }
  if (lower === 'anilibriatv' || lower === 'anilibria.tv') {
    return { label: 'AniLibria', isSub: false };
  }
  return { label: trimmed, isSub: false };
}

export async function searchAnimeGO(query: string): Promise<RawAnimeGOSearchItem[]> {
  const clean = query.trim();
  const key = clean.toLowerCase();
  if (key.length < 2) return [];
  const cached = searchCache.get(key);
  if (cached) return cached;

  let items: RawAnimeGOSearchItem[] = [];
  const bridge = getWailsAnimeGO();
  if (bridge?.AnimeGOSearch) {
    try {
      const raw = await bridge.AnimeGOSearch(clean);
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        items = parsed as RawAnimeGOSearchItem[];
      }
    } catch {
      items = [];
    }
  }
  if (items.length === 0) {
    try {
      const res = await fetchJsonWithTimeout<RawAnimeGOSearchItem[]>(
        `/ext/animego/search?q=${encodeURIComponent(clean)}`,
        undefined,
        7500
      );
      if (Array.isArray(res)) {
        items = res;
      }
    } catch {
      // Ignore HTTP fallback failures when desktop IPC already ran or network is down
    }
  }
  searchCache.set(key, items);
  return items;
}

export async function fetchAnimeGOPlayer(animeId: number): Promise<RawAnimeGOPlayer | null> {
  if (!Number.isFinite(animeId) || animeId <= 0) return null;
  const cached = playerCache.get(animeId);
  if (cached) return cached;

  let result: RawAnimeGOPlayer | null = null;
  const bridge = getWailsAnimeGO();
  if (bridge?.AnimeGOPlayer) {
    try {
      const raw = await bridge.AnimeGOPlayer(animeId);
      const parsed = JSON.parse(raw) as RawAnimeGOPlayer;
      if (parsed && parsed.animeId > 0) {
        result = parsed;
      }
    } catch {
      result = null;
    }
  }
  if (!result) {
    try {
      result = await fetchJsonWithTimeout<RawAnimeGOPlayer>(
        `/ext/animego/player?id=${encodeURIComponent(String(animeId))}`,
        undefined,
        8500
      );
    } catch {
      result = null;
    }
  }
  if (result && Array.isArray(result.dubs) && result.dubs.length > 0) {
    playerCache.set(animeId, result);
  }
  return result;
}

export async function fetchAnimeGOEpisodes(
  animeId: number,
  translationId: string,
  embedUrl = '',
  providerSlug = ''
): Promise<PlayableEpisode[]> {
  const cacheKey = `${animeId}:${translationId}`;
  const cached = episodesByKey.get(cacheKey);
  if (cached) return cached;

  const playerData = await fetchAnimeGOPlayer(animeId);
  if (!playerData) return [];

  const matchedDub = playerData.dubs.find((d) => d.translationId === translationId);
  const resolvedEmbed = embedUrl || embedByKey.get(cacheKey) || matchedDub?.embedUrl || '';
  const resolvedSlug = providerSlug || matchedDub?.providerSlug || (resolvedEmbed.includes('kodik') ? 'kodik' : 'aniboom');
  if (!resolvedEmbed) return [];
  embedByKey.set(cacheKey, resolvedEmbed);

  const epList =
    playerData.episodes.length > 0
      ? playerData.episodes
      : [{ ordinal: 1, episodeId: 0 }];

  const episodes: PlayableEpisode[] = epList.map((ep) => ({
    id: `animego:${animeId}:${translationId}:${ep.ordinal}`,
    ordinal: ep.ordinal,
    name: `Episode ${ep.ordinal}`,
    duration: 0,
    ...buildAnimeGOEpisodeStream(animeId, ep, translationId, resolvedEmbed, resolvedSlug)
  }));

  if (episodes.length > 0) {
    episodesByKey.set(cacheKey, episodes);
  }
  return episodes;
}

export async function findAnimeGOMatch(
  targetTitles: string[],
  targetYear?: number | null
): Promise<AnimeGOMatch | null> {
  const queries = dedupeSearchQueries(targetTitles).slice(0, 4);
  if (queries.length === 0) return null;

  let bestItem: RawAnimeGOSearchItem | null = null;
  let bestScore = -1;

  for (const q of queries) {
    try {
      const candidates = await searchAnimeGO(q);
      for (const cand of candidates) {
        const candTitles = [cand.title ?? '', cand.originalTitle ?? '']
          .map((s) => s.trim())
          .filter(Boolean);
        const score = scoreTitleSimilarity(candTitles, targetTitles, cand.year ?? null, targetYear);
        if (score > bestScore) {
          bestScore = score;
          bestItem = cand;
        }
      }
      if (bestScore >= 850) break;
    } catch {
      // Try next query variant
    }
  }

  if (!bestItem || bestScore < 120) return null;
  const playerData = await fetchAnimeGOPlayer(bestItem.id);
  if (!playerData || !Array.isArray(playerData.dubs) || playerData.dubs.length === 0) {
    return null;
  }

  const epList =
    playerData.episodes.length > 0
      ? playerData.episodes
      : [{ ordinal: 1, episodeId: 0 }];

  const dubs: AnimeGODubOption[] = [];
  const seenLabels = new Set<string>();
  for (const dub of playerData.dubs) {
    if (!dub.embedUrl) continue;
    const { label, isSub } = normalizeAnimeGODubLabel(dub.title);
    const labelKey = `${label.toLowerCase()}:${dub.translationId}`;
    if (seenLabels.has(labelKey)) continue;
    seenLabels.add(labelKey);

    const trId = dub.translationId || String(dubs.length + 1);
    const cacheKey = `${bestItem.id}:${trId}`;
    embedByKey.set(cacheKey, dub.embedUrl);
    const isKodik = dub.providerSlug === 'kodik';

    const episodes: PlayableEpisode[] = epList.map((ep) => ({
      id: `animego:${bestItem.id}:${trId}:${ep.ordinal}`,
      ordinal: ep.ordinal,
      name: `Episode ${ep.ordinal}`,
      duration: 0,
      ...buildAnimeGOEpisodeStream(bestItem.id, ep, trId, dub.embedUrl, dub.providerSlug)
    }));
    episodesByKey.set(cacheKey, episodes);

    const qualityTag = isKodik
      ? isSub
        ? 'Kodik 720p Sub'
        : 'Kodik 720p'
      : isSub
        ? '1080p Original'
        : '1080p HLS';

    dubs.push({
      translationId: trId,
      embedUrl: dub.embedUrl,
      voice: {
        id: `animego:${bestItem.id}:${trId}`,
        label,
        sublabel: `${episodes.length} ep · ${qualityTag}`,
        provider: 'animego',
        providerId: bestItem.id,
        sourceId: `${dub.providerSlug || 'aniboom'}|${trId}|${dub.embedUrl}`,
        kind: 'hls',
        isSub
      },
      episodes
    });
  }

  if (dubs.length === 0) return null;

  return {
    animeId: bestItem.id,
    title: bestItem.title || bestItem.originalTitle || queries[0],
    poster: bestItem.poster,
    year: bestItem.year,
    dubs
  };
}
