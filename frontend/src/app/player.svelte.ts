import {
  ensureShikimoriArtwork,
  getCachedArtwork,
  loadFranchiseSeasons,
  resolvePlayableRelease,
  type AnimeSeasonEntry,
  type PlayableRelease,
  type ShikiArtwork,
  type WatchTarget
} from '../api/anime';
import { navigate } from './router.svelte';

export const player = $state<{
  loading: boolean;
  error: string;
  title: string;
  target: WatchTarget | null;
  shikiArt: ShikiArtwork | null;
  release: PlayableRelease | null;
  seasons: AnimeSeasonEntry[];
  episodeIndex: number;
  autoplay: boolean;
}>({
  loading: false,
  error: '',
  title: '',
  target: null,
  shikiArt: null,
  release: null,
  seasons: [],
  episodeIndex: 0,
  autoplay: true
});

let activeRequestToken = 0;

async function loadTargetIntoPlayer(token: number, target: WatchTarget): Promise<PlayableRelease | null> {
  player.loading = true;
  player.error = '';
  player.title = target.title;
  player.target = target;
  player.shikiArt = target.shikiId ? getCachedArtwork(target.shikiId) ?? null : null;
  player.release = null;
  player.episodeIndex = 0;
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
    player.episodeIndex = 0;
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
  player.autoplay = true;
  player.episodeIndex = index;
}

export function nextEpisode(): void {
  if (!player.release) return;
  if (player.episodeIndex + 1 < player.release.episodes.length) {
    player.autoplay = true;
    player.episodeIndex += 1;
  }
}

export function resetPlayer(): void {
  activeRequestToken += 1;
  player.loading = false;
  player.release = null;
  player.seasons = [];
  player.error = '';
}
