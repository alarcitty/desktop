import { buildMetaTags, ensureShikimoriArtwork } from '../api/anime';
import {
  createDebouncedStorageWriter,
  registerBeforeCloseFlush,
  safeStorageSet
} from './lifecycle';

export interface WatchHistoryEntry {
  id: string;
  shikiId?: number;
  releaseAlias?: string;
  searchQueries?: string[];
  title: string;
  description: string;
  poster: string;
  posterPosition?: string;
  score?: number;
  tags: string[];
  episodeOrdinal: number;
  episodeIndex: number;
  totalEpisodes?: number;
  currentTime: number;
  duration: number;
  updatedAt: number;
}

export interface RecordHistoryInput {
  shikiId?: number;
  releaseAlias?: string;
  searchQueries?: string[];
  title: string;
  description?: string;
  poster?: string;
  posterPosition?: string;
  score?: number;
  tags?: string[];
  episodeOrdinal: number;
  episodeIndex: number;
  totalEpisodes?: number;
  currentTime?: number;
  duration?: number;
}

export interface EpisodeProgressEntry {
  currentTime: number;
  duration: number;
  watched: boolean;
  updatedAt: number;
}

const STORAGE_KEY = 'mila.watch.history.v1';
const EPISODE_PROGRESS_KEY = 'mila.watch.episodeProgress.v1';
const MAX_HISTORY_ENTRIES = 100;
const MAX_EPISODE_PROGRESS_KEYS = 800;

export type HistoryMutationEvent =
  | { type: 'upsert'; entry: WatchHistoryEntry; episodeProgress?: Record<string, EpisodeProgressEntry> }
  | { type: 'delete'; id: string }
  | { type: 'clear' };

export type HistoryMutationListener = (event: HistoryMutationEvent) => void;

const historyListeners = new Set<HistoryMutationListener>();

export function subscribeHistoryMutations(fn: HistoryMutationListener): () => void {
  historyListeners.add(fn);
  return () => {
    historyListeners.delete(fn);
  };
}

function emitHistoryMutation(event: HistoryMutationEvent): void {
  for (const fn of historyListeners) {
    try {
      fn(event);
    } catch {
      // Ignore subscriber errors
    }
  }
}

function normalizeTitleKey(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9а-яё]+/gi, '-')
    .replace(/^-+|-+$/g, '');
}

function buildEntryId(input: Pick<RecordHistoryInput, 'shikiId' | 'releaseAlias' | 'title'>): string {
  if (input.shikiId && input.shikiId > 0) return `shiki:${input.shikiId}`;
  if (input.releaseAlias?.trim()) return `alias:${input.releaseAlias.trim()}`;
  return `title:${normalizeTitleKey(input.title) || 'anime'}`;
}

function loadHistory(): WatchHistoryEntry[] {
  if (typeof localStorage === 'undefined') return [];
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter((x): x is WatchHistoryEntry => Boolean(x && typeof x.id === 'string' && typeof x.title === 'string'))
      .sort((a, b) => (b.updatedAt || 0) - (a.updatedAt || 0))
      .slice(0, MAX_HISTORY_ENTRIES);
  } catch {
    return [];
  }
}

const HISTORY_SAVE_DEBOUNCE_MS = 1000;
const EPISODE_PROGRESS_DEBOUNCE_MS = 300;

function writeHistoryEntries(entries: WatchHistoryEntry[]): void {
  if (typeof localStorage === 'undefined') return;
  try {
    safeStorageSet(STORAGE_KEY, JSON.stringify(entries));
  } catch {
    // Ignore serialization errors.
  }
}

const historyWriter = createDebouncedStorageWriter<WatchHistoryEntry[]>(
  HISTORY_SAVE_DEBOUNCE_MS,
  writeHistoryEntries
);

function saveHistory(entries: WatchHistoryEntry[], immediate = false): void {
  if (immediate) {
    historyWriter.cancel();
    writeHistoryEntries(entries);
    return;
  }
  historyWriter.schedule(entries);
}

export const watchHistory = $state<{ entries: WatchHistoryEntry[] }>({
  entries: loadHistory()
});

function loadEpisodeProgressMap(): Record<string, EpisodeProgressEntry> {
  if (typeof localStorage === 'undefined') return {};
  try {
    const raw = localStorage.getItem(EPISODE_PROGRESS_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? (parsed as Record<string, EpisodeProgressEntry>)
      : {};
  } catch {
    return {};
  }
}

export const episodeProgressMap = $state<{ items: Record<string, EpisodeProgressEntry> }>({
  items: loadEpisodeProgressMap()
});

const episodeProgressWriter = createDebouncedStorageWriter<Record<string, EpisodeProgressEntry>>(
  EPISODE_PROGRESS_DEBOUNCE_MS,
  (items) => {
    if (typeof localStorage === 'undefined') return;
    try {
      const entries = Object.entries(items)
        .sort((a, b) => (b[1]?.updatedAt ?? 0) - (a[1]?.updatedAt ?? 0))
        .slice(0, MAX_EPISODE_PROGRESS_KEYS);
      safeStorageSet(EPISODE_PROGRESS_KEY, JSON.stringify(Object.fromEntries(entries)));
    } catch {
      // Ignore storage quota errors.
    }
  }
);

function scheduleSaveEpisodeProgress(): void {
  if (typeof localStorage === 'undefined') return;
  episodeProgressWriter.schedule(episodeProgressMap.items);
}

/** Synchronously flushes any debounced history/progress writes to localStorage. */
export function flushHistoryPersistence(): void {
  historyWriter.flush();
  episodeProgressWriter.flush();
}

let flushHooksRegistered = false;
function ensureFlushHooks(): void {
  if (flushHooksRegistered) return;
  flushHooksRegistered = true;
  registerBeforeCloseFlush(flushHistoryPersistence);
  if (typeof window === 'undefined') return;
  window.addEventListener('beforeunload', flushHistoryPersistence);
  window.addEventListener('pagehide', flushHistoryPersistence);
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'hidden') {
        flushHistoryPersistence();
      }
    });
  }
}
ensureFlushHooks();

function episodeProgressKeys(
  input: { shikiId?: number; releaseAlias?: string; title?: string },
  ordinal: number
): string[] {
  const keys: string[] = [];
  if (input.shikiId && input.shikiId > 0) {
    keys.push(`s:${input.shikiId}:ep:${ordinal}`);
  }
  if (input.releaseAlias?.trim()) {
    keys.push(`a:${input.releaseAlias.trim()}:ep:${ordinal}`);
  }
  if (keys.length === 0 && input.title?.trim()) {
    keys.push(`t:${normalizeTitleKey(input.title) || 'anime'}:ep:${ordinal}`);
  }
  return keys;
}

export function getEpisodeProgress(
  input: { shikiId?: number; releaseAlias?: string; title?: string },
  ordinal: number
): EpisodeProgressEntry | undefined {
  for (const key of episodeProgressKeys(input, ordinal)) {
    const found = episodeProgressMap.items[key];
    if (found) return found;
  }
  return undefined;
}

export function formatTimeCompact(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0:00';
  const total = Math.floor(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) {
    return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  }
  return `${m}:${String(s).padStart(2, '0')}`;
}

export function recordHistoryEntry(input: RecordHistoryInput): void {
  const title = input.title.trim();
  if (!title) return;

  const id = buildEntryId(input);
  const normTitle = normalizeTitleKey(title);
  const existingIdx = watchHistory.entries.findIndex(
    (item) =>
      item.id === id ||
      (input.shikiId && item.shikiId === input.shikiId) ||
      (input.releaseAlias && item.releaseAlias === input.releaseAlias) ||
      (normTitle && normalizeTitleKey(item.title) === normTitle)
  );

  const prev = existingIdx >= 0 ? watchHistory.entries[existingIdx] : undefined;
  const sameEpisode =
    prev != null &&
    prev.episodeOrdinal === input.episodeOrdinal &&
    prev.episodeIndex === input.episodeIndex;

  const nextTime =
    typeof input.currentTime === 'number' && Number.isFinite(input.currentTime)
      ? Math.max(0, input.currentTime)
      : sameEpisode
        ? prev.currentTime
        : 0;

  const nextDuration =
    typeof input.duration === 'number' && Number.isFinite(input.duration) && input.duration > 0
      ? input.duration
      : sameEpisode
        ? prev.duration
        : 0;

  const entry: WatchHistoryEntry = {
    id: prev?.id && id.startsWith('shiki:') ? id : prev?.id || id,
    shikiId: input.shikiId || prev?.shikiId,
    releaseAlias: input.releaseAlias || prev?.releaseAlias,
    searchQueries: input.searchQueries?.length ? input.searchQueries : prev?.searchQueries,
    title: title || prev?.title || 'Anime',
    description: input.description?.trim() || prev?.description || '',
    poster: input.poster || prev?.poster || '',
    posterPosition: input.posterPosition || prev?.posterPosition,
    score: input.score && input.score > 0 ? input.score : prev?.score,
    tags: input.tags && input.tags.length > 0 ? input.tags : prev?.tags || [],
    episodeOrdinal: input.episodeOrdinal,
    episodeIndex: input.episodeIndex,
    totalEpisodes: input.totalEpisodes ?? prev?.totalEpisodes,
    currentTime: nextTime,
    duration: nextDuration,
    updatedAt: Date.now()
  };

  const remaining = watchHistory.entries.filter((_, idx) => idx !== existingIdx);
  const next = [entry, ...remaining].slice(0, MAX_HISTORY_ENTRIES);
  watchHistory.entries = next;
  saveHistory(next);

  // Sync per-episode watched & progress markers
  if (input.episodeOrdinal > 0) {
    const keys = episodeProgressKeys(
      { shikiId: entry.shikiId, releaseAlias: entry.releaseAlias, title: entry.title },
      input.episodeOrdinal
    );
    const existingEp = keys.map((k) => episodeProgressMap.items[k]).find(Boolean);
    const dur = nextDuration > 0 ? nextDuration : existingEp?.duration ?? 0;
    const isWatchedNow = dur > 0 && nextTime / dur >= 0.85;
    const nextWatched = Boolean(existingEp?.watched || isWatchedNow);
    const allKeysPresent = keys.every((k) => Boolean(episodeProgressMap.items[k]));
    const timeChanged =
      !existingEp ||
      Math.abs(existingEp.currentTime - nextTime) >= 0.5 ||
      existingEp.watched !== nextWatched ||
      existingEp.duration !== dur ||
      !allKeysPresent;
    if (timeChanged) {
      const mergedEp: EpisodeProgressEntry = {
        currentTime: nextTime,
        duration: dur,
        watched: nextWatched,
        updatedAt: Date.now()
      };
      const nextMap = { ...episodeProgressMap.items };
      for (const key of keys) {
        nextMap[key] = mergedEp;
      }
      episodeProgressMap.items = nextMap;
      scheduleSaveEpisodeProgress();
      const partialProgress: Record<string, EpisodeProgressEntry> = {};
      for (const key of keys) {
        partialProgress[key] = mergedEp;
      }
      emitHistoryMutation({ type: 'upsert', entry, episodeProgress: partialProgress });
      return;
    }
  }
  emitHistoryMutation({ type: 'upsert', entry });
}

export function removeHistoryEntry(id: string): void {
  const next = watchHistory.entries.filter((entry) => entry.id !== id);
  watchHistory.entries = next;
  saveHistory(next, true);
  emitHistoryMutation({ type: 'delete', id });
}

export function resetAllEpisodeProgress(): void {
  episodeProgressWriter.cancel();
  episodeProgressMap.items = {};
  if (typeof localStorage !== 'undefined') {
    try {
      localStorage.removeItem(EPISODE_PROGRESS_KEY);
    } catch {
      // Ignore storage errors.
    }
  }
}

export function clearWatchHistory(): void {
  watchHistory.entries = [];
  saveHistory([], true);
  resetAllEpisodeProgress();
  emitHistoryMutation({ type: 'clear' });
}

/** Merges server history and episode progress cleanly into local state without re-emitting mutations. */
export function mergeCloudHistoryState(
  remoteEntries: WatchHistoryEntry[],
  remoteProgress: Record<string, EpisodeProgressEntry>
): {
  entries: WatchHistoryEntry[];
  episodeProgress: Record<string, EpisodeProgressEntry>;
} {
  const merged = [...watchHistory.entries];
  for (const remote of remoteEntries) {
    if (!remote || typeof remote.id !== 'string' || typeof remote.title !== 'string') continue;
    const normRemote = normalizeTitleKey(remote.title);
    const idx = merged.findIndex(
      (local) =>
        local.id === remote.id ||
        (remote.shikiId && local.shikiId === remote.shikiId) ||
        (remote.releaseAlias && local.releaseAlias === remote.releaseAlias) ||
        (normRemote && normalizeTitleKey(local.title) === normRemote)
    );
    if (idx < 0) {
      merged.push(remote);
      continue;
    }
    const local = merged[idx];
    const remoteNewer = (remote.updatedAt ?? 0) >= (local.updatedAt ?? 0);
    merged[idx] = {
      ...local,
      id: remote.id.startsWith('shiki:') ? remote.id : local.id,
      shikiId: local.shikiId || remote.shikiId,
      releaseAlias: local.releaseAlias || remote.releaseAlias,
      searchQueries: local.searchQueries?.length ? local.searchQueries : remote.searchQueries,
      title: remoteNewer ? remote.title : local.title,
      description: local.description || remote.description,
      poster: local.poster || remote.poster,
      posterPosition: local.posterPosition || remote.posterPosition,
      score: local.score || remote.score,
      tags: local.tags.length ? local.tags : remote.tags,
      episodeOrdinal: remoteNewer ? remote.episodeOrdinal : local.episodeOrdinal,
      episodeIndex: remoteNewer ? remote.episodeIndex : local.episodeIndex,
      totalEpisodes: remote.totalEpisodes ?? local.totalEpisodes,
      currentTime: remoteNewer ? remote.currentTime : local.currentTime,
      duration: Math.max(local.duration ?? 0, remote.duration ?? 0),
      updatedAt: Math.max(local.updatedAt ?? 0, remote.updatedAt ?? 0)
    };
  }
  merged.sort((a, b) => (b.updatedAt ?? 0) - (a.updatedAt ?? 0));
  const sliced = merged.slice(0, MAX_HISTORY_ENTRIES);
  watchHistory.entries = sliced;
  saveHistory(sliced, true);

  const mergedEp: Record<string, EpisodeProgressEntry> = { ...episodeProgressMap.items };
  for (const [k, rp] of Object.entries(remoteProgress ?? {})) {
    if (!rp || typeof rp.currentTime !== 'number') continue;
    const lp = mergedEp[k];
    if (!lp) {
      mergedEp[k] = rp;
      continue;
    }
    const remoteNewer = (rp.updatedAt ?? 0) >= (lp.updatedAt ?? 0);
    mergedEp[k] = {
      currentTime: remoteNewer ? rp.currentTime : lp.currentTime,
      duration: Math.max(lp.duration ?? 0, rp.duration ?? 0),
      watched: Boolean(lp.watched || rp.watched),
      updatedAt: Math.max(lp.updatedAt ?? 0, rp.updatedAt ?? 0)
    };
  }
  episodeProgressMap.items = mergedEp;
  episodeProgressWriter.schedule(mergedEp);

  return { entries: sliced, episodeProgress: mergedEp };
}

let enrichedHistoryArtwork = false;
export async function enrichHistoryArtwork(): Promise<void> {
  if (enrichedHistoryArtwork) return;
  enrichedHistoryArtwork = true;
  const ids = Array.from(
    new Set(watchHistory.entries.map((e) => e.shikiId ?? 0).filter((id) => id > 0))
  );
  if (ids.length === 0) return;
  const map = await ensureShikimoriArtwork(ids);
  const next = watchHistory.entries.map((entry) => {
    const shiki = entry.shikiId ? map.get(entry.shikiId) : undefined;
    if (!shiki) return entry;
    const nextTags = buildMetaTags(shiki);
    const mergedQueries = Array.from(
      new Set([
        ...(entry.searchQueries ?? []),
        shiki.name,
        shiki.english,
        shiki.russian
      ].filter(Boolean))
    );
    return {
      ...entry,
      searchQueries: mergedQueries.length > 0 ? mergedQueries : entry.searchQueries,
      poster: shiki.posterOriginal || shiki.posterMain || entry.poster,
      description: shiki.description ? shiki.description : entry.description,
      score: shiki.score > 0 ? Number(shiki.score.toFixed(1)) : entry.score,
      tags: nextTags.length > 0 ? nextTags : entry.tags
    };
  });
  watchHistory.entries = next;
  saveHistory(next);
}
