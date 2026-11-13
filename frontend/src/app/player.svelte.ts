import {
  ensureShikimoriArtwork,
  getCachedArtwork,
  resolvePlayableRelease,
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
  episodeIndex: number;
  autoplay: boolean;
}>({
  loading: false,
  error: '',
  title: '',
  target: null,
  shikiArt: null,
  release: null,
  episodeIndex: 0,
  autoplay: true
});

let activeRequestToken = 0;

export async function openWatch(target: WatchTarget): Promise<void> {
  const token = ++activeRequestToken;
  player.loading = true;
  player.error = '';
  player.title = target.title;
  player.target = target;
  player.shikiArt = target.shikiId ? getCachedArtwork(target.shikiId) ?? null : null;
  player.release = null;
  player.episodeIndex = 0;
  player.autoplay = true;

  navigate('anime');

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
    if (token !== activeRequestToken) return;
    player.release = rel;
    player.title = player.shikiArt?.english || player.shikiArt?.name || rel.title || target.title;
    player.episodeIndex = 0;
  } catch (err) {
    if (token !== activeRequestToken) return;
    player.error = err instanceof Error ? err.message : 'Failed to load stream';
  } finally {
    if (token === activeRequestToken) {
      player.loading = false;
    }
  }
}

export async function retryWatch(): Promise<void> {
  if (!player.target) return;
  await openWatch(player.target);
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
  player.error = '';
}
