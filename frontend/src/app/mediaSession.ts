/**
 * Thin, DOM-free wrapper around the browser Media Session API.
 * Powers OS media keys (play/pause/next/prev) and the Windows lock-screen / SMTC overlay.
 * Every call is a no-op when the API is unavailable, so it is safe inside `vite dev` too.
 */

export interface MediaSessionHandlers {
  play: () => void;
  pause: () => void;
  next?: () => void;
  previous?: () => void;
  seek?: (time: number) => void;
}

export interface NowPlayingInfo {
  title: string;
  artist?: string;
  album?: string;
  artwork?: string;
}

type PlaybackState = 'none' | 'paused' | 'playing';

function getSession(): MediaSession | null {
  if (typeof navigator === 'undefined' || !navigator.mediaSession) {
    return null;
  }
  return navigator.mediaSession;
}

function setHandler(action: MediaSessionAction, handler: MediaSessionActionHandler | null): void {
  const session = getSession();
  if (!session) return;
  try {
    session.setActionHandler(action, handler);
  } catch {
    // Action is not supported by this engine; nothing else to do.
  }
}

/** Registers OS-level media key handlers, or removes them when `handlers` is null. */
export function installMediaSessionHandlers(handlers: MediaSessionHandlers | null): void {
  if (!getSession()) return;

  if (!handlers) {
    setHandler('play', null);
    setHandler('pause', null);
    setHandler('stop', null);
    setHandler('nexttrack', null);
    setHandler('previoustrack', null);
    setHandler('seekto', null);
    return;
  }

  setHandler('play', () => handlers.play());
  setHandler('pause', () => handlers.pause());
  setHandler('stop', () => handlers.pause());
  setHandler('nexttrack', handlers.next ? () => handlers.next?.() : null);
  setHandler('previoustrack', handlers.previous ? () => handlers.previous?.() : null);
  setHandler(
    'seekto',
    handlers.seek
      ? (details) => {
          if (typeof details.seekTime === 'number' && Number.isFinite(details.seekTime)) {
            handlers.seek?.(details.seekTime);
          }
        }
      : null
  );
}

/** Publishes title/episode/artwork to the OS overlay. Pass null to clear it. */
export function updateNowPlaying(info: NowPlayingInfo | null): void {
  const session = getSession();
  if (!session) return;

  if (!info || !info.title.trim() || typeof MediaMetadata === 'undefined') {
    try {
      session.metadata = null;
    } catch {
      // Ignore engines that reject clearing metadata.
    }
    return;
  }

  const artwork = info.artwork?.trim();
  try {
    session.metadata = new MediaMetadata({
      title: info.title.trim(),
      artist: info.artist?.trim() ?? '',
      album: info.album?.trim() ?? '',
      artwork: artwork ? [{ src: artwork }] : []
    });
  } catch {
    // Ignore metadata construction failures.
  }
}

/** Updates the progress bar shown on the lock screen / media overlay. */
export function updateNowPlayingPosition(
  positionSec: number,
  durationSec: number,
  playbackRate = 1
): void {
  const session = getSession();
  if (!session || typeof session.setPositionState !== 'function') return;
  if (!Number.isFinite(durationSec) || durationSec <= 0) return;

  const position = Math.min(
    Math.max(Number.isFinite(positionSec) ? positionSec : 0, 0),
    durationSec
  );
  const rate = Number.isFinite(playbackRate) && playbackRate > 0 ? playbackRate : 1;

  try {
    session.setPositionState({ duration: durationSec, position, playbackRate: rate });
  } catch {
    // Ignore inconsistent states while the stream is still buffering.
  }
}

/** Mirrors the play/pause state into the OS overlay controls. */
export function setMediaSessionPlaybackState(state: PlaybackState): void {
  const session = getSession();
  if (!session) return;
  try {
    session.playbackState = state;
  } catch {
    // Ignore unsupported states.
  }
}
