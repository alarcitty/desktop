import { invokeHentaiGetEpisodes, invokeHentaiSearch } from '../../app/bindings.svelte';
import type { VoiceTrackOption } from '../../marlakit/player/types';
import type { PlayableEpisode } from '../anime';
import { dedupeSearchQueries, fetchJsonWithTimeout, scoreTitleSimilarity } from './shared';

export type HentaiProviderName = 'hentaimama' | 'hentaigasm';

export interface RawHentaiSearchItem {
  provider: HentaiProviderName | string;
  target: string;
  title: string;
  originalTitle?: string;
  year?: number;
  poster?: string;
  variant?: string;
  episodeCount?: number;
}

export interface RawHentaiEpisodeItem {
  ordinal: number;
  name?: string;
  duration?: number;
  hls_1080?: string;
  mp4_1080?: string;
  mp4_720?: string;
  mp4_480?: string;
}

export interface HentaiTrackOption {
  provider: HentaiProviderName;
  target: string;
  title: string;
  year?: number;
  poster?: string;
  score: number;
  voice: VoiceTrackOption;
}

export interface HentaiMatch {
  releaseId: number;
  provider: HentaiProviderName;
  target: string;
  title: string;
  year?: number;
  poster?: string;
  voice: VoiceTrackOption;
  episodes: PlayableEpisode[];
  allTracks: HentaiTrackOption[];
}

const searchCache = new Map<string, RawHentaiSearchItem[]>();
const episodesCache = new Map<string, PlayableEpisode[]>();

const SUFFIX_STRIP_RE = /\s+(?:the\s+animation|ova|oad|specials?|18\+)\s*$/i;

function hashTargetId(input: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < input.length; i++) {
    h ^= input.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return 900_000_000 + (Math.abs(h) % 90_000_000);
}

export function buildHentaiQueries(titles: string[]): string[] {
  const candidates: string[] = [];
  for (const raw of titles) {
    const clean = raw?.trim() ?? '';
    if (clean.length < 2) continue;
    // Prefer Latin/Romaji titles first since HentaiMama/HentaiGasm index Romaji & English names.
    if (/[a-zA-Z]{2}/.test(clean)) {
      candidates.push(clean);
      const stripped = clean.replace(SUFFIX_STRIP_RE, '').trim();
      if (stripped.length >= 3 && stripped !== clean) {
        candidates.push(stripped);
      }
    }
  }
  for (const raw of titles) {
    const clean = raw?.trim() ?? '';
    if (clean.length >= 2 && !/[a-zA-Z]{2}/.test(clean)) {
      candidates.push(clean);
    }
  }
  return dedupeSearchQueries(candidates).slice(0, 3);
}

export async function searchHentaiSources(query: string): Promise<RawHentaiSearchItem[]> {
  const clean = query.trim();
  const key = clean.toLowerCase();
  if (key.length < 2) return [];
  const cached = searchCache.get(key);
  if (cached) return cached;

  const rawJson = await invokeHentaiSearch(clean);
  let items: RawHentaiSearchItem[];
  if (rawJson) {
    items = JSON.parse(rawJson) as RawHentaiSearchItem[];
  } else {
    items = await fetchJsonWithTimeout<RawHentaiSearchItem[]>(
      `/ext/hentai/search?q=${encodeURIComponent(clean)}`,
      undefined,
      12_000
    );
  }
  const normalized = Array.isArray(items) ? items.filter((it) => it?.provider && it?.target) : [];
  searchCache.set(key, normalized);
  return normalized;
}

export async function fetchHentaiEpisodes(
  provider: HentaiProviderName | string,
  target: string
): Promise<PlayableEpisode[]> {
  const cleanProv = provider.trim().toLowerCase();
  const cleanTarget = target.trim();
  if (!cleanProv || !cleanTarget) return [];
  const cacheKey = `${cleanProv}::${cleanTarget}`;
  const cached = episodesCache.get(cacheKey);
  if (cached) return cached;

  const rawJson = await invokeHentaiGetEpisodes(cleanProv, cleanTarget);
  let list: RawHentaiEpisodeItem[];
  if (rawJson) {
    list = JSON.parse(rawJson) as RawHentaiEpisodeItem[];
  } else {
    list = await fetchJsonWithTimeout<RawHentaiEpisodeItem[]>(
      `/ext/hentai/episodes?provider=${encodeURIComponent(cleanProv)}&target=${encodeURIComponent(cleanTarget)}`,
      undefined,
      20_000
    );
  }

  const out: PlayableEpisode[] = (Array.isArray(list) ? list : [])
    .filter((ep) => Boolean(ep && (ep.hls_1080 || ep.mp4_1080 || ep.mp4_720 || ep.mp4_480)))
    .map((ep, idx) => {
      const ordinal = ep.ordinal > 0 ? ep.ordinal : idx + 1;
      return {
        id: `${cleanProv}-${ordinal}-${hashTargetId(`${cleanTarget}#${ordinal}`)}`,
        ordinal,
        name: ep.name?.trim() || `Episode ${ordinal}`,
        duration: ep.duration ?? 0,
        hls_1080: ep.hls_1080 || null,
        mp4_1080: ep.mp4_1080 || null,
        mp4_720: ep.mp4_720 || null,
        mp4_480: ep.mp4_480 || null
      };
    });

  if (out.length > 0) {
    episodesCache.set(cacheKey, out);
  }
  return out;
}

function formatHentaiTrackLabel(provider: HentaiProviderName, variant?: string): {
  label: string;
  sublabel: string;
  isSub: boolean;
} {
  const cleanVariant = (variant || '').trim();
  if (provider === 'hentaimama') {
    return {
      label: 'HentaiMama',
      sublabel: cleanVariant ? `${cleanVariant} · 1080p` : '1080p HLS',
      isSub: true
    };
  }
  const isUnc = cleanVariant.toLowerCase() === 'uncensored';
  return {
    label: cleanVariant ? `HentaiGasm · ${cleanVariant}` : 'HentaiGasm',
    sublabel: isUnc ? 'Без цензуры · MP4' : 'MP4 Sub',
    isSub: cleanVariant.toLowerCase() !== 'dubbed'
  };
}

function buildTrackOption(item: RawHentaiSearchItem, score: number): HentaiTrackOption | null {
  const prov = item.provider === 'hentaimama' ? 'hentaimama' : item.provider === 'hentaigasm' ? 'hentaigasm' : null;
  if (!prov) return null;

  const { label, sublabel, isSub } = formatHentaiTrackLabel(prov, item.variant);
  const trackId = `${prov}:${hashTargetId(item.target)}`;
  return {
    provider: prov,
    target: item.target,
    title: item.title,
    year: item.year,
    poster: item.poster,
    score,
    voice: {
      id: trackId,
      label,
      sublabel: item.episodeCount ? `${sublabel} · ${item.episodeCount} ep` : sublabel,
      releaseAlias: trackId,
      provider: prov,
      providerId: hashTargetId(item.target),
      sourceId: item.target,
      kind: prov === 'hentaimama' ? 'hls' : 'mp4',
      isSub
    }
  };
}

function trackPriority(track: HentaiTrackOption): number {
  const provBonus = track.provider === 'hentaimama' ? 60 : 20;
  const uncBonus = track.voice.label.toLowerCase().includes('uncensored') ? 25 : 0;
  return track.score + provBonus + uncBonus;
}

export async function findAllHentaiTracks(
  titles: string[],
  targetYear?: number | null
): Promise<HentaiTrackOption[]> {
  const queries = buildHentaiQueries(titles);
  if (queries.length === 0) return [];

  const collected = new Map<string, HentaiTrackOption>();
  const compareTitles = dedupeSearchQueries([
    ...titles,
    ...queries
  ]);

  for (const query of queries) {
    let candidates: RawHentaiSearchItem[] = [];
    try {
      candidates = await searchHentaiSources(query);
    } catch {
      continue;
    }

    for (const item of candidates) {
      const candTitles = [item.title, item.originalTitle ?? ''].filter(Boolean);
      const score = scoreTitleSimilarity(candTitles, compareTitles, item.year ?? null, targetYear);
      if (score < 420) continue;

      const track = buildTrackOption(item, score);
      if (!track) continue;

      const dedupeKey = `${track.provider}::${track.target}`;
      const prev = collected.get(dedupeKey);
      if (!prev || track.score > prev.score) {
        collected.set(dedupeKey, track);
      }
    }

    if (collected.size > 0 && Array.from(collected.values()).some((t) => t.score >= 850)) {
      break;
    }
  }

  return Array.from(collected.values()).sort((a, b) => trackPriority(b) - trackPriority(a));
}

export async function findHentaiMatch(
  titles: string[],
  targetYear?: number | null
): Promise<HentaiMatch | null> {
  const tracks = await findAllHentaiTracks(titles, targetYear);
  if (tracks.length === 0) return null;

  for (const candidate of tracks.slice(0, 3)) {
    try {
      const episodes = await fetchHentaiEpisodes(candidate.provider, candidate.target);
      if (episodes.length === 0) continue;

      return {
        releaseId: hashTargetId(candidate.target),
        provider: candidate.provider,
        target: candidate.target,
        title: candidate.title,
        year: candidate.year,
        poster: candidate.poster,
        voice: candidate.voice,
        episodes,
        allTracks: tracks
      };
    } catch {
      // Try next hentai track/provider
    }
  }

  return null;
}
