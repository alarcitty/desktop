import { invokeZokoProbe } from '../../app/bindings.svelte';
import type { VoiceTrackOption } from '../../marlakit/player/types';
import type { PlayableEpisode } from '../anime';
import { dedupeSearchQueries, fetchJsonWithTimeout } from './shared';

export interface RawZokoProbe {
  malId: number;
  hasDub: boolean;
  hasSub: boolean;
  episodes: number[];
}

export interface ZokoDubOption {
  mode: 'dub' | 'sub';
  voice: VoiceTrackOption;
  episodes: PlayableEpisode[];
}

export interface ZokoMatch {
  malId: number;
  dubs: ZokoDubOption[];
}

const probeCache = new Map<string, RawZokoProbe | null>();

export function buildZokoMasterUrl(malId: number, ordinal: number, mode: 'dub' | 'sub' = 'dub'): string {
  return `/ext/zoko/master.m3u8?mal=${encodeURIComponent(String(malId))}&ep=${encodeURIComponent(
    String(ordinal)
  )}&mode=${mode}`;
}

export function pickAsciiQuery(titles: string[]): string {
  const deduped = dedupeSearchQueries(titles);
  const ascii = deduped.find((t) => /[a-zA-Z]{2}/.test(t));
  return ascii ?? deduped[0] ?? '';
}

export async function probeZoko(
  malId?: number | null,
  episodeCount?: number | null,
  query?: string
): Promise<RawZokoProbe | null> {
  const cleanMal = malId && malId > 0 ? Math.trunc(malId) : 0;
  const cleanEps = episodeCount && episodeCount > 0 ? Math.trunc(episodeCount) : 0;
  const cleanQuery = (query ?? '').trim();
  if (cleanMal <= 0 && cleanQuery.length < 2) return null;

  const key = `${cleanMal}|${cleanEps}|${cleanQuery.toLowerCase()}`;
  if (probeCache.has(key)) {
    return probeCache.get(key) ?? null;
  }

  let parsed: RawZokoProbe | null = null;
  const rawJson = await invokeZokoProbe(cleanMal, cleanEps, cleanQuery);
  if (rawJson) {
    try {
      parsed = JSON.parse(rawJson) as RawZokoProbe;
    } catch {
      parsed = null;
    }
  } else {
    try {
      parsed = await fetchJsonWithTimeout<RawZokoProbe>(
        `/ext/zoko/probe?mal=${encodeURIComponent(String(cleanMal))}&eps=${encodeURIComponent(
          String(cleanEps)
        )}&q=${encodeURIComponent(cleanQuery)}`,
        undefined,
        9000
      );
    } catch {
      parsed = null;
    }
  }

  const valid =
    parsed && parsed.malId > 0 && (parsed.hasDub || parsed.hasSub) && Array.isArray(parsed.episodes)
      ? parsed
      : null;
  probeCache.set(key, valid);
  return valid;
}

export function buildZokoEpisodes(
  malId: number,
  ordinals: number[],
  mode: 'dub' | 'sub'
): PlayableEpisode[] {
  const uniqueOrds = Array.from(new Set(ordinals.filter((n) => Number.isFinite(n) && n > 0))).sort(
    (a, b) => a - b
  );
  return uniqueOrds.map((ord) => ({
    id: `zoko-${malId}-${mode}-${ord}`,
    ordinal: ord,
    name: `Episode ${ord}`,
    duration: 0,
    hls_1080: buildZokoMasterUrl(malId, ord, mode)
  }));
}

export async function fetchZokoEpisodes(
  malId: number,
  mode: 'dub' | 'sub',
  episodeCount?: number | null,
  titles: string[] = []
): Promise<PlayableEpisode[]> {
  const query = pickAsciiQuery(titles);
  const probe = await probeZoko(malId, episodeCount, query);
  const ords =
    probe?.episodes && probe.episodes.length > 0
      ? probe.episodes
      : Array.from({ length: Math.max(1, Math.min(200, episodeCount || 12)) }, (_, i) => i + 1);
  return buildZokoEpisodes(probe?.malId || malId, ords, mode);
}

export async function findZokoMatch(
  titles: string[],
  shikiId?: number | null,
  episodeCount?: number | null
): Promise<ZokoMatch | null> {
  const query = pickAsciiQuery(titles);
  const probe = await probeZoko(shikiId, episodeCount, query);
  if (!probe || !probe.malId || probe.episodes.length === 0) {
    return null;
  }

  const epTag = `${probe.episodes.length} ep · 1080p`;
  const dubs: ZokoDubOption[] = [];

  if (probe.hasDub) {
    dubs.push({
      mode: 'dub',
      voice: {
        id: `zoko:${probe.malId}:dub`,
        label: 'English Dub',
        sublabel: epTag,
        provider: 'zoko',
        providerId: probe.malId,
        sourceId: 'dub',
        kind: 'hls',
        isSub: false
      },
      episodes: buildZokoEpisodes(probe.malId, probe.episodes, 'dub')
    });
  }

  if (probe.hasSub) {
    dubs.push({
      mode: 'sub',
      voice: {
        id: `zoko:${probe.malId}:sub`,
        label: 'HiAnime · Eng Sub',
        sublabel: epTag,
        provider: 'zoko',
        providerId: probe.malId,
        sourceId: 'sub',
        kind: 'hls',
        isSub: true
      },
      episodes: buildZokoEpisodes(probe.malId, probe.episodes, 'sub')
    });
  }

  if (dubs.length === 0) return null;
  return {
    malId: probe.malId,
    dubs
  };
}
