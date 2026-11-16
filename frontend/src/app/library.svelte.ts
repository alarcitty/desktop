import { buildMetaTags, ensureShikimoriArtwork } from '../api/anime';

export type LibraryListKind = 'likes' | 'watch_later' | 'favorites';

export interface LibraryItem {
  id: string;
  shikiId: number;
  releaseAlias?: string;
  searchQueries?: string[];
  title: string;
  description: string;
  poster: string;
  posterPosition?: string;
  score?: number;
  tags: string[];
  lists: LibraryListKind[];
  updatedAt?: number;
}

export interface LibraryMutationInput {
  shikiId?: number;
  releaseAlias?: string;
  searchQueries?: string[];
  title: string;
  description?: string;
  poster?: string;
  score?: number;
  tags?: string[];
}

export type LibraryMutationListener = (event: {
  type: 'upsert' | 'delete';
  item: LibraryItem;
}) => void;

const STORAGE_KEY = 'mila.library.v3';
const LEGACY_KEYS = ['mila.library.v2', 'mila.library.v1'] as const;
const LEGACY_SEEDED_IDS = new Set(['alya', 'black-clover', 'tsukimichi']);

const listeners = new Set<LibraryMutationListener>();

export function subscribeLibraryMutations(fn: LibraryMutationListener): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

function emitLibraryMutation(type: 'upsert' | 'delete', item: LibraryItem): void {
  for (const fn of listeners) {
    try {
      fn({ type, item });
    } catch {
      // Ignore subscriber errors
    }
  }
}

function isValidLibraryItem(item: unknown): item is LibraryItem {
  if (!item || typeof item !== 'object') return false;
  const candidate = item as Partial<LibraryItem>;
  return Boolean(
    typeof candidate.id === 'string' &&
      typeof candidate.title === 'string' &&
      Array.isArray(candidate.lists)
  );
}

export function normalizeLibraryTitle(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9а-яё]+/gi, '-')
    .replace(/^-+|-+$/g, '');
}

function loadLibrary(): LibraryItem[] {
  if (typeof localStorage === 'undefined') return [];
  try {
    const rawV3 = localStorage.getItem(STORAGE_KEY);
    if (rawV3 !== null) {
      const parsed = JSON.parse(rawV3);
      if (Array.isArray(parsed)) {
        return parsed.filter(isValidLibraryItem).filter((item) => item.lists.length > 0);
      }
    }

    for (const legacyKey of LEGACY_KEYS) {
      const rawLegacy = localStorage.getItem(legacyKey);
      if (rawLegacy === null) continue;
      const parsedLegacy = JSON.parse(rawLegacy);
      if (!Array.isArray(parsedLegacy)) continue;
      // Strip untouched default stubs from older versions while preserving real user saves.
      const migrated = parsedLegacy
        .filter(isValidLibraryItem)
        .filter((item) => !(LEGACY_SEEDED_IDS.has(item.id) && (!item.updatedAt || item.updatedAt <= 3)))
        .filter((item) => item.lists.length > 0);
      saveLibrary(migrated);
      return migrated;
    }

    return [];
  } catch {
    return [];
  }
}

function saveLibrary(items: LibraryItem[]): void {
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
  } catch {
    // Ignore storage quota or private-mode errors.
  }
}

export const localLibrary = $state<{ items: LibraryItem[] }>({
  items: loadLibrary()
});

function matchIndex(
  items: LibraryItem[],
  target: { id?: string; shikiId?: number; releaseAlias?: string; title?: string }
): number {
  const normTitle = target.title ? normalizeLibraryTitle(target.title) : '';
  return items.findIndex(
    (item) =>
      (target.id && item.id === target.id) ||
      (target.shikiId && target.shikiId > 0 && item.shikiId === target.shikiId) ||
      (target.releaseAlias && item.releaseAlias === target.releaseAlias) ||
      (normTitle && normalizeLibraryTitle(item.title) === normTitle)
  );
}

export function findLibraryEntry(target: {
  shikiId?: number;
  releaseAlias?: string;
  title?: string;
}): LibraryItem | undefined {
  const idx = matchIndex(localLibrary.items, target);
  return idx >= 0 ? localLibrary.items[idx] : undefined;
}

export function isInLibraryList(
  target: { shikiId?: number; releaseAlias?: string; title?: string },
  list: LibraryListKind
): boolean {
  const entry = findLibraryEntry(target);
  return Boolean(entry?.lists.includes(list));
}

export function isInWatchLater(target: {
  shikiId?: number;
  releaseAlias?: string;
  title?: string;
}): boolean {
  return isInLibraryList(target, 'watch_later');
}

export function isLiked(target: {
  shikiId?: number;
  releaseAlias?: string;
  title?: string;
}): boolean {
  return isInLibraryList(target, 'likes');
}

export function toggleLibraryList(list: LibraryListKind, input: LibraryMutationInput): boolean {
  const idx = matchIndex(localLibrary.items, input);
  if (idx >= 0) {
    const current = localLibrary.items[idx];
    const hasList = current.lists.includes(list);
    const nextLists = hasList
      ? current.lists.filter((l) => l !== list)
      : [...current.lists, list];

    const updated: LibraryItem = {
      ...current,
      shikiId: input.shikiId || current.shikiId,
      releaseAlias: input.releaseAlias || current.releaseAlias,
      searchQueries: input.searchQueries?.length ? input.searchQueries : current.searchQueries,
      description: input.description?.trim() || current.description,
      poster: input.poster || current.poster,
      score: input.score && input.score > 0 ? input.score : current.score,
      tags: input.tags?.length ? input.tags : current.tags,
      lists: nextLists,
      updatedAt: Date.now()
    };

    const nextItems =
      nextLists.length === 0
        ? localLibrary.items.filter((_, i) => i !== idx)
        : localLibrary.items.map((item, i) => (i === idx ? updated : item));

    localLibrary.items = nextItems;
    saveLibrary(nextItems);
    emitLibraryMutation(nextLists.length === 0 ? 'delete' : 'upsert', updated);
    return !hasList;
  }

  const newId =
    input.shikiId && input.shikiId > 0
      ? `shiki-${input.shikiId}`
      : input.releaseAlias?.trim() || normalizeLibraryTitle(input.title) || `lib-${Date.now()}`;

  const created: LibraryItem = {
    id: newId,
    shikiId: input.shikiId ?? 0,
    releaseAlias: input.releaseAlias,
    searchQueries: input.searchQueries,
    title: input.title,
    description: input.description?.trim() || '',
    poster: input.poster || '',
    score: input.score,
    tags: input.tags ?? [],
    lists: [list],
    updatedAt: Date.now()
  };

  const nextItems = [created, ...localLibrary.items];
  localLibrary.items = nextItems;
  saveLibrary(nextItems);
  emitLibraryMutation('upsert', created);
  return true;
}

export function toggleWatchLater(input: LibraryMutationInput): boolean {
  return toggleLibraryList('watch_later', input);
}

export function toggleLike(input: LibraryMutationInput): boolean {
  return toggleLibraryList('likes', input);
}

export function removeFromLibraryList(id: string, list: LibraryListKind): void {
  const existing = localLibrary.items.find((item) => item.id === id);
  if (!existing) return;
  const nextLists = existing.lists.filter((l) => l !== list);
  const updated: LibraryItem = { ...existing, lists: nextLists, updatedAt: Date.now() };
  const nextItems =
    nextLists.length === 0
      ? localLibrary.items.filter((item) => item.id !== id)
      : localLibrary.items.map((item) => (item.id === id ? updated : item));
  localLibrary.items = nextItems;
  saveLibrary(nextItems);
  emitLibraryMutation(nextLists.length === 0 ? 'delete' : 'upsert', updated);
}

/** Merges server library items into local state (without emitting cloud-write recursion). */
export function mergeCloudLibraryItems(remoteItems: LibraryItem[]): LibraryItem[] {
  const merged = [...localLibrary.items];
  for (const remote of remoteItems) {
    if (!isValidLibraryItem(remote) || remote.lists.length === 0) continue;
    const idx = matchIndex(merged, remote);
    if (idx < 0) {
      merged.push(remote);
      continue;
    }
    const local = merged[idx];
    const unionLists = Array.from(new Set([...local.lists, ...remote.lists]));
    const useRemoteLists = (remote.updatedAt ?? 0) >= (local.updatedAt ?? 0);
    merged[idx] = {
      ...local,
      shikiId: local.shikiId || remote.shikiId,
      releaseAlias: local.releaseAlias || remote.releaseAlias,
      searchQueries: local.searchQueries?.length ? local.searchQueries : remote.searchQueries,
      description: local.description || remote.description,
      poster: local.poster || remote.poster,
      posterPosition: local.posterPosition || remote.posterPosition,
      score: local.score || remote.score,
      tags: local.tags.length ? local.tags : remote.tags,
      lists: useRemoteLists ? remote.lists : unionLists,
      updatedAt: Math.max(local.updatedAt ?? 0, remote.updatedAt ?? 0)
    };
  }
  merged.sort((a, b) => (b.updatedAt ?? 0) - (a.updatedAt ?? 0));
  localLibrary.items = merged;
  saveLibrary(merged);
  return merged;
}

let enrichedArtwork = false;
export async function enrichLibraryArtwork(): Promise<void> {
  if (enrichedArtwork) return;
  enrichedArtwork = true;
  const ids = Array.from(new Set(localLibrary.items.map((i) => i.shikiId).filter((id) => id > 0)));
  if (ids.length === 0) return;
  const map = await ensureShikimoriArtwork(ids);
  const next = localLibrary.items.map((item) => {
    const shiki = map.get(item.shikiId);
    if (!shiki) return item;
    const nextTags = buildMetaTags(shiki);
    const mergedQueries = Array.from(
      new Set(
        [...(item.searchQueries ?? []), shiki.name, shiki.english, shiki.russian].filter(Boolean)
      )
    );
    return {
      ...item,
      searchQueries: mergedQueries.length > 0 ? mergedQueries : item.searchQueries,
      poster: shiki.posterOriginal || shiki.posterMain || item.poster,
      description: shiki.description ? shiki.description : item.description,
      score: shiki.score > 0 ? Number(shiki.score.toFixed(1)) : item.score,
      tags: nextTags.length > 0 ? nextTags : item.tags
    };
  });
  localLibrary.items = next;
  saveLibrary(next);
}
