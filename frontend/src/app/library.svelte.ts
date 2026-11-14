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

const STORAGE_KEY = 'mila.library.v2';
const LEGACY_STORAGE_KEY = 'mila.library.v1';

const DEFAULT_LIBRARY_ITEMS: LibraryItem[] = [
  {
    id: 'alya',
    shikiId: 54744,
    searchQueries: ['Tokidoki Bosotto', 'Аля иногда кокетничает'],
    title: 'Alya Sometimes Hides Her Feelings in Russian',
    description:
      'The story centers on Alisa Mikhailovna "Alya" Kujou, a stunning and brilliant student who acts cold and distant to everyone, and her unmotivated seatmate, Masachika Kuze. To hide her true feelings of embarrassment, affection, or frustration, Alya frequently mutters sweet.',
    poster: 'https://shikimori.io/uploads/poster/animes/54744/de6fc036f101d94a4f738be3f0415d2c.jpeg',
    score: 7.7,
    tags: ['2024', '12 ep', 'Comedy', 'Romance', 'School'],
    lists: ['likes'],
    updatedAt: 1
  },
  {
    id: 'black-clover',
    shikiId: 34572,
    searchQueries: ['Black Clover', 'Чёрный клевер'],
    title: 'Black Clover',
    description:
      'is a popular fantasy anime about two orphans, Asta and Yuno, who grew up together and strive to become the Wizard King. In a world where magic is everything, Yuno is a prodigy born with a rare grimoire, while Asta is the only person completely devoid of magical power. Refusing to give up, Asta obtains a mysterious five-leaf clover grimoire that grants him Anti-Magic.',
    poster: 'https://shikimori.io/uploads/poster/animes/34572/d4c5bf9cac831c03fc5ebed2676b7b10.jpeg',
    score: 8.1,
    tags: ['2017', '170 ep', 'Action', 'Comedy', 'Fantasy'],
    lists: ['likes'],
    updatedAt: 2
  },
  {
    id: 'tsukimichi',
    shikiId: 43523,
    releaseAlias: 'tsuki-ga-michibiku-isekai-douchuu',
    searchQueries: ['Tsuki ga Michibiku Isekai Douchuu', 'Лунное путешествие'],
    title: 'Tsukimichi: Moonlit Fantasy',
    description:
      'follows Makoto Misumi, an ordinary high schooler summoned to another world as a hero, only to be cast away by the goddess for his lack of beauty. Stripped of his title and sent to a barren wasteland, he discovers an overwhelming power of his own and slowly gathers allies among the monsters and outcasts of the new world.',
    poster: 'https://shikimori.io/uploads/poster/animes/43523/9f56a98052f05ad81a43452ecacf657d.jpeg',
    score: 7.7,
    tags: ['2021', '12 ep', 'Action', 'Adventure', 'Fantasy'],
    lists: ['likes'],
    updatedAt: 3
  }
];

const SEEDED_IDS = new Set(['alya', 'black-clover', 'tsukimichi']);

function isValidLibraryItem(item: unknown): item is LibraryItem {
  if (!item || typeof item !== 'object') return false;
  const candidate = item as Partial<LibraryItem>;
  return Boolean(
    typeof candidate.id === 'string' &&
      typeof candidate.title === 'string' &&
      Array.isArray(candidate.lists)
  );
}

function normalizeTitle(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9а-яё]+/gi, '-')
    .replace(/^-+|-+$/g, '');
}

function loadLibrary(): LibraryItem[] {
  if (typeof localStorage === 'undefined') return DEFAULT_LIBRARY_ITEMS;
  try {
    const rawV2 = localStorage.getItem(STORAGE_KEY);
    if (rawV2 !== null) {
      const parsed = JSON.parse(rawV2);
      if (Array.isArray(parsed)) {
        return parsed.filter(isValidLibraryItem);
      }
    }

    const rawLegacy = localStorage.getItem(LEGACY_STORAGE_KEY);
    if (rawLegacy !== null) {
      const parsedLegacy = JSON.parse(rawLegacy);
      if (Array.isArray(parsedLegacy)) {
        // Strip auto-seeded watch_later/favorites from untouched seed items while preserving real user saves.
        const migrated = parsedLegacy
          .filter(isValidLibraryItem)
          .map((item) => {
            if (SEEDED_IDS.has(item.id) && (!item.updatedAt || item.updatedAt <= 3)) {
              return {
                ...item,
                lists: item.lists.filter((l) => l === 'likes')
              };
            }
            return item;
          })
          .filter((item) => item.lists.length > 0);
        saveLibrary(migrated);
        return migrated;
      }
    }

    return DEFAULT_LIBRARY_ITEMS;
  } catch {
    return DEFAULT_LIBRARY_ITEMS;
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

function matchIndex(target: { shikiId?: number; releaseAlias?: string; title?: string }): number {
  const normTitle = target.title ? normalizeTitle(target.title) : '';
  return localLibrary.items.findIndex(
    (item) =>
      (target.shikiId && target.shikiId > 0 && item.shikiId === target.shikiId) ||
      (target.releaseAlias && item.releaseAlias === target.releaseAlias) ||
      (normTitle && normalizeTitle(item.title) === normTitle)
  );
}

export function findLibraryEntry(target: {
  shikiId?: number;
  releaseAlias?: string;
  title?: string;
}): LibraryItem | undefined {
  const idx = matchIndex(target);
  return idx >= 0 ? localLibrary.items[idx] : undefined;
}

export function isInWatchLater(target: {
  shikiId?: number;
  releaseAlias?: string;
  title?: string;
}): boolean {
  const entry = findLibraryEntry(target);
  return Boolean(entry?.lists.includes('watch_later'));
}

export function toggleWatchLater(input: {
  shikiId?: number;
  releaseAlias?: string;
  searchQueries?: string[];
  title: string;
  description?: string;
  poster?: string;
  score?: number;
  tags?: string[];
}): boolean {
  const idx = matchIndex(input);
  if (idx >= 0) {
    const current = localLibrary.items[idx];
    const hasWatchLater = current.lists.includes('watch_later');
    const nextLists = hasWatchLater
      ? current.lists.filter((l) => l !== 'watch_later')
      : [...current.lists, 'watch_later' as const];

    let nextItems: LibraryItem[];
    if (nextLists.length === 0) {
      nextItems = localLibrary.items.filter((_, i) => i !== idx);
    } else {
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
      nextItems = localLibrary.items.map((item, i) => (i === idx ? updated : item));
    }

    localLibrary.items = nextItems;
    saveLibrary(nextItems);
    return !hasWatchLater;
  }

  const newId =
    input.shikiId && input.shikiId > 0
      ? `shiki-${input.shikiId}`
      : input.releaseAlias?.trim() || normalizeTitle(input.title) || `lib-${Date.now()}`;

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
    lists: ['watch_later'],
    updatedAt: Date.now()
  };

  const nextItems = [created, ...localLibrary.items];
  localLibrary.items = nextItems;
  saveLibrary(nextItems);
  return true;
}

export function removeFromLibraryList(id: string, list: LibraryListKind): void {
  const nextItems = localLibrary.items
    .map((item) => (item.id === id ? { ...item, lists: item.lists.filter((l) => l !== list) } : item))
    .filter((item) => item.lists.length > 0);
  localLibrary.items = nextItems;
  saveLibrary(nextItems);
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
      new Set([
        ...(item.searchQueries ?? []),
        shiki.name,
        shiki.english,
        shiki.russian
      ].filter(Boolean))
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
