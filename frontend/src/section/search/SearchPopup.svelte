<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { cubicOut } from 'svelte/easing';
  import { fade } from 'svelte/transition';
  import Hls from 'hls.js';
  import Search from '@lucide/svelte/icons/search';
  import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';

  import MorphPopup from '../../marlakit/popup/MorphPopup.svelte';
  import Input from '../../marlakit/input/Input.svelte';
  import ComboBox from '../../marlakit/combo/ComboBox.svelte';
  import ScrollArea from '../../marlakit/scroll/ScrollArea.svelte';
  import LayerBlur from '../../marlakit/blur/LayerBlur.svelte';
  import MetaLine from '../../marlakit/meta/MetaLine.svelte';
  import type { ComboOption } from '../../marlakit/combo/types';
  import { formatTimeCompact, watchHistory, type WatchHistoryEntry } from '../../app/history.svelte';
  import { localLibrary, type LibraryItem } from '../../app/library.svelte';
  import {
    search,
    openSearch,
    closeSearch,
    finishSearchClose,
    getSearchAnchorRect
  } from '../../app/search.svelte';
  import { openWatch } from '../../app/player.svelte';
  import {
    ensureShikimoriArtwork,
    getCachedArtwork,
    getCachedHeroOpeningPreview,
    loadCatalogGenres,
    resolveHeroOpeningPreview,
    searchGroupedAnime,
    warmHeroOpeningPreview,
    type CatalogGenreOption,
    type HeroOpeningPreview,
    type SearchFilterMode,
    type SearchGroupItem
  } from '../../api/anime';
  import { catalog, type AnimeEntry } from '../../data/catalog';

  interface UnifiedSearchItem extends SearchGroupItem {
    sourceBadge?: 'History' | 'Library';
    progressSub?: string;
    historyResume?: {
      episodeOrdinal: number;
      episodeIndex: number;
      resumeTime: number;
    };
  }

  type ComboMenuId = 'scope' | 'type' | 'genre' | 'year' | 'score' | 'episodes' | 'sort';

  const YEAR_MIN = 1970;
  const YEAR_MAX = 2026;
  const EP_MIN = 1;
  const EP_MAX = 100; // 100 represents 100+ (no upper limit)

  const ROW_H = 88;
  const ROW_GAP = 6;
  const MAX_ROWS = 5;
  const MAX_RESULT_ITEMS = 50;
  const EMPTY_H = 88;
  const SKELETON_COUNT = 4;

  const SCOPE_OPTIONS: ComboOption[] = [
    { value: 'all', label: 'All anime' },
    { value: 'ongoing', label: 'Ongoing' },
    { value: 'library', label: 'My Library' }
  ];

  const TYPE_OPTIONS: ComboOption[] = [
    { value: '', label: 'Any type' },
    { value: 'TV', label: 'TV Series' },
    { value: 'MOVIE', label: 'Movie' },
    { value: 'OVA', label: 'OVA' }
  ];

  const YEAR_RANGES: Array<{ value: string; label: string; from: number; to: number }> = [
    { value: 'all', label: 'Any year', from: YEAR_MIN, to: YEAR_MAX },
    { value: '2025-2026', label: '2025 – 2026', from: 2025, to: 2026 },
    { value: '2020-2024', label: '2020 – 2024', from: 2020, to: 2024 },
    { value: '2015-2019', label: '2015 – 2019', from: 2015, to: 2019 },
    { value: '2010-2014', label: '2010 – 2014', from: 2010, to: 2014 },
    { value: '2000-2009', label: '2000 – 2009', from: 2000, to: 2009 },
    { value: '1970-1999', label: 'Classics (<2000)', from: 1970, to: 1999 }
  ];

  const YEAR_OPTIONS: ComboOption[] = YEAR_RANGES.map((r) => ({
    value: r.value,
    label: r.label
  }));

  const EP_RANGES: Array<{ value: string; label: string; min: number; max: number }> = [
    { value: 'all', label: 'Any length', min: EP_MIN, max: EP_MAX },
    { value: '1-12', label: '1 – 12 ep', min: 1, max: 12 },
    { value: '13-26', label: '13 – 26 ep', min: 13, max: 26 },
    { value: '27-60', label: '27 – 60 ep', min: 27, max: 60 },
    { value: '60-100', label: '60+ ep', min: 60, max: EP_MAX }
  ];

  const EP_OPTIONS: ComboOption[] = EP_RANGES.map((r) => ({
    value: r.value,
    label: r.label
  }));

  const SCORE_OPTIONS: ComboOption[] = [
    { value: 0, label: 'Any rating' },
    { value: 8.5, label: '★ 8.5+' },
    { value: 8.0, label: '★ 8.0+' },
    { value: 7.5, label: '★ 7.5+' },
    { value: 7.0, label: '★ 7.0+' },
    { value: 6.0, label: '★ 6.0+' }
  ];

  const SORT_OPTIONS: ComboOption[] = [
    { value: '', label: 'Relevance' },
    { value: 'RATING_DESC', label: 'Top rated' },
    { value: 'FRESH_AT_DESC', label: 'Newest' }
  ];

  const RU_TO_LAT: Record<string, string> = {
    а: 'a', б: 'b', в: 'v', г: 'g', д: 'd', е: 'e', ё: 'e', ж: 'zh',
    з: 'z', и: 'i', й: 'y', к: 'k', л: 'l', м: 'm', н: 'n', о: 'o',
    п: 'p', р: 'r', с: 's', т: 't', у: 'u', ф: 'f', х: 'h', ц: 'ts',
    ч: 'ch', ш: 'sh', щ: 'sch', ъ: '', ы: 'y', ь: '', э: 'e', ю: 'yu', я: 'ya'
  };

  function toSearchKey(raw: string): string {
    let out = '';
    const lower = raw.toLowerCase();
    for (let i = 0; i < lower.length; i++) {
      const ch = lower[i];
      out += RU_TO_LAT[ch] ?? ch;
    }
    return out.replace(/ph/g, 'f').replace(/ee/g, 'i').replace(/oo/g, 'u');
  }

  function matchesLocalQuery(candidates: Array<string | undefined>, q: string): boolean {
    if (!q) return true;
    const qKey = toSearchKey(q);
    for (const raw of candidates) {
      if (!raw) continue;
      const low = raw.toLowerCase();
      if (low.includes(q)) return true;
      if (qKey.length >= 2 && toSearchKey(low).includes(qKey)) return true;
    }
    return false;
  }

  function normalizeTitle(title: string): string {
    return title.trim().toLowerCase();
  }

  function sameTarget(
    a: { shikiId?: number; releaseAlias?: string; title: string },
    b: { shikiId?: number; releaseAlias?: string; title: string }
  ): boolean {
    if (a.shikiId && b.shikiId && a.shikiId === b.shikiId) return true;
    if (a.releaseAlias && b.releaseAlias && a.releaseAlias === b.releaseAlias) return true;
    return normalizeTitle(a.title) === normalizeTitle(b.title);
  }

  function extractEpisodesFromTags(tags?: string[], fallbackTotal?: number): number | undefined {
    if (fallbackTotal && fallbackTotal > 0) return fallbackTotal;
    if (!tags) return undefined;
    for (const tag of tags) {
      const m = tag.match(/(\d+)\s*ep/i);
      if (m) return Number(m[1]);
    }
    return undefined;
  }

  let selected = $state(0);
  let activeFilter = $state<SearchFilterMode>('all');
  let activeCombo = $state<ComboMenuId | null>(null);
  let vh = $state(900);
  let shikiTick = $state(0);

  let yearRangeKey = $state('all');
  let epRangeKey = $state('all');
  let minScore = $state(0);
  let typeFilter = $state<'' | 'TV' | 'MOVIE' | 'OVA'>('');
  let sortFilter = $state<'' | 'RATING_DESC' | 'FRESH_AT_DESC'>('');
  let selectedGenreIds = $state<number[]>([]);
  let genreOptions = $state<CatalogGenreOption[]>([]);

  let remoteResults = $state<SearchGroupItem[]>([]);
  let searching = $state(false);
  let remoteKey = $state('');
  let searchSeq = 0;
  let rootEl = $state<HTMLDivElement | null>(null);
  let resultRefs: Array<HTMLButtonElement | undefined> = [];

  onMount(() => {
    loadCatalogGenres().then((list) => {
      if (list.length > 0) genreOptions = list;
    });

    const seedShikiIds = [
      ...catalog.map((c) => c.shikiId),
      ...localLibrary.items.map((i) => i.shikiId),
      ...watchHistory.entries.map((e) => e.shikiId ?? 0)
    ].filter((id) => id > 0);
    if (seedShikiIds.length > 0) {
      ensureShikimoriArtwork(seedShikiIds)
        .then(() => {
          shikiTick += 1;
        })
        .catch(() => {});
    }

    // Pre-warm opening streams + initial HLS segments for top catalog titles so hover is instantaneous
    for (const entry of catalog.slice(0, 8)) {
      warmHeroOpeningPreview({
        title: entry.title,
        releaseAlias: entry.releaseAlias,
        shikiId: entry.shikiId,
        searchQueries: entry.searchQueries
      });
    }
  });

  const activeYearRange = $derived(
    YEAR_RANGES.find((r) => r.value === yearRangeKey) ?? YEAR_RANGES[0]
  );
  const yearFrom = $derived(activeYearRange.from);
  const yearTo = $derived(activeYearRange.to);

  const activeEpRange = $derived(
    EP_RANGES.find((r) => r.value === epRangeKey) ?? EP_RANGES[0]
  );
  const epMin = $derived(activeEpRange.min);
  const epMax = $derived(activeEpRange.max);

  const hasYearFilter = $derived(yearRangeKey !== 'all');
  const hasEpisodeFilter = $derived(epRangeKey !== 'all');
  const hasScoreFilter = $derived(minScore > 0);
  const hasTypeFilter = $derived(typeFilter !== '');
  const hasSortFilter = $derived(sortFilter !== '');
  const hasGenreFilter = $derived(selectedGenreIds.length > 0);
  const hasScopeFilter = $derived(activeFilter !== 'all');

  const activeCustomFilterCount = $derived(
    (hasScopeFilter ? 1 : 0) +
      (hasYearFilter ? 1 : 0) +
      (hasEpisodeFilter ? 1 : 0) +
      (hasScoreFilter ? 1 : 0) +
      (hasTypeFilter ? 1 : 0) +
      (hasSortFilter ? 1 : 0) +
      selectedGenreIds.length
  );

  const hasAnyCustomFilter = $derived(activeCustomFilterCount > 0);

  const scopeComboLabel = $derived(
    SCOPE_OPTIONS.find((o) => o.value === activeFilter)?.label ?? 'All anime'
  );

  const typeComboLabel = $derived(
    typeFilter === ''
      ? 'Type'
      : (TYPE_OPTIONS.find((o) => o.value === typeFilter)?.label ?? typeFilter)
  );

  const yearComboLabel = $derived(
    yearRangeKey === 'all' ? 'Years' : activeYearRange.label
  );

  const epComboLabel = $derived(
    epRangeKey === 'all' ? 'Episodes' : activeEpRange.label
  );

  const scoreComboLabel = $derived(
    minScore > 0 ? `★ ${minScore.toFixed(1)}+` : 'Rating'
  );

  const sortComboLabel = $derived(
    sortFilter === ''
      ? 'Sort'
      : (SORT_OPTIONS.find((o) => o.value === sortFilter)?.label ?? 'Sort')
  );

  const genreComboLabel = $derived.by(() => {
    if (selectedGenreIds.length === 0) return 'Genres';
    if (selectedGenreIds.length === 1) {
      return genreOptions.find((g) => g.id === selectedGenreIds[0])?.name ?? '1 genre';
    }
    return `Genres (${selectedGenreIds.length})`;
  });

  const genreComboOptions = $derived<ComboOption[]>([
    {
      value: 0,
      label: 'All genres',
      selected: selectedGenreIds.length === 0
    },
    ...genreOptions.map((g) => ({
      value: g.id,
      label: g.name,
      selected: selectedGenreIds.includes(g.id)
    }))
  ]);

  const selectedGenreNames = $derived(
    selectedGenreIds
      .map((id) => genreOptions.find((g) => g.id === id)?.name?.toLowerCase())
      .filter((n): n is string => Boolean(n))
  );

  function resetCustomFilters(): void {
    activeFilter = 'all';
    yearRangeKey = 'all';
    epRangeKey = 'all';
    minScore = 0;
    typeFilter = '';
    sortFilter = '';
    selectedGenreIds = [];
    activeCombo = null;
  }

  function toggleGenre(id: number): void {
    if (id === 0) {
      selectedGenreIds = [];
      activeCombo = null;
      return;
    }
    if (selectedGenreIds.includes(id)) {
      selectedGenreIds = selectedGenreIds.filter((g) => g !== id);
    } else {
      selectedGenreIds = [...selectedGenreIds, id];
    }
  }

  function setComboOpen(id: ComboMenuId, isOpen: boolean): void {
    if (isOpen) {
      activeCombo = id;
    } else if (activeCombo === id) {
      activeCombo = null;
    }
  }

  function historyToSearchItem(entry: WatchHistoryEntry): UnifiedSearchItem {
    const shiki = entry.shikiId ? getCachedArtwork(entry.shikiId) : undefined;
    const epPart = entry.totalEpisodes
      ? `Ep ${entry.episodeOrdinal}/${entry.totalEpisodes}`
      : `Ep ${entry.episodeOrdinal}`;
    const timePart =
      entry.currentTime >= 5 ? `${epPart} · ${formatTimeCompact(entry.currentTime)}` : epPart;
    const yearTag = entry.tags.find((t) => /^\d{4}$/.test(t));
    const rawScore = entry.score || shiki?.score || 0;

    return {
      id: `hist-${entry.id}`,
      title: entry.title,
      year: yearTag ? Number(yearTag) : shiki?.year,
      score: rawScore > 0 ? Number(rawScore.toFixed(1)) : undefined,
      typeLabel: 'TV',
      episodesTotal:
        extractEpisodesFromTags(entry.tags, entry.totalEpisodes) ??
        shiki?.episodes ??
        shiki?.episodesAired,
      genres: entry.tags.length > 0 ? entry.tags : shiki?.genres,
      poster: entry.poster || shiki?.posterMain || '',
      posterPosition: entry.posterPosition,
      shikiId: entry.shikiId,
      releaseAlias: entry.releaseAlias,
      searchQueries: entry.searchQueries ?? [entry.title],
      seasonCount: 1,
      sourceBadge: 'History',
      progressSub: timePart,
      historyResume: {
        episodeOrdinal: entry.episodeOrdinal,
        episodeIndex: entry.episodeIndex,
        resumeTime: entry.currentTime
      }
    };
  }

  function libraryToSearchItem(item: LibraryItem): UnifiedSearchItem {
    const shiki = item.shikiId > 0 ? getCachedArtwork(item.shikiId) : undefined;
    const yearTag = item.tags.find((t) => /^\d{4}$/.test(t));
    const rawScore = item.score || shiki?.score || 0;

    return {
      id: `lib-${item.id}`,
      title: item.title,
      year: yearTag ? Number(yearTag) : shiki?.year,
      score: rawScore > 0 ? Number(rawScore.toFixed(1)) : undefined,
      typeLabel: 'TV',
      episodesTotal: extractEpisodesFromTags(item.tags) ?? shiki?.episodes ?? shiki?.episodesAired,
      genres: item.tags.length > 0 ? item.tags : shiki?.genres,
      poster: item.poster || shiki?.posterMain || '',
      posterPosition: item.posterPosition,
      shikiId: item.shikiId > 0 ? item.shikiId : undefined,
      releaseAlias: item.releaseAlias,
      searchQueries: item.searchQueries ?? [item.title],
      seasonCount: 1,
      sourceBadge: 'Library'
    };
  }

  function catalogToSearchItem(a: AnimeEntry): UnifiedSearchItem {
    const shiki = a.shikiId > 0 ? getCachedArtwork(a.shikiId) : undefined;
    const rawScore = (shiki?.score && shiki.score > 0 ? shiki.score : 0) || a.score || 0;
    return {
      id: `local-${a.id}`,
      title: a.title,
      year: shiki?.year ?? a.year,
      score: rawScore > 0 ? Number(rawScore.toFixed(1)) : undefined,
      typeLabel: a.typeLabel ?? 'TV',
      episodesTotal: shiki?.episodes ?? shiki?.episodesAired ?? a.episodesTotal,
      genres: shiki?.genres && shiki.genres.length > 0 ? shiki.genres : a.genres,
      poster: shiki?.posterMain || shiki?.posterOriginal || a.poster,
      posterPosition: a.posterPosition,
      shikiId: a.shikiId,
      releaseAlias: a.releaseAlias,
      searchQueries: a.searchQueries ?? [a.title],
      seasonCount: 1
    };
  }

  function passesCustomFilters(item: UnifiedSearchItem): boolean {
    if (hasYearFilter) {
      if (!item.year || item.year < yearFrom || item.year > yearTo) return false;
    }
    if (hasEpisodeFilter) {
      const ep = item.episodesTotal;
      if (ep == null) return false;
      if (ep < epMin) return false;
      if (epMax < EP_MAX && ep > epMax) return false;
    }
    if (hasScoreFilter) {
      if (!item.score || item.score < minScore) return false;
    }
    if (hasTypeFilter) {
      const t = (item.typeLabel ?? 'TV').toUpperCase();
      if (typeFilter === 'MOVIE' && t !== 'MOVIE') return false;
      if (typeFilter === 'OVA' && t !== 'OVA') return false;
      if (typeFilter === 'TV' && (t === 'MOVIE' || t === 'OVA')) return false;
    }
    if (hasGenreFilter) {
      const itemGenreIds = item.genreIds ?? [];
      const itemGenreNames = (item.genres ?? []).map((g) => g.toLowerCase());
      const matchesById =
        itemGenreIds.length > 0 && selectedGenreIds.every((id) => itemGenreIds.includes(id));
      const matchesByName =
        itemGenreNames.length > 0 &&
        selectedGenreNames.every((name) =>
          itemGenreNames.some((g) => g === name || g.includes(name) || name.includes(g))
        );
      if (!matchesById && !matchesByName) return false;
    }
    return true;
  }

  $effect(() => {
    const isOpen = search.open;
    const isActive = search.active;
    const rawQ = search.query.trim();
    const normQ = rawQ.toLowerCase();
    const filter = activeFilter;
    const fYearFrom = hasYearFilter ? yearFrom : undefined;
    const fYearTo = hasYearFilter ? yearTo : undefined;
    const fEpMin = epMin > EP_MIN ? epMin : undefined;
    const fEpMax = epMax < EP_MAX ? epMax : undefined;
    const fScore = minScore > 0 ? minScore : undefined;
    const fType = typeFilter;
    const fSort = sortFilter;
    const fGenres = selectedGenreIds;

    const requestKey = [
      filter,
      normQ,
      fYearFrom ?? '',
      fYearTo ?? '',
      fEpMin ?? '',
      fEpMax ?? '',
      fScore ?? '',
      fType,
      fSort,
      fGenres.join(',')
    ].join('|');

    if (!isActive || filter === 'library') {
      searchSeq += 1;
      remoteResults = [];
      remoteKey = '';
      searching = false;
      return;
    }

    if (!isOpen) {
      searching = false;
      return;
    }

    const seq = ++searchSeq;
    const ctrl = new AbortController();
    searching = true;
    const delayMs = normQ.length === 0 ? 60 : 200;

    const timer = setTimeout(() => {
      searchGroupedAnime(rawQ, {
        signal: ctrl.signal,
        filter,
        fromYear: fYearFrom,
        toYear: fYearTo,
        minEpisodes: fEpMin,
        maxEpisodes: fEpMax,
        minScore: fScore,
        type: fType,
        genreIds: fGenres,
        sorting: fSort
      })
        .then((items) => {
          if (seq !== searchSeq || ctrl.signal.aborted) return;
          remoteResults = items;
          remoteKey = requestKey;
        })
        .catch(() => {
          if (seq !== searchSeq || ctrl.signal.aborted) return;
          remoteResults = [];
          remoteKey = requestKey;
        })
        .finally(() => {
          if (seq === searchSeq && !ctrl.signal.aborted) {
            searching = false;
          }
        });
    }, delayMs);

    return () => {
      ctrl.abort();
      clearTimeout(timer);
    };
  });

  const currentRemoteExpectedKey = $derived(
    [
      activeFilter,
      search.query.trim().toLowerCase(),
      hasYearFilter ? yearFrom : '',
      hasYearFilter ? yearTo : '',
      epMin > EP_MIN ? epMin : '',
      epMax < EP_MAX ? epMax : '',
      minScore > 0 ? minScore : '',
      typeFilter,
      sortFilter,
      selectedGenreIds.join(',')
    ].join('|')
  );

  const results = $derived.by(() => {
    shikiTick;
    const q = search.query.trim().toLowerCase();
    const filter = activeFilter;

    const merged: UnifiedSearchItem[] = [];
    const indexByShiki = new Map<number, number>();
    const indexByAlias = new Map<string, number>();
    const indexByTitle = new Map<string, number>();

    const mergeIntoExisting = (existingIdx: number, incoming: UnifiedSearchItem) => {
      const target = merged[existingIdx];
      if (!target) return;
      if (!target.releaseAlias && incoming.releaseAlias) {
        target.releaseAlias = incoming.releaseAlias;
      }
      if (!target.shikiId && incoming.shikiId) {
        target.shikiId = incoming.shikiId;
      }
      if (!target.score && incoming.score) target.score = incoming.score;
      if (!target.episodesTotal && incoming.episodesTotal) {
        target.episodesTotal = incoming.episodesTotal;
      }
      if (!target.typeLabel && incoming.typeLabel) target.typeLabel = incoming.typeLabel;
      if ((!target.genres || target.genres.length === 0) && incoming.genres) {
        target.genres = incoming.genres;
      }
      if (!target.genreIds && incoming.genreIds) target.genreIds = incoming.genreIds;
      if (incoming.searchQueries && incoming.searchQueries.length > 0) {
        target.searchQueries = Array.from(
          new Set([...(target.searchQueries ?? []), ...incoming.searchQueries])
        );
      }
      if (!target.subTitle && incoming.subTitle && incoming.subTitle !== target.title) {
        target.subTitle = incoming.subTitle;
      }
      if (incoming.seasonCount > target.seasonCount) {
        target.seasonCount = incoming.seasonCount;
      }
    };

    const pushUnique = (item: UnifiedSearchItem) => {
      const titleKey = normalizeTitle(item.title);
      const existingIdx =
        (item.shikiId ? indexByShiki.get(item.shikiId) : undefined) ??
        (item.releaseAlias ? indexByAlias.get(item.releaseAlias) : undefined) ??
        (titleKey ? indexByTitle.get(titleKey) : undefined);

      if (existingIdx !== undefined) {
        mergeIntoExisting(existingIdx, item);
        return;
      }

      const idx = merged.length;
      if (item.shikiId) indexByShiki.set(item.shikiId, idx);
      if (item.releaseAlias) indexByAlias.set(item.releaseAlias, idx);
      if (titleKey) indexByTitle.set(titleKey, idx);
      merged.push(item);
    };

    if (filter !== 'ongoing') {
      // 1. User's Watch History (highest priority, instant resume)
      for (const entry of watchHistory.entries) {
        const candidate = historyToSearchItem(entry);
        const hShiki = entry.shikiId ? getCachedArtwork(entry.shikiId) : undefined;
        if (
          matchesLocalQuery(
            [entry.title, entry.releaseAlias, hShiki?.russian, hShiki?.english, hShiki?.name, ...(entry.searchQueries ?? [])],
            q
          ) &&
          passesCustomFilters(candidate)
        ) {
          pushUnique(candidate);
        }
      }

      // 2. User's Local Library (Likes, Watch Later, Favorites)
      for (const item of localLibrary.items) {
        const candidate = libraryToSearchItem(item);
        const lShiki = item.shikiId > 0 ? getCachedArtwork(item.shikiId) : undefined;
        if (
          matchesLocalQuery(
            [item.title, item.releaseAlias, lShiki?.russian, lShiki?.english, lShiki?.name, ...(item.searchQueries ?? [])],
            q
          ) &&
          passesCustomFilters(candidate)
        ) {
          pushUnique(candidate);
        }
      }

      // 3. Curated catalog cards (in 'all' mode)
      if (filter === 'all') {
        for (const entry of catalog) {
          const candidate = catalogToSearchItem(entry);
          const cShiki = entry.shikiId > 0 ? getCachedArtwork(entry.shikiId) : undefined;
          if (
            matchesLocalQuery(
              [entry.title, entry.releaseAlias, cShiki?.russian, cShiki?.english, cShiki?.name, ...(entry.searchQueries ?? [])],
              q
            ) &&
            passesCustomFilters(candidate)
          ) {
            pushUnique(candidate);
          }
        }
      }
    }

    // 4. Live remote results (up to 50 items, deduplicated & enriched with local state if saved)
    if (filter !== 'library' && remoteKey === currentRemoteExpectedKey && remoteResults.length > 0) {
      for (const remote of remoteResults) {
        const histMatch = watchHistory.entries.find((e) => sameTarget(e, remote));
        if (histMatch) {
          const histItem = historyToSearchItem(histMatch);
          pushUnique({
            ...remote,
            score: remote.score ?? histItem.score,
            sourceBadge: 'History',
            progressSub: histItem.progressSub,
            historyResume: histItem.historyResume
          });
          continue;
        }
        const libMatch = localLibrary.items.find((l) => sameTarget(l, remote));
        pushUnique(
          libMatch
            ? { ...remote, score: remote.score ?? libMatch.score, sourceBadge: 'Library' }
            : remote
        );
      }
    }

    if (sortFilter === 'RATING_DESC') {
      merged.sort((a, b) => (b.score ?? 0) - (a.score ?? 0));
    } else if (sortFilter === 'FRESH_AT_DESC') {
      merged.sort((a, b) => (b.year ?? 0) - (a.year ?? 0));
    }

    return merged.slice(0, MAX_RESULT_ITEMS);
  });

  const ambientPoster = $derived(
    results[selected]?.poster ?? results[0]?.poster ?? ''
  );

  const selectedItem = $derived(results[selected] ?? null);

  let openingVideoEl = $state<HTMLVideoElement | null>(null);
  let openingClip = $state<HeroOpeningPreview | null>(null);
  let openingItemId = $state<string>('');
  let openingVideoReady = $state(false);
  let openingHls: Hls | null = null;
  let openingReqToken = 0;

  function destroyOpeningStream(immediate = false) {
    const hls = openingHls;
    const vid = openingVideoEl;
    openingHls = null;
    if (vid) {
      try {
        vid.pause();
      } catch {}
    }
    if (hls) {
      if (immediate) {
        try {
          hls.destroy();
        } catch {}
      } else {
        window.setTimeout(() => {
          try {
            hls.destroy();
          } catch {}
        }, 380);
      }
    }
  }

  $effect(() => {
    if (!search.open || results.length === 0) return;
    const idx = selected;
    const warmIndices = new Set<number>([0, 1, 2, 3, 4, 5, 6, 7, idx - 1, idx, idx + 1, idx + 2, idx + 3]);
    for (const i of warmIndices) {
      const candidate = results[i];
      if (!candidate) continue;
      warmHeroOpeningPreview({
        title: candidate.title,
        releaseAlias: candidate.releaseAlias,
        shikiId: candidate.shikiId,
        searchQueries: candidate.searchQueries
      });
    }
  });

  $effect(() => {
    const isOpen = search.open;
    const item = selectedItem;

    untrack(() => {
      destroyOpeningStream(!isOpen);
      openingVideoReady = false;
      openingClip = null;
      openingItemId = '';
    });

    if (!isOpen || !item) {
      openingReqToken += 1;
      return;
    }

    const token = ++openingReqToken;
    const targetId = item.id;
    const targetPayload = {
      title: item.title,
      releaseAlias: item.releaseAlias,
      shikiId: item.shikiId,
      searchQueries: item.searchQueries
    };

    const cached = getCachedHeroOpeningPreview(targetPayload);
    if (cached !== undefined) {
      if (cached) {
        openingItemId = targetId;
        openingClip = cached;
      }
      return;
    }

    resolveHeroOpeningPreview(targetPayload)
      .then((clip) => {
        if (token !== openingReqToken || !search.open) return;
        if (!clip) return;
        openingItemId = targetId;
        openingClip = clip;
      })
      .catch(() => {});
  });

  $effect(() => {
    const clip = openingClip;
    const video = openingVideoEl;
    if (!clip || !video) return;

    let cancelled = false;
    untrack(() => {
      if (openingHls) {
        try {
          openingHls.destroy();
        } catch {}
        openingHls = null;
      }
    });

    video.muted = true;
    video.volume = 0;

    const trySeekAndPlay = () => {
      if (cancelled || !video) return;
      const dur = Number.isFinite(video.duration) && video.duration > 0 ? video.duration : Infinity;
      const endSec = Math.min(clip.endSec, dur - 0.25);
      if (video.currentTime < clip.startSec - 0.25 || video.currentTime >= endSec) {
        try {
          video.currentTime = clip.startSec;
        } catch {}
      }
      video.play().catch(() => {});
    };

    let localHls: Hls | null = null;
    const streamSource = clip.streamUrlFast || clip.streamUrl;

    if (clip.isDirectVideo) {
      video.src = streamSource;
      video.load();
      trySeekAndPlay();
      video.addEventListener('loadedmetadata', trySeekAndPlay, { once: true });
    } else if (Hls.isSupported()) {
      const hls = new Hls({
        enableWorker: true,
        lowLatencyMode: false,
        startPosition: clip.startSec,
        startFragPrefetch: true,
        maxBufferLength: 6,
        maxMaxBufferLength: 12,
        backBufferLength: 2
      });
      localHls = hls;
      openingHls = hls;
      hls.loadSource(streamSource);
      hls.attachMedia(video);
      hls.on(Hls.Events.MANIFEST_PARSED, trySeekAndPlay);
      hls.on(Hls.Events.ERROR, (_ev, data) => {
        if (data.fatal && !cancelled) {
          try {
            hls.destroy();
          } catch {}
          if (openingHls === hls) openingHls = null;
        }
      });
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = streamSource;
      video.addEventListener('loadedmetadata', trySeekAndPlay, { once: true });
    }

    return () => {
      cancelled = true;
      try {
        video.pause();
      } catch {}
      if (localHls) {
        const hlsToDestroy = localHls;
        if (openingHls === hlsToDestroy) openingHls = null;
        window.setTimeout(() => {
          try {
            hlsToDestroy.destroy();
          } catch {}
        }, 380);
      }
    };
  });

  $effect(() => () => {
    destroyOpeningStream(true);
  });

  const showSkeletons = $derived(searching && results.length === 0);

  const filterBarH = $derived(search.filtersOpen ? 32 : 0);
  const filterBarGap = $derived(search.filtersOpen ? 10 : 0);
  const maxViewportListH = $derived(
    Math.max(182, vh - 96 - (16 + 48 + filterBarGap + filterBarH + 10 + 16) - 24)
  );

  const rawListHeight = $derived.by(() => {
    if (showSkeletons) return SKELETON_COUNT * (ROW_H + ROW_GAP) - ROW_GAP;
    if (results.length > 0) return Math.min(results.length, MAX_ROWS) * (ROW_H + ROW_GAP) - ROW_GAP;
    return EMPTY_H;
  });

  const listHeight = $derived(Math.min(rawListHeight, maxViewportListH));

  const popupHeight = $derived(
    16 + 48 + filterBarGap + filterBarH + 10 + listHeight + 16
  );

  $effect(() => {
    results;
    selected = 0;
  });

  $effect(() => {
    if (!search.open) {
      activeCombo = null;
    }
    if (!search.active) {
      activeFilter = 'all';
    }
  });

  $effect(() => {
    if (!search.filtersOpen) {
      activeCombo = null;
    }
  });

  function focusSearchInput() {
    const el = rootEl?.querySelector('input[type="text"], input:not([type="range"])') as HTMLInputElement | null;
    if (el) {
      el.focus();
      const len = el.value.length;
      el.setSelectionRange(len, len);
    }
  }

  function focusInput(aNode: HTMLElement) {
    rootEl = aNode as HTMLDivElement;
    focusSearchInput();
    return {
      destroy() {
        if (rootEl === aNode) rootEl = null;
      }
    };
  }

  function scrollSelectedIntoView(idx: number) {
    tick().then(() => {
      resultRefs[idx]?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    });
  }

  function pick(aItem: UnifiedSearchItem | undefined) {
    if (!aItem) {
      return;
    }
    closeSearch();
    void openWatch(
      {
        title: aItem.title,
        shikiId: aItem.shikiId,
        releaseAlias: aItem.releaseAlias,
        searchQueries: aItem.searchQueries
      },
      aItem.historyResume
    );
  }

  function formatMetaTags(item: UnifiedSearchItem): string[] {
    const tags: string[] = [];
    if (item.year) tags.push(String(item.year));
    if (item.typeLabel) tags.push(item.typeLabel);
    if (item.episodesTotal && item.episodesTotal > 0) tags.push(`${item.episodesTotal} ep`);
    if (item.seasonCount > 1) tags.push(`${item.seasonCount} seasons`);
    if (item.progressSub) {
      tags.push(item.progressSub);
    } else if (item.genres && item.genres.length > 0) {
      const genreClean = item.genres
        .filter((g) => !/^\d{4}$/.test(g) && !/\d+\s*ep/i.test(g))
        .slice(0, 2);
      tags.push(...genreClean);
    }
    if (tags.length === 0) tags.push('Anime');
    return tags;
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown' && results.length > 0) {
      e.preventDefault();
      selected = (selected + 1) % results.length;
      scrollSelectedIntoView(selected);
    } else if (e.key === 'ArrowUp' && results.length > 0) {
      e.preventDefault();
      selected = (selected - 1 + results.length) % results.length;
      scrollSelectedIntoView(selected);
    } else if (e.key === 'Enter' && results.length > 0) {
      e.preventDefault();
      pick(results[selected]);
    }
  }

  function isEditableTarget(target: EventTarget | null): boolean {
    if (!(target instanceof HTMLElement)) return false;
    const tag = target.tagName;
    return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable;
  }

  function handleWindowKeydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      if (search.open) {
        closeSearch();
      } else {
        openSearch();
      }
      return;
    }

    if (
      e.key === '/' &&
      !e.ctrlKey &&
      !e.metaKey &&
      !e.altKey &&
      !search.open &&
      !isEditableTarget(e.target)
    ) {
      e.preventDefault();
      openSearch();
    }
  }
</script>

<svelte:window bind:innerHeight={vh} onkeydown={handleWindowKeydown} />

<MorphPopup
  open={search.open}
  getAnchor={getSearchAnchorRect}
  width={760}
  height={popupHeight}
  onclose={closeSearch}
  onclosed={finishSearchClose}
>
  {#snippet children(expanded)}
    <div class="search-body" class:expanded use:focusInput>
      <div class="search-ambient" aria-hidden="true">
        {#key ambientPoster}
          {#if ambientPoster}
            <div
              class="ambient-layer"
              in:fade={{ duration: 420, easing: cubicOut }}
              out:fade={{ duration: 420, easing: cubicOut }}
            >
              <LayerBlur src={ambientPoster} blur={64} opacity={0.32} scale={1.25} />
            </div>
          {/if}
        {/key}
      </div>

      <Input
        bind:value={search.query}
        onkeydown={handleKeydown}
        placeholder="Search anime, titles, genres..."
        class="search-popup-input"
      >
        {#snippet icon()}
          <Search size={17} strokeWidth={1.8} />
        {/snippet}
        {#snippet trailingIcon()}
          <div class="input-trailing-actions">
            <button
              type="button"
              class="modal-filter-btn"
              class:is-open={search.filtersOpen}
              class:has-active={hasAnyCustomFilter}
              aria-label="Toggle filters"
              aria-expanded={search.filtersOpen}
              title="Filters"
              onclick={() => {
                search.filtersOpen = !search.filtersOpen;
              }}
            >
              <SlidersHorizontal size={15} strokeWidth={1.8} />
              {#if activeCustomFilterCount > 0}
                <span class="filter-badge">{activeCustomFilterCount}</span>
              {/if}
            </button>
            <button type="button" class="esc-chip" onclick={closeSearch}>Esc</button>
          </div>
        {/snippet}
      </Input>

      {#if search.filtersOpen}
        <div class="combo-filter-bar" role="toolbar" aria-label="Search filters">
          <div class="combo-filter-group">
            <ComboBox
              open={activeCombo === 'scope'}
              value={activeFilter}
              options={SCOPE_OPTIONS}
              label={scopeComboLabel}
              ariaLabel="Search scope"
              placement="bottom-left"
              minWidth={144}
              class="search-combo {hasScopeFilter ? 'is-filtered' : ''}"
              ontoggle={(isOpen) => setComboOpen('scope', isOpen)}
              onselect={(opt) => {
                activeFilter = opt.value as SearchFilterMode;
                tick().then(focusSearchInput);
              }}
            >
              {#snippet trigger({ open, toggle, label })}
                <button
                  type="button"
                  class="combo-pill"
                  class:is-open={open}
                  class:is-filtered={hasScopeFilter}
                  onclick={toggle}
                >
                  <span>{label}</span>
                  <ChevronDown size={12} strokeWidth={2} class="combo-chevron" />
                </button>
              {/snippet}
            </ComboBox>

            <ComboBox
              open={activeCombo === 'type'}
              value={typeFilter}
              options={TYPE_OPTIONS}
              label={typeComboLabel}
              ariaLabel="Format type"
              placement="bottom-left"
              minWidth={138}
              class="search-combo {hasTypeFilter ? 'is-filtered' : ''}"
              ontoggle={(isOpen) => setComboOpen('type', isOpen)}
              onselect={(opt) => {
                typeFilter = opt.value as '' | 'TV' | 'MOVIE' | 'OVA';
              }}
            >
              {#snippet trigger({ open, toggle, label })}
                <button
                  type="button"
                  class="combo-pill"
                  class:is-open={open}
                  class:is-filtered={hasTypeFilter}
                  onclick={toggle}
                >
                  <span>{label}</span>
                  <ChevronDown size={12} strokeWidth={2} class="combo-chevron" />
                </button>
              {/snippet}
            </ComboBox>

            <ComboBox
              open={activeCombo === 'genre'}
              options={genreComboOptions}
              label={genreComboLabel}
              ariaLabel="Genres"
              placement="bottom-left"
              minWidth={188}
              maxHeight={268}
              closeOnSelect={false}
              class="search-combo {hasGenreFilter ? 'is-filtered' : ''}"
              ontoggle={(isOpen) => setComboOpen('genre', isOpen)}
              onselect={(opt) => toggleGenre(Number(opt.value))}
            >
              {#snippet trigger({ open, toggle, label })}
                <button
                  type="button"
                  class="combo-pill"
                  class:is-open={open}
                  class:is-filtered={hasGenreFilter}
                  onclick={toggle}
                >
                  <span>{label}</span>
                  <ChevronDown size={12} strokeWidth={2} class="combo-chevron" />
                </button>
              {/snippet}
            </ComboBox>

            <ComboBox
              open={activeCombo === 'year'}
              value={yearRangeKey}
              options={YEAR_OPTIONS}
              label={yearComboLabel}
              ariaLabel="Release years"
              placement="bottom-left"
              minWidth={150}
              class="search-combo {hasYearFilter ? 'is-filtered' : ''}"
              ontoggle={(isOpen) => setComboOpen('year', isOpen)}
              onselect={(opt) => {
                yearRangeKey = String(opt.value);
              }}
            >
              {#snippet trigger({ open, toggle, label })}
                <button
                  type="button"
                  class="combo-pill"
                  class:is-open={open}
                  class:is-filtered={hasYearFilter}
                  onclick={toggle}
                >
                  <span>{label}</span>
                  <ChevronDown size={12} strokeWidth={2} class="combo-chevron" />
                </button>
              {/snippet}
            </ComboBox>

            <ComboBox
              open={activeCombo === 'score'}
              value={minScore}
              options={SCORE_OPTIONS}
              label={scoreComboLabel}
              ariaLabel="Minimum rating"
              placement="bottom-left"
              minWidth={140}
              class="search-combo {hasScoreFilter ? 'is-filtered' : ''}"
              ontoggle={(isOpen) => setComboOpen('score', isOpen)}
              onselect={(opt) => {
                minScore = Number(opt.value);
              }}
            >
              {#snippet trigger({ open, toggle, label })}
                <button
                  type="button"
                  class="combo-pill"
                  class:is-open={open}
                  class:is-filtered={hasScoreFilter}
                  onclick={toggle}
                >
                  <span>{label}</span>
                  <ChevronDown size={12} strokeWidth={2} class="combo-chevron" />
                </button>
              {/snippet}
            </ComboBox>

            <ComboBox
              open={activeCombo === 'episodes'}
              value={epRangeKey}
              options={EP_OPTIONS}
              label={epComboLabel}
              ariaLabel="Episode count"
              placement="bottom-left"
              minWidth={146}
              class="search-combo {hasEpisodeFilter ? 'is-filtered' : ''}"
              ontoggle={(isOpen) => setComboOpen('episodes', isOpen)}
              onselect={(opt) => {
                epRangeKey = String(opt.value);
              }}
            >
              {#snippet trigger({ open, toggle, label })}
                <button
                  type="button"
                  class="combo-pill"
                  class:is-open={open}
                  class:is-filtered={hasEpisodeFilter}
                  onclick={toggle}
                >
                  <span>{label}</span>
                  <ChevronDown size={12} strokeWidth={2} class="combo-chevron" />
                </button>
              {/snippet}
            </ComboBox>

            <ComboBox
              open={activeCombo === 'sort'}
              value={sortFilter}
              options={SORT_OPTIONS}
              label={sortComboLabel}
              ariaLabel="Sort order"
              placement="bottom-right"
              minWidth={144}
              class="search-combo {hasSortFilter ? 'is-filtered' : ''}"
              ontoggle={(isOpen) => setComboOpen('sort', isOpen)}
              onselect={(opt) => {
                sortFilter = opt.value as '' | 'RATING_DESC' | 'FRESH_AT_DESC';
              }}
            >
              {#snippet trigger({ open, toggle, label })}
                <button
                  type="button"
                  class="combo-pill"
                  class:is-open={open}
                  class:is-filtered={hasSortFilter}
                  onclick={toggle}
                >
                  <span>{label}</span>
                  <ChevronDown size={12} strokeWidth={2} class="combo-chevron" />
                </button>
              {/snippet}
            </ComboBox>
          </div>

          {#if hasAnyCustomFilter}
            <button type="button" class="reset-inline-btn" onclick={resetCustomFilters}>
              Reset
            </button>
          {/if}
        </div>
      {/if}

      <div class="search-results" style:height="{listHeight}px">
        {#if showSkeletons}
          <div class="skeleton-list" aria-hidden="true">
            {#each Array(SKELETON_COUNT) as _, idx (idx)}
              <div class="skeleton-row">
                <div class="skeleton-thumb shimmer"></div>
                <div class="skeleton-meta">
                  <div class="skeleton-line title shimmer"></div>
                  <div class="skeleton-line sub shimmer"></div>
                </div>
              </div>
            {/each}
          </div>
        {:else if results.length > 0}
          <ScrollArea class="results-scroll">
            <ul class="result-list">
              {#each results as item, i (item.id)}
                <li class="result-item">
                  <button
                    bind:this={resultRefs[i]}
                    type="button"
                    class="result"
                    class:selected={i === selected}
                    class:is-opening={i === selected && openingVideoReady && openingItemId === item.id}
                    onmouseenter={() => (selected = i)}
                    onclick={() => pick(item)}
                  >
                    <img
                      src={item.poster}
                      alt=""
                      class="result-bg"
                      style:object-position={item.posterPosition}
                      loading="lazy"
                      decoding="async"
                    />
                    {#if i === selected && openingClip && openingItemId === item.id}
                      <div
                        class="result-opening-stage"
                        class:is-ready={openingVideoReady}
                        out:fade={{ duration: 340, easing: cubicOut }}
                        aria-hidden="true"
                      >
                        <video
                          bind:this={openingVideoEl}
                          class="result-opening-video"
                          muted
                          playsinline
                          preload="auto"
                          onloadeddata={(e) => {
                            const vid = e.currentTarget;
                            if (!openingClip || vid !== openingVideoEl) return;
                            if (vid.currentTime < openingClip.startSec - 0.25) {
                              try {
                                vid.currentTime = openingClip.startSec;
                              } catch {}
                              return;
                            }
                            if (!vid.seeking && vid.readyState >= 2) {
                              openingVideoReady = true;
                            }
                          }}
                          onplaying={(e) => {
                            const vid = e.currentTarget;
                            if (!openingClip || vid !== openingVideoEl) return;
                            if (vid.currentTime < openingClip.startSec - 0.25) {
                              try {
                                vid.currentTime = openingClip.startSec;
                              } catch {}
                              return;
                            }
                            if (!vid.seeking) {
                              openingVideoReady = true;
                            }
                          }}
                          ontimeupdate={(e) => {
                            const vid = e.currentTarget;
                            if (!openingClip || vid !== openingVideoEl) return;
                            const t = vid.currentTime;
                            if (!vid.seeking && t < openingClip.startSec - 0.25) {
                              vid.currentTime = openingClip.startSec;
                              return;
                            }
                            if (!openingVideoReady && !vid.seeking && t >= openingClip.startSec - 0.25) {
                              openingVideoReady = true;
                            }
                            const dur =
                              Number.isFinite(vid.duration) && vid.duration > 0
                                ? vid.duration
                                : Infinity;
                            const endSec = Math.min(openingClip.endSec, dur - 0.25);
                            if (t >= endSec) {
                              vid.currentTime = openingClip.startSec;
                              vid.play().catch(() => {});
                            }
                          }}
                        ></video>
                      </div>
                    {/if}
                    <div class="result-dim" aria-hidden="true"></div>

                    <img
                      src={item.poster}
                      alt=""
                      class="thumb"
                      style:object-position={item.posterPosition}
                      loading="lazy"
                      decoding="async"
                    />
                    <span class="meta">
                      <span class="name">{item.title}</span>
                      <MetaLine
                        score={item.score}
                        tags={formatMetaTags(item)}
                        size="sm"
                        class="result-meta-line"
                      />
                    </span>
                    {#if item.sourceBadge}
                      <span class="source-badge" class:is-history={item.sourceBadge === 'History'}>
                        {item.sourceBadge}
                      </span>
                    {/if}
                  </button>
                </li>
              {/each}
            </ul>
          </ScrollArea>
        {:else}
          <div class="empty">
            {#if search.query.trim()}
              Nothing found for “{search.query.trim()}”
            {:else if activeFilter === 'library'}
              Your library & history list is empty
            {:else}
              Nothing matches these filters
            {/if}
          </div>
        {/if}
      </div>
    </div>
  {/snippet}
</MorphPopup>

<style>
  .search-body {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 10px;
    width: 100%;
    height: 100%;
  }

  .search-ambient {
    position: absolute;
    inset: -16px;
    border-radius: var(--radius-card, 16px);
    overflow: hidden;
    pointer-events: none;
    z-index: 0;
    opacity: 0;
    transition: opacity calc(var(--popup-duration, 480ms) * 0.75) cubic-bezier(0.22, 1, 0.36, 1);
    -webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 1) 0%, rgba(0, 0, 0, 0.25) 75%, rgba(0, 0, 0, 0) 100%);
    mask-image: linear-gradient(180deg, rgba(0, 0, 0, 1) 0%, rgba(0, 0, 0, 0.25) 75%, rgba(0, 0, 0, 0) 100%);
  }

  .search-body.expanded .search-ambient {
    opacity: 1;
  }

  .ambient-layer {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    will-change: opacity;
  }

  .search-body :global(.search-popup-input) {
    position: relative;
    z-index: 2;
    width: 100%;
    height: 38px;
    border-radius: 9px;
    flex-shrink: 0;
    background: rgba(255, 255, 255, 0);
    border: none;
    box-shadow: none;
    -webkit-backdrop-filter: none;
    backdrop-filter: none;
    transition:
      height var(--popup-duration, 480ms) cubic-bezier(0.22, 1, 0.36, 1),
      border-radius var(--popup-duration, 480ms) cubic-bezier(0.22, 1, 0.36, 1),
      background-color var(--popup-duration, 480ms) cubic-bezier(0.22, 1, 0.36, 1);
  }

  .search-body.expanded :global(.search-popup-input) {
    height: 48px;
    border-radius: 12px;
    background: rgba(255, 255, 255, 0.08);
  }

  .search-body.expanded :global(.search-popup-input:focus-within) {
    background: rgba(255, 255, 255, 0.11);
  }

  .search-body :global(.search-popup-input .marlakit-input-field) {
    transition: font-size var(--popup-duration, 480ms) cubic-bezier(0.22, 1, 0.36, 1);
  }

  .search-body.expanded :global(.search-popup-input .marlakit-input-field) {
    font-size: 16px;
  }

  .input-trailing-actions {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    height: 24px;
  }

  .modal-filter-btn {
    position: relative;
    height: 24px;
    min-width: 24px;
    padding: 0 2px;
    margin: 0;
    box-sizing: border-box;
    border: none;
    outline: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.48);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 5px;
    line-height: 1;
    cursor: pointer;
    transition:
      color 0.16s var(--ease-out),
      transform 0.1s ease;
  }

  .modal-filter-btn :global(svg) {
    display: block;
    flex-shrink: 0;
  }

  .modal-filter-btn:hover,
  .modal-filter-btn.is-open,
  .modal-filter-btn.has-active {
    color: #ffffff;
    background: transparent;
  }

  .modal-filter-btn:active {
    transform: scale(0.93);
  }

  .filter-badge {
    min-width: 15px;
    height: 15px;
    padding: 0 4px;
    border-radius: 999px;
    background: #ffffff;
    color: #0c0c0c;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 10px;
    font-weight: 700;
    line-height: 1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .esc-chip {
    height: 24px;
    max-width: 0;
    padding: 0;
    margin: 0 0 0 -8px;
    overflow: hidden;
    opacity: 0;
    pointer-events: none;
    box-sizing: border-box;
    border: none;
    outline: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-family: 'Medeo Sans Display', 'Medeo Sans', system-ui, sans-serif;
    font-size: 11.5px;
    font-weight: 500;
    color: rgba(255, 255, 255, 0.48);
    background: rgba(255, 255, 255, 0.08);
    border-radius: 7px;
    line-height: 1;
    cursor: pointer;
    transition:
      opacity calc(var(--popup-duration, 480ms) * 0.65) cubic-bezier(0.22, 1, 0.36, 1),
      max-width var(--popup-duration, 480ms) cubic-bezier(0.22, 1, 0.36, 1),
      padding var(--popup-duration, 480ms) cubic-bezier(0.22, 1, 0.36, 1),
      margin-left var(--popup-duration, 480ms) cubic-bezier(0.22, 1, 0.36, 1),
      color 0.15s ease,
      background-color 0.15s ease;
  }

  .search-body.expanded .esc-chip {
    max-width: 52px;
    padding: 0 9px;
    margin-left: 0;
    opacity: 1;
    pointer-events: auto;
  }

  .esc-chip:hover {
    color: #ffffff;
    background: rgba(255, 255, 255, 0.14);
  }

  .combo-filter-bar {
    position: relative;
    z-index: 30;
    height: 32px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    opacity: 0;
    transform: translateY(-6px);
    pointer-events: none;
    transition:
      opacity calc(var(--popup-duration, 480ms) * 0.55) cubic-bezier(0.22, 1, 0.36, 1),
      transform calc(var(--popup-duration, 480ms) * 0.65) cubic-bezier(0.22, 1, 0.36, 1);
  }

  .search-body.expanded .combo-filter-bar {
    opacity: 1;
    transform: translateY(0);
    pointer-events: auto;
    transition-delay: 0.06s;
  }

  .combo-filter-group {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: nowrap;
  }

  .combo-pill {
    height: 30px;
    padding: 0 11px;
    border: none;
    outline: none;
    border-radius: 9px;
    background: rgba(255, 255, 255, 0.07);
    color: rgba(255, 255, 255, 0.78);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 500;
    white-space: nowrap;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    transition:
      background-color 0.16s var(--ease-out),
      color 0.16s var(--ease-out),
      transform 0.1s ease;
  }

  .combo-pill:hover,
  .combo-pill.is-open {
    background: rgba(255, 255, 255, 0.13);
    color: #ffffff;
  }

  .combo-pill.is-filtered {
    background: #ffffff;
    color: #0c0c0c;
    font-weight: 600;
  }

  .combo-pill:active {
    transform: scale(0.96);
  }

  .combo-pill :global(.combo-chevron) {
    opacity: 0.6;
    transition: transform 0.16s var(--ease-out), opacity 0.16s ease;
  }

  .combo-pill.is-open :global(.combo-chevron) {
    transform: rotate(180deg);
    opacity: 0.95;
  }

  .reset-inline-btn {
    height: 28px;
    padding: 0 10px;
    border: none;
    outline: none;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.06);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12px;
    color: rgba(255, 255, 255, 0.62);
    cursor: pointer;
    flex-shrink: 0;
    transition:
      color 0.16s ease,
      background-color 0.16s ease;
  }

  .reset-inline-btn:hover {
    color: #ffffff;
    background: rgba(255, 255, 255, 0.12);
  }

  .search-results {
    position: relative;
    z-index: 2;
    flex-shrink: 0;
    margin-right: -13px;
    border-radius: 14px;
    opacity: 0;
    transform: translateY(-8px);
    pointer-events: none;
    overflow: hidden;
    transition:
      opacity calc(var(--popup-duration, 480ms) * 0.58) cubic-bezier(0.22, 1, 0.36, 1),
      transform calc(var(--popup-duration, 480ms) * 0.68) cubic-bezier(0.22, 1, 0.36, 1),
      height var(--popup-duration, 480ms) cubic-bezier(0.22, 1, 0.36, 1);
  }

  .search-results :global(.marlakit-scroll-track) {
    top: 6px;
    right: 2px;
    bottom: 6px;
    width: 6px;
    border-radius: 999px;
  }

  .search-results :global(.marlakit-scroll-thumb) {
    width: 5px;
    border-radius: 999px;
  }

  .search-body.expanded .search-results {
    opacity: 1;
    transform: translateY(0);
    pointer-events: auto;
    transition-delay: 0.08s, 0.08s, 0s;
  }

  .skeleton-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding-right: 13px;
    box-sizing: border-box;
  }

  .skeleton-row {
    flex-shrink: 0;
    height: 88px;
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 0 16px 0 0;
    box-sizing: border-box;
    border-radius: 14px;
    overflow: hidden;
    background: rgba(255, 255, 255, 0.04);
  }

  .skeleton-thumb {
    flex: 0 0 auto;
    width: 82px;
    height: 100%;
    border-radius: 0;
  }

  .skeleton-meta {
    flex: 1 1 auto;
    display: flex;
    flex-direction: column;
    gap: 9px;
    min-width: 0;
  }

  .skeleton-line {
    border-radius: 6px;
  }

  .skeleton-line.title {
    width: 54%;
    height: 16px;
  }

  .skeleton-line.sub {
    width: 34%;
    height: 12px;
  }

  .shimmer {
    background: linear-gradient(
      90deg,
      rgba(255, 255, 255, 0.04) 0%,
      rgba(255, 255, 255, 0.1) 50%,
      rgba(255, 255, 255, 0.04) 100%
    );
    background-size: 200% 100%;
    animation: searchShimmer 1.3s ease-in-out infinite;
  }

  @keyframes searchShimmer {
    0% {
      background-position: 100% 0;
    }
    100% {
      background-position: -100% 0;
    }
  }

  .result-list {
    list-style: none;
    margin: 0;
    padding: 0 13px 0 0;
    box-sizing: border-box;
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .result-item {
    flex-shrink: 0;
    width: 100%;
  }

  .result {
    position: relative;
    overflow: hidden;
    isolation: isolate;
    flex-shrink: 0;
    width: 100%;
    min-height: 88px;
    height: 88px;
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 0 16px 0 0;
    margin: 0;
    box-sizing: border-box;
    background: #101013;
    border: none;
    outline: none;
    border-radius: 14px;
    cursor: pointer;
    text-align: left;
    color: #ffffff;
    contain: layout paint;
  }

  .result-bg {
    position: absolute;
    top: 50%;
    left: 50%;
    width: 28%;
    height: 28%;
    object-fit: cover;
    filter: blur(7px) saturate(1.3);
    transform: translate3d(-50%, -50%, 0) scale(4.4);
    transform-origin: center center;
    z-index: 0;
    pointer-events: none;
    user-select: none;
    opacity: 0.68;
    transition:
      opacity 0.58s cubic-bezier(0.22, 1, 0.36, 1),
      transform 0.58s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .result.selected .result-bg {
    opacity: 1;
    transform: translate3d(-50%, -50%, 0) scale(4.65);
  }

  .result.is-opening .result-bg {
    opacity: 0;
    transform: translate3d(-50%, -50%, 0) scale(4.85);
  }

  .result-opening-stage {
    position: absolute;
    inset: 0;
    z-index: 1;
    pointer-events: none;
    overflow: hidden;
    opacity: 0;
    transform: scale(1.06);
    filter: blur(10px);
    transition:
      opacity 0.58s cubic-bezier(0.22, 1, 0.36, 1),
      transform 0.64s cubic-bezier(0.22, 1, 0.36, 1),
      filter 0.58s cubic-bezier(0.22, 1, 0.36, 1);
    will-change: opacity, transform, filter;
  }

  .result-opening-stage.is-ready {
    opacity: 1;
    transform: scale(1);
    filter: blur(0px);
  }

  .result-opening-video {
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center center;
    display: block;
    pointer-events: none;
  }

  .result-dim {
    position: absolute;
    inset: -2px;
    z-index: 2;
    background: linear-gradient(
      90deg,
      rgba(10, 10, 12, 0.92) 0%,
      rgba(10, 10, 12, 0.78) 55%,
      rgba(10, 10, 12, 0.52) 100%
    );
    opacity: 0.94;
    pointer-events: none;
    transition: opacity 0.45s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .result.selected .result-dim {
    opacity: 0.52;
  }

  .result.is-opening .result-dim {
    opacity: 0.42;
  }

  .thumb {
    position: relative;
    z-index: 3;
    flex: 0 0 auto;
    width: 82px;
    height: 100%;
    object-fit: cover;
    border-radius: 0;
    pointer-events: none;
  }

  .meta {
    position: relative;
    z-index: 3;
    flex: 1 1 auto;
    display: flex;
    flex-direction: column;
    gap: 7px;
    min-width: 0;
  }

  .name {
    font-family: 'RF Dewi Extended', 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 16px;
    font-weight: 700;
    line-height: 1.2;
    color: #ffffff;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .meta :global(.result-meta-line) {
    flex-wrap: nowrap;
    overflow: hidden;
  }

  .source-badge {
    position: relative;
    z-index: 3;
    flex: 0 0 auto;
    padding: 5px 11px;
    border: none;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.12);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
    font-weight: 600;
    color: rgba(255, 255, 255, 0.88);
    line-height: 1;
  }

  .source-badge.is-history {
    background: rgba(166, 227, 161, 0.18);
    color: var(--marlakit-green, #a6e3a1);
  }

  .empty {
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 15px;
    font-weight: 700;
    color: rgba(255, 255, 255, 0.48);
    user-select: none;
  }
</style>
