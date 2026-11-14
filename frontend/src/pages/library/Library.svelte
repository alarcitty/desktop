<script lang="ts">
  import { onMount } from 'svelte';
  import { cubicIn, cubicOut } from 'svelte/easing';
  import LayoutList from '@lucide/svelte/icons/layout-list';
  import LayoutGrid from '@lucide/svelte/icons/layout-grid';
  import Play from '@lucide/svelte/icons/play';
  import X from '@lucide/svelte/icons/x';

  import MainButton from '../../marlakit/button/MainButton.svelte';
  import LayerBlur from '../../marlakit/blur/LayerBlur.svelte';
  import MetaLine from '../../marlakit/meta/MetaLine.svelte';
  import {
    clearWatchHistory,
    formatTimeCompact,
    removeHistoryEntry,
    watchHistory
  } from '../../app/history.svelte';
  import {
    enrichLibraryArtwork,
    localLibrary,
    type LibraryListKind
  } from '../../app/library.svelte';
  import { openWatch } from '../../app/player.svelte';

  interface DisplayLibraryItem
  {
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
    episodeOrdinal?: number;
    episodeIndex?: number;
    resumeTime?: number;
    progressPct?: number;
  }

  const tabs = ['Likes', 'watch later', 'favorites', 'history'];
  const listByTab: Array<LibraryListKind | null> = ['likes', 'watch_later', 'favorites', null];
  const TAB_GAP = 22;
  const TAB_INACTIVE_SCALE = 0.5;

  let activeTab = $state(0);
  let hasSwitchedTab = $state(false);
  let transitionKind = $state<'tab' | 'view'>('tab');
  let tabDirection = $state<1 | -1>(1);
  let tabsReady = $state(false);
  let tabWidths = $state<Array<{ active: number; inactive: number }>>([]);
  let tabBaselineY = $state(38);
  let regularRefs: Array<HTMLSpanElement | undefined> = [];
  let blackRefs: Array<HTMLSpanElement | undefined> = [];
  let probeEl: HTMLSpanElement | undefined;
  let probeMarkEl: HTMLSpanElement | undefined;

  let viewMode = $state<'list' | 'grid'>('list');
  let hasToggledView = $state(false);
  let loadingId = $state<string | null>(null);

  let tabLayout = $derived.by(() =>
  {
    if (!tabsReady || tabWidths.length !== tabs.length) {
      return tabs.map((_, i) => ({ x: 0, scale: i === activeTab ? 1 : TAB_INACTIVE_SCALE }));
    }
    let cursor = 0;
    return tabs.map((_, i) =>
    {
      const isActive = i === activeTab;
      const scale = isActive ? 1 : TAB_INACTIVE_SCALE;
      const visualWidth = isActive
        ? tabWidths[i].active
        : tabWidths[i].inactive * TAB_INACTIVE_SCALE;
      const x = cursor;
      cursor += visualWidth + TAB_GAP;
      return { x, scale };
    });
  });

  function measureTabs()
  {
    if (regularRefs.length !== tabs.length || blackRefs.length !== tabs.length) return;
    const widths: Array<{ active: number; inactive: number }> = [];
    for (let i = 0; i < tabs.length; i++) {
      const reg = regularRefs[i];
      const blk = blackRefs[i];
      if (!reg || !blk) return;
      widths.push({
        active: blk.offsetWidth,
        inactive: reg.offsetWidth
      });
    }
    if (probeEl && probeMarkEl) {
      const baseline =
        probeMarkEl.getBoundingClientRect().bottom - probeEl.getBoundingClientRect().top;
      if (baseline > 0) {
        tabBaselineY = baseline;
      }
    }
    tabWidths = widths;
    tabsReady = true;
  }

  onMount(() =>
  {
    void enrichLibraryArtwork();
  });

  $effect(() =>
  {
    measureTabs();
    const fonts = document.fonts;
    const onFonts = () => measureTabs();
    fonts?.ready.then(onFonts);
    fonts?.addEventListener('loadingdone', onFonts);
    return () =>
    {
      fonts?.removeEventListener('loadingdone', onFonts);
    };
  });

  const historyItems = $derived.by((): DisplayLibraryItem[] =>
  {
    const fallbackPoster = localLibrary.items[0]?.poster || '';
    return watchHistory.entries.map((entry) =>
    {
      const epTag = entry.totalEpisodes
        ? `Ep ${entry.episodeOrdinal}/${entry.totalEpisodes}`
        : `Ep ${entry.episodeOrdinal}`;
      const progressTag =
        entry.currentTime >= 5 ? `${epTag} · ${formatTimeCompact(entry.currentTime)}` : epTag;
      const baseTags = entry.tags.filter((t) => !/^\d+\s*ep$/i.test(t));
      const progressPct =
        entry.duration > 0
          ? Math.min(100, Math.max(0, (entry.currentTime / entry.duration) * 100))
          : 0;

      return {
        id: entry.id,
        shikiId: entry.shikiId ?? 0,
        releaseAlias: entry.releaseAlias,
        searchQueries: entry.searchQueries,
        title: entry.title,
        description: entry.description || 'Stopped at your last watched episode.',
        poster: entry.poster || fallbackPoster,
        posterPosition: entry.posterPosition,
        score: entry.score,
        tags: [progressTag, ...baseTags].slice(0, 5),
        episodeOrdinal: entry.episodeOrdinal,
        episodeIndex: entry.episodeIndex,
        resumeTime: entry.currentTime,
        progressPct
      };
    });
  });

  const isHistoryTab = $derived(activeTab === 3);
  const displayedItems = $derived.by((): DisplayLibraryItem[] =>
  {
    if (isHistoryTab) return historyItems;
    const listKind = listByTab[activeTab];
    if (!listKind) return localLibrary.items;
    return localLibrary.items.filter((item) => item.lists.includes(listKind));
  });
  const ambientPoster = $derived(displayedItems[0]?.poster || localLibrary.items[0]?.poster || '');

  function selectTab(index: number)
  {
    if (activeTab === index) return;
    tabDirection = index > activeTab ? 1 : -1;
    transitionKind = 'tab';
    hasSwitchedTab = true;
    activeTab = index;
  }

  function setViewMode(mode: 'list' | 'grid')
  {
    if (viewMode === mode) return;
    transitionKind = 'view';
    hasToggledView = true;
    viewMode = mode;
  }

  function contentEnter(_node: Element)
  {
    if (!hasSwitchedTab && !hasToggledView) {
      return { duration: 0 };
    }
    const isTab = transitionKind === 'tab';
    const xOffset = isTab ? tabDirection * 20 : 0;
    const yOffset = isTab ? 0 : 10;
    return {
      delay: 75,
      duration: 240,
      easing: cubicOut,
      css: (t: number) =>
        `opacity: ${t}; transform: translate3d(${(1 - t) * xOffset}px, ${(1 - t) * yOffset}px, 0);`
    };
  }

  function contentLeave(_node: Element)
  {
    if (!hasSwitchedTab && !hasToggledView) {
      return { duration: 0 };
    }
    const isTab = transitionKind === 'tab';
    const xOffset = isTab ? -tabDirection * 14 : 0;
    const yOffset = isTab ? 0 : -8;
    return {
      duration: 130,
      easing: cubicIn,
      css: (t: number) =>
        `opacity: ${t}; transform: translate3d(${(1 - t) * xOffset}px, ${(1 - t) * yOffset}px, 0);`
    };
  }

  async function handleWatch(aId: string)
  {
    const item = displayedItems.find((x) => x.id === aId);
    if (!item)
    {
      return;
    }
    loadingId = aId;
    try
    {
      await openWatch(
        {
          title: item.title,
          shikiId: item.shikiId > 0 ? item.shikiId : undefined,
          releaseAlias: item.releaseAlias,
          searchQueries: item.searchQueries
        },
        item.episodeOrdinal != null
          ? {
              episodeOrdinal: item.episodeOrdinal,
              episodeIndex: item.episodeIndex,
              resumeTime: item.resumeTime
            }
          : undefined
      );
    }
    finally
    {
      loadingId = null;
    }
  }
</script>

<div class="library-page">
  <!-- Ambient glow from the first poster, fading out downwards -->
  <div class="library-ambient" aria-hidden="true">
    {#if ambientPoster}
      <LayerBlur src={ambientPoster} blur={60} opacity={0.18} scale={1.2} />
    {/if}
  </div>

  <div class="library-head">
    <div
      class="library-tabs"
      class:is-ready={tabsReady}
      class:is-animated={hasSwitchedTab}
    >
      <span class="tab-baseline-probe" aria-hidden="true" bind:this={probeEl}>
        Ag<span class="probe-mark" bind:this={probeMarkEl}></span>
      </span>
      {#each tabs as tab, index}
        <button
          type="button"
          class="library-tab"
          class:active={activeTab === index}
          style:transform={`translate3d(${tabLayout[index].x}px, 0, 0) scale(${tabLayout[index].scale})`}
          style:transform-origin={`0px ${tabBaselineY}px`}
          onclick={() => selectTab(index)}
        >
          <span class="tab-label tab-regular" bind:this={regularRefs[index]}>{tab}</span>
          <span class="tab-label tab-black" aria-hidden="true" bind:this={blackRefs[index]}>{tab}</span>
        </button>
      {/each}
    </div>

    <div class="library-controls">
      {#if isHistoryTab && displayedItems.length > 0}
        <button type="button" class="clear-history-btn" onclick={clearWatchHistory}>
          Clear history
        </button>
      {/if}

      <!-- View mode switch -->
      <div class="library-view-modes" class:is-grid={viewMode === 'grid'}>
        <span class="view-pill" aria-hidden="true"></span>
        <button
          type="button"
          class="view-btn"
          class:active={viewMode === 'list'}
          aria-label="List view"
          aria-pressed={viewMode === 'list'}
          onclick={() => setViewMode('list')}
        >
          <LayoutList size={18} strokeWidth={1.7} />
        </button>
        <button
          type="button"
          class="view-btn"
          class:active={viewMode === 'grid'}
          aria-label="Grid view"
          aria-pressed={viewMode === 'grid'}
          onclick={() => setViewMode('grid')}
        >
          <LayoutGrid size={18} strokeWidth={1.7} />
        </button>
      </div>
    </div>
  </div>

  <div class="library-views">
    {#key `${activeTab}:${viewMode}`}
      <div class="library-stage" in:contentEnter out:contentLeave>
        {#if displayedItems.length === 0}
          <div class="library-empty">
            <p class="empty-title">{isHistoryTab ? 'No watch history yet' : 'Nothing saved here yet'}</p>
          </div>
        {:else if viewMode === 'list'}
          <ul class="library-list">
        {#each displayedItems as item, index (item.id)}
          <li class="library-row">
            <img
              src={item.poster}
              alt=""
              class="row-bg"
              style:object-position={item.posterPosition}
              loading={index > 1 ? 'lazy' : undefined}
              decoding="async"
            />
            <div class="row-dim"></div>

            <div class="row-poster-wrap">
              <img
                src={item.poster}
                alt={item.title}
                class="row-poster"
                style:object-position={item.posterPosition}
                loading={index > 1 ? 'lazy' : undefined}
                decoding="async"
              />
              {#if item.progressPct && item.progressPct > 0}
                <div class="progress-track" aria-hidden="true">
                  <div class="progress-fill" style:width={`${item.progressPct}%`}></div>
                </div>
              {/if}
            </div>

            <div class="row-info">
              <h3 class="row-title">{item.title}</h3>
              <div class="row-meta">
                <MetaLine score={item.score} tags={item.tags} />
              </div>
              <p class="row-description">{item.description}</p>
            </div>

            <div class="row-action">
              {#if isHistoryTab}
                <button
                  type="button"
                  class="remove-entry-btn"
                  aria-label={`Remove ${item.title} from history`}
                  onclick={() => removeHistoryEntry(item.id)}
                >
                  <X size={15} strokeWidth={2} />
                </button>
              {/if}
              <MainButton loading={loadingId === item.id} onclick={() => handleWatch(item.id)}>
                {#snippet icon()}
                  <Play size={16} fill="currentColor" strokeWidth={0} />
                {/snippet}
                {isHistoryTab ? 'Continue' : 'Watch'}
              </MainButton>
            </div>
          </li>
        {/each}
      </ul>
    {:else}
      <ul class="library-grid">
        {#each displayedItems as item, index (item.id)}
          <li class="library-tile">
            <img
              src={item.poster}
              alt={item.title}
              class="tile-poster"
              style:object-position={item.posterPosition}
              loading={index > 3 ? 'lazy' : undefined}
              decoding="async"
            />
            <div class="tile-overlay"></div>
            <div class="tile-hover-dim"></div>

            {#if isHistoryTab}
              <button
                type="button"
                class="remove-entry-btn tile-remove"
                aria-label={`Remove ${item.title} from history`}
                onclick={() => removeHistoryEntry(item.id)}
              >
                <X size={14} strokeWidth={2} />
              </button>
            {/if}

            <p class="tile-description">{item.description}</p>

            <div class="tile-footer">
              <h3 class="tile-title">{item.title}</h3>
              <MetaLine score={item.score} tags={item.tags.slice(0, 3)} size="sm" />
              {#if item.progressPct && item.progressPct > 0}
                <div class="progress-track tile-progress" aria-hidden="true">
                  <div class="progress-fill" style:width={`${item.progressPct}%`}></div>
                </div>
              {/if}
              <div class="tile-action">
                <MainButton loading={loadingId === item.id} onclick={() => handleWatch(item.id)}>
                  {#snippet icon()}
                    <Play size={16} fill="currentColor" strokeWidth={0} />
                  {/snippet}
                  {isHistoryTab ? 'Continue' : 'Watch'}
                </MainButton>
              </div>
            </div>
          </li>
        {/each}
      </ul>
        {/if}
      </div>
    {/key}
  </div>
</div>

<style>
  .library-page {
    position: relative;
    width: 100%;
    min-height: 100vh;
    padding: 72px 0 0 0;
    box-sizing: border-box;
    background: #0c0c0c;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    contain: layout style;
    -webkit-font-smoothing: antialiased;
  }

  .library-ambient {
    position: absolute;
    top: 0;
    left: 0;
    width: 100%;
    height: 700px;
    pointer-events: none;
    z-index: 0;
    contain: strict;
    transform: translate3d(0, 0, 0);
    -webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 1) 0%, rgba(0, 0, 0, 0) 100%);
    mask-image: linear-gradient(180deg, rgba(0, 0, 0, 1) 0%, rgba(0, 0, 0, 0) 100%);
  }

  .library-head {
    position: relative;
    z-index: 3;
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    height: 48px;
    min-height: 48px;
    max-height: 48px;
    flex-shrink: 0;
    contain: layout style;
    isolation: isolate;
    padding: 0 var(--page-pad-x);
    box-sizing: content-box;
    margin: 30px 0 32px 0;
  }

  .library-tabs {
    position: relative;
    height: 48px;
    flex: 1 1 auto;
    min-width: 0;
    opacity: 0;
  }

  .library-tabs.is-ready {
    opacity: 1;
  }

  .tab-baseline-probe {
    position: absolute;
    left: 0;
    bottom: 0;
    font-family: 'PP Monument Extended', sans-serif;
    font-size: 48px;
    line-height: 1;
    white-space: nowrap;
    visibility: hidden;
    pointer-events: none;
  }

  .probe-mark {
    display: inline-block;
    width: 0;
    height: 0;
    vertical-align: baseline;
  }

  /* Two stacked labels (Regular + Black) cross-fade while GPU transform glides
     at a fixed 48px font size, eliminating reflow jitter and glyph hinting snaps. */
  .library-tab {
    position: absolute;
    left: 0;
    bottom: 0;
    display: grid;
    margin: 0;
    padding: 0;
    background: transparent;
    border: none;
    cursor: pointer;
    font-family: 'PP Monument Extended', sans-serif;
    font-size: 48px;
    line-height: 1;
    white-space: nowrap;
    text-rendering: geometricPrecision;
    -webkit-font-smoothing: antialiased;
    will-change: transform;
    backface-visibility: hidden;
  }

  .library-tabs.is-animated .library-tab {
    transition: transform 0.45s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .tab-label {
    grid-area: 1 / 1;
    justify-self: start;
    text-rendering: geometricPrecision;
    -webkit-font-smoothing: antialiased;
    transition:
      opacity 0.35s ease,
      color 0.35s ease;
  }

  .tab-regular {
    font-weight: 400;
    color: #5f5f5f;
    opacity: 1;
  }

  .tab-black {
    font-weight: 900;
    color: #ffffff;
    text-shadow: 0 4px 4px rgba(0, 0, 0, 0.25);
    opacity: 0;
    pointer-events: none;
  }

  .library-tab:hover .tab-regular {
    color: #8a8a8a;
  }

  .library-tab.active .tab-regular {
    opacity: 0;
  }

  .library-tab.active .tab-black {
    opacity: 1;
  }

  .library-controls {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-shrink: 0;
  }

  .clear-history-btn {
    height: 36px;
    padding: 0 13px;
    margin-bottom: 2px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    border-radius: 10px;
    background: rgba(255, 255, 255, 0.04);
    color: rgba(255, 255, 255, 0.56);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition:
      color 0.2s ease,
      background-color 0.2s ease,
      border-color 0.2s ease,
      transform 0.1s ease;
  }

  .clear-history-btn:hover {
    color: #ffffff;
    background: rgba(255, 255, 255, 0.09);
    border-color: rgba(255, 255, 255, 0.16);
  }

  .clear-history-btn:active {
    transform: scale(0.96);
  }

  .library-view-modes {
    position: relative;
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 3px;
    border-radius: 11px;
    background: rgba(255, 255, 255, 0.04);
    margin-bottom: 2px;
  }

  .view-pill {
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

  .library-view-modes.is-grid .view-pill {
    transform: translate3d(34px, 0, 0);
  }

  .view-btn {
    position: relative;
    z-index: 1;
    width: 30px;
    height: 30px;
    margin: 0;
    padding: 0;
    background: transparent;
    border: none;
    border-radius: 8px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    color: rgba(255, 255, 255, 0.35);
    transition: color 0.2s ease, transform 0.1s ease;
  }

  .view-btn:hover {
    color: rgba(255, 255, 255, 0.72);
  }

  .view-btn:active {
    transform: scale(0.92);
  }

  .view-btn.active {
    color: #ffffff;
  }

  .library-views {
    position: relative;
    z-index: 2;
    display: grid;
    width: 100%;
    min-width: 0;
    overflow-x: hidden;
    contain: layout style;
    isolation: isolate;
    transform: translate3d(0, 0, 0);
  }

  .library-stage {
    grid-area: 1 / 1;
    width: 100%;
    min-width: 0;
    will-change: opacity, transform;
  }

  .library-empty {
    min-height: calc(100vh - 230px);
    padding: 24px var(--page-pad-x) 48px var(--page-pad-x);
    display: flex;
    align-items: center;
    justify-content: center;
    text-align: center;
  }

  .empty-title {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 22px;
    font-weight: 700;
    color: rgba(255, 255, 255, 0.56);
    user-select: none;
  }

  .library-list {
    grid-area: 1 / 1;
    position: relative;
    list-style: none;
    margin: 0 var(--page-pad-x) 48px var(--page-pad-x);
    padding: 0;
    display: flex;
    flex-direction: column;
    border-radius: var(--radius-card);
    overflow: hidden;
  }

  .library-row {
    position: relative;
    height: 267px;
    overflow: hidden;
    background: #0c0c0c;
    isolation: isolate;
    display: flex;
    align-items: flex-start;
    gap: 24px;
    padding: 26px;
    box-sizing: border-box;
    flex-shrink: 0;
    contain: layout paint;
  }

  /* Blurred poster as the row background (1/4-scale for low GPU cost while scrolling) */
  .row-bg {
    position: absolute;
    top: 50%;
    left: 50%;
    width: 25%;
    height: 25%;
    object-fit: cover;
    filter: blur(6px);
    transform: translate3d(-50%, -50%, 0) scale(4.6);
    transform-origin: center center;
    z-index: 0;
    pointer-events: none;
    user-select: none;
  }

  .row-dim {
    position: absolute;
    inset: -2px;
    z-index: 1;
    background: rgba(4, 4, 4, 0.78);
    pointer-events: none;
    transition: background-color var(--hover-dur) var(--ease-out);
  }

  .library-row:hover .row-dim {
    background: rgba(4, 4, 4, 0.62);
  }

  .row-poster-wrap {
    position: relative;
    z-index: 2;
    flex: 0 0 245px;
    width: 245px;
    height: 215px;
    border-radius: var(--radius-card);
    overflow: hidden;
  }

  .row-poster {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: cover;
    user-select: none;
    pointer-events: none;
    backface-visibility: hidden;
    transition: transform var(--hover-dur) var(--ease-out);
  }

  .library-row:hover .row-poster {
    transform: scale(1.02);
  }

  .progress-track {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 3px;
    background: rgba(255, 255, 255, 0.16);
    overflow: hidden;
    pointer-events: none;
  }

  .progress-fill {
    height: 100%;
    background: #ffffff;
    border-radius: 999px;
  }

  .progress-track.tile-progress {
    position: relative;
    border-radius: 999px;
    margin-top: -2px;
  }

  .row-info {
    position: relative;
    z-index: 2;
    flex: 1 1 auto;
    min-width: 0;
    max-width: 741px;
  }

  .row-title {
    margin: 0;
    padding: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 36px;
    font-weight: 700;
    line-height: 1.1;
    color: #ffffff;
  }

  .row-meta {
    margin-top: 8px;
  }

  .row-description {
    margin: 10px 0 0 0;
    padding: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 20px;
    font-weight: 400;
    line-height: 1.3;
    color: var(--text-muted);
    display: -webkit-box;
    line-clamp: 4;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .row-action {
    position: absolute;
    z-index: 2;
    top: 26px;
    right: 26px;
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .remove-entry-btn {
    width: 34px;
    height: 34px;
    padding: 0;
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 999px;
    background: rgba(12, 12, 12, 0.65);
    color: rgba(255, 255, 255, 0.64);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    opacity: 0;
    transition:
      opacity 0.2s ease,
      color 0.2s ease,
      background-color 0.2s ease,
      transform 0.1s ease;
  }

  .library-row:hover .remove-entry-btn,
  .library-tile:hover .remove-entry-btn,
  .remove-entry-btn:focus-visible {
    opacity: 1;
  }

  .remove-entry-btn:hover {
    color: #ffffff;
    background: rgba(255, 255, 255, 0.14);
  }

  .remove-entry-btn:active {
    transform: scale(0.92);
  }

  .remove-entry-btn.tile-remove {
    position: absolute;
    top: 14px;
    right: 14px;
    z-index: 4;
    width: 30px;
    height: 30px;
  }

  .library-grid {
    grid-area: 1 / 1;
    position: relative;
    list-style: none;
    margin: 0 var(--page-pad-x) 48px var(--page-pad-x);
    padding: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 18px;
  }

  .library-tile {
    position: relative;
    aspect-ratio: 1 / 1;
    border-radius: var(--radius-card);
    overflow: hidden;
    background: #0c0c0c;
    contain: layout paint;
    isolation: isolate;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
  }

  .library-tile::after {
    content: '';
    position: absolute;
    inset: -3px;
    border-radius: calc(var(--radius-card) + 3px);
    border: 4px solid #0c0c0c;
    transform: translateZ(0);
    pointer-events: none;
    z-index: 5;
  }

  .tile-poster {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    backface-visibility: hidden;
    user-select: none;
    pointer-events: none;
    will-change: transform;
    transition: transform 0.4s var(--ease-out);
  }

  .library-tile:hover .tile-poster {
    transform: scale(1.04);
  }

  .tile-overlay {
    position: absolute;
    inset: -2px;
    background: linear-gradient(
      180deg,
      rgba(7, 7, 7, 0.06) 0%,
      rgba(7, 7, 7, 0.36) 45%,
      rgba(7, 7, 7, 0.92) 100%
    );
    pointer-events: none;
    transition: opacity var(--hover-dur) var(--ease-out);
  }

  .library-tile:hover .tile-overlay {
    opacity: 0.9;
  }

  .tile-hover-dim {
    position: absolute;
    inset: -2px;
    background: linear-gradient(
      180deg,
      rgba(7, 7, 7, 0.82) 0%,
      rgba(7, 7, 7, 0.52) 56%,
      rgba(7, 7, 7, 0.1) 88%
    );
    opacity: 0;
    pointer-events: none;
    transition: opacity var(--hover-dur) var(--ease-out);
  }

  .library-tile:hover .tile-hover-dim,
  .library-tile:focus-within .tile-hover-dim {
    opacity: 1;
  }

  .tile-description {
    position: relative;
    z-index: 2;
    flex: 1 1 auto;
    min-height: 0;
    margin: 18px 18px 4px 18px;
    padding: 0 2px 22px 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 15px;
    font-weight: 400;
    line-height: 1.4;
    color: rgba(255, 255, 255, 0.72);
    overflow-y: auto;
    overflow-x: hidden;
    overscroll-behavior: contain;
    scrollbar-width: none;
    -ms-overflow-style: none;
    -webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 1) 0%, rgba(0, 0, 0, 1) 66%, rgba(0, 0, 0, 0) 98%);
    mask-image: linear-gradient(180deg, rgba(0, 0, 0, 1) 0%, rgba(0, 0, 0, 1) 66%, rgba(0, 0, 0, 0) 98%);
    opacity: 0;
    transform: translate3d(0, -6px, 0);
    pointer-events: none;
    transition:
      opacity 0.28s var(--ease-out),
      transform 0.28s var(--ease-out);
  }

  .tile-description::-webkit-scrollbar {
    display: none;
  }

  .library-tile:hover .tile-description,
  .library-tile:focus-within .tile-description {
    opacity: 1;
    pointer-events: auto;
    transform: translate3d(0, 0, 0);
  }

  .tile-footer {
    position: relative;
    z-index: 2;
    flex-shrink: 0;
    padding: 6px 18px 18px 18px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .tile-title {
    margin: 0;
    padding: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 19px;
    font-weight: 700;
    line-height: 1.15;
    color: #ffffff;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .tile-action {
    display: flex;
    align-items: center;
  }
</style>
