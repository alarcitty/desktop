import type { VoiceTrackOption } from '../../marlakit/player/types';
import type { PlayableEpisode } from '../anime';
import { dedupeSearchQueries, fetchJsonWithTimeout, scoreTitleSimilarity } from './shared';

const KODIK_PROXY_BASE = '/ext/kodik';
const KODIK_TOKEN_STORAGE_KEY = 'mila_kodik_token';

interface WailsMilaTokenBridge {
  GetKodikToken?: () => Promise<string>;
}

export interface KodikTranslation {
  id: number;
  title: string;
  type: 'voice' | 'subtitles' | string;
}

export interface KodikEpisodeData {
  link?: string;
  title?: string;
  screenshots?: string[];
}

export interface KodikSeasonData {
  link?: string;
  episodes?: Record<string, string | KodikEpisodeData>;
}

export interface KodikElement {
  id: string;
  type: string;
  link: string;
  title: string;
  title_orig?: string;
  other_title?: string;
  translation: KodikTranslation;
  year?: number;
  last_season?: number;
  last_episode?: number;
  episodes_count?: number;
  shikimori_id?: string;
  kinopoisk_id?: string;
  imdb_id?: string;
  quality?: string;
  camrip?: boolean;
  screenshots?: string[];
  seasons?: Record<string, KodikSeasonData>;
}

export interface KodikApiResponse {
  time?: string;
  total?: number;
  prev_page?: string | null;
  next_page?: string | null;
  results?: KodikElement[];
}

export interface KodikSearchParams {
  token?: string;
  title?: string;
  title_orig?: string;
  strict?: boolean;
  full_match?: boolean;
  shikimori_id?: number | string;
  kinopoisk_id?: string;
  imdb_id?: string;
  id?: string;
  types?: string;
  year?: number;
  limit?: number;
  translation_id?: string | number;
  translation_type?: 'voice' | 'subtitles';
  anime_kind?: string;
  anime_status?: 'released' | 'ongoing' | 'anons';
  with_seasons?: boolean;
  with_episodes?: boolean;
  with_episodes_data?: boolean;
  with_material_data?: boolean;
  prioritize_translations?: string;
}

export interface KodikListParams {
  token?: string;
  limit?: number;
  types?: string;
  year?: number;
  anime_kind?: string;
  anime_status?: 'released' | 'ongoing' | 'anons';
  sort?: 'year' | 'created_at' | 'updated_at' | 'kinopoisk_rating' | 'imdb_rating' | 'shikimori_rating';
  order?: 'asc' | 'desc';
  with_material_data?: boolean;
}

const kodikEpisodesCache = new Map<string, PlayableEpisode[]>();

let cachedEnvToken: string | null = null;

function normalizeKodikUrl(raw?: string | null): string {
  const trimmed = (raw ?? '').trim();
  if (!trimmed) return '';
  if (trimmed.startsWith('//')) return `https:${trimmed}`;
  if (trimmed.startsWith('http://')) return `https://${trimmed.slice('http://'.length)}`;
  return trimmed;
}

export function setKodikToken(token: string): void {
  const clean = token.trim();
  cachedEnvToken = clean;
  if (typeof window !== 'undefined' && window.localStorage) {
    try {
      if (clean) {
        window.localStorage.setItem(KODIK_TOKEN_STORAGE_KEY, clean);
      } else {
        window.localStorage.removeItem(KODIK_TOKEN_STORAGE_KEY);
      }
    } catch {
      // Ignore storage access errors
    }
  }
}

export async function getKodikToken(): Promise<string> {
  if (typeof window !== 'undefined' && window.localStorage) {
    try {
      const saved = window.localStorage.getItem(KODIK_TOKEN_STORAGE_KEY)?.trim();
      if (saved) return saved;
    } catch {
      // Ignore storage access errors
    }
  }

  const viteToken = (import.meta as { env?: Record<string, string | undefined> }).env?.VITE_KODIK_TOKEN?.trim();
  if (viteToken) return viteToken;

  if (cachedEnvToken !== null) return cachedEnvToken;

  if (typeof window !== 'undefined') {
    const goObj = (window as { go?: { mila?: { Mila?: WailsMilaTokenBridge } } }).go;
    const bridge = goObj?.mila?.Mila;
    if (bridge?.GetKodikToken) {
      try {
        cachedEnvToken = (await bridge.GetKodikToken()).trim();
        return cachedEnvToken;
      } catch {
        // Fall through to empty
      }
    }
  }

  cachedEnvToken = '';
  return '';
}

function buildKodikQuery(token: string, params: Record<string, unknown>): URLSearchParams {
  const qs = new URLSearchParams();
  qs.set('token', token);
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '' || k === 'token') continue;
    qs.set(k, String(v));
  }
  return qs;
}

async function postKodikEndpoint<T>(path: string, token: string, params: Record<string, unknown> = {}): Promise<T> {
  const qs = buildKodikQuery(token, params);
  return fetchJsonWithTimeout<T>(`${KODIK_PROXY_BASE}${path}?${qs.toString()}`, { method: 'POST' }, 6500);
}

export async function searchKodik(params: KodikSearchParams): Promise<KodikApiResponse> {
  const token = params.token?.trim() || (await getKodikToken());
  if (!token) return { total: 0, results: [] };
  return postKodikEndpoint<KodikApiResponse>('/search', token, params as Record<string, unknown>);
}

export async function listKodik(params: KodikListParams = {}): Promise<KodikApiResponse> {
  const token = params.token?.trim() || (await getKodikToken());
  if (!token) return { total: 0, results: [] };
  return postKodikEndpoint<KodikApiResponse>('/list', token, params as Record<string, unknown>);
}

export async function fetchKodikTranslations(token?: string): Promise<KodikTranslation[]> {
  const resolved = token?.trim() || (await getKodikToken());
  if (!resolved) return [];
  const data = await postKodikEndpoint<KodikTranslation[]>('/translations', resolved);
  return Array.isArray(data) ? data : [];
}

function extractEpisodesFromElement(el: KodikElement): PlayableEpisode[] {
  const episodes: PlayableEpisode[] = [];
  if (el.seasons && typeof el.seasons === 'object') {
    const seasonKeys = Object.keys(el.seasons).sort((a, b) => Number(a) - Number(b));
    const targetSeasonKey =
      el.last_season && el.seasons[String(el.last_season)]
        ? String(el.last_season)
        : seasonKeys[seasonKeys.length - 1];
    const season = targetSeasonKey ? el.seasons[targetSeasonKey] : undefined;
    if (season?.episodes) {
      const epNums = Object.keys(season.episodes).sort((a, b) => Number(a) - Number(b));
      for (const epKey of epNums) {
        const ordinal = Number(epKey);
        if (!Number.isFinite(ordinal) || ordinal <= 0) continue;
        const rawEp = season.episodes[epKey];
        const rawLink = typeof rawEp === 'string' ? rawEp : rawEp?.link;
        const embedUrl = normalizeKodikUrl(rawLink);
        if (!embedUrl) continue;
        const epTitle = typeof rawEp === 'object' ? rawEp?.title?.trim() : undefined;
        const preview =
          typeof rawEp === 'object' && Array.isArray(rawEp?.screenshots) && rawEp.screenshots[0]
            ? normalizeKodikUrl(rawEp.screenshots[0])
            : undefined;
        episodes.push({
          id: `kodik:${el.id}:${ordinal}`,
          ordinal,
          name: epTitle || `Серия ${ordinal}`,
          duration: 0,
          preview,
          embed_url: embedUrl
        });
      }
    }
  }

  if (episodes.length === 0 && el.link) {
    const fallbackUrl = normalizeKodikUrl(el.link);
    if (fallbackUrl) {
      episodes.push({
        id: `kodik:${el.id}:1`,
        ordinal: 1,
        name: el.title || 'Смотреть',
        duration: 0,
        preview: el.screenshots?.[0] ? normalizeKodikUrl(el.screenshots[0]) : undefined,
        embed_url: fallbackUrl
      });
    }
  }

  return episodes;
}

export function getCachedKodikEpisodes(elementId: string): PlayableEpisode[] {
  return kodikEpisodesCache.get(elementId) ?? [];
}

export async function findKodikDiscovery(
  shikiId?: number,
  targetTitles: string[] = [],
  targetYear?: number | null,
  opts?: { hasNativeAniLiberty?: boolean; hasNativeAnimeVost?: boolean; hasNativeDreamCast?: boolean }
): Promise<VoiceTrackOption[]> {
  const token = await getKodikToken();
  if (!token) return [];

  let results: KodikElement[] = [];
  if (shikiId && shikiId > 0) {
    const res = await searchKodik({
      token,
      shikimori_id: shikiId,
      types: 'anime,anime-serial',
      limit: 100,
      with_episodes_data: true
    }).catch(() => null);
    results = Array.isArray(res?.results) ? res.results : [];
  }

  if (results.length === 0) {
    const queries = dedupeSearchQueries(targetTitles).slice(0, 2);
    for (const q of queries) {
      const res = await searchKodik({
        token,
        title: q,
        types: 'anime,anime-serial',
        limit: 50,
        with_episodes_data: true
      }).catch(() => null);
      const batch = Array.isArray(res?.results) ? res.results : [];
      const matched = batch.filter((item) => {
        const candTitles = [item.title ?? '', item.title_orig ?? '', ...(item.other_title?.split('/') ?? [])]
          .map((s) => s.trim())
          .filter(Boolean);
        return scoreTitleSimilarity(candTitles, targetTitles, item.year ?? null, targetYear) >= 150;
      });
      if (matched.length > 0) {
        results = matched;
        break;
      }
    }
  }

  if (results.length === 0) return [];

  const voices: VoiceTrackOption[] = [];
  const seenTranslations = new Set<string>();
  const sorted = [...results].sort((a, b) => {
    const aSub = a.translation?.type === 'subtitles' ? 1 : 0;
    const bSub = b.translation?.type === 'subtitles' ? 1 : 0;
    if (aSub !== bSub) return aSub - bSub;
    return (b.episodes_count ?? 0) - (a.episodes_count ?? 0);
  });

  for (const el of sorted) {
    const trName = (el.translation?.title ?? '').trim();
    if (!trName) continue;
    const lower = trName.toLowerCase();

    if (opts?.hasNativeAnimeVost && lower === 'animevost') continue;
    if (opts?.hasNativeDreamCast && (lower === 'dream cast' || lower === 'dreamcast')) continue;
    if (seenTranslations.has(lower)) continue;
    seenTranslations.add(lower);

    const episodes = extractEpisodesFromElement(el);
    if (episodes.length === 0) continue;
    kodikEpisodesCache.set(el.id, episodes);

    const displayName =
      opts?.hasNativeAniLiberty && (lower === 'anilibria' || lower === 'anilibria.tv' || lower === 'aniliberty')
        ? `${trName} (Mirror)`
        : trName;
    const isSub = el.translation?.type === 'subtitles';
    const epCount = el.episodes_count || episodes.length;
    const epTag = epCount > 0 ? `${epCount} ep` : '';
    const kindTag = isSub ? 'Sub' : 'Dub';

    voices.push({
      id: `kodik:${el.id}`,
      label: displayName,
      sublabel: [epTag, kindTag, 'External'].filter(Boolean).join(' · '),
      provider: 'kodik',
      providerId: el.translation?.id,
      sourceId: el.id,
      kind: 'embed',
      isSub
    });
  }

  return voices;
}
