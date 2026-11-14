import {
  buildMetaTags,
  buildPrimaryVoiceOption,
  ensureShikimoriArtwork,
  findVoiceOptions,
  getCachedArtwork,
  loadFranchiseSeasons,
  resolvePlayableRelease,
  resolveVoiceTrackRelease,
  type AnimeSeasonEntry,
  type PlayableRelease,
  type ShikiArtwork,
  type WatchTarget
} from '../api/anime';
import { openTorrentMagnet, type TorrentSearchItem } from '../api/providers/torrent';
import type { VoiceTrackOption } from '../marlakit/player/types';
import { recordHistoryEntry } from './history.svelte';
import {
  navigate,
  onRouteRestore,
  router,
  updateCurrentRouteParams,
  type RouteParams
} from './router.svelte';

export type PipCorner = 'bottom-right' | 'bottom-left' | 'top-right' | 'top-left';

const PIP_CORNER_STORAGE_KEY = 'mila.player.pipCorner';
const DUB_OFFSET_STORAGE_PREFIX = 'mila.player.dubOffset:';

function readSavedDubOffset(releaseAlias: string, dubId: string): number {
  if (typeof localStorage === 'undefined' || !dubId) return 0;
  const raw = Number(localStorage.getItem(`${DUB_OFFSET_STORAGE_PREFIX}${releaseAlias}:${dubId}`));
  return Number.isFinite(raw) ? Math.max(-5, Math.min(5, raw)) : 0;
}

function saveDubOffset(releaseAlias: string, dubId: string, sec: number): void {
  if (typeof localStorage === 'undefined' || !dubId) return;
  localStorage.setItem(`${DUB_OFFSET_STORAGE_PREFIX}${releaseAlias}:${dubId}`, String(sec));
}

function readSavedPipCorner(): PipCorner {
  if (typeof localStorage === 'undefined') return 'bottom-right';
  const raw = localStorage.getItem(PIP_CORNER_STORAGE_KEY);
  if (raw === 'bottom-right' || raw === 'bottom-left' || raw === 'top-right' || raw === 'top-left') {
    return raw;
  }
  return 'bottom-right';
}

export const player = $state<{
  loading: boolean;
  error: string;
  title: string;
  target: WatchTarget | null;
  shikiArt: ShikiArtwork | null;
  release: PlayableRelease | null;
  seasons: AnimeSeasonEntry[];
  voices: VoiceTrackOption[];
  selectedVoiceId: string;
  dubRelease: PlayableRelease | null;
  dubVoiceId: string;
  dubOffsetSec: number;
  episodeIndex: number;
  resumeTime: number;
  autoplay: boolean;
  isPlaying: boolean;
  pipMode: boolean;
  pipManual: boolean;
  pipDismissed: boolean;
  pipCorner: PipCorner;
  inlineStageEl: HTMLElement | null;
}>({
  loading: false,
  error: '',
  title: '',
  target: null,
  shikiArt: null,
  release: null,
  seasons: [],
  voices: [],
  selectedVoiceId: '',
  dubRelease: null,
  dubVoiceId: '',
  dubOffsetSec: 0,
  episodeIndex: 0,
  resumeTime: 0,
  autoplay: true,
  isPlaying: false,
  pipMode: false,
  pipManual: false,
  pipDismissed: false,
  pipCorner: readSavedPipCorner(),
  inlineStageEl: null
});

let activeRequestToken = 0;
let lastHistorySyncMs = 0;
let voiceDiscoveryPromise: Promise<VoiceTrackOption[]> | null = null;
let pauseControlCallback: (() => void) | null = null;
const failedVoiceIds = new Set<string>();
const torrentReleaseCache = new Map<string, PlayableRelease>();

function syncPlayerHistory(currentTime?: number, duration?: number): void {
  const rel = player.release;
  const art = player.shikiArt;
  const target = player.target;
  if (!rel && !target) return;

  const ep = rel?.episodes[player.episodeIndex];
  const episodeOrdinal = ep?.ordinal ?? player.episodeIndex + 1;
  const title = art?.english || art?.name || rel?.title || player.title || target?.title || '';
  if (!title) return;

  const poster = art?.posterOriginal || art?.posterMain || rel?.poster || '';
  const tags = buildMetaTags(art ?? undefined);
  const score = art?.score && art.score > 0 ? Number(art.score.toFixed(1)) : undefined;

  recordHistoryEntry({
    shikiId: art?.id ?? target?.shikiId,
    releaseAlias: rel?.alias ?? target?.releaseAlias,
    searchQueries: target?.searchQueries,
    title,
    description: art?.description?.trim() || '',
    poster,
    score,
    tags,
    episodeOrdinal,
    episodeIndex: player.episodeIndex,
    totalEpisodes: rel?.episodes.length,
    currentTime,
    duration: duration && duration > 0 ? duration : ep?.duration
  });
}

export function reportPlaybackProgress(currentTime: number, duration: number, force = false): void {
  const now = Date.now();
  if (!force && now - lastHistorySyncMs < 3000) return;
  lastHistorySyncMs = now;
  syncPlayerHistory(currentTime, duration);
}

function applyPreservedEpisode(
  rel: PlayableRelease,
  preserveEpisodeOrdinal?: number,
  preserveEpisodeIndex?: number
): number {
  if (preserveEpisodeOrdinal != null) {
    const idx = rel.episodes.findIndex((e) => e.ordinal === preserveEpisodeOrdinal);
    if (idx >= 0) return idx;
  }
  if (preserveEpisodeIndex != null) {
    return Math.min(preserveEpisodeIndex, Math.max(0, rel.episodes.length - 1));
  }
  return 0;
}

async function loadTargetIntoPlayer(
  token: number,
  target: WatchTarget,
  opts?: { preserveEpisodeOrdinal?: number; preserveEpisodeIndex?: number; preserveTime?: number; keepVoices?: boolean }
): Promise<PlayableRelease | null> {
  player.loading = true;
  player.error = '';
  player.title = target.title;
  player.target = target;
  player.shikiArt = target.shikiId ? getCachedArtwork(target.shikiId) ?? null : null;
  player.release = null;
  if (!opts?.keepVoices) {
    player.voices = [];
    player.selectedVoiceId = '';
    player.dubRelease = null;
    player.dubVoiceId = '';
    player.dubOffsetSec = 0;
    voiceDiscoveryPromise = null;
    failedVoiceIds.clear();
  }
  player.episodeIndex = opts?.preserveEpisodeIndex ?? 0;
  player.resumeTime = opts?.preserveTime ?? 0;
  player.autoplay = true;

  if (target.shikiId && !player.shikiArt) {
    ensureShikimoriArtwork([target.shikiId])
      .then((map) => {
        if (token !== activeRequestToken) return;
        const art = map.get(target.shikiId!);
        if (art) {
          player.shikiArt = art;
          if (player.release) {
            syncPlayerHistory();
          }
        }
      })
      .catch(() => {});
  }

  try {
    const rel = await resolvePlayableRelease(target);
    if (token !== activeRequestToken) return null;
    player.release = rel;
    player.title = player.shikiArt?.english || player.shikiArt?.name || rel.title || target.title;
    player.selectedVoiceId = rel.alias;
    if (!opts?.keepVoices || player.voices.length === 0) {
      player.voices = [buildPrimaryVoiceOption(rel)];
      const extraTitles = [target.title, ...(target.searchQueries ?? [])];
      const discovery = findVoiceOptions(rel, target.shikiId, extraTitles)
        .then((voices) => {
          if (token === activeRequestToken && voices.length > 0) {
            player.voices = voices;
          }
          return voices;
        })
        .catch(() => player.voices);
      voiceDiscoveryPromise = discovery;
    }

    player.episodeIndex = applyPreservedEpisode(
      rel,
      opts?.preserveEpisodeOrdinal,
      opts?.preserveEpisodeIndex
    );

    const activeEp = rel.episodes[player.episodeIndex];
    if (router.current === 'anime') {
      updateCurrentRouteParams({
        shikiId: player.shikiArt?.id ?? target.shikiId,
        alias: rel.alias,
        title: player.title,
        searchQueries: target.searchQueries,
        episode: player.episodeIndex,
        episodeOrdinal: activeEp?.ordinal
      });
    }
    syncPlayerHistory(player.resumeTime, activeEp?.duration ?? 0);
    lastHistorySyncMs = Date.now();
    return rel;
  } catch (err) {
    if (token !== activeRequestToken) return null;
    player.error = err instanceof Error ? err.message : 'Failed to load stream';
    return null;
  } finally {
    if (token === activeRequestToken) {
      player.loading = false;
    }
  }
}

export async function switchVoice(voice: VoiceTrackOption, preserveTime = 0): Promise<void> {
  if (voice.id === player.selectedVoiceId) return;
  const currentEp = player.release?.episodes[player.episodeIndex];
  const prevOrdinal = currentEp?.ordinal;
  const prevIndex = player.episodeIndex;
  const baseRelease = player.release;
  const baseTarget: WatchTarget = player.target ?? {
    title: player.title,
    shikiId: player.shikiArt?.id
  };
  const token = ++activeRequestToken;

  player.loading = true;
  player.error = '';
  player.resumeTime = Number.isFinite(preserveTime) && preserveTime > 0.5 ? preserveTime : 0;
  player.autoplay = true;

  try {
    const cachedTorrent = voice.provider === 'torrent' ? torrentReleaseCache.get(voice.id) : undefined;
    const nextRel =
      cachedTorrent ?? (await resolveVoiceTrackRelease(voice, baseRelease, baseTarget));
    if (token !== activeRequestToken) return;
    player.release = nextRel;
    player.selectedVoiceId = nextRel.alias;
    if (player.dubVoiceId === voice.id) {
      player.dubVoiceId = '';
      player.dubRelease = null;
      player.dubOffsetSec = 0;
    } else if (player.dubVoiceId) {
      player.dubOffsetSec = readSavedDubOffset(nextRel.alias, player.dubVoiceId);
    }
    player.episodeIndex = applyPreservedEpisode(nextRel, prevOrdinal, prevIndex);
  } catch (err) {
    if (token !== activeRequestToken) return;
    player.error = err instanceof Error ? err.message : 'Failed to switch voiceover';
  } finally {
    if (token === activeRequestToken) {
      player.loading = false;
    }
  }
}

export async function openTorrentStream(
  magnetOrItem: string | TorrentSearchItem,
  opts?: { preserveTime?: number }
): Promise<void> {
  const magnet = typeof magnetOrItem === 'string' ? magnetOrItem : magnetOrItem.magnet;
  const label =
    typeof magnetOrItem === 'string'
      ? 'Torrent 1080p'
      : `Torrent · ${magnetOrItem.quality || '1080p'}`;
  const currentEp = player.release?.episodes[player.episodeIndex];
  const prevOrdinal = currentEp?.ordinal;
  const prevIndex = player.episodeIndex;
  const baseRelease = player.release;
  const shikiArt = player.shikiArt;
  const token = ++activeRequestToken;

  player.loading = true;
  player.error = '';
  player.resumeTime = opts?.preserveTime ?? 0;
  player.autoplay = true;

  try {
    const info = await openTorrentMagnet(magnet);
    if (token !== activeRequestToken) return;

    const voiceId = `torrent:${info.infoHash}`;
    const episodes = info.files.map((f) => ({
      id: `${voiceId}:${f.fileIndex}`,
      ordinal: f.ordinal,
      name: `Серия ${f.ordinal}`,
      duration: 0,
      preview: baseRelease?.episodes.find((ep) => ep.ordinal === f.ordinal)?.preview,
      opening: baseRelease?.episodes.find((ep) => ep.ordinal === f.ordinal)?.opening,
      ending: baseRelease?.episodes.find((ep) => ep.ordinal === f.ordinal)?.ending,
      remux: {
        hash: info.infoHash,
        fileIndex: f.fileIndex,
        needsRemux: f.needsRemux,
        label: typeof magnetOrItem === 'string' ? 'BDRip 1080p' : magnetOrItem.quality || '1080p'
      }
    }));

    const torrentRel: PlayableRelease = {
      id: -1,
      alias: voiceId,
      title: shikiArt?.english || shikiArt?.name || baseRelease?.title || info.name || player.title,
      titleEnglish: shikiArt?.english || shikiArt?.name,
      year: shikiArt?.year ?? baseRelease?.year,
      provider: 'torrent',
      voiceLabel: label,
      poster: shikiArt?.posterOriginal || shikiArt?.posterMain || baseRelease?.poster,
      banner: shikiArt?.screenshots?.[0] || baseRelease?.banner,
      episodes
    };

    torrentReleaseCache.set(voiceId, torrentRel);
    const torrentVoice: VoiceTrackOption = {
      id: voiceId,
      label,
      sublabel: `${episodes.length} ep · ${info.files.some((f) => f.needsRemux) ? 'MKV Remux' : 'MP4 Direct'}`,
      provider: 'torrent',
      sourceId: info.infoHash,
      kind: 'mp4'
    };
    if (!player.voices.some((v) => v.id === voiceId)) {
      player.voices = [torrentVoice, ...player.voices];
    }

    player.release = torrentRel;
    player.selectedVoiceId = voiceId;
    player.dubVoiceId = '';
    player.dubRelease = null;
    player.dubOffsetSec = 0;
    player.episodeIndex = applyPreservedEpisode(torrentRel, prevOrdinal, prevIndex);
  } catch (err) {
    if (token !== activeRequestToken) return;
    player.error = err instanceof Error ? err.message : 'Failed to open torrent stream';
  } finally {
    if (token === activeRequestToken) {
      player.loading = false;
    }
  }
}

export async function selectDubVoice(voice: VoiceTrackOption | null): Promise<void> {
  if (!voice) {
    player.dubVoiceId = '';
    player.dubRelease = null;
    player.dubOffsetSec = 0;
    return;
  }
  const baseRelease = player.release;
  const baseTarget: WatchTarget = player.target ?? {
    title: player.title,
    shikiId: player.shikiArt?.id
  };
  const token = activeRequestToken;
  try {
    const dubRel = await resolveVoiceTrackRelease(voice, baseRelease, baseTarget);
    if (token !== activeRequestToken) return;
    player.dubVoiceId = voice.id;
    player.dubRelease = dubRel;
    player.dubOffsetSec = readSavedDubOffset(baseRelease?.alias ?? '', voice.id);
  } catch (err) {
    if (token !== activeRequestToken) return;
    player.error = err instanceof Error ? err.message : 'Failed to attach dub audio stream';
  }
}

export function setDubAudioOffset(sec: number): void {
  const rounded = Math.max(-5, Math.min(5, Math.round(sec * 10) / 10));
  player.dubOffsetSec = rounded;
  if (player.dubVoiceId) {
    saveDubOffset(player.release?.alias ?? '', player.dubVoiceId, rounded);
  }
}

export async function fallbackVoice(autoFromFailure = false): Promise<boolean> {
  const activeId = player.selectedVoiceId;
  const activeProvider = player.release?.provider ?? 'aniliberty';
  if (activeId) {
    failedVoiceIds.add(activeId);
  }

  const startToken = activeRequestToken;
  if (voiceDiscoveryPromise) {
    await voiceDiscoveryPromise.catch(() => {});
  }
  if (startToken !== activeRequestToken) return false;

  const candidates = player.voices.filter((v) => {
    if (v.id === activeId || failedVoiceIds.has(v.id)) return false;
    if (autoFromFailure && activeProvider === 'aniliberty' && v.provider === 'aniliberty') {
      return false;
    }
    return true;
  });

  if (candidates.length === 0) return false;

  // Prefer native Dream Cast 1080p HLS, then native AnimeVost MP4, then any remaining internal source.
  const chosen =
    candidates.find((v) => v.provider === 'dreamcast') ??
    candidates.find((v) => v.provider === 'animevost') ??
    candidates[0];

  await switchVoice(chosen, player.resumeTime);
  return player.selectedVoiceId === chosen.id;
}

export async function openWatch(
  target: WatchTarget,
  opts?: { episodeOrdinal?: number; episodeIndex?: number; resumeTime?: number }
): Promise<void> {
  const token = ++activeRequestToken;
  player.seasons = [];
  player.pipMode = false;
  player.pipManual = false;
  player.pipDismissed = false;
  navigate({
    page: 'anime',
    shikiId: target.shikiId,
    alias: target.releaseAlias,
    title: target.title,
    searchQueries: target.searchQueries,
    autoPlay: true,
    episode: opts?.episodeIndex,
    episodeOrdinal: opts?.episodeOrdinal,
    resumeTime: opts?.resumeTime
  });

  const rel = await loadTargetIntoPlayer(token, target, {
    preserveEpisodeOrdinal: opts?.episodeOrdinal,
    preserveEpisodeIndex: opts?.episodeIndex,
    preserveTime: opts?.resumeTime
  });
  if (!rel || token !== activeRequestToken) return;

  loadFranchiseSeasons(rel.id)
    .then((seasons) => {
      if (token === activeRequestToken) {
        player.seasons = seasons;
      }
    })
    .catch(() => {});
}

export async function switchSeason(season: AnimeSeasonEntry): Promise<void> {
  if (player.release && (player.release.id === season.id || player.release.alias === season.alias)) {
    return;
  }
  const token = ++activeRequestToken;
  const target: WatchTarget = {
    title: season.titleEnglish || season.title,
    shikiId: season.shikiId,
    releaseAlias: season.alias,
    searchQueries: [season.titleEnglish ?? '', season.title].filter(Boolean)
  };
  navigate({
    page: 'anime',
    shikiId: target.shikiId,
    alias: target.releaseAlias,
    title: target.title,
    searchQueries: target.searchQueries,
    autoPlay: true,
    episode: 0
  });
  const rel = await loadTargetIntoPlayer(token, target);
  if (rel && token === activeRequestToken && player.seasons.length === 0) {
    loadFranchiseSeasons(rel.id)
      .then((seasons) => {
        if (token === activeRequestToken) {
          player.seasons = seasons;
        }
      })
      .catch(() => {});
  }
}

async function restoreRouteTarget(params: RouteParams): Promise<void> {
  const matchesCurrentRelease = Boolean(
    player.release &&
      ((params.alias && player.release.alias === params.alias) ||
        (!params.alias &&
          params.shikiId &&
          (player.shikiArt?.id === params.shikiId || player.target?.shikiId === params.shikiId)))
  );

  if (matchesCurrentRelease && player.release) {
    const nextIdx = applyPreservedEpisode(
      player.release,
      params.episodeOrdinal,
      params.episode ?? player.episodeIndex
    );
    if (nextIdx !== player.episodeIndex) {
      player.episodeIndex = nextIdx;
    }
    return;
  }

  const title = params.title || params.alias || player.title || 'Anime';
  const target: WatchTarget = {
    title,
    shikiId: params.shikiId,
    releaseAlias: params.alias,
    searchQueries: params.searchQueries
  };
  const token = ++activeRequestToken;
  player.seasons = [];
  const rel = await loadTargetIntoPlayer(token, target, {
    preserveEpisodeOrdinal: params.episodeOrdinal,
    preserveEpisodeIndex: params.episode,
    preserveTime: params.resumeTime
  });
  if (!rel || token !== activeRequestToken) return;

  loadFranchiseSeasons(rel.id)
    .then((seasons) => {
      if (token === activeRequestToken) {
        player.seasons = seasons;
      }
    })
    .catch(() => {});
}

onRouteRestore((entry) => {
  if (entry.page === 'anime' && entry.params) {
    void restoreRouteTarget(entry.params);
  }
});

export async function retryWatch(): Promise<void> {
  if (!player.target) return;
  const token = ++activeRequestToken;
  const rel = await loadTargetIntoPlayer(token, player.target);
  if (rel && token === activeRequestToken && player.seasons.length === 0) {
    loadFranchiseSeasons(rel.id)
      .then((seasons) => {
        if (token === activeRequestToken) {
          player.seasons = seasons;
        }
      })
      .catch(() => {});
  }
}

export function selectEpisode(index: number): void {
  if (!player.release) return;
  if (index < 0 || index >= player.release.episodes.length) return;
  failedVoiceIds.clear();
  player.resumeTime = 0;
  player.autoplay = true;
  player.pipDismissed = false;
  player.episodeIndex = index;
  const ep = player.release.episodes[index];
  if (router.current === 'anime') {
    updateCurrentRouteParams({
      episode: index,
      episodeOrdinal: ep?.ordinal,
      resumeTime: 0
    });
  }
  syncPlayerHistory(0, ep?.duration ?? 0);
  lastHistorySyncMs = Date.now();
}

export function nextEpisode(): void {
  if (!player.release) return;
  if (player.episodeIndex + 1 < player.release.episodes.length) {
    failedVoiceIds.clear();
    player.resumeTime = 0;
    player.autoplay = true;
    player.pipDismissed = false;
    player.episodeIndex += 1;
    const ep = player.release.episodes[player.episodeIndex];
    if (router.current === 'anime') {
      updateCurrentRouteParams({
        episode: player.episodeIndex,
        episodeOrdinal: ep?.ordinal,
        resumeTime: 0
      });
    }
    syncPlayerHistory(0, ep?.duration ?? 0);
    lastHistorySyncMs = Date.now();
  }
}

export function setInlinePlayerStage(el: HTMLElement | null): void {
  player.inlineStageEl = el;
}

export function registerPlayerPauseControl(fn: (() => void) | null): void {
  pauseControlCallback = fn;
}

export function setPlayerPlaying(playing: boolean): void {
  player.isPlaying = playing;
  if (playing) {
    player.pipDismissed = false;
  }
}

export function setPipCorner(corner: PipCorner): void {
  player.pipCorner = corner;
  if (typeof localStorage !== 'undefined') {
    try {
      localStorage.setItem(PIP_CORNER_STORAGE_KEY, corner);
    } catch {
      // Ignore storage errors
    }
  }
}

export function toggleCustomPip(): void {
  if (player.pipMode) {
    expandCustomPip();
  } else {
    player.pipMode = true;
    player.pipManual = true;
    player.pipDismissed = false;
  }
}

export function expandCustomPip(): void {
  player.pipMode = false;
  player.pipManual = false;
  player.pipDismissed = false;
  if (router.current !== 'anime') {
    navigate({
      page: 'anime',
      shikiId: player.shikiArt?.id ?? player.target?.shikiId,
      alias: player.release?.alias ?? player.target?.releaseAlias,
      title: player.title,
      searchQueries: player.target?.searchQueries,
      autoPlay: true,
      episode: player.episodeIndex,
      episodeOrdinal: player.release?.episodes[player.episodeIndex]?.ordinal
    });
  }
}

export function closeCustomPip(): void {
  player.pipMode = false;
  player.pipManual = false;
  player.pipDismissed = true;
  player.isPlaying = false;
  pauseControlCallback?.();
}

export function resetPlayer(): void {
  activeRequestToken += 1;
  player.loading = false;
  player.release = null;
  player.seasons = [];
  player.voices = [];
  player.selectedVoiceId = '';
  player.resumeTime = 0;
  player.error = '';
  player.isPlaying = false;
  player.pipMode = false;
  player.pipManual = false;
  player.pipDismissed = false;
}
