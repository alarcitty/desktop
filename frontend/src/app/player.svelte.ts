import {
  resolvePlayableRelease,
  type PlayableRelease,
  type WatchTarget
} from '../api/anime';

export const player = $state<{
  open: boolean;
  loading: boolean;
  error: string;
  title: string;
  release: PlayableRelease | null;
  episodeIndex: number;
}>({
  open: false,
  loading: false,
  error: '',
  title: '',
  release: null,
  episodeIndex: 0
});

let activeRequestToken = 0;

export async function openWatch(target: WatchTarget): Promise<void> {
  const token = ++activeRequestToken;
  player.open = true;
  player.loading = true;
  player.error = '';
  player.title = target.title;
  player.release = null;
  player.episodeIndex = 0;

  try {
    const rel = await resolvePlayableRelease(target);
    if (token !== activeRequestToken || !player.open) return;
    player.release = rel;
    player.title = rel.title || target.title;
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

export function selectEpisode(index: number): void {
  if (!player.release) return;
  if (index < 0 || index >= player.release.episodes.length) return;
  player.episodeIndex = index;
}

export function nextEpisode(): void {
  if (!player.release) return;
  if (player.episodeIndex + 1 < player.release.episodes.length) {
    player.episodeIndex += 1;
  }
}

export function closePlayer(): void {
  activeRequestToken += 1;
  player.open = false;
  player.loading = false;
  player.release = null;
  player.error = '';
}
