import {
  buildPrimaryVoiceOption,
  ensureShikimoriArtwork,
  findVoiceOptions,
  getCachedArtwork,
  loadFranchiseSeasons,
  resolvePlayableRelease,
  type AnimeSeasonEntry,
  type PlayableRelease,
  type ShikiArtwork,
  type WatchTarget
} from '../api/anime';
import type { VoiceTrackOption } from '../marlakit/player/types';
import { navigate } from './router.svelte';

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
  episodeIndex: number;
  resumeTime: number;
  autoplay: boolean;
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
  episodeIndex: 0,
  resumeTime: 0,
  autoplay: true
});

let activeRequestToken = 0;

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
      findVoiceOptions(rel, target.shikiId)
        .then((voices) => {
          if (token === activeRequestToken && voices.length > 0) {
            player.voices = voices;
          }
        })
        .catch(() => {});
    }

    if (opts?.preserveEpisodeOrdinal != null) {
      const idx = rel.episodes.findIndex((e) => e.ordinal === opts.preserveEpisodeOrdinal);
      player.episodeIndex =
        idx >= 0 ? idx : Math.min(opts.preserveEpisodeIndex ?? 0, Math.max(0, rel.episodes.length - 1));
    } else if (opts?.preserveEpisodeIndex != null) {
      player.episodeIndex = Math.min(opts.preserveEpisodeIndex, Math.max(0, rel.episodes.length - 1));
    } else {
      player.episodeIndex = 0;
    }
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
  if (!voice.releaseAlias || voice.id === player.selectedVoiceId) return;
  const currentEp = player.release?.episodes[player.episodeIndex];
  const prevOrdinal = currentEp?.ordinal;
  const prevIndex = player.episodeIndex;
  const token = ++activeRequestToken;
  const baseTarget = player.target;
  const nextTarget: WatchTarget = {
    title: baseTarget?.title || player.title,
    shikiId: baseTarget?.shikiId,
    releaseAlias: voice.releaseAlias,
    searchQueries: baseTarget?.searchQueries
  };
  await loadTargetIntoPlayer(token, nextTarget, {
    preserveEpisodeOrdinal: prevOrdinal,
    preserveEpisodeIndex: prevIndex,
    preserveTime: Number.isFinite(preserveTime) && preserveTime > 0.5 ? preserveTime : 0,
    keepVoices: true
  });
}

export async function openWatch(target: WatchTarget): Promise<void> {
  const token = ++activeRequestToken;
  player.seasons = [];
  navigate('anime');

  const rel = await loadTargetIntoPlayer(token, target);
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
  await loadTargetIntoPlayer(token, target);
}

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
  player.resumeTime = 0;
  player.autoplay = true;
  player.episodeIndex = index;
}

export function nextEpisode(): void {
  if (!player.release) return;
  if (player.episodeIndex + 1 < player.release.episodes.length) {
    player.resumeTime = 0;
    player.autoplay = true;
    player.episodeIndex += 1;
  }
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
}
