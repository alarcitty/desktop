<script lang="ts">
  import { onDestroy } from 'svelte';
  import { fade, scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Play from '@lucide/svelte/icons/play';
  import Bookmark from '@lucide/svelte/icons/bookmark';
  import Check from '@lucide/svelte/icons/check';
  import Share2 from '@lucide/svelte/icons/share-2';
  import LayoutGrid from '@lucide/svelte/icons/layout-grid';
  import LayoutList from '@lucide/svelte/icons/layout-list';
  import Search from '@lucide/svelte/icons/search';
  import Star from '@lucide/svelte/icons/star';
  import PictureInPicture2 from '@lucide/svelte/icons/picture-in-picture-2';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import HardDriveDownload from '@lucide/svelte/icons/hard-drive-download';
  import X from '@lucide/svelte/icons/x';

  import MainButton from '../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../marlakit/button/SecondButton.svelte';
  import MetaLine from '../../marlakit/meta/MetaLine.svelte';
  import LayerBlur from '../../marlakit/blur/LayerBlur.svelte';
  import { portal } from '../../marlakit/portal';
  import EpisodeThumb from './EpisodeThumb.svelte';
  import TorrentPopup from './TorrentPopup.svelte';
  import {
    buildMetaTags,
    type AnimeSeasonEntry,
    type ShikiCharacterCast,
    type ShikiRelatedAnime,
    type ShikiSimilarAnime,
    type ShikiVideoItem
  } from '../../api/anime';
  import { searchTorrents, type TorrentSearchItem } from '../../api/providers/torrent';
  import { episodeProgressMap, getEpisodeProgress } from '../../app/history.svelte';
  import { isInWatchLater, toggleWatchLater } from '../../app/library.svelte';
  import { encodeMilaShareLink, goBack } from '../../app/router.svelte';
  import {
    expandCustomPip,
    openTorrentStream,
    openWatch,
    player,
    retryWatch,
    selectEpisode,
    setInlinePlayerStage,
    switchSeason
  } from '../../app/player.svelte';

  const EP_PAGE_SIZE = 16;
  const EP_VIEW_MODE_KEY = 'mila.anime.episodesViewMode';
  const TRAILER_VIEW_MODE_KEY = 'mila.anime.trailersViewMode';
  const CHAR_VIEW_MODE_KEY = 'mila.anime.charactersViewMode';

  type EpSortKey = 'order' | 'duration' | 'unwatched';
  type CharSortKey = 'default' | 'name' | 'va';

  function readSavedViewMode(key: string, fallback: 'grid' | 'list' | 'strip'): string {
    if (typeof localStorage === 'undefined') return fallback;
    return localStorage.getItem(key) || fallback;
  }

  let viewportW = $state(typeof window !== 'undefined' ? window.innerWidth : 1440);
  let playerSectionEl = $state<HTMLElement | null>(null);
  let inlineStageEl = $state<HTMLDivElement | null>(null);
  let descTextEl = $state<HTMLParagraphElement | null>(null);
  let descExpanded = $state(false);
  let descCollapsedH = $state(78);
  let descExpandedH = $state(0);
  let isPlanLoading = $state(false);
  let shareCopied = $state(false);
  let shareTimer: number | undefined;

  let epViewMode = $state<'grid' | 'list'>(readSavedViewMode(EP_VIEW_MODE_KEY, 'grid') === 'list' ? 'list' : 'grid');
  let epSortBy = $state<EpSortKey>('order');
  let epSortDir = $state<'asc' | 'desc'>('asc');
  let epRangeIndex = $state(0);

  let trailerViewMode = $state<'grid' | 'list'>(readSavedViewMode(TRAILER_VIEW_MODE_KEY, 'grid') === 'list' ? 'list' : 'grid');
  let lightboxUrl = $state<string | null>(null);
  let activeTrailer = $state<ShikiVideoItem | null>(null);

  let charViewMode = $state<'strip' | 'grid'>(readSavedViewMode(CHAR_VIEW_MODE_KEY, 'strip') === 'grid' ? 'grid' : 'strip');
  let charRoleFilter = $state<'all' | 'main' | 'supporting'>('all');
  let charGenderFilter = $state<'all' | 'Female' | 'Male'>('all');
  let charSortBy = $state<CharSortKey>('default');
  let charSortDir = $state<'asc' | 'desc'>('asc');
  let charQuery = $state('');
  let charsExpanded = $state(false);
  let failedCharPortraits = $state<Record<number, boolean>>({});
  let charStripEl = $state<HTMLDivElement | null>(null);
  let canScrollCharLeft = $state(false);
  let canScrollCharRight = $state(false);

  let moreStripEl = $state<HTMLDivElement | null>(null);
  let moreScrollLeft = $state(0);
  let canScrollMoreLeft = $state(false);
  let canScrollMoreRight = $state(false);

  let torrentsOpen = $state(false);
  let torrentsLoading = $state(false);
  let torrentsError = $state('');
  let torrentResults = $state<TorrentSearchItem[]>([]);
  let torrentResultsFor = $state('');

  const currentShikiId = $derived(player.shikiArt?.id ?? player.target?.shikiId ?? 0);

  const displayTitle = $derived(
    player.shikiArt?.english || player.shikiArt?.name || player.release?.title || player.title || 'Anime'
  );

  const subTitle = $derived.by(() => {
    const romaji = player.shikiArt?.name;
    const ru = player.shikiArt?.russian;
    if (romaji && romaji !== displayTitle) return romaji;
    if (ru && ru !== displayTitle) return ru;
    return '';
  });

  const posterUrl = $derived(
    player.shikiArt?.posterOriginal ||
      player.shikiArt?.posterMain ||
      player.release?.poster ||
      ''
  );

  const backdropUrl = $derived(
    player.shikiArt?.screenshots[0] ||
      player.release?.banner ||
      posterUrl
  );

  const score = $derived(
    player.shikiArt?.score && player.shikiArt.score > 0
      ? Number(player.shikiArt.score.toFixed(1))
      : undefined
  );

  const metaTags = $derived.by(() => {
    const tags = buildMetaTags(player.shikiArt ?? undefined);
    const epCount = player.release?.episodes.length ?? 0;
    if (tags.length > 0) {
      if (
        epCount > 0 &&
        player.shikiArt?.status !== 'ongoing' &&
        player.shikiArt?.episodes &&
        player.shikiArt.episodes !== epCount
      ) {
        return tags.map((tag) => (/^\d+ ep$/.test(tag) ? `${epCount} ep` : tag));
      }
      return tags;
    }
    return epCount > 0 ? [`${epCount} ep`] : [];
  });

  function formatSingleSeasonLabel(typeLabel?: string): string {
    switch ((typeLabel ?? '').toUpperCase()) {
      case 'MOVIE':
        return 'Movie';
      case 'OVA':
        return 'OVA';
      case 'SPECIAL':
        return 'Special';
      default:
        return 'Season 1';
    }
  }

  const displaySeasons = $derived.by((): AnimeSeasonEntry[] => {
    if (player.seasons.length > 0) {
      return player.seasons;
    }
    if (!player.release) {
      return [];
    }
    return [
      {
        id: player.release.id,
        alias: player.release.alias,
        shikiId: player.shikiArt?.id,
        title: displayTitle,
        shortLabel: formatSingleSeasonLabel(player.release.typeLabel),
        year: player.release.year ?? player.shikiArt?.year,
        episodesTotal: player.release.episodes.length,
        poster: posterUrl
      }
    ];
  });

  const fullDescription = $derived(
    player.shikiArt?.description?.trim() || ''
  );

  const isLongDescription = $derived(
    descExpandedH > 0 ? descExpandedH > descCollapsedH + 8 : fullDescription.length > 290
  );

  $effect(() => {
    void fullDescription;
    descExpanded = false;
  });

  $effect(() => {
    const el = descTextEl;
    if (!el || !fullDescription) {
      descExpandedH = 0;
      return;
    }

    const measure = () => {
      const lh = parseFloat(getComputedStyle(el).lineHeight) || 26;
      descCollapsedH = Math.round(lh * 3);
      descExpandedH = Math.ceil(el.scrollHeight);
    };

    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => {
      ro.disconnect();
    };
  });

  const episodes = $derived(player.release?.episodes ?? []);

  const currentEpisode = $derived(episodes[player.episodeIndex] ?? null);

  $effect(() => {
    setInlinePlayerStage(inlineStageEl);
    return () => {
      setInlinePlayerStage(null);
    };
  });

  function gridCornerFlags(index: number, total: number, cols: number) {
    if (total <= 0) {
      return { tl: false, tr: false, bl: false, br: false };
    }
    const safeCols = Math.max(1, cols);
    const col = index % safeCols;
    const row = Math.floor(index / safeCols);
    const lastRow = Math.floor((total - 1) / safeCols);
    return {
      tl: index === 0,
      tr: index === Math.min(safeCols - 1, total - 1),
      bl: row === lastRow && col === 0,
      br: index === total - 1
    };
  }

  const epCols = $derived(viewportW <= 960 ? 2 : viewportW <= 1220 ? 3 : 4);
  const mediaCols = $derived(viewportW <= 960 ? 2 : 3);

  const sortedEpisodes = $derived.by(() => {
    const dir = epSortDir === 'asc' ? 1 : -1;
    const list = episodes.map((ep, idx) => ({ ep, idx }));
    if (epSortBy === 'order' && epSortDir === 'asc') {
      return list;
    }
    if (epSortBy === 'unwatched') {
      episodeProgressMap.items;
      return [...list].sort((a, b) => {
        const wa = getEpProgressInfo(a.idx, a.ep.ordinal).watched ? 1 : 0;
        const wb = getEpProgressInfo(b.idx, b.ep.ordinal).watched ? 1 : 0;
        if (wa !== wb) return (wa - wb) * dir;
        return a.ep.ordinal - b.ep.ordinal;
      });
    }
    if (epSortBy === 'duration') {
      return [...list].sort((a, b) => {
        const da = a.ep.duration || 0;
        const db = b.ep.duration || 0;
        if (da !== db) return (da - db) * dir;
        return a.ep.ordinal - b.ep.ordinal;
      });
    }
    return [...list].sort((a, b) => (a.ep.ordinal - b.ep.ordinal) * dir);
  });

  const episodeRanges = $derived.by(() => {
    if (sortedEpisodes.length <= EP_PAGE_SIZE) return [];
    const ranges: Array<{ label: string; start: number; end: number }> = [];
    for (let start = 0; start < sortedEpisodes.length; start += EP_PAGE_SIZE) {
      const end = Math.min(start + EP_PAGE_SIZE, sortedEpisodes.length);
      if (epSortBy === 'order' && epSortDir === 'asc') {
        const firstOrd = sortedEpisodes[start]?.ep.ordinal ?? start + 1;
        const lastOrd = sortedEpisodes[end - 1]?.ep.ordinal ?? end;
        ranges.push({ label: `${firstOrd}–${lastOrd}`, start, end });
      } else {
        ranges.push({ label: `${start + 1}–${end}`, start, end });
      }
    }
    return ranges;
  });

  const visibleEpisodes = $derived.by(() => {
    if (episodeRanges.length === 0) {
      return sortedEpisodes;
    }
    const range = episodeRanges[epRangeIndex] ?? episodeRanges[0];
    return sortedEpisodes.slice(range.start, range.end);
  });

  const galleryShots = $derived((player.shikiArt?.screenshots ?? []).slice(0, 6));

  const trailerVideos = $derived.by(() => {
    const raw = player.extras?.videos?.length
      ? player.extras.videos
      : (player.shikiArt?.videos ?? []);
    return raw.slice(0, 6);
  });

  const relatedEntries = $derived(player.extras?.related ?? []);
  const allCharacters = $derived(player.extras?.characters ?? []);
  const hasMainCharacters = $derived(allCharacters.some((c) => c.isMain));
  const hasSupportingCharacters = $derived(allCharacters.some((c) => !c.isMain));
  const hasGenderMeta = $derived(
    allCharacters.some((c) => c.gender === 'Female' || c.gender === 'Male')
  );

  const filteredCharacters = $derived.by(() => {
    const q = charQuery.trim().toLowerCase();
    const list = allCharacters.filter((c) => {
      if (charRoleFilter === 'main' && !c.isMain) return false;
      if (charRoleFilter === 'supporting' && c.isMain) return false;
      if (charGenderFilter !== 'all' && c.gender !== charGenderFilter) return false;
      if (q) {
        const match =
          c.name.toLowerCase().includes(q) ||
          c.russian.toLowerCase().includes(q) ||
          (c.voiceActor?.name.toLowerCase().includes(q) ?? false);
        if (!match) return false;
      }
      return true;
    });

    if (charSortBy === 'default' && charSortDir === 'asc') {
      return list;
    }
    const dir = charSortDir === 'asc' ? 1 : -1;
    if (charSortBy === 'name') {
      return [...list].sort((a, b) => a.name.localeCompare(b.name) * dir);
    }
    if (charSortBy === 'va') {
      return [...list].sort((a, b) => {
        const vaA = a.voiceActor?.name ?? '~~~';
        const vaB = b.voiceActor?.name ?? '~~~';
        const cmp = vaA.localeCompare(vaB);
        return (cmp !== 0 ? cmp : a.name.localeCompare(b.name)) * dir;
      });
    }
    return [...list].reverse();
  });

  const visibleCharacters = $derived(
    charViewMode === 'strip' || charsExpanded || charQuery.trim().length > 0
      ? filteredCharacters
      : filteredCharacters.slice(0, 12)
  );

  const similarEntries = $derived((player.extras?.similar ?? []).slice(0, 12));

  function updateCharScrollFlags() {
    if (!charStripEl) {
      canScrollCharLeft = false;
      canScrollCharRight = false;
      return;
    }
    const maxScroll = Math.max(0, charStripEl.scrollWidth - charStripEl.clientWidth);
    canScrollCharLeft = charStripEl.scrollLeft > 8;
    canScrollCharRight = charStripEl.scrollLeft < maxScroll - 8;
  }

  function scrollCharStrip(dir: -1 | 1) {
    if (!charStripEl) return;
    const step = Math.max(240, charStripEl.clientWidth * 0.72);
    charStripEl.scrollBy({ left: dir * step, behavior: 'smooth' });
  }

  function updateMoreScrollFlags() {
    if (!moreStripEl) {
      moreScrollLeft = 0;
      canScrollMoreLeft = false;
      canScrollMoreRight = false;
      return;
    }
    moreScrollLeft = moreStripEl.scrollLeft;
    const maxScroll = Math.max(0, moreStripEl.scrollWidth - moreStripEl.clientWidth);
    canScrollMoreLeft = moreStripEl.scrollLeft > 8;
    canScrollMoreRight = moreStripEl.scrollLeft < maxScroll - 8;
  }

  function scrollMoreStrip(dir: -1 | 1) {
    if (!moreStripEl) return;
    const step = Math.max(300, moreStripEl.clientWidth);
    moreStripEl.scrollBy({ left: dir * step, behavior: 'smooth' });
  }

  $effect(() => {
    void visibleCharacters.length;
    void charViewMode;
    void viewportW;
    queueMicrotask(updateCharScrollFlags);
  });

  $effect(() => {
    void similarEntries.length;
    void viewportW;
    queueMicrotask(updateMoreScrollFlags);
  });

  $effect(() => {
    // Reset page-local transient modals/filters when switching to a different title.
    void currentShikiId;
    void player.target?.releaseAlias;
    lightboxUrl = null;
    activeTrailer = null;
    epSortBy = 'order';
    epSortDir = 'asc';
    epRangeIndex = 0;
    charQuery = '';
    charRoleFilter = 'all';
    charGenderFilter = 'all';
    charSortBy = 'default';
    charSortDir = 'asc';
    charsExpanded = false;
    failedCharPortraits = {};
    if (charStripEl) charStripEl.scrollLeft = 0;
    if (moreStripEl) moreStripEl.scrollLeft = 0;
  });

  onDestroy(() => {
    if (shareTimer !== undefined) {
      window.clearTimeout(shareTimer);
    }
  });

  function toggleEpSort(key: EpSortKey) {
    if (epSortBy === key) {
      epSortDir = epSortDir === 'asc' ? 'desc' : 'asc';
    } else {
      epSortBy = key;
      epSortDir = 'asc';
    }
    epRangeIndex = 0;
  }

  function toggleCharSort(key: CharSortKey) {
    if (charSortBy === key) {
      charSortDir = charSortDir === 'asc' ? 'desc' : 'asc';
    } else {
      charSortBy = key;
      charSortDir = 'asc';
    }
  }

  function saveViewModePref(key: string, mode: string) {
    if (typeof localStorage === 'undefined') return;
    try {
      localStorage.setItem(key, mode);
    } catch {
      // Ignore storage errors
    }
  }

  function setEpisodeViewMode(mode: 'grid' | 'list') {
    if (epViewMode === mode) return;
    epViewMode = mode;
    saveViewModePref(EP_VIEW_MODE_KEY, mode);
  }

  function setTrailerViewMode(mode: 'grid' | 'list') {
    if (trailerViewMode === mode) return;
    trailerViewMode = mode;
    saveViewModePref(TRAILER_VIEW_MODE_KEY, mode);
  }

  function setCharViewMode(mode: 'strip' | 'grid') {
    if (charViewMode === mode) return;
    charViewMode = mode;
    saveViewModePref(CHAR_VIEW_MODE_KEY, mode);
  }

  function getEpProgressInfo(epIdx: number, ordinal: number) {
    // Subscribe to reactive store changes
    episodeProgressMap.items;
    const ep = episodes[epIdx];
    const entry = getEpisodeProgress(
      {
        shikiId: currentShikiId || undefined,
        releaseAlias: player.release?.alias ?? player.target?.releaseAlias,
        title: displayTitle
      },
      ordinal
    );
    if (!entry) return { ratio: 0, watched: false, currentTime: 0 };
    const dur = entry.duration > 0 ? entry.duration : ep?.duration ?? 0;
    const ratio = dur > 0 ? Math.min(1, Math.max(0, entry.currentTime / dur)) : 0;
    return {
      ratio: entry.watched && ratio < 0.85 ? 1 : ratio,
      watched: entry.watched || ratio >= 0.85,
      currentTime: entry.currentTime
    };
  }

  function formatEpDuration(sec: number): string {
    if (!sec || sec <= 0) return '';
    const mins = Math.round(sec / 60);
    return mins > 0 ? `${mins} min` : `${sec} sec`;
  }

  function fallbackCopyText(text: string): boolean {
    try {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.focus();
      ta.select();
      const ok = document.execCommand('copy');
      document.body.removeChild(ta);
      return ok;
    } catch {
      return false;
    }
  }

  async function handleShareCopy() {
    const shareText = encodeMilaShareLink({
      shikiId: currentShikiId || undefined,
      alias: player.release?.alias ?? player.target?.releaseAlias,
      episodeOrdinal: currentEpisode?.ordinal
    });
    let ok = false;
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(shareText);
        ok = true;
      }
    } catch {
      ok = false;
    }
    if (!ok) {
      ok = fallbackCopyText(shareText);
    }
    if (!ok) return;
    shareCopied = true;
    if (shareTimer !== undefined) {
      window.clearTimeout(shareTimer);
    }
    shareTimer = window.setTimeout(() => {
      shareCopied = false;
      shareTimer = undefined;
    }, 2000);
  }

  function openRelatedAnime(item: ShikiRelatedAnime) {
    if (item.isCurrent) {
      scrollToPlayer();
      return;
    }
    void openWatch({
      title: item.title,
      shikiId: item.id,
      searchQueries: [item.title, item.subTitle ?? ''].filter(Boolean)
    });
  }

  function openSimilarAnime(item: ShikiSimilarAnime) {
    void openWatch({
      title: item.title,
      shikiId: item.id,
      searchQueries: [item.title, item.subTitle ?? ''].filter(Boolean)
    });
  }

  function charInitials(name: string): string {
    const parts = name.trim().split(/\s+/).filter(Boolean);
    if (parts.length === 0) return '?';
    if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
    return `${parts[0][0] ?? ''}${parts[1][0] ?? ''}`.toUpperCase();
  }

  const episodeFallbackImages = $derived.by(() => {
    const out: string[] = [];
    for (const url of [...galleryShots, backdropUrl, posterUrl]) {
      const clean = url?.trim();
      if (clean && !out.includes(clean)) {
        out.push(clean);
      }
    }
    return out;
  });

  $effect(() => {
    // Keep active episode range synced when auto-advancing to the next episode.
    if (episodes.length > EP_PAGE_SIZE) {
      epRangeIndex = Math.floor(player.episodeIndex / EP_PAGE_SIZE);
    } else {
      epRangeIndex = 0;
    }
  });

  const inLibrary = $derived(
    isInWatchLater({
      shikiId: player.shikiArt?.id ?? player.target?.shikiId,
      releaseAlias: player.release?.alias ?? player.target?.releaseAlias,
      title: displayTitle
    })
  );

  function scrollToPlayer() {
    playerSectionEl?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }

  function handleChooseEpisode(index: number) {
    selectEpisode(index);
    scrollToPlayer();
  }

  function handlePlanToggle() {
    if (!displayTitle) return;
    isPlanLoading = true;
    toggleWatchLater({
      shikiId: player.shikiArt?.id ?? player.target?.shikiId,
      releaseAlias: player.release?.alias ?? player.target?.releaseAlias,
      searchQueries: player.target?.searchQueries,
      title: displayTitle,
      description: fullDescription,
      poster: posterUrl,
      score,
      tags: metaTags
    });
    isPlanLoading = false;
  }

  function handleWindowKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (activeTrailer) {
        activeTrailer = null;
        return;
      }
      if (lightboxUrl) {
        lightboxUrl = null;
      }
    }
  }

  async function toggleTorrentsDrawer() {
    torrentsOpen = !torrentsOpen;
    if (!torrentsOpen) return;
    const key = `${player.shikiArt?.id ?? 0}:${displayTitle}`;
    if (torrentResultsFor === key && torrentResults.length > 0) return;

    torrentsLoading = true;
    torrentsError = '';
    try {
      const q = (player.shikiArt?.english || player.shikiArt?.name || displayTitle || '').trim();
      const libId = player.release?.provider === 'aniliberty' ? player.release.id : 0;
      const items = await searchTorrents(q, libId);
      torrentResults = items;
      torrentResultsFor = key;
    } catch (err) {
      torrentsError = err instanceof Error ? err.message : 'Failed to search torrents';
    } finally {
      torrentsLoading = false;
    }
  }

  async function handleLaunchTorrent(item: TorrentSearchItem | string) {
    torrentsOpen = false;
    await openTorrentStream(item);
    scrollToPlayer();
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} bind:innerWidth={viewportW} />

<div class="anime-page">
  <!-- Spacious asymmetrical cinema showcase hero -->
  <div class="anime-hero-bg">
    {#if backdropUrl}
      <img src={backdropUrl} alt="" aria-hidden="true" class="backdrop-img" decoding="async" />
    {/if}
    <div class="backdrop-dim" aria-hidden="true"></div>
    <div class="backdrop-fade" aria-hidden="true"></div>

    <div class="anime-top-inner">
      <div class="anime-overview">
        <div class="overview-meta-col">
          {#if subTitle}
            <span class="top-origin-title">{subTitle}</span>
          {/if}
          <h1 class="anime-title">{displayTitle}</h1>

          {#if score || metaTags.length > 0}
            <div class="anime-meta-line">
              <MetaLine {score} tags={metaTags} />
            </div>
          {/if}

          {#if fullDescription}
            {#key fullDescription}
              <div class="desc-block">
                <div
                  class="desc-viewport"
                  class:is-collapsible={isLongDescription}
                  class:is-expanded={descExpanded}
                  style:max-height={isLongDescription
                    ? `${descExpanded ? (descExpandedH || 600) : descCollapsedH}px`
                    : 'none'}
                >
                  <p
                    bind:this={descTextEl}
                    class="anime-description"
                    class:is-expanded={descExpanded || !isLongDescription}
                  >
                    {fullDescription}
                  </p>
                </div>
                {#if isLongDescription}
                  <button
                    type="button"
                    class="desc-toggle"
                    aria-expanded={descExpanded}
                    onclick={() => { descExpanded = !descExpanded; }}
                  >
                    {descExpanded ? 'Show less' : 'Read more'}
                  </button>
                {/if}
              </div>
            {/key}
          {/if}

          <div class="overview-actions">
            <MainButton loading={player.loading} onclick={scrollToPlayer}>
              {#snippet icon()}
                <Play size={16} fill="currentColor" strokeWidth={0} />
              {/snippet}
              Watch Ep {currentEpisode?.ordinal ?? 1}
            </MainButton>

            <SecondButton loading={isPlanLoading} onclick={handlePlanToggle}>
              {#snippet icon()}
                {#if inLibrary}
                  <Check size={16} strokeWidth={2.2} />
                {:else}
                  <Bookmark size={16} strokeWidth={2} />
                {/if}
              {/snippet}
              {inLibrary ? 'In Plan to Watch' : 'Plan to Watch'}
            </SecondButton>

            <button
              type="button"
              class="share-icon-btn"
              class:is-copied={shareCopied}
              onclick={() => void handleShareCopy()}
              aria-label={shareCopied ? 'Link copied' : 'Copy share link'}
              title={shareCopied ? 'Copied!' : 'Copy link / code'}
            >
              <span class="share-icon-stack">
                {#if shareCopied}
                  <span class="share-icon-item is-check" transition:scale={{ start: 0.65, duration: 180, easing: cubicOut }}>
                    <Check size={17} strokeWidth={2.2} />
                  </span>
                {:else}
                  <span class="share-icon-item" transition:scale={{ start: 0.72, duration: 160, easing: cubicOut }}>
                    <Share2 size={17} strokeWidth={1.9} />
                  </span>
                {/if}
              </span>
            </button>

            <button
              type="button"
              class="back-btn"
              onclick={goBack}
              aria-label="Go back"
              title="Go back"
            >
              <ArrowLeft size={16} strokeWidth={2} />
              <span>Back</span>
            </button>
          </div>
        </div>

        {#if posterUrl}
          <div class="showcase-art">
            <div class="poster-glow" aria-hidden="true">
              <LayerBlur src={posterUrl} blur={72} opacity={0.54} scale={1.24} offsetY={0} />
            </div>
            <div class="poster-wrap">
              <img src={posterUrl} alt={displayTitle} class="poster-img" decoding="async" />
              <div class="poster-edge-vignette" aria-hidden="true"></div>
            </div>
          </div>
        {/if}
      </div>

      {#if displaySeasons.length > 0}
        <nav class="seasons-bar" aria-label="Seasons">
          {#each displaySeasons as season (season.id)}
            {@const isActive = player.release
              ? player.release.id === season.id || player.release.alias === season.alias
              : player.target?.releaseAlias === season.alias ||
                (player.target?.shikiId != null && player.target.shikiId === season.shikiId)}
            {@const isInteractive = displaySeasons.length > 1 && !isActive}
            <button
              type="button"
              class="season-tab"
              class:active={isActive}
              class:is-static={displaySeasons.length === 1}
              title={season.title}
              aria-current={isActive ? 'page' : undefined}
              onclick={() => {
                if (isInteractive) void switchSeason(season);
              }}
            >
              <span class="season-label">{season.shortLabel}</span>
              {#if season.year}
                <span class="season-year">{season.year}</span>
              {/if}
            </button>
          {/each}
        </nav>
      {/if}
    </div>
  </div>

  <!-- Inline player stage -->
  <section
    class="player-section"
    bind:this={playerSectionEl}
    aria-label="Video player"
  >
    <div class="player-frame-wrap">
      <div class="player-stage">
      {#if player.loading}
        <div class="stage-state" role="status" aria-label="Loading" transition:fade={{ duration: 180 }}>
          <div class="spinner" aria-hidden="true"></div>
        </div>
      {:else if player.error}
        <div class="stage-state stage-error">
          <p class="error-text">{player.error}</p>
          <MainButton onclick={retryWatch}>
            {#snippet icon()}
              <RefreshCw size={15} strokeWidth={2} />
            {/snippet}
            Retry
          </MainButton>
        </div>
      {:else if currentEpisode}
        <div
          class="player-inline-slot"
          class:is-hidden={player.pipMode && !player.pipDismissed}
          bind:this={inlineStageEl}
        ></div>
        {#if player.pipMode && !player.pipDismissed}
          <div class="stage-state" transition:fade={{ duration: 180 }}>
            <span>Playing in Picture-in-Picture</span>
            <SecondButton onclick={expandCustomPip}>
              {#snippet icon()}
                <PictureInPicture2 size={15} strokeWidth={2} />
              {/snippet}
              Return to player
            </SecondButton>
          </div>
        {/if}
      {:else}
        <div class="stage-state">
          <span>Select a title with Watch to begin playback.</span>
        </div>
      {/if}
      </div>
    </div>
  </section>

  <!-- Episode picker, frames & trailers -->
  <div class="anime-body">

    {#if episodes.length > 0}
      <section class="episodes-section" aria-label="Episodes">
        <div class="section-head">
          <div class="section-head-left">
            <h2 class="section-heading">Episodes</h2>
            <span class="section-count">{episodes.length}</span>
          </div>

          <div class="range-tabs" role="tablist" aria-label="Episode controls">
            {#if episodeRanges.length > 1}
              {#each episodeRanges as range, i (range.label)}
                <button
                  type="button"
                  class="range-pill"
                  class:is-active={epRangeIndex === i}
                  onclick={() => { epRangeIndex = i; }}
                >
                  {range.label}
                </button>
              {/each}
            {/if}

            <div class="lib-sort-group" role="group" aria-label="Sort episodes">
              <button
                type="button"
                class="sort-chip-btn"
                class:active={epSortBy === 'order'}
                onclick={() => toggleEpSort('order')}
              >
                <span>Order</span>
                {#if epSortBy === 'order'}
                  {#if epSortDir === 'asc'}
                    <ArrowUp size={13} strokeWidth={2.1} />
                  {:else}
                    <ArrowDown size={13} strokeWidth={2.1} />
                  {/if}
                {/if}
              </button>
              <button
                type="button"
                class="sort-chip-btn"
                class:active={epSortBy === 'duration'}
                onclick={() => toggleEpSort('duration')}
              >
                <span>Duration</span>
                {#if epSortBy === 'duration'}
                  {#if epSortDir === 'asc'}
                    <ArrowUp size={13} strokeWidth={2.1} />
                  {:else}
                    <ArrowDown size={13} strokeWidth={2.1} />
                  {/if}
                {/if}
              </button>
              <button
                type="button"
                class="sort-chip-btn"
                class:active={epSortBy === 'unwatched'}
                onclick={() => toggleEpSort('unwatched')}
              >
                <span>Unwatched</span>
                {#if epSortBy === 'unwatched'}
                  {#if epSortDir === 'asc'}
                    <ArrowUp size={13} strokeWidth={2.1} />
                  {:else}
                    <ArrowDown size={13} strokeWidth={2.1} />
                  {/if}
                {/if}
              </button>
            </div>

            <button
              type="button"
              class="range-pill torrent-toggle-pill"
              class:is-active={torrentsOpen || player.release?.provider === 'torrent'}
              onclick={() => void toggleTorrentsDrawer()}
            >
              <HardDriveDownload size={14} strokeWidth={1.9} />
              <span>Torrents</span>
            </button>

            <div class="ep-view-modes" class:is-grid={epViewMode === 'grid'} role="group" aria-label="Episodes layout">
              <div class="ep-view-pill" aria-hidden="true"></div>
              <button
                type="button"
                class="ep-view-btn"
                class:active={epViewMode === 'list'}
                aria-label="List view"
                aria-pressed={epViewMode === 'list'}
                onclick={() => setEpisodeViewMode('list')}
              >
                <LayoutList size={17} strokeWidth={1.7} />
              </button>
              <button
                type="button"
                class="ep-view-btn"
                class:active={epViewMode === 'grid'}
                aria-label="Grid view"
                aria-pressed={epViewMode === 'grid'}
                onclick={() => setEpisodeViewMode('grid')}
              >
                <LayoutGrid size={17} strokeWidth={1.7} />
              </button>
            </div>
          </div>
        </div>

        {#if epViewMode === 'grid'}
          <div class="episodes-grid" in:fade={{ duration: 180, easing: cubicOut }}>
            {#each visibleEpisodes as entry, i (entry.ep.id)}
              {@const epProg = getEpProgressInfo(entry.idx, entry.ep.ordinal)}
              {@const corners = gridCornerFlags(i, visibleEpisodes.length, epCols)}
              <EpisodeThumb
                episode={entry.ep}
                index={entry.idx}
                variant="grid"
                isCurrent={entry.idx === player.episodeIndex}
                isWatched={epProg.watched}
                progressRatio={epProg.ratio}
                cornerTL={corners.tl}
                cornerTR={corners.tr}
                cornerBL={corners.bl}
                cornerBR={corners.br}
                fallbackImages={episodeFallbackImages}
                onchoose={() => handleChooseEpisode(entry.idx)}
              />
            {/each}
          </div>
        {:else}
          <div class="episodes-list" in:fade={{ duration: 180, easing: cubicOut }}>
            {#each visibleEpisodes as entry (entry.ep.id)}
              {@const epProg = getEpProgressInfo(entry.idx, entry.ep.ordinal)}
              <EpisodeThumb
                episode={entry.ep}
                index={entry.idx}
                variant="list"
                isCurrent={entry.idx === player.episodeIndex}
                isWatched={epProg.watched}
                progressRatio={epProg.ratio}
                cornerTL={true}
                cornerBL={true}
                fallbackImages={episodeFallbackImages}
                onchoose={() => handleChooseEpisode(entry.idx)}
              />
            {/each}
          </div>
        {/if}
      </section>
    {/if}

    {#if galleryShots.length > 0 || trailerVideos.length > 0}
      <section class="frames-section" aria-label="Frames and Trailers">
        <div class="section-head">
          <div class="section-head-left">
            <h2 class="section-heading">
              {galleryShots.length > 0 && trailerVideos.length > 0
                ? 'Frames & Trailers'
                : trailerVideos.length > 0
                  ? 'Trailers'
                  : 'Frames'}
            </h2>
            <span class="section-count">{galleryShots.length + trailerVideos.length}</span>
          </div>

          {#if trailerVideos.length > 0}
            <div class="ep-view-modes" class:is-grid={trailerViewMode === 'grid'} role="group" aria-label="Trailers layout">
              <div class="ep-view-pill" aria-hidden="true"></div>
              <button
                type="button"
                class="ep-view-btn"
                class:active={trailerViewMode === 'list'}
                aria-label="Trailers list view"
                aria-pressed={trailerViewMode === 'list'}
                onclick={() => setTrailerViewMode('list')}
              >
                <LayoutList size={17} strokeWidth={1.7} />
              </button>
              <button
                type="button"
                class="ep-view-btn"
                class:active={trailerViewMode === 'grid'}
                aria-label="Trailers grid view"
                aria-pressed={trailerViewMode === 'grid'}
                onclick={() => setTrailerViewMode('grid')}
              >
                <LayoutGrid size={17} strokeWidth={1.7} />
              </button>
            </div>
          {/if}
        </div>

        {#if galleryShots.length > 0}
          <div class="frames-grid">
            {#each galleryShots as shot, i (shot)}
              {@const corners = gridCornerFlags(i, galleryShots.length, mediaCols)}
              <button
                type="button"
                class="frame-card"
                class:corner-tl={corners.tl}
                class:corner-tr={corners.tr}
                class:corner-bl={corners.bl}
                class:corner-br={corners.br}
                onclick={() => { lightboxUrl = shot; }}
                aria-label="Open frame {i + 1}"
              >
                <img src={shot} alt="{displayTitle} frame {i + 1}" class="frame-img" loading="lazy" decoding="async" />
              </button>
            {/each}
          </div>
        {/if}

        {#if trailerVideos.length > 0}
          {#if trailerViewMode === 'grid'}
            <div class="trailers-grid" class:has-top-gap={galleryShots.length > 0} in:fade={{ duration: 180, easing: cubicOut }}>
              {#each trailerVideos as video, i (video.id)}
                {@const thumb = video.imageUrl || galleryShots[i % Math.max(1, galleryShots.length)] || backdropUrl}
                {@const corners = gridCornerFlags(i, trailerVideos.length, mediaCols)}
                <button
                  type="button"
                  class="trailer-card"
                  class:corner-tl={corners.tl}
                  class:corner-tr={corners.tr}
                  class:corner-bl={corners.bl}
                  class:corner-br={corners.br}
                  onclick={() => { activeTrailer = video; }}
                  aria-label="Play {video.kindLabel}: {video.name}"
                >
                  {#if thumb}
                    <img src={thumb} alt={video.name} class="trailer-bg" loading="lazy" decoding="async" />
                  {/if}
                  <div class="trailer-scrim" aria-hidden="true"></div>
                  <div class="trailer-play-badge" aria-hidden="true">
                    <Play size={20} fill="currentColor" strokeWidth={0} />
                  </div>
                  <div class="trailer-meta">
                    <span class="trailer-kind">{video.kindLabel}</span>
                    <span class="trailer-name">{video.name}</span>
                  </div>
                </button>
              {/each}
            </div>
          {:else}
            <div class="trailers-list" class:has-top-gap={galleryShots.length > 0} in:fade={{ duration: 180, easing: cubicOut }}>
              {#each trailerVideos as video, i (video.id)}
                {@const thumb = video.imageUrl || galleryShots[i % Math.max(1, galleryShots.length)] || backdropUrl}
                <button
                  type="button"
                  class="trailer-row"
                  onclick={() => { activeTrailer = video; }}
                  aria-label="Play {video.kindLabel}: {video.name}"
                >
                  {#if thumb}
                    <div class="trailer-row-bg" aria-hidden="true">
                      <img src={thumb} alt="" class="trailer-row-bg-img" loading="lazy" decoding="async" />
                      <div class="trailer-row-bg-scrim"></div>
                    </div>
                  {/if}
                  <div class="trailer-row-thumb-box">
                    {#if thumb}
                      <img src={thumb} alt={video.name} class="trailer-row-thumb" loading="lazy" decoding="async" />
                    {/if}
                  </div>
                  <div class="trailer-row-info">
                    <div class="trailer-row-top">
                      <span class="trailer-kind">{video.kindLabel}</span>
                      <span class="trailer-row-title">{video.name}</span>
                    </div>
                    <span class="trailer-row-sub">Video preview</span>
                  </div>
                  <div class="trailer-row-aside" aria-hidden="true">
                    <span>#{String(i + 1).padStart(2, '0')}</span>
                    <span>HD</span>
                  </div>
                </button>
              {/each}
            </div>
          {/if}
        {/if}
      </section>
    {/if}
  </div>

  {#if relatedEntries.length > 0}
    <section class="franchise-section" aria-label="Franchise and Related">
      <div class="section-head is-bleed-head">
        <div class="section-head-left">
          <h2 class="section-heading">Franchise & Related</h2>
          <span class="section-count">{relatedEntries.length}</span>
        </div>
      </div>

      <div class="franchise-strip">
        {#each relatedEntries as item (`rel-${item.id}`)}
          <button
            type="button"
            class="franchise-card"
            class:is-current={item.isCurrent}
            onclick={() => openRelatedAnime(item)}
            aria-label="{item.title} — {item.relationLabel}"
          >
            {#if item.poster}
              <img src={item.poster} alt={item.title} class="franchise-poster" loading="lazy" decoding="async" />
            {/if}
            <div class="franchise-scrim" aria-hidden="true"></div>
            <div class="franchise-top-badge">
              <span class="franchise-kind-pill" class:is-current={item.isCurrent}>{item.relationLabel}</span>
            </div>
            <div class="franchise-caption">
              <span class="franchise-title">{item.title}</span>
              <span class="franchise-sub">
                {[
                  item.year ? String(item.year) : '',
                  item.kind,
                  item.episodes ? `${item.episodes} ep` : ''
                ].filter(Boolean).join(' · ')}
              </span>
            </div>
          </button>
        {/each}
      </div>
    </section>
  {/if}

  {#if allCharacters.length > 0}
    <section class="cast-section" aria-label="Characters and Cast">
      <div class="section-head cast-section-head is-bleed-head">
        <div class="section-head-left">
          <h2 class="section-heading">Characters & Cast</h2>
          <span class="section-count">{filteredCharacters.length}</span>
        </div>

        <div class="cast-controls">
          {#if hasMainCharacters || hasSupportingCharacters}
            <div class="lib-sort-group" role="group" aria-label="Filter character role">
              <button
                type="button"
                class="sort-chip-btn"
                class:active={charRoleFilter === 'all'}
                onclick={() => { charRoleFilter = 'all'; }}
              >
                All
              </button>
              {#if hasMainCharacters}
                <button
                  type="button"
                  class="sort-chip-btn"
                  class:active={charRoleFilter === 'main'}
                  onclick={() => { charRoleFilter = 'main'; }}
                >
                  Main
                </button>
              {/if}
              {#if hasSupportingCharacters}
                <button
                  type="button"
                  class="sort-chip-btn"
                  class:active={charRoleFilter === 'supporting'}
                  onclick={() => { charRoleFilter = 'supporting'; }}
                >
                  Support
                </button>
              {/if}
            </div>
          {/if}

          {#if hasGenderMeta}
            <div class="lib-sort-group" role="group" aria-label="Filter character gender">
              <button
                type="button"
                class="sort-chip-btn"
                class:active={charGenderFilter === 'all'}
                onclick={() => { charGenderFilter = 'all'; }}
              >
                Any
              </button>
              <button
                type="button"
                class="sort-chip-btn"
                class:active={charGenderFilter === 'Female'}
                onclick={() => { charGenderFilter = 'Female'; }}
              >
                Female
              </button>
              <button
                type="button"
                class="sort-chip-btn"
                class:active={charGenderFilter === 'Male'}
                onclick={() => { charGenderFilter = 'Male'; }}
              >
                Male
              </button>
            </div>
          {/if}

          <div class="lib-sort-group" role="group" aria-label="Sort characters">
            <button
              type="button"
              class="sort-chip-btn"
              class:active={charSortBy === 'default'}
              onclick={() => toggleCharSort('default')}
            >
              <span>Order</span>
              {#if charSortBy === 'default' && charSortDir === 'desc'}
                <ArrowDown size={13} strokeWidth={2.1} />
              {/if}
            </button>
            <button
              type="button"
              class="sort-chip-btn"
              class:active={charSortBy === 'name'}
              onclick={() => toggleCharSort('name')}
            >
              <span>Name</span>
              {#if charSortBy === 'name'}
                {#if charSortDir === 'asc'}
                  <ArrowUp size={13} strokeWidth={2.1} />
                {:else}
                  <ArrowDown size={13} strokeWidth={2.1} />
                {/if}
              {/if}
            </button>
            <button
              type="button"
              class="sort-chip-btn"
              class:active={charSortBy === 'va'}
              onclick={() => toggleCharSort('va')}
            >
              <span>Cast</span>
              {#if charSortBy === 'va'}
                {#if charSortDir === 'asc'}
                  <ArrowUp size={13} strokeWidth={2.1} />
                {:else}
                  <ArrowDown size={13} strokeWidth={2.1} />
                {/if}
              {/if}
            </button>
          </div>

          <label class="cast-search-box">
            <Search size={14} strokeWidth={1.8} class="cast-search-icon" />
            <input
              type="text"
              bind:value={charQuery}
              placeholder="Character or VA..."
              class="cast-search-input"
              aria-label="Filter characters or voice actors"
            />
          </label>

          <div class="ep-view-modes" class:is-grid={charViewMode === 'grid'} role="group" aria-label="Characters layout">
            <div class="ep-view-pill" aria-hidden="true"></div>
            <button
              type="button"
              class="ep-view-btn"
              class:active={charViewMode === 'strip'}
              aria-label="Strip view"
              aria-pressed={charViewMode === 'strip'}
              onclick={() => setCharViewMode('strip')}
            >
              <LayoutList size={17} strokeWidth={1.7} />
            </button>
            <button
              type="button"
              class="ep-view-btn"
              class:active={charViewMode === 'grid'}
              aria-label="Grid view"
              aria-pressed={charViewMode === 'grid'}
              onclick={() => setCharViewMode('grid')}
            >
              <LayoutGrid size={17} strokeWidth={1.7} />
            </button>
          </div>
        </div>
      </div>

      {#if visibleCharacters.length > 0}
        {#if charViewMode === 'strip'}
          <div class="cast-strip-wrap" in:fade={{ duration: 180, easing: cubicOut }}>
            <div
              class="cast-strip"
              bind:this={charStripEl}
              onscroll={updateCharScrollFlags}
            >
              {#each visibleCharacters as ch (ch.id)}
                <div class="cast-strip-card">
                  {#if ch.poster && !failedCharPortraits[ch.id]}
                    <img
                      src={ch.poster}
                      alt={ch.name}
                      class="cast-strip-img"
                      loading="lazy"
                      decoding="async"
                      onerror={() => {
                        failedCharPortraits = { ...failedCharPortraits, [ch.id]: true };
                      }}
                    />
                  {:else}
                    <div class="cast-portrait-fallback cast-strip-fallback" aria-hidden="true">
                      <span>{charInitials(ch.name)}</span>
                    </div>
                  {/if}
                  <div class="cast-strip-scrim" aria-hidden="true"></div>
                  <div class="cast-strip-top">
                    <span class="cast-strip-pill" class:is-main={ch.isMain}>
                      {ch.isMain ? 'Main' : 'Support'}
                    </span>
                    {#if ch.gender}
                      <span class="cast-strip-gender">{ch.gender}</span>
                    {/if}
                  </div>
                  <div class="cast-strip-overlay">
                    <span class="cast-strip-name">{ch.name}</span>
                    {#if ch.russian && ch.russian.toLowerCase() !== ch.name.toLowerCase()}
                      <span class="cast-strip-sub">{ch.russian}</span>
                    {/if}
                    {#if ch.voiceActor}
                      <div class="cast-va-row cast-strip-va">
                        {#if ch.voiceActor.poster}
                          <img src={ch.voiceActor.poster} alt={ch.voiceActor.name} class="cast-va-avatar" loading="lazy" decoding="async" />
                        {/if}
                        <span class="cast-va-name">{ch.voiceActor.name}</span>
                      </div>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>

            {#if canScrollCharLeft}
              <button
                type="button"
                class="strip-arrow-btn is-left"
                aria-label="Scroll characters left"
                onclick={() => scrollCharStrip(-1)}
              >
                <ChevronLeft size={20} strokeWidth={2} />
              </button>
            {/if}
            {#if canScrollCharRight}
              <button
                type="button"
                class="strip-arrow-btn is-right"
                aria-label="Scroll characters right"
                onclick={() => scrollCharStrip(1)}
              >
                <ChevronRight size={20} strokeWidth={2} />
              </button>
            {/if}
          </div>
        {:else}
          <div class="cast-grid-wrap" in:fade={{ duration: 180, easing: cubicOut }}>
            <div class="cast-grid">
              {#each visibleCharacters as ch (ch.id)}
                <div class="cast-card">
                  <div class="cast-portrait-wrap">
                    {#if ch.poster && !failedCharPortraits[ch.id]}
                      <img
                        src={ch.poster}
                        alt={ch.name}
                        class="cast-portrait"
                        loading="lazy"
                        decoding="async"
                        onerror={() => {
                          failedCharPortraits = { ...failedCharPortraits, [ch.id]: true };
                        }}
                      />
                    {:else}
                      <div class="cast-portrait-fallback" aria-hidden="true">
                        <span>{charInitials(ch.name)}</span>
                      </div>
                    {/if}
                  </div>

                  <div class="cast-info">
                    <div class="cast-name-row">
                      <span class="cast-name">{ch.name}</span>
                    </div>
                    <div class="cast-badges-line">
                      <span class="cast-role-tag" class:is-main={ch.isMain}>
                        {ch.isMain ? 'Main' : 'Supporting'}
                      </span>
                      {#if ch.gender}
                        <span class="cast-gender-tag">{ch.gender}</span>
                      {/if}
                    </div>
                    {#if ch.russian && ch.russian.toLowerCase() !== ch.name.toLowerCase()}
                      <span class="cast-sub-name">{ch.russian}</span>
                    {/if}
                    {#if ch.voiceActor}
                      <div class="cast-va-row">
                        {#if ch.voiceActor.poster}
                          <img src={ch.voiceActor.poster} alt={ch.voiceActor.name} class="cast-va-avatar" loading="lazy" decoding="async" />
                        {/if}
                        <span class="cast-va-name">{ch.voiceActor.name}</span>
                      </div>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>

            {#if !charsExpanded && charQuery.trim().length === 0 && filteredCharacters.length > 12}
              <div class="cast-more-row">
                <button
                  type="button"
                  class="cast-more-btn"
                  onclick={() => { charsExpanded = true; }}
                >
                  Show all {filteredCharacters.length}
                </button>
              </div>
            {/if}
          </div>
        {/if}
      {:else}
        <div class="cast-empty is-bleed-head">No matching characters or voice actors</div>
      {/if}
    </section>
  {/if}

  {#if similarEntries.length > 0}
    <section class="more-section" aria-label="More like this">
      <div class="more-heading-wrap">
        <h2 class="more-title">More like this</h2>
      </div>

      <div class="more-stage">
        <div class="more-underlay-blur" aria-hidden="true">
          <div
            class="more-underlay-track"
            style:transform={`translate3d(${-moreScrollLeft}px, 0, 0)`}
          >
            {#each similarEntries as rec (`blur-${rec.id}`)}
              <div class="more-underlay-tile" class:is-fewer={similarEntries.length < 3}>
                <img src={rec.poster} alt="" class="more-underlay-img" decoding="async" />
              </div>
            {/each}
          </div>
        </div>

        <div
          class="more-strip"
          bind:this={moreStripEl}
          onscroll={updateMoreScrollFlags}
        >
          {#each similarEntries as rec (`sim-${rec.id}`)}
            <button
              type="button"
              class="more-card"
              class:is-fewer={similarEntries.length < 3}
              onclick={() => openSimilarAnime(rec)}
              aria-label={rec.title}
            >
              <img src={rec.poster} alt={rec.title} class="more-card-img" loading="lazy" decoding="async" />
              <div class="more-card-overlay" aria-hidden="true"></div>
              <div class="more-card-info">
                <span class="more-card-title">{rec.title}</span>
                <span class="more-card-meta">
                  {#if rec.score}
                    <span class="more-card-score">
                      <Star size={11} fill="currentColor" strokeWidth={0} />
                      <span>{rec.score}</span>
                    </span>
                  {/if}
                  {#if rec.year}
                    <span>{rec.year}</span>
                  {/if}
                  {#if rec.episodes}
                    <span>{rec.episodes} ep</span>
                  {/if}
                </span>
              </div>
            </button>
          {/each}
        </div>

        {#if canScrollMoreLeft}
          <button
            type="button"
            class="strip-arrow-btn is-left"
            aria-label="Previous recommendations"
            onclick={() => scrollMoreStrip(-1)}
          >
            <ChevronLeft size={20} strokeWidth={2} />
          </button>
        {/if}
        {#if canScrollMoreRight}
          <button
            type="button"
            class="strip-arrow-btn is-right"
            aria-label="More recommendations"
            onclick={() => scrollMoreStrip(1)}
          >
            <ChevronRight size={20} strokeWidth={2} />
          </button>
        {/if}
      </div>
    </section>
  {/if}
</div>

  {#if lightboxUrl}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    use:portal
    class="lightbox-backdrop"
    transition:fade={{ duration: 180, easing: cubicOut }}
    onclick={(e) => {
      if (e.target === e.currentTarget) {
        lightboxUrl = null;
      }
    }}
  >
    <button
      type="button"
      class="lightbox-close"
      aria-label="Close frame preview"
      onclick={() => { lightboxUrl = null; }}
    >
      <X size={20} strokeWidth={2} />
    </button>
    <img
      src={lightboxUrl}
      alt={displayTitle}
      class="lightbox-img"
      transition:scale={{ start: 0.96, duration: 200, easing: cubicOut }}
    />
  </div>
{/if}

{#if activeTrailer}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    use:portal
    class="lightbox-backdrop"
    transition:fade={{ duration: 180, easing: cubicOut }}
    onclick={(e) => {
      if (e.target === e.currentTarget) {
        activeTrailer = null;
      }
    }}
  >
    <button
      type="button"
      class="lightbox-close"
      aria-label="Close trailer preview"
      onclick={() => { activeTrailer = null; }}
    >
      <X size={20} strokeWidth={2} />
    </button>
    <div
      class="trailer-modal-shell"
      transition:scale={{ start: 0.96, duration: 200, easing: cubicOut }}
    >
      <div class="trailer-modal-header">
        <span class="trailer-modal-kind">{activeTrailer.kindLabel}</span>
        <span class="trailer-modal-title">{activeTrailer.name}</span>
      </div>
      <div class="trailer-modal-frame">
        <iframe
          src={activeTrailer.embedUrl}
          title={activeTrailer.name}
          allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
          allowfullscreen
        ></iframe>
      </div>
    </div>
  </div>
{/if}

<TorrentPopup
  open={torrentsOpen}
  title={displayTitle}
  poster={posterUrl}
  loading={torrentsLoading}
  error={torrentsError}
  results={torrentResults}
  onclose={() => { torrentsOpen = false; }}
  onselect={(item) => void handleLaunchTorrent(item)}
/>

<style>
  .anime-page {
    width: 100%;
    min-height: 100%;
    background: #0c0c0c;
    color: #ffffff;
    display: flex;
    flex-direction: column;
    padding-bottom: 0;
    box-sizing: border-box;
  }

  .anime-hero-bg {
    position: relative;
    width: 100%;
    min-height: clamp(440px, 64vh, 650px);
    padding-top: 72px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    contain: layout style paint;
  }

  .backdrop-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center 22%;
    opacity: 0.56;
    pointer-events: none;
    user-select: none;
    z-index: 0;
  }

  .backdrop-dim {
    position: absolute;
    inset: 0;
    background: linear-gradient(
      90deg,
      rgba(12, 12, 12, 0.9) 0%,
      rgba(12, 12, 12, 0.56) 52%,
      rgba(12, 12, 12, 0.44) 100%
    );
    pointer-events: none;
    z-index: 1;
  }

  .backdrop-fade {
    position: absolute;
    inset: 0;
    background: linear-gradient(
      180deg,
      rgba(12, 12, 12, 0.42) 0%,
      rgba(12, 12, 12, 0.22) 38%,
      rgba(12, 12, 12, 0.78) 78%,
      #0c0c0c 100%
    );
    pointer-events: none;
    z-index: 2;
  }

  .anime-top-inner {
    position: relative;
    z-index: 3;
    flex: 1 1 auto;
    padding: 28px var(--page-pad-x) 24px var(--page-pad-x);
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
  }

  .top-origin-title {
    margin-bottom: 12px;
    font-family: 'PP Monument Extended', system-ui, sans-serif;
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: rgba(255, 255, 255, 0.42);
    user-select: none;
  }

  .back-btn {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 43px;
    padding: 0 16px;
    border: none;
    outline: none;
    border-radius: 21px;
    background: transparent;
    color: rgba(255, 255, 255, 0.48);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
    transition: background-color 0.2s var(--ease-out), color 0.2s var(--ease-out), transform 0.2s var(--ease-out);
  }

  .back-btn:hover {
    background: rgba(255, 255, 255, 0.06);
    color: rgba(255, 255, 255, 0.92);
    transform: translateX(-2px);
  }

  .anime-overview {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: clamp(24px, 3.2vw, 48px);
    margin-top: auto;
  }

  .overview-meta-col {
    display: flex;
    flex-direction: column;
    min-width: 0;
    flex: 1 1 auto;
    max-width: 780px;
  }

  .anime-title {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(28px, 4.4vw, 66px);
    font-weight: 700;
    line-height: 1.02;
    letter-spacing: -0.02em;
    color: #ffffff;
  }

  .anime-meta-line {
    margin-top: 16px;
  }

  .desc-block {
    margin-top: 16px;
    max-width: 740px;
    animation: descReveal 0.36s cubic-bezier(0.22, 1, 0.36, 1) both;
  }

  @keyframes descReveal {
    from {
      opacity: 0;
      transform: translate3d(0, 6px, 0);
    }
    to {
      opacity: 1;
      transform: translate3d(0, 0, 0);
    }
  }

  .desc-viewport {
    overflow: hidden;
    transition:
      max-height 0.44s cubic-bezier(0.22, 1, 0.36, 1),
      -webkit-mask-position 0.38s cubic-bezier(0.22, 1, 0.36, 1),
      mask-position 0.38s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .desc-viewport.is-collapsible {
    -webkit-mask-image: linear-gradient(to bottom, #000 0%, #000 80%, transparent 100%);
    mask-image: linear-gradient(to bottom, #000 0%, #000 80%, transparent 100%);
    -webkit-mask-size: 100% 210%;
    mask-size: 100% 210%;
    -webkit-mask-repeat: no-repeat;
    mask-repeat: no-repeat;
    -webkit-mask-position: 0% 100%;
    mask-position: 0% 100%;
  }

  .desc-viewport.is-collapsible.is-expanded {
    -webkit-mask-position: 0% 0%;
    mask-position: 0% 0%;
  }

  .anime-description {
    margin: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: clamp(14.5px, 1.45vw, 18px);
    line-height: 1.44;
    color: rgba(255, 255, 255, 0.62);
    transition: color 0.36s var(--ease-out);
  }

  .anime-description.is-expanded {
    color: rgba(255, 255, 255, 0.72);
  }

  .desc-toggle {
    margin-top: 8px;
    padding: 0;
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.65);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    cursor: pointer;
    transition: color 0.16s ease;
  }

  .desc-toggle:hover {
    color: #ffffff;
  }

  .overview-actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 12px;
    margin-top: 22px;
  }

  .share-icon-btn {
    position: relative;
    width: 40px;
    height: 40px;
    padding: 0;
    border: none;
    outline: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.56);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    -webkit-tap-highlight-color: transparent;
    transition: color 0.2s var(--ease-out), transform 0.16s var(--ease-out);
  }

  .share-icon-btn:hover {
    color: #ffffff;
    transform: translateY(-1px);
  }

  .share-icon-btn:active {
    transform: scale(0.92);
  }

  .share-icon-btn.is-copied {
    color: #a6e3a1;
  }

  .share-icon-stack {
    position: relative;
    width: 18px;
    height: 18px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .share-icon-item {
    position: absolute;
    inset: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .seasons-bar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 22px;
    margin-top: 24px;
  }

  .season-tab {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    padding: 4px 0;
    border: none;
    outline: none;
    border-radius: 0;
    background: transparent;
    color: rgba(255, 255, 255, 0.46);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 15px;
    font-weight: 400;
    line-height: 1;
    letter-spacing: 0;
    white-space: nowrap;
    cursor: pointer;
    user-select: none;
    transition:
      color 180ms var(--ease-out),
      transform 140ms var(--ease-out);
  }

  .season-tab:hover {
    background: transparent;
    color: rgba(255, 255, 255, 0.86);
  }

  .season-tab:active {
    transform: scale(0.97);
  }

  .season-tab.active {
    background: transparent;
    color: #ffffff;
  }

  .season-tab.active:hover {
    background: transparent;
    color: #ffffff;
  }

  .season-tab.is-static {
    cursor: default;
  }

  .season-tab.is-static:active {
    transform: none;
  }

  .season-label {
    display: inline-flex;
    align-items: center;
    line-height: 1;
  }

  .season-year {
    display: inline-flex;
    align-items: center;
    font-size: 13px;
    font-weight: 400;
    line-height: 1;
    font-variant-numeric: tabular-nums;
    color: inherit;
    opacity: 0.62;
  }

  .season-tab.active .season-year {
    opacity: 0.72;
  }

  /* Atmospheric borderless poster showcase on right side of hero */
  .showcase-art {
    position: relative;
    width: clamp(185px, 22vw, 350px);
    flex-shrink: 0;
    display: flex;
    align-items: flex-end;
    justify-content: flex-end;
  }

  .poster-glow {
    position: absolute;
    inset: -32px;
    pointer-events: none;
    overflow: hidden;
    -webkit-mask-image: radial-gradient(ellipse at center, rgba(0, 0, 0, 1) 38%, rgba(0, 0, 0, 0) 74%);
    mask-image: radial-gradient(ellipse at center, rgba(0, 0, 0, 1) 38%, rgba(0, 0, 0, 0) 74%);
  }

  .poster-wrap {
    position: relative;
    width: 100%;
    aspect-ratio: 2 / 3;
    z-index: 2;
    overflow: hidden;
    -webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.55) 0%, #000 10%, #000 58%, transparent 100%);
    mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.55) 0%, #000 10%, #000 58%, transparent 100%);
  }

  .poster-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    user-select: none;
    pointer-events: none;
  }

  .poster-edge-vignette {
    position: absolute;
    inset: 0;
    background: radial-gradient(
      88% 84% at 52% 42%,
      rgba(12, 12, 12, 0) 50%,
      rgba(12, 12, 12, 0.42) 76%,
      rgba(12, 12, 12, 0.92) 100%
    );
    pointer-events: none;
  }

  /* Full-bleed edge-to-edge player stage */
  .player-section {
    position: relative;
    width: 100%;
    padding: 10px 0 4px 0;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
  }

  .player-frame-wrap {
    position: relative;
    width: 100%;
  }

  .player-stage {
    position: relative;
    z-index: 1;
    width: 100%;
    aspect-ratio: 16 / 9;
    max-height: min(82vh, 56.25vw);
    border-radius: 0;
    clip-path: inset(0);
    overflow: hidden;
    background: #070709;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .player-stage :global(.hls-player) {
    border-radius: 0;
  }

  .player-inline-slot {
    width: 100%;
    height: 100%;
  }

  .player-inline-slot.is-hidden {
    display: none;
  }

  .anime-body {
    padding: clamp(24px, 3vw, 36px) var(--page-pad-x) 0 var(--page-pad-x);
    display: flex;
    flex-direction: column;
    gap: clamp(32px, 3.8vw, 48px);
  }

  .stage-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 14px;
    padding: 24px;
    text-align: center;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 15px;
    color: rgba(255, 255, 255, 0.6);
  }

  .error-text {
    margin: 0;
    color: #f87171;
  }

  .spinner {
    width: 30px;
    height: 30px;
    border-radius: 50%;
    border: 2.5px solid rgba(255, 255, 255, 0.14);
    border-top-color: #ffffff;
    animation: spin 0.75s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  /* Episodes Section */
  .section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 18px;
  }

  .section-head-left {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }

  .section-heading {
    margin: 0;
    font-family: 'PP Monument Extended', system-ui, sans-serif;
    font-size: clamp(20px, 2vw, 24px);
    font-weight: 900;
    color: #e4e4e7;
    letter-spacing: -0.01em;
  }

  .section-count {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 14px;
    color: rgba(255, 255, 255, 0.42);
  }

  .range-tabs {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }

  .range-pill {
    height: 32px;
    padding: 0 13px;
    border: none;
    outline: none;
    border-radius: 10px;
    background: rgba(255, 255, 255, 0.06);
    color: rgba(255, 255, 255, 0.72);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 400;
    cursor: pointer;
    transition: background-color 0.18s var(--ease-out), color 0.18s var(--ease-out);
  }

  .range-pill:hover {
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
  }

  .range-pill.is-active {
    background: rgba(255, 255, 255, 0.16);
    color: #ffffff;
    font-weight: 400;
  }

  .torrent-toggle-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  /* Shared Library.svelte Sort Group & View Switcher */
  .lib-sort-group {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    height: 36px;
    padding: 3px;
    border-radius: 11px;
    background: rgba(255, 255, 255, 0.04);
    box-sizing: border-box;
  }

  .sort-chip-btn {
    height: 30px;
    padding: 0 10px;
    border: none;
    outline: none;
    border-radius: 8px;
    background: transparent;
    color: rgba(255, 255, 255, 0.44);
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 500;
    cursor: pointer;
    transition: background-color 0.2s var(--ease-out), color 0.2s var(--ease-out);
    white-space: nowrap;
  }

  .sort-chip-btn:hover {
    color: rgba(255, 255, 255, 0.8);
  }

  .sort-chip-btn.active {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .ep-view-modes {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    height: 36px;
    padding: 3px;
    border-radius: 11px;
    background: rgba(255, 255, 255, 0.04);
    box-sizing: border-box;
  }

  .ep-view-pill {
    position: absolute;
    top: 3px;
    left: 3px;
    width: 30px;
    height: 30px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.1);
    pointer-events: none;
    transition: transform 0.32s var(--ease-out);
  }

  .ep-view-modes.is-grid .ep-view-pill {
    transform: translate3d(34px, 0, 0);
  }

  .ep-view-btn {
    position: relative;
    z-index: 1;
    width: 30px;
    height: 30px;
    padding: 0;
    border: none;
    outline: none;
    border-radius: 8px;
    background: transparent;
    color: rgba(255, 255, 255, 0.42);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: color 0.2s var(--ease-out);
  }

  .ep-view-btn:hover {
    color: rgba(255, 255, 255, 0.8);
  }

  .ep-view-btn.active {
    color: #ffffff;
  }

  /* Connected Corner-Only Mosaic Grids (Episodes, Frames, Trailers) */
  .episodes-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 0;
  }

  @media (max-width: 1220px) {
    .episodes-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (max-width: 960px) {
    .episodes-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  .episodes-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  /* Frames Gallery */
  .frames-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0;
  }

  @media (max-width: 960px) {
    .frames-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  .frame-card {
    position: relative;
    padding: 0;
    border: none;
    outline: none;
    border-radius: 0;
    overflow: hidden;
    aspect-ratio: 16 / 9;
    background: #0c0c0c;
    isolation: isolate;
    transform: translate3d(0, 0, 0);
    cursor: pointer;
  }

  .frame-card.corner-tl {
    border-top-left-radius: var(--radius-card);
  }

  .frame-card.corner-tr {
    border-top-right-radius: var(--radius-card);
  }

  .frame-card.corner-bl {
    border-bottom-left-radius: var(--radius-card);
  }

  .frame-card.corner-br {
    border-bottom-right-radius: var(--radius-card);
  }

  .frame-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    backface-visibility: hidden;
    transition: transform 0.45s var(--ease-out);
  }

  .frame-card:hover .frame-img {
    transform: scale(1.04);
  }

  .lightbox-backdrop {
    position: fixed;
    inset: 0;
    z-index: 900;
    background: rgba(6, 6, 8, 0.88);
    backdrop-filter: blur(18px);
    -webkit-backdrop-filter: blur(18px);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 36px;
  }

  .lightbox-close {
    position: absolute;
    top: 24px;
    right: 28px;
    width: 40px;
    height: 40px;
    border: none;
    outline: none;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
  }

  .lightbox-img {
    max-width: 92vw;
    max-height: 86vh;
    border-radius: var(--radius-card);
    object-fit: contain;
    display: block;
  }

  /* Trailers Grid (connected mosaic) + Trailers List (blurred flush thumb) */
  .trailers-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0;
  }

  .trailers-grid.has-top-gap,
  .trailers-list.has-top-gap {
    margin-top: 16px;
  }

  @media (max-width: 960px) {
    .trailers-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  .trailer-card {
    position: relative;
    padding: 0;
    border: none;
    outline: none;
    border-radius: 0;
    overflow: hidden;
    aspect-ratio: 16 / 9;
    background: #0c0c0c;
    isolation: isolate;
    transform: translate3d(0, 0, 0);
    cursor: pointer;
    text-align: left;
  }

  .trailer-card.corner-tl {
    border-top-left-radius: var(--radius-card);
  }

  .trailer-card.corner-tr {
    border-top-right-radius: var(--radius-card);
  }

  .trailer-card.corner-bl {
    border-bottom-left-radius: var(--radius-card);
  }

  .trailer-card.corner-br {
    border-bottom-right-radius: var(--radius-card);
  }

  .trailer-bg {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    filter: brightness(0.86);
    transition: transform 0.45s var(--ease-out), filter 0.35s var(--ease-out);
  }

  .trailer-card:hover .trailer-bg {
    transform: scale(1.05);
    filter: brightness(0.98);
  }

  .trailer-scrim {
    position: absolute;
    inset: -2px;
    z-index: 1;
    background: linear-gradient(
      180deg,
      rgba(12, 12, 12, 0.18) 0%,
      rgba(12, 12, 12, 0.34) 48%,
      rgba(12, 12, 12, 0.88) 100%
    );
    pointer-events: none;
  }

  .trailer-play-badge {
    position: absolute;
    top: 50%;
    left: 50%;
    z-index: 2;
    width: 48px;
    height: 48px;
    border-radius: 50%;
    background: rgba(12, 12, 12, 0.56);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    color: #ffffff;
    display: flex;
    align-items: center;
    justify-content: center;
    transform: translate(-50%, -56%);
    transition: background-color 0.25s var(--ease-out), transform 0.28s var(--ease-out);
  }

  .trailer-play-badge :global(svg) {
    transform: translateX(1px);
  }

  .trailer-card:hover .trailer-play-badge {
    background: rgba(255, 255, 255, 0.92);
    color: #0c0c0c;
    transform: translate(-50%, -56%) scale(1.08);
  }

  .trailer-meta {
    position: absolute;
    left: 14px;
    right: 14px;
    bottom: 12px;
    z-index: 3;
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .trailer-kind {
    flex-shrink: 0;
    padding: 0;
    border-radius: 0;
    background: transparent;
    color: #ffffff;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 11.5px;
    font-weight: 700;
    letter-spacing: -0.01em;
    text-shadow: 0 1px 8px rgba(0, 0, 0, 0.85);
  }

  .trailer-name {
    min-width: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    color: rgba(255, 255, 255, 0.84);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .trailers-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .trailer-row {
    position: relative;
    width: 100%;
    height: 92px;
    padding: 0 16px 0 0;
    border: none;
    outline: none;
    border-radius: var(--radius-card) 0 0 var(--radius-card);
    overflow: hidden;
    background: #0e0e11;
    display: flex;
    align-items: center;
    gap: 16px;
    text-align: left;
    color: #ffffff;
    cursor: pointer;
    isolation: isolate;
  }

  .trailer-row-bg {
    position: absolute;
    inset: 0;
    z-index: 0;
    pointer-events: none;
    overflow: hidden;
  }

  .trailer-row-bg-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    filter: blur(30px) saturate(1.22) brightness(0.42);
    transform: scale(1.22);
    opacity: 0.72;
    transition: opacity 0.3s var(--ease-out), filter 0.3s var(--ease-out);
  }

  .trailer-row:hover .trailer-row-bg-img {
    opacity: 0.94;
    filter: blur(28px) saturate(1.28) brightness(0.5);
  }

  .trailer-row-bg-scrim {
    position: absolute;
    inset: 0;
    background: linear-gradient(
      90deg,
      rgba(12, 12, 12, 0.25) 0%,
      rgba(12, 12, 12, 0.72) 38%,
      rgba(12, 12, 12, 0.88) 100%
    );
  }

  .trailer-row-thumb-box {
    position: relative;
    z-index: 1;
    width: 164px;
    height: 100%;
    flex-shrink: 0;
    border-radius: var(--radius-card) 0 0 var(--radius-card);
    overflow: hidden;
    background: #09090b;
  }

  .trailer-row-thumb {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.38s var(--ease-out);
  }

  .trailer-row:hover .trailer-row-thumb {
    transform: scale(1.04);
  }

  .trailer-row-info {
    position: relative;
    z-index: 1;
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .trailer-row-top {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }

  .trailer-row-title {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 14px;
    font-weight: 600;
    color: #ffffff;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .trailer-row-sub {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
    color: rgba(255, 255, 255, 0.5);
  }

  .trailer-row-aside {
    position: relative;
    z-index: 1;
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    gap: 10px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-variant-numeric: tabular-nums;
    font-size: 11.5px;
    color: rgba(255, 255, 255, 0.34);
  }

  .trailer-modal-shell {
    width: min(980px, 90vw);
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .trailer-modal-header {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 4px;
  }

  .trailer-modal-kind {
    padding: 4px 10px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
  }

  .trailer-modal-title {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 16px;
    font-weight: 600;
    color: #ffffff;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .trailer-modal-frame {
    width: 100%;
    aspect-ratio: 16 / 9;
    border-radius: var(--radius-card);
    overflow: hidden;
    background: #070709;
    box-shadow: 0 28px 80px rgba(0, 0, 0, 0.72);
  }

  .trailer-modal-frame iframe {
    width: 100%;
    height: 100%;
    border: none;
    display: block;
  }

  /* Bleed Head Utility */
  .is-bleed-head {
    padding-left: var(--page-pad-x);
    padding-right: var(--page-pad-x);
  }

  /* Shared Horizontal Strip Chevron Arrows */
  .strip-arrow-btn {
    position: absolute;
    top: 50%;
    z-index: 6;
    width: 40px;
    height: 40px;
    border: none;
    outline: none;
    border-radius: 50%;
    background: rgba(12, 12, 14, 0.68);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    color: rgba(255, 255, 255, 0.9);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transform: translateY(-50%);
    transition: background-color 0.2s var(--ease-out), transform 0.2s var(--ease-out), color 0.2s var(--ease-out);
  }

  .strip-arrow-btn.is-left {
    left: 16px;
  }

  .strip-arrow-btn.is-right {
    right: 16px;
  }

  .strip-arrow-btn:hover {
    background: rgba(255, 255, 255, 0.92);
    color: #0c0c0c;
    transform: translateY(-50%) scale(1.06);
  }

  /* Franchise & Related Section (full-bleed edge-to-edge strip, zero gap, zero radius) */
  .franchise-section {
    width: 100%;
    padding-top: clamp(32px, 3.8vw, 48px);
  }

  .franchise-strip {
    display: flex;
    flex-direction: row;
    width: 100%;
    overflow-x: auto;
    scrollbar-width: none;
    gap: 0;
  }

  .franchise-strip::-webkit-scrollbar {
    display: none;
  }

  .franchise-card {
    position: relative;
    flex: 1 1 0;
    min-width: 210px;
    height: clamp(290px, 36vh, 410px);
    padding: 0;
    margin: 0;
    border: none;
    outline: none;
    border-radius: 0;
    overflow: hidden;
    background: #101014;
    isolation: isolate;
    cursor: pointer;
    text-align: left;
  }

  .franchise-poster {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.55s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .franchise-card:hover .franchise-poster {
    transform: scale(1.04);
  }

  .franchise-scrim {
    position: absolute;
    inset: -2px;
    z-index: 1;
    background: linear-gradient(
      180deg,
      rgba(12, 12, 12, 0.16) 0%,
      rgba(12, 12, 12, 0.24) 48%,
      rgba(12, 12, 12, 0.92) 100%
    );
    pointer-events: none;
  }

  .franchise-top-badge {
    position: absolute;
    top: 12px;
    left: 12px;
    z-index: 3;
  }

  .franchise-kind-pill {
    display: inline-block;
    padding: 0;
    border-radius: 0;
    background: transparent;
    color: rgba(255, 255, 255, 0.82);
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 11.5px;
    font-weight: 700;
    letter-spacing: -0.01em;
    text-shadow: 0 1px 8px rgba(0, 0, 0, 0.88);
  }

  .franchise-kind-pill.is-current {
    background: transparent;
    color: #ffffff;
    font-weight: 700;
  }

  .franchise-caption {
    position: absolute;
    left: 14px;
    right: 14px;
    bottom: 14px;
    z-index: 3;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .franchise-title {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 13.5px;
    font-weight: 600;
    line-height: 1.25;
    color: #ffffff;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .franchise-sub {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
    color: rgba(255, 255, 255, 0.58);
  }

  /* Characters & Cast Section (View 1 Strip + View 2 Grid) */
  .cast-section {
    width: 100%;
    padding-top: clamp(32px, 3.8vw, 48px);
  }

  .cast-section-head {
    flex-wrap: wrap;
  }

  .cast-controls {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .cast-search-box {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 36px;
    padding: 0 12px;
    border-radius: 11px;
    background: rgba(255, 255, 255, 0.04);
    color: rgba(255, 255, 255, 0.48);
    transition: background-color 0.2s var(--ease-out);
  }

  .cast-search-box:focus-within {
    background: rgba(255, 255, 255, 0.08);
    color: rgba(255, 255, 255, 0.85);
  }

  .cast-search-input {
    width: 152px;
    border: none;
    outline: none;
    background: transparent;
    color: #ffffff;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
  }

  .cast-search-input::placeholder {
    color: rgba(255, 255, 255, 0.36);
  }

  /* View 1: Full-Bleed Characters Strip (default) */
  .cast-strip-wrap {
    position: relative;
    width: 100%;
  }

  .cast-strip {
    display: flex;
    flex-direction: row;
    width: 100%;
    overflow-x: auto;
    scrollbar-width: none;
    scroll-behavior: smooth;
    gap: 0;
  }

  .cast-strip::-webkit-scrollbar {
    display: none;
  }

  .cast-strip-card {
    position: relative;
    flex: 0 0 calc(100% / 5);
    min-width: 220px;
    height: clamp(320px, 38vh, 420px);
    border-radius: 0;
    margin: 0;
    overflow: hidden;
    background: #101014;
  }

  @media (max-width: 1180px) {
    .cast-strip-card {
      flex: 0 0 25%;
    }
  }

  @media (max-width: 820px) {
    .cast-strip-card {
      flex: 0 0 50%;
    }
  }

  .cast-strip-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center top;
    display: block;
    transition: transform 0.55s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .cast-strip-card:hover .cast-strip-img {
    transform: scale(1.04);
  }

  .cast-strip-fallback {
    font-size: 26px;
  }

  .cast-strip-scrim {
    position: absolute;
    inset: -2px;
    z-index: 1;
    background: linear-gradient(
      180deg,
      rgba(12, 12, 12, 0.16) 0%,
      rgba(12, 12, 12, 0.12) 42%,
      rgba(12, 12, 12, 0.92) 100%
    );
    pointer-events: none;
  }

  .cast-strip-top {
    position: absolute;
    top: 12px;
    left: 12px;
    z-index: 2;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .cast-strip-pill {
    padding: 0;
    border-radius: 0;
    background: transparent;
    color: rgba(255, 255, 255, 0.78);
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 11px;
    font-weight: 700;
    letter-spacing: -0.01em;
    text-shadow: 0 1px 8px rgba(0, 0, 0, 0.88);
  }

  .cast-strip-pill.is-main {
    background: transparent;
    color: #ffffff;
    font-weight: 700;
  }

  .cast-strip-gender {
    padding: 0;
    border-radius: 0;
    background: transparent;
    color: rgba(255, 255, 255, 0.56);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 10.5px;
    text-shadow: 0 1px 6px rgba(0, 0, 0, 0.85);
  }

  .cast-strip-overlay {
    position: absolute;
    left: 14px;
    right: 14px;
    bottom: 14px;
    z-index: 2;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .cast-strip-name {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 14px;
    font-weight: 600;
    line-height: 1.2;
    color: #ffffff;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cast-strip-sub {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
    color: rgba(255, 255, 255, 0.54);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cast-strip-va {
    margin-top: 4px;
  }

  /* View 2: Refined Editorial Cast Grid */
  .cast-grid-wrap {
    padding: 0 var(--page-pad-x);
  }

  .cast-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 10px;
  }

  @media (max-width: 1120px) {
    .cast-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  @media (max-width: 680px) {
    .cast-grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  .cast-card {
    position: relative;
    display: flex;
    align-items: center;
    gap: 14px;
    height: 98px;
    padding: 0 14px 0 0;
    border-radius: 14px;
    overflow: hidden;
    background: rgba(255, 255, 255, 0.032);
    transition: background-color 0.22s var(--ease-out);
  }

  .cast-card:hover {
    background: rgba(255, 255, 255, 0.06);
  }

  .cast-portrait-wrap {
    position: relative;
    width: 72px;
    height: 100%;
    flex-shrink: 0;
    background: rgba(255, 255, 255, 0.05);
    -webkit-mask-image: linear-gradient(90deg, #000 72%, transparent 100%);
    mask-image: linear-gradient(90deg, #000 72%, transparent 100%);
  }

  .cast-portrait {
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center top;
    display: block;
    transition: transform 0.35s var(--ease-out);
  }

  .cast-card:hover .cast-portrait {
    transform: scale(1.04);
  }

  .cast-portrait-fallback {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 14px;
    font-weight: 700;
    color: rgba(255, 255, 255, 0.45);
    background: radial-gradient(circle at 50% 35%, rgba(255, 255, 255, 0.09), rgba(255, 255, 255, 0.02));
  }

  .cast-info {
    min-width: 0;
    flex: 1 1 auto;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .cast-name-row {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .cast-name {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 13.5px;
    font-weight: 600;
    color: #ffffff;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cast-badges-line {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .cast-role-tag {
    flex-shrink: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 10.5px;
    color: rgba(255, 255, 255, 0.46);
  }

  .cast-role-tag.is-main {
    color: rgba(255, 255, 255, 0.88);
    font-weight: 500;
  }

  .cast-gender-tag {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 10.5px;
    color: rgba(255, 255, 255, 0.34);
  }

  .cast-sub-name {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11px;
    color: rgba(255, 255, 255, 0.38);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cast-va-row {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-top: 2px;
    min-width: 0;
  }

  .cast-va-avatar {
    width: 17px;
    height: 17px;
    border-radius: 50%;
    object-fit: cover;
    flex-shrink: 0;
  }

  .cast-va-name {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
    color: rgba(255, 255, 255, 0.6);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cast-more-row {
    margin-top: 14px;
    display: flex;
    justify-content: center;
  }

  .cast-more-btn {
    padding: 8px 16px;
    border: none;
    outline: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.56);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: color 0.18s var(--ease-out);
  }

  .cast-more-btn:hover {
    color: #ffffff;
  }

  .cast-empty {
    padding: 24px 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13.5px;
    color: rgba(255, 255, 255, 0.42);
  }

  /* More Like This Section (max 3 visible cards, right chevron, unified blur, flush bottom) */
  .more-section {
    position: relative;
    width: 100%;
    margin-top: auto;
    padding-top: clamp(42px, 5vw, 64px);
    background: #0c0c0c;
    overflow: visible;
    display: flex;
    flex-direction: column;
  }

  .more-heading-wrap {
    padding: 0 var(--page-pad-x);
    margin-bottom: clamp(18px, 2.2vw, 26px);
    position: relative;
    z-index: 2;
  }

  .more-title {
    margin: 0;
    font-family: 'PP Monument Extended', system-ui, sans-serif;
    font-size: clamp(26px, 3.2vw, 48px);
    font-weight: 900;
    color: #d4d4d4;
    line-height: 1;
    user-select: none;
  }

  .more-stage {
    position: relative;
    width: 100%;
    overflow: visible;
  }

  .more-underlay-blur {
    position: absolute;
    top: -120px;
    left: -60px;
    right: -60px;
    bottom: 0;
    z-index: 0;
    pointer-events: none;
    overflow: hidden;
    filter: blur(88px) saturate(1.24);
    opacity: 0.44;
    -webkit-mask-image: linear-gradient(to bottom, transparent 0%, black 28%, black 100%);
    mask-image: linear-gradient(to bottom, transparent 0%, black 28%, black 100%);
  }

  .more-underlay-track {
    display: flex;
    flex-direction: row;
    width: 100%;
    height: 100%;
    padding: 0 60px;
    box-sizing: content-box;
    will-change: transform;
  }

  .more-underlay-tile {
    flex: 0 0 calc(100% / 3);
    height: 100%;
  }

  .more-underlay-tile.is-fewer {
    flex: 1 1 0;
  }

  .more-underlay-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .more-strip {
    position: relative;
    z-index: 1;
    display: flex;
    flex-direction: row;
    width: 100%;
    height: clamp(380px, 50vh, 560px);
    overflow-x: auto;
    scrollbar-width: none;
    scroll-behavior: smooth;
  }

  .more-strip::-webkit-scrollbar {
    display: none;
  }

  .more-card {
    position: relative;
    flex: 0 0 calc(100% / 3);
    min-width: 0;
    height: 100%;
    padding: 0;
    border: none;
    outline: none;
    border-radius: 0;
    margin: 0;
    overflow: hidden;
    background: #0d0d10;
    cursor: pointer;
    text-align: left;
  }

  .more-card.is-fewer {
    flex: 1 1 0;
  }

  .more-card-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.6s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .more-card:hover .more-card-img {
    transform: scale(1.03);
  }

  .more-card-overlay {
    position: absolute;
    inset: -2px;
    background: linear-gradient(
      180deg,
      rgba(0, 0, 0, 0.28) 0%,
      rgba(0, 0, 0, 0.38) 54%,
      rgba(0, 0, 0, 0.9) 100%
    );
    pointer-events: none;
    transition: background 0.35s ease;
  }

  .more-card:hover .more-card-overlay {
    background: linear-gradient(
      180deg,
      rgba(0, 0, 0, 0.12) 0%,
      rgba(0, 0, 0, 0.22) 52%,
      rgba(0, 0, 0, 0.88) 100%
    );
  }

  .more-card-info {
    position: absolute;
    left: 20px;
    right: 20px;
    bottom: 20px;
    z-index: 2;
    display: flex;
    flex-direction: column;
    gap: 5px;
    opacity: 0.84;
    transform: translate3d(0, 4px, 0);
    transition: opacity 0.28s var(--ease-out), transform 0.28s var(--ease-out);
  }

  .more-card:hover .more-card-info {
    opacity: 1;
    transform: translate3d(0, 0, 0);
  }

  .more-card-title {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 16px;
    font-weight: 600;
    line-height: 1.24;
    color: #ffffff;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .more-card-meta {
    display: flex;
    align-items: center;
    gap: 8px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12px;
    color: rgba(255, 255, 255, 0.66);
  }

  .more-card-score {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    color: #f9e2af;
  }

  @media (max-width: 900px) {
    .more-strip {
      height: clamp(300px, 40vh, 420px);
    }

    .more-card {
      flex: 0 0 50%;
    }
  }
</style>
