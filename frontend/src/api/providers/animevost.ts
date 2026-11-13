import type { VoiceTrackOption } from '../../marlakit/player/types';
import type { PlayableEpisode } from '../anime';
import { dedupeSearchQueries, fetchJsonWithTimeout, scoreTitleSimilarity } from './shared';

const ANIMEVOST_PROXY_BASE = '/ext/animevost';

interface WailsMilaBridge {
  AnimeVostSearch?: (query: string) => Promise<string>;
  AnimeVostPlaylist?: (id: number) => Promise<string>;
}

function getWailsMilaBridge(): WailsMilaBridge | null {
  if (typeof window === 'undefined') return null;
  const goObj = (window as { go?: { mila?: { Mila?: WailsMilaBridge } } }).go;
  return goObj?.mila?.Mila ?? null;
}

async function fetchAnimeVostViaProxy<T>(path: string, formBody: string, timeoutMs: number): Promise<T> {
  return fetchJsonWithTimeout<T>(
    `${ANIMEVOST_PROXY_BASE}${path}`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: formBody
    },
    timeoutMs
  );
}

interface RawAnimeVostItem {
  id: number;
  title?: string;
  year?: string | number;
  urlImagePreview?: string;
  type?: string;
}

interface RawAnimeVostSearchResponse {
  data?: RawAnimeVostItem[];
}

interface RawAnimeVostPlaylistItem {
  name?: string;
  hd?: string | null;
  std?: string | null;
  preview?: string | null;
}

const avSearchCache = new Map<string, RawAnimeVostItem[]>();
const avPlaylistCache = new Map<number, PlayableEpisode[]>();

function upgradeAnimetopUrl(raw?: string | null): string | null {
  const trimmed = raw?.trim();
  if (!trimmed) return null;
  if (trimmed.startsWith('http://video.animetop.info/') || trimmed.startsWith('http://media.animetop.info/')) {
    return trimmed.replace(/^http:\/\//i, 'https://');
  }
  return trimmed.startsWith('//') ? `https:${trimmed}` : trimmed;
}

function splitAnimeVostTitles(rawTitle?: string): string[] {
  if (!rawTitle) return [];
  const cleaned = rawTitle.replace(/\[[^\]]*\]\s*$/u, '').trim();
  return cleaned
    .split('/')
    .map((part) => part.trim())
    .filter(Boolean);
}

function parseEpisodeOrdinal(name: string | undefined, fallbackIdx: number): number {
  if (!name) return fallbackIdx + 1;
  const leading = name.match(/(\d+)/);
  if (leading) {
    const n = Number(leading[1]);
    if (Number.isFinite(n) && n > 0) return n;
  }
  return fallbackIdx + 1;
}

export async function searchAnimeVost(query: string): Promise<RawAnimeVostItem[]> {
  const clean = query.trim();
  const key = clean.toLowerCase();
  if (key.length < 2) return [];
  const cached = avSearchCache.get(key);
  if (cached) return cached;

  const bridge = getWailsMilaBridge();
  let payload: RawAnimeVostSearchResponse;
  if (bridge?.AnimeVostSearch) {
    const rawJson = await bridge.AnimeVostSearch(clean);
    payload = JSON.parse(rawJson) as RawAnimeVostSearchResponse;
  } else {
    payload = await fetchAnimeVostViaProxy<RawAnimeVostSearchResponse>(
      '/search',
      `name=${encodeURIComponent(clean)}`,
      6000
    );
  }
  const items = Array.isArray(payload?.data) ? payload.data.filter((item) => item?.id) : [];
  avSearchCache.set(key, items);
  return items;
}

export async function fetchAnimeVostEpisodes(releaseId: number): Promise<PlayableEpisode[]> {
  const cached = avPlaylistCache.get(releaseId);
  if (cached) return cached;

  const bridge = getWailsMilaBridge();
  let list: RawAnimeVostPlaylistItem[];
  if (bridge?.AnimeVostPlaylist) {
    const rawJson = await bridge.AnimeVostPlaylist(releaseId);
    list = JSON.parse(rawJson) as RawAnimeVostPlaylistItem[];
  } else {
    list = await fetchAnimeVostViaProxy<RawAnimeVostPlaylistItem[]>(
      '/playlist',
      `id=${encodeURIComponent(String(releaseId))}`,
      6500
    );
  }
  if (!Array.isArray(list)) return [];

  const episodes: PlayableEpisode[] = [];
  for (let i = 0; i < list.length; i++) {
    const ep = list[i];
    const mp4_720 = upgradeAnimetopUrl(ep.hd);
    const mp4_480 = upgradeAnimetopUrl(ep.std);
    if (!mp4_720 && !mp4_480) continue;
    const ordinal = parseEpisodeOrdinal(ep.name, i);
    episodes.push({
      id: `animevost:${releaseId}:${ordinal}`,
      ordinal,
      name: ep.name?.trim() || undefined,
      duration: 0,
      preview: upgradeAnimetopUrl(ep.preview) ?? undefined,
      mp4_720,
      mp4_480
    });
  }

  episodes.sort((a, b) => a.ordinal - b.ordinal);
  if (episodes.length > 0) {
    avPlaylistCache.set(releaseId, episodes);
  }
  return episodes;
}

export interface AnimeVostMatch {
  releaseId: number;
  title: string;
  poster?: string;
  year?: number;
  episodes: PlayableEpisode[];
  voice: VoiceTrackOption;
}

export async function findAnimeVostMatch(
  targetTitles: string[],
  targetYear?: number | null
): Promise<AnimeVostMatch | null> {
  const queries = dedupeSearchQueries(targetTitles).slice(0, 2);
  if (queries.length === 0) return null;

  let bestItem: RawAnimeVostItem | null = null;
  let bestScore = -1;

  for (const q of queries) {
    try {
      const candidates = await searchAnimeVost(q);
      for (const cand of candidates) {
        const candTitles = splitAnimeVostTitles(cand.title);
        const candYear = Number(cand.year) || null;
        const score = scoreTitleSimilarity(candTitles, targetTitles, candYear, targetYear);
        if (score > bestScore) {
          bestScore = score;
          bestItem = cand;
        }
      }
      if (bestScore >= 850) break;
    } catch {
      // Try next query on failure
    }
  }

  if (!bestItem || bestScore < 120) return null;

  const episodes = await fetchAnimeVostEpisodes(bestItem.id).catch(() => []);
  if (episodes.length === 0) return null;

  const cleanTitle = splitAnimeVostTitles(bestItem.title)[0] || bestItem.title || 'Anime';
  const yearNum = Number(bestItem.year) || undefined;
  return {
    releaseId: bestItem.id,
    title: cleanTitle,
    poster: bestItem.urlImagePreview,
    year: yearNum,
    episodes,
    voice: {
      id: `animevost:${bestItem.id}`,
      label: 'AnimeVost',
      sublabel: `${episodes.length} ep · MP4`,
      provider: 'animevost',
      providerId: bestItem.id,
      kind: 'mp4'
    }
  };
}
