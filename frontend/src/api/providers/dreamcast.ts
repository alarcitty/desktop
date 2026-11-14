import type { VoiceTrackOption } from '../../marlakit/player/types';
import type { PlayableEpisode } from '../anime';
import { dedupeSearchQueries, scoreTitleSimilarity } from './shared';

const DREAMCAST_PROXY_BASE = '/ext/dreamerscast';
const PLAY_DC_ORIGIN = 'https://play.dreamerscast.com';
const PLAY_DC_PROXY_BASE = '/ext/play-dc';

interface RawDreamCastRelease {
  id: number;
  russian?: string;
  original?: string;
  image?: string | null;
  dateissue?: number | string | null;
  series?: number | null;
  currentSeries?: number | null;
  dubbers?: string | null;
  url?: string | null;
}

interface RawDreamCastSearchResponse {
  releases?: RawDreamCastRelease[];
  count?: number;
}

const dcSearchCache = new Map<string, RawDreamCastRelease[]>();
const dcEpisodesCache = new Map<number, PlayableEpisode[]>();
const dcUrlById = new Map<number, string>();

function rewritePlayDcUrl(rawUrl: string): string {
  const trimmed = rawUrl.trim();
  if (trimmed.startsWith(`${PLAY_DC_ORIGIN}/`)) {
    return `${PLAY_DC_PROXY_BASE}/${trimmed.slice(PLAY_DC_ORIGIN.length + 1)}`;
  }
  if (trimmed.startsWith('http://play.dreamerscast.com/')) {
    return `${PLAY_DC_PROXY_BASE}/${trimmed.slice('http://play.dreamerscast.com/'.length)}`;
  }
  if (trimmed.startsWith('//play.dreamerscast.com/')) {
    return `${PLAY_DC_PROXY_BASE}/${trimmed.slice('//play.dreamerscast.com/'.length)}`;
  }
  return trimmed;
}

function normalizeImage(raw?: string | null): string | undefined {
  const trimmed = raw?.trim();
  if (!trimmed) return undefined;
  return trimmed.startsWith('//') ? `https:${trimmed}` : trimmed;
}

function decodeBase64List(b64: string): string[] {
  const binary = atob(b64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  const decoded = new TextDecoder('utf-8').decode(bytes);
  return decoded
    .split('||||')
    .map((item) => item.trim())
    .filter((item) => item.length > 0 && item.includes('.m3u8'));
}

async function fetchTextWithTimeout(url: string, init?: RequestInit, timeoutMs = 7000): Promise<string> {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const res = await fetch(url, { ...init, signal: ctrl.signal });
    if (!res.ok) {
      throw new Error(`HTTP ${res.status} on ${url}`);
    }
    return await res.text();
  } finally {
    clearTimeout(timer);
  }
}

export async function searchDreamCast(query: string): Promise<RawDreamCastRelease[]> {
  const clean = query.trim();
  const key = clean.toLowerCase();
  if (key.length < 2) return [];
  const cached = dcSearchCache.get(key);
  if (cached) return cached;

  const rawText = await fetchTextWithTimeout(
    `${DREAMCAST_PROXY_BASE}/`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: `search=${encodeURIComponent(clean)}&pageSize=12&pageNumber=1`
    },
    6500
  );
  const payload = JSON.parse(rawText) as RawDreamCastSearchResponse;
  const items = Array.isArray(payload?.releases)
    ? payload.releases.filter((item) => item?.id && item?.url)
    : [];
  dcSearchCache.set(key, items);
  return items;
}

export async function fetchDreamCastEpisodes(
  releaseId: number,
  releaseUrl = ''
): Promise<PlayableEpisode[]> {
  const cached = dcEpisodesCache.get(releaseId);
  if (cached) return cached;

  const resolvedUrl = releaseUrl || dcUrlById.get(releaseId) || '';
  if (!resolvedUrl) return [];
  dcUrlById.set(releaseId, resolvedUrl);

  const cleanPath = resolvedUrl.startsWith('/') ? resolvedUrl : `/${resolvedUrl}`;
  const html = await fetchTextWithTimeout(`${DREAMCAST_PROXY_BASE}${cleanPath}`, undefined, 7000);
  const match = html.match(/atob\(\s*['"]([A-Za-z0-9+/=]+)['"]\s*\)/);
  if (!match) return [];

  const urls = decodeBase64List(match[1]);
  const episodes: PlayableEpisode[] = urls.map((rawUrl, idx) => {
    const ordinal = idx + 1;
    return {
      id: `dreamcast:${releaseId}:${ordinal}`,
      ordinal,
      name: `Серия ${ordinal}`,
      duration: 0,
      hls_1080: rewritePlayDcUrl(rawUrl)
    };
  });

  if (episodes.length > 0) {
    dcEpisodesCache.set(releaseId, episodes);
  }
  return episodes;
}

export interface DreamCastMatch {
  releaseId: number;
  releaseUrl: string;
  title: string;
  poster?: string;
  year?: number;
  voiceActors?: string[];
  episodes: PlayableEpisode[];
  voice: VoiceTrackOption;
}

export async function findDreamCastMatch(
  targetTitles: string[],
  targetYear?: number | null
): Promise<DreamCastMatch | null> {
  const queries = dedupeSearchQueries(targetTitles).slice(0, 3);
  if (queries.length === 0) return null;

  let bestItem: RawDreamCastRelease | null = null;
  let bestScore = -1;

  for (const q of queries) {
    try {
      const candidates = await searchDreamCast(q);
      for (const cand of candidates) {
        const candTitles = [cand.russian ?? '', cand.original ?? '']
          .map((s) => s.trim())
          .filter(Boolean);
        const candYear = Number(cand.dateissue) || null;
        const score = scoreTitleSimilarity(candTitles, targetTitles, candYear, targetYear);
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

  if (!bestItem || !bestItem.url || bestScore < 120) return null;
  dcUrlById.set(bestItem.id, bestItem.url);

  const episodes = await fetchDreamCastEpisodes(bestItem.id, bestItem.url).catch(() => []);
  if (episodes.length === 0) return null;

  const voiceActors = (bestItem.dubbers ?? '')
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
  const title = bestItem.russian?.trim() || bestItem.original?.trim() || queries[0];

  return {
    releaseId: bestItem.id,
    releaseUrl: bestItem.url,
    title,
    poster: normalizeImage(bestItem.image),
    year: Number(bestItem.dateissue) || undefined,
    voiceActors: voiceActors.length > 0 ? voiceActors : undefined,
    episodes,
    voice: {
      id: `dreamcast:${bestItem.id}`,
      label: 'Dream Cast',
      sublabel: `${episodes.length} ep · 1080p HLS`,
      provider: 'dreamcast',
      providerId: bestItem.id,
      sourceId: bestItem.url,
      kind: 'hls'
    }
  };
}
