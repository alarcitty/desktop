import { buildMetaTags, ensureShikimoriArtwork } from '../api/anime';

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

const STORAGE_KEY = 'mila.watch.history.v1';
const MAX_HISTORY_ENTRIES = 100;

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

function saveHistory(entries: WatchHistoryEntry[]): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(entries));
  } catch {
    // Ignore storage quota or private-mode errors.
  }
}

export const watchHistory = $state<{ entries: WatchHistoryEntry[] }>({
  entries: loadHistory()
});

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
}

export function removeHistoryEntry(id: string): void {
  const next = watchHistory.entries.filter((entry) => entry.id !== id);
  watchHistory.entries = next;
  saveHistory(next);
}

export function clearWatchHistory(): void {
  watchHistory.entries = [];
  saveHistory([]);
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
