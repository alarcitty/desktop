import type { VoiceTrackOption } from '../../marlakit/player/types';
import type { PlayableEpisode } from '../anime';
import { dedupeSearchQueries, fetchJsonWithTimeout, scoreTitleSimilarity } from './shared';

const ANIXART_BASE = 'https://api.anixart.tv';

interface RawAnixartRelease {
  id: number;
  title_ru?: string | null;
  title_original?: string | null;
  title_alt?: string | null;
  year?: string | number | null;
  image?: string | null;
}

interface RawAnixartSearchResponse {
  content?: RawAnixartRelease[];
}

interface RawAnixartType {
  id: number;
  name?: string | null;
  is_sub?: boolean;
  episodes_count?: number;
  view_count?: number;
  pinned?: boolean;
}

interface RawAnixartTypesResponse {
  types?: RawAnixartType[];
}

interface RawAnixartSource {
  id: number;
  name?: string | null;
  episodes_count?: number;
}

interface RawAnixartSourcesResponse {
  sources?: RawAnixartSource[];
}

interface RawAnixartEpisode {
  position: number;
  name?: string | null;
  url?: string | null;
  iframe?: boolean;
}

interface RawAnixartEpisodesResponse {
  episodes?: RawAnixartEpisode[];
}

const axSearchCache = new Map<string, RawAnixartRelease[]>();
const axTypesCache = new Map<number, RawAnixartType[]>();
const axEpisodesCache = new Map<string, PlayableEpisode[]>();

async function searchAnixart(query: string): Promise<RawAnixartRelease[]> {
  const key = query.trim().toLowerCase();
  if (key.length < 2) return [];
  const cached = axSearchCache.get(key);
  if (cached) return cached;

  const payload = await fetchJsonWithTimeout<RawAnixartSearchResponse>(
    `${ANIXART_BASE}/search/releases/0`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query: query.trim(), searchBy: 0 })
    },
    6000
  );
  const items = Array.isArray(payload?.content) ? payload.content.filter((r) => r?.id) : [];
  axSearchCache.set(key, items);
  return items;
}

async function fetchAnixartTypes(releaseId: number): Promise<RawAnixartType[]> {
  const cached = axTypesCache.get(releaseId);
  if (cached) return cached;

  const payload = await fetchJsonWithTimeout<RawAnixartTypesResponse>(
    `${ANIXART_BASE}/episode/${releaseId}`,
    undefined,
    6000
  );
  const types = Array.isArray(payload?.types)
    ? payload.types.filter((t) => t?.id && t.name && (t.episodes_count ?? 1) > 0)
    : [];
  axTypesCache.set(releaseId, types);
  return types;
}

export async function fetchAnixartEpisodes(
  releaseId: number,
  typeId: number
): Promise<PlayableEpisode[]> {
  const cacheKey = `${releaseId}:${typeId}`;
  const cached = axEpisodesCache.get(cacheKey);
  if (cached) return cached;

  const srcPayload = await fetchJsonWithTimeout<RawAnixartSourcesResponse>(
    `${ANIXART_BASE}/episode/${releaseId}/${typeId}`,
    undefined,
    6000
  );
  const sources = Array.isArray(srcPayload?.sources)
    ? srcPayload.sources.filter((s) => s?.id && (s.episodes_count ?? 1) > 0)
    : [];
  if (sources.length === 0) return [];

  // Prefer Kodik iframe source first (verified HTTP 200 without X-Frame-Options block),
  // otherwise pick the first available source on Anixart.
  const chosenSource =
    sources.find((s) => /kodik/i.test(s.name ?? '')) ??
    sources.find((s) => /libria/i.test(s.name ?? '')) ??
    sources[0];

  const epPayload = await fetchJsonWithTimeout<RawAnixartEpisodesResponse>(
    `${ANIXART_BASE}/episode/${releaseId}/${typeId}/${chosenSource.id}`,
    undefined,
    6500
  );
  const rawEps = Array.isArray(epPayload?.episodes) ? epPayload.episodes : [];
  const episodes: PlayableEpisode[] = [];
  for (let i = 0; i < rawEps.length; i++) {
    const raw = rawEps[i];
    const url = raw.url?.trim();
    if (!url) continue;
    const normalizedUrl = url.startsWith('//') ? `https:${url}` : url;
    const ordinal = raw.position > 0 ? raw.position : i + 1;
    episodes.push({
      id: `anixart:${releaseId}:${typeId}:${ordinal}`,
      ordinal,
      name: raw.name?.trim() || undefined,
      duration: 0,
      embed_url: normalizedUrl
    });
  }

  episodes.sort((a, b) => a.ordinal - b.ordinal);
  if (episodes.length > 0) {
    axEpisodesCache.set(cacheKey, episodes);
  }
  return episodes;
}

export interface AnixartDiscovery {
  releaseId: number;
  title: string;
  poster?: string;
  year?: number;
  voices: VoiceTrackOption[];
}

export async function findAnixartDiscovery(
  targetTitles: string[],
  targetYear?: number | null,
  opts?: { hasNativeAniLiberty?: boolean; hasNativeAnimeVost?: boolean }
): Promise<AnixartDiscovery | null> {
  const queries = dedupeSearchQueries(targetTitles).slice(0, 2);
  if (queries.length === 0) return null;

  let bestItem: RawAnixartRelease | null = null;
  let bestScore = -1;

  for (const q of queries) {
    try {
      const candidates = await searchAnixart(q);
      for (const cand of candidates) {
        const candTitles = [cand.title_ru ?? '', cand.title_original ?? '', cand.title_alt ?? '']
          .flatMap((s) => s.split('/'))
          .map((s) => s.trim())
          .filter(Boolean);
        const candYear = Number(cand.year) || null;
        const score = scoreTitleSimilarity(candTitles, targetTitles, candYear, targetYear);
        if (score > bestScore) {
          bestScore = score;
          bestItem = cand;
        }
      }
      if (bestScore >= 850) break;
    } catch {
      // Try next query
    }
  }

  if (!bestItem || bestScore < 120) return null;

  const rawTypes = await fetchAnixartTypes(bestItem.id).catch(() => []);
  if (rawTypes.length === 0) return null;

  const sorted = [...rawTypes].sort((a, b) => {
    const pinDiff = Number(Boolean(b.pinned)) - Number(Boolean(a.pinned));
    if (pinDiff !== 0) return pinDiff;
    return (b.view_count ?? 0) - (a.view_count ?? 0);
  });

  const voices: VoiceTrackOption[] = [];
  const seenNames = new Set<string>();
  for (const t of sorted) {
    const rawName = (t.name ?? '').trim();
    if (!rawName) continue;
    const lower = rawName.toLowerCase();

    if (opts?.hasNativeAnimeVost && lower === 'animevost') continue;
    if (seenNames.has(lower)) continue;
    seenNames.add(lower);

    const displayName =
      opts?.hasNativeAniLiberty && (lower === 'anilibria' || lower === 'aniliberty')
        ? `${rawName} (Mirror)`
        : rawName;

    const kindTag = t.is_sub ? 'Sub' : 'Dub';
    const epTag = t.episodes_count ? `${t.episodes_count} ep` : '';
    voices.push({
      id: `anixart:${bestItem.id}:${t.id}`,
      label: displayName,
      sublabel: [epTag, kindTag, 'External'].filter(Boolean).join(' · '),
      provider: 'anixart',
      providerId: bestItem.id,
      sourceId: t.id,
      kind: 'embed',
      isSub: Boolean(t.is_sub)
    });

    if (voices.length >= 24) break;
  }

  return {
    releaseId: bestItem.id,
    title: bestItem.title_ru?.trim() || bestItem.title_original?.trim() || 'Anime',
    poster: bestItem.image ?? undefined,
    year: Number(bestItem.year) || undefined,
    voices
  };
}
