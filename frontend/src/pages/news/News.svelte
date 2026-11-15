<script lang="ts">
  import { onMount } from 'svelte';
  import { fade, fly, scale } from 'svelte/transition';
  import { cubicIn, cubicOut } from 'svelte/easing';
  import Play from '@lucide/svelte/icons/play';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import BookOpen from '@lucide/svelte/icons/book-open';
  import MessageSquare from '@lucide/svelte/icons/message-square';
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import X from '@lucide/svelte/icons/x';

  import MainButton from '../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../marlakit/button/SecondButton.svelte';
  import MetaLine from '../../marlakit/meta/MetaLine.svelte';
  import { portal } from '../../marlakit/portal';
  import { getCachedNewsFeed, loadNewsFeed, type NewsItem } from '../../api/news';
  import { openWatch } from '../../app/player.svelte';

  const tabs = ['All', 'News', 'Episodes'] as const;
  const TAB_GAP = 22;
  const TAB_INACTIVE_SCALE = 0.5;

  let activeTab = $state(0);
  let hasSwitchedTab = $state(false);
  let tabsReady = $state(false);
  let tabWidths = $state<Array<{ active: number; inactive: number }>>([]);
  let tabBaselineY = $state(38);
  let regularRefs: Array<HTMLSpanElement | undefined> = [];
  let blackRefs: Array<HTMLSpanElement | undefined> = [];
  let probeEl: HTMLSpanElement | undefined;
  let probeMarkEl: HTMLSpanElement | undefined;

  const initialCache = getCachedNewsFeed();
  let items = $state<NewsItem[]>(initialCache?.items ?? []);
  let loading = $state(!initialCache);
  let refreshing = $state(false);
  let error = $state<string | null>(null);
  let activeStory = $state<NewsItem | null>(null);
  let watchingId = $state<string | null>(null);
  let viewportWidth = $state(typeof window !== 'undefined' ? window.innerWidth : 1280);

  const tabLayout = $derived.by(() =>
  {
    if (!tabsReady || tabWidths.length !== tabs.length)
    {
      return tabs.map((_, i) => ({ x: 0, scale: i === activeTab ? 1 : TAB_INACTIVE_SCALE }));
    }
    let cursor = 0;
    return tabs.map((_, i) =>
    {
      const isActive = i === activeTab;
      const itemScale = isActive ? 1 : TAB_INACTIVE_SCALE;
      const visualWidth = isActive
        ? tabWidths[i].active
        : tabWidths[i].inactive * TAB_INACTIVE_SCALE;
      const x = cursor;
      cursor += visualWidth + TAB_GAP;
      return { x, scale: itemScale };
    });
  });

  const filteredItems = $derived.by(() =>
  {
    if (activeTab === 1) return items.filter((item) => item.kind === 'news');
    if (activeTab === 2) return items.filter((item) => item.kind === 'release');
    return items;
  });

  const leadItem = $derived.by(() =>
  {
    if (filteredItems.length === 0) return null;
    return (
      filteredItems.find((item) => item.isLandscape && Boolean(item.image)) ??
      filteredItems.find((item) => Boolean(item.image)) ??
      filteredItems[0]
    );
  });

  const feedItems = $derived.by(() =>
  {
    if (!leadItem) return [];
    return filteredItems.filter((item) => item.id !== leadItem.id);
  });

  const colCount = $derived(viewportWidth <= 1120 ? 2 : 3);

  const masonryColumns = $derived.by(() =>
  {
    const cols: Array<Array<{ item: NewsItem; index: number }>> = Array.from(
      { length: colCount },
      () => []
    );
    const heights = new Array<number>(colCount).fill(0);
    for (let index = 0; index < feedItems.length; index++)
    {
      const item = feedItems[index];
      let targetCol = 0;
      for (let c = 1; c < colCount; c++)
      {
        if (heights[c] < heights[targetCol])
        {
          targetCol = c;
        }
      }
      cols[targetCol].push({ item, index });
      const titleLines = item.title.length > 36 ? 2 : 1;
      const estimatedHeight = (item.image ? 190 : 0) + 155 + titleLines * 22;
      heights[targetCol] += estimatedHeight;
    }
    return cols;
  });

  function measureTabs()
  {
    if (regularRefs.length !== tabs.length || blackRefs.length !== tabs.length) return;
    const widths: Array<{ active: number; inactive: number }> = [];
    for (let i = 0; i < tabs.length; i++)
    {
      const reg = regularRefs[i];
      const blk = blackRefs[i];
      if (!reg || !blk) return;
      widths.push({
        active: blk.offsetWidth,
        inactive: reg.offsetWidth
      });
    }
    if (probeEl && probeMarkEl)
    {
      const baseline =
        probeMarkEl.getBoundingClientRect().bottom - probeEl.getBoundingClientRect().top;
      if (baseline > 0)
      {
        tabBaselineY = baseline;
      }
    }
    tabWidths = widths;
    tabsReady = true;
  }

  $effect(() =>
  {
    viewportWidth;
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

  async function fetchFeed(force = false)
  {
    if (force) refreshing = true;
    else if (items.length === 0) loading = true;
    error = null;
    try
    {
      const feed = await loadNewsFeed(force);
      items = feed.items;
      if (items.length === 0)
      {
        error = 'No news or fresh releases returned right now.';
      }
    }
    catch (err)
    {
      if (items.length === 0)
      {
        error = err instanceof Error ? err.message : 'Failed to load news feed';
      }
    }
    finally
    {
      loading = false;
      refreshing = false;
    }
  }

  onMount(() =>
  {
    void fetchFeed(false);
  });

  function selectTab(index: number)
  {
    if (activeTab === index) return;
    hasSwitchedTab = true;
    activeTab = index;
  }

  function stageEnter(_node: Element)
  {
    if (!hasSwitchedTab)
    {
      return { duration: 0 };
    }
    return {
      delay: 70,
      duration: 240,
      easing: cubicOut,
      css: (t: number) =>
        `opacity: ${t}; transform: translate3d(0, ${(1 - t) * 10}px, 0);`
    };
  }

  function openStory(item: NewsItem)
  {
    activeStory = item;
  }

  function closeStory()
  {
    activeStory = null;
  }

  async function watchItem(item: NewsItem, e?: MouseEvent)
  {
    e?.stopPropagation();
    watchingId = item.id;
    activeStory = null;
    try
    {
      await openWatch({
        title: item.title,
        shikiId: item.shikiId,
        releaseAlias: item.releaseAlias,
        searchQueries: [item.title]
      });
    }
    finally
    {
      watchingId = null;
    }
  }

  function handleWindowKeydown(e: KeyboardEvent)
  {
    if (e.key === 'Escape' && activeStory)
    {
      closeStory();
    }
  }
</script>

<svelte:window bind:innerWidth={viewportWidth} onkeydown={handleWindowKeydown} />

<div class="news-page">
  <!-- Full-bleed Home-style atmospheric hero banner -->
  <section class="news-hero-wrap">
    {#if leadItem?.image}
      {#key leadItem.image}
        <img
          src={leadItem.image}
          alt=""
          aria-hidden="true"
          class="hero-banner-img"
          class:is-portrait={!leadItem.isLandscape}
          transition:fade={{ duration: hasSwitchedTab ? 280 : 0, easing: cubicOut }}
          decoding="async"
        />
      {/key}
    {/if}
    <div class="hero-vignette" aria-hidden="true"></div>

    <div class="news-head">
      <div
        class="news-tabs"
        class:is-ready={tabsReady}
        class:is-animated={hasSwitchedTab}
        role="tablist"
        aria-label="News sections"
      >
        <span class="tab-baseline-probe" aria-hidden="true" bind:this={probeEl}>
          A<span class="probe-mark" bind:this={probeMarkEl}></span>
        </span>

        {#each tabs as tab, index (tab)}
          {@const layout = tabLayout[index] ?? { x: 0, scale: index === activeTab ? 1 : TAB_INACTIVE_SCALE }}
          <button
            type="button"
            class="news-tab"
            class:active={activeTab === index}
            style="transform-origin: 0 {tabBaselineY}px; transform: translate3d({layout.x}px, 0, 0) scale({layout.scale});"
            role="tab"
            aria-selected={activeTab === index}
            onclick={() => selectTab(index)}
          >
            <span class="tab-label tab-regular" bind:this={regularRefs[index]}>{tab}</span>
            <span class="tab-label tab-black" aria-hidden="true" bind:this={blackRefs[index]}>{tab}</span>
          </button>
        {/each}
      </div>

      <button
        type="button"
        class="sync-btn"
        class:is-spinning={refreshing}
        aria-label="Refresh feed"
        title="Refresh news and latest episodes"
        onclick={() => fetchFeed(true)}
      >
        <RefreshCw size={15} strokeWidth={2} class="sync-icon" />
        <span>Sync</span>
      </button>
    </div>

    {#if loading}
      <div class="hero-skeleton"></div>
    {:else if leadItem}
      <div class="news-hero-stages">
        {#key activeTab}
          <div
            class="hero-stage"
            in:stageEnter
            out:fly={{ y: -8, duration: 150, easing: cubicIn }}
          >
            <h1 class="hero-title">{leadItem.title}</h1>

            <div class="meta-strip">
              <MetaLine
                score={leadItem.score}
                tags={[
                  ...(leadItem.relativeTime ? [leadItem.relativeTime] : []),
                  ...leadItem.hashtags.slice(0, 4)
                ]}
              />
            </div>

            <p class="hero-excerpt">{leadItem.excerpt}</p>

        <div class="hero-actions">
          {#if leadItem.releaseAlias}
            <MainButton loading={watchingId === leadItem.id} onclick={(e) => watchItem(leadItem, e)}>
              {#snippet icon()}
                <Play size={16} fill="currentColor" strokeWidth={0} />
              {/snippet}
              Watch Ep {leadItem.episodeOrdinal ?? 1}
            </MainButton>
            <SecondButton onclick={() => openStory(leadItem)}>
              {#snippet icon()}
                <BookOpen size={16} strokeWidth={2} />
              {/snippet}
              Details
            </SecondButton>
          {:else}
            <MainButton onclick={() => openStory(leadItem)}>
              {#snippet icon()}
                <BookOpen size={16} strokeWidth={2} />
              {/snippet}
              Read Story
            </MainButton>
            {#if leadItem.shikiId}
              <SecondButton loading={watchingId === leadItem.id} onclick={(e) => watchItem(leadItem, e)}>
                {#snippet icon()}
                  <Play size={16} fill="currentColor" strokeWidth={0} />
                {/snippet}
                Watch
              </SecondButton>
            {/if}
          {/if}
        </div>
      </div>
        {/key}
      </div>
    {/if}
  </section>

  <div class="news-body">
    {#if loading}
      <div class="news-grid">
        {#each [0, 1, 2, 3, 4, 5] as i (i)}
          <div class="card-skeleton"></div>
        {/each}
      </div>
    {:else if error && items.length === 0}
      <div class="news-empty">
        <p class="empty-title">Could not load anime feed</p>
        <p class="empty-sub">{error}</p>
        <MainButton onclick={() => fetchFeed(true)}>Retry</MainButton>
      </div>
    {:else}
      <div class="news-views">
        {#key activeTab}
          <div
            class="news-masonry"
            in:stageEnter
            out:fly={{ y: -8, duration: 120, easing: cubicIn }}
          >
            {#each masonryColumns as col, colIdx (colIdx)}
              <div class="news-col">
                {#each col as entry (entry.item.id)}
                  {@const item = entry.item}
                  <!-- svelte-ignore a11y_click_events_have_key_events -->
                  <!-- svelte-ignore a11y_no_static_element_interactions -->
                  <div
                    class="story-card"
                    class:is-compact={!item.image}
                    onclick={() => openStory(item)}
                  >
                    {#if item.image}
                      <div class="story-media">
                        <img
                          src={item.image}
                          alt={item.title}
                          class="media-img"
                          class:is-portrait={!item.isLandscape}
                          loading="lazy"
                          decoding="async"
                        />
                        <div class="media-fade"></div>
                      </div>
                    {/if}

                    <div class="story-info">
                      <MetaLine score={item.score} tags={item.hashtags.slice(0, 3)} size="sm" />

                      <h3 class="story-title">{item.title}</h3>
                      <p class="story-excerpt">{item.excerpt}</p>

                      <div class="story-footer">
                        <div class="footer-meta">
                          {#if item.relativeTime}
                            <span>{item.relativeTime}</span>
                          {/if}
                          {#if item.commentsCount && item.commentsCount > 0}
                            <span class="meta-comments">
                              <MessageSquare size={12} strokeWidth={2} />
                              {item.commentsCount}
                            </span>
                          {/if}
                        </div>

                        {#if item.releaseAlias}
                          <MainButton loading={watchingId === item.id} onclick={(e) => watchItem(item, e)}>
                            {#snippet icon()}
                              <Play size={15} fill="currentColor" strokeWidth={0} />
                            {/snippet}
                            Watch
                          </MainButton>
                        {:else}
                          <SecondButton onclick={(e) => { e.stopPropagation(); openStory(item); }}>
                            {#snippet icon()}
                              <BookOpen size={15} strokeWidth={2} />
                            {/snippet}
                            Read
                          </SecondButton>
                        {/if}
                      </div>
                    </div>
                  </div>
                {/each}
              </div>
            {/each}
          </div>
        {/key}
      </div>
    {/if}
  </div>
</div>

{#if activeStory}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    use:portal
    class="reader-backdrop"
    transition:fade={{ duration: 200, easing: cubicOut }}
    onclick={closeStory}
  >
    <div
      class="reader-modal"
      transition:scale={{ start: 0.96, duration: 230, easing: cubicOut }}
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-modal="true"
      aria-label={activeStory.title}
      tabindex="-1"
    >
      {#if activeStory.image}
        <div class="reader-hero">
          <img
            src={activeStory.image}
            alt={activeStory.title}
            class="reader-hero-img"
            class:is-portrait={!activeStory.isLandscape}
          />
          <div class="reader-hero-fade"></div>
        </div>
      {/if}

      <button
        type="button"
        class="reader-close"
        aria-label="Close article"
        onclick={closeStory}
      >
        <X size={18} strokeWidth={2} />
      </button>

      <div class="reader-content">
        <MetaLine
          score={activeStory.score}
          tags={[
            ...(activeStory.relativeTime ? [activeStory.relativeTime] : []),
            ...activeStory.hashtags
          ]}
        />

        <h2 class="reader-title">{activeStory.title}</h2>

        <div class="reader-paragraphs">
          {#each activeStory.paragraphs as para, idx (idx)}
            <p class="reader-para">{para}</p>
          {/each}
        </div>

        {#if activeStory.quote}
          <blockquote class="reader-quote">{activeStory.quote}</blockquote>
        {/if}

        <div class="reader-actions">
          {#if activeStory.releaseAlias || activeStory.shikiId}
            <MainButton loading={watchingId === activeStory.id} onclick={() => watchItem(activeStory!)}>
              {#snippet icon()}
                <Play size={16} fill="currentColor" strokeWidth={0} />
              {/snippet}
              Watch on Mila
            </MainButton>
          {/if}

          {#if activeStory.externalUrl}
            <SecondButton
              onclick={() => window.open(activeStory!.externalUrl, '_blank', 'noopener,noreferrer')}
            >
              {#snippet icon()}
                <ExternalLink size={15} strokeWidth={2} />
              {/snippet}
              Open on Shikimori
            </SecondButton>
          {/if}
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  .news-page {
    position: relative;
    width: 100%;
    min-height: 100vh;
    padding: 0 0 56px 0;
    box-sizing: border-box;
    background: #0c0c0c;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    contain: style;
    -webkit-font-smoothing: antialiased;
  }

  /* Full-bleed edge-to-edge Home-style banner hero */
  .news-hero-wrap {
    position: relative;
    width: 100%;
    min-height: clamp(410px, 58vh, 540px);
    padding: 68px 0 clamp(28px, 4vw, 42px) 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .hero-banner-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center 22%;
    z-index: 0;
    pointer-events: none;
    user-select: none;
  }

  .hero-banner-img.is-portrait {
    filter: blur(18px) brightness(0.65);
    transform: scale(1.1);
  }

  .hero-vignette {
    position: absolute;
    inset: 0;
    z-index: 1;
    pointer-events: none;
    background:
      linear-gradient(90deg, rgba(12, 12, 12, 0.94) 18%, rgba(12, 12, 12, 0.62) 55%, rgba(12, 12, 12, 0.25) 100%),
      linear-gradient(180deg, rgba(12, 12, 12, 0.45) 0%, rgba(12, 12, 12, 0.2) 45%, rgba(12, 12, 12, 1) 100%);
  }

  .news-head {
    position: relative;
    z-index: 3;
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 16px;
    height: clamp(32px, 3.4vw, 48px);
    flex-shrink: 0;
    padding: 0 var(--page-pad-x);
    box-sizing: content-box;
    margin: clamp(18px, 2.5vw, 30px) 0 auto 0;
  }

  .news-tabs {
    position: relative;
    height: clamp(32px, 3.4vw, 48px);
    flex: 1 1 auto;
    min-width: 0;
    opacity: 0;
  }

  .news-tabs.is-ready {
    opacity: 1;
  }

  .tab-baseline-probe {
    position: absolute;
    left: 0;
    bottom: 0;
    font-family: 'PP Monument Extended', sans-serif;
    font-size: clamp(28px, 3.2vw, 48px);
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

  .news-tab {
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
    font-size: clamp(28px, 3.2vw, 48px);
    line-height: 1;
    white-space: nowrap;
    text-rendering: geometricPrecision;
    -webkit-font-smoothing: antialiased;
    will-change: transform;
    backface-visibility: hidden;
  }

  .news-tabs.is-animated .news-tab {
    transition: transform 0.45s var(--ease-out);
  }

  .tab-label {
    grid-area: 1 / 1;
    justify-self: start;
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

  .news-tab:hover .tab-regular {
    color: #8a8a8a;
  }

  .news-tab.active .tab-regular {
    opacity: 0;
  }

  .news-tab.active .tab-black {
    opacity: 1;
  }

  .sync-btn {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 36px;
    padding: 0 14px;
    border: none;
    border-radius: 11px;
    background: rgba(255, 255, 255, 0.06);
    backdrop-filter: blur(12px);
    color: rgba(255, 255, 255, 0.72);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    cursor: pointer;
    margin-bottom: 2px;
    transition:
      background-color 0.2s var(--ease-out),
      color 0.2s var(--ease-out),
      transform 0.1s ease;
  }

  .sync-btn:hover {
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
  }

  .sync-btn:active {
    transform: scale(0.96);
  }

  .sync-btn.is-spinning :global(.sync-icon) {
    animation: spin 0.9s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .news-hero-stages {
    position: relative;
    z-index: 2;
    display: grid;
    width: 100%;
    contain: layout style;
  }

  .hero-stage {
    grid-area: 1 / 1;
    position: relative;
    z-index: 2;
    padding: clamp(28px, 4vw, 48px) var(--page-pad-x) 0 var(--page-pad-x);
    max-width: 920px;
    display: flex;
    flex-direction: column;
  }

  .hero-title {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(28px, 3.8vw, 54px);
    font-weight: 700;
    line-height: 1.06;
    letter-spacing: -0.02em;
    color: #ffffff;
    display: -webkit-box;
    line-clamp: 2;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .meta-strip {
    margin-top: 12px;
  }

  .hero-excerpt {
    margin: 12px 0 0 0;
    max-width: 780px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: clamp(14.5px, 1.45vw, 18px);
    line-height: 1.4;
    color: rgba(255, 255, 255, 0.65);
    display: -webkit-box;
    line-clamp: 3;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .hero-actions {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 20px;
  }

  .hero-skeleton {
    position: relative;
    z-index: 2;
    margin: 72px var(--page-pad-x) 0 var(--page-pad-x);
    height: 220px;
    max-width: 740px;
    border-radius: var(--radius-card);
    background: linear-gradient(90deg, rgba(255, 255, 255, 0.03) 25%, rgba(255, 255, 255, 0.06) 50%, rgba(255, 255, 255, 0.03) 75%);
    background-size: 200% 100%;
    animation: shimmer 1.4s infinite linear;
  }

  .news-body {
    position: relative;
    z-index: 2;
    padding: 16px var(--page-pad-x) 0 var(--page-pad-x);
    display: flex;
    flex-direction: column;
    gap: 24px;
  }

  .news-views {
    position: relative;
    z-index: 2;
    display: grid;
    width: 100%;
    min-width: 0;
    contain: layout style;
    isolation: isolate;
    transform: translate3d(0, 0, 0);
  }

  /* Borderless responsive masonry */
  .news-grid {
    grid-area: 1 / 1;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 20px;
  }

  .news-masonry {
    grid-area: 1 / 1;
    display: flex;
    align-items: flex-start;
    gap: 20px;
    width: 100%;
    min-width: 0;
  }

  .news-col {
    flex: 1 1 0;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  @media (max-width: 1120px) {
    .news-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  .story-card {
    position: relative;
    display: flex;
    flex-direction: column;
    width: 100%;
    border-radius: var(--radius-card);
    overflow: hidden;
    background: #0c0c0c;
    cursor: pointer;
    isolation: isolate;
    contain: layout paint;
    content-visibility: auto;
    contain-intrinsic-size: 1px 360px;
    transition: transform 0.3s var(--ease-out);
  }

  .story-card::after {
    content: '';
    position: absolute;
    inset: -3px;
    border-radius: calc(var(--radius-card) + 3px);
    border: 4px solid #0c0c0c;
    transform: translateZ(0);
    pointer-events: none;
    z-index: 5;
  }

  .story-card:hover {
    transform: translate3d(0, -2px, 0);
  }

  .story-media {
    position: relative;
    width: 100%;
    aspect-ratio: 2 / 1;
    overflow: hidden;
    background: #101012;
    flex-shrink: 0;
  }

  .media-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    backface-visibility: hidden;
    transition: transform 0.55s var(--ease-out), filter 0.35s var(--ease-out);
    pointer-events: none;
    user-select: none;
  }

  .media-img.is-portrait {
    object-position: center 22%;
  }

  .story-card:hover .media-img {
    transform: scale(1.045);
    filter: brightness(1.07);
  }

  .media-fade {
    position: absolute;
    inset: -3px;
    z-index: 1;
    background: linear-gradient(180deg, rgba(16, 16, 18, 0) 36%, #101012 95%);
    pointer-events: none;
  }

  .story-info {
    position: relative;
    z-index: 2;
    margin-top: -2px;
    background: #101012;
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 14px 18px 16px 18px;
    flex: 0 0 auto;
  }

  .story-card.is-compact .story-info {
    margin-top: 0;
    padding-top: 16px;
  }

  .story-title {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(16px, 1.45vw, 18px);
    font-weight: 700;
    line-height: 1.24;
    color: #f3f3f3;
    display: -webkit-box;
    line-clamp: 2;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .story-excerpt {
    margin: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 14px;
    line-height: 1.42;
    color: #8d8d8d;
    display: -webkit-box;
    line-clamp: 2;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .story-footer {
    padding-top: 6px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .footer-meta {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    color: #6b6b70;
  }

  .meta-comments {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  /* Loading skeletons */
  .card-skeleton {
    height: 340px;
    border-radius: var(--radius-card);
    background: linear-gradient(90deg, rgba(255, 255, 255, 0.03) 25%, rgba(255, 255, 255, 0.06) 50%, rgba(255, 255, 255, 0.03) 75%);
    background-size: 200% 100%;
    animation: shimmer 1.4s infinite linear;
  }

  @keyframes shimmer {
    0% {
      background-position: 200% 0;
    }
    100% {
      background-position: -200% 0;
    }
  }

  .news-empty {
    padding: 72px 24px;
    border-radius: var(--radius-card);
    background: rgba(255, 255, 255, 0.03);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    text-align: center;
  }

  .empty-title {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 20px;
    color: #ffffff;
  }

  .empty-sub {
    margin: 0 0 8px 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 14px;
    color: #8d8d8d;
  }

  /* Borderless Article Reader Modal */
  .reader-backdrop {
    position: fixed;
    inset: 0;
    z-index: 220;
    background: rgba(4, 4, 5, 0.72);
    backdrop-filter: blur(18px);
    -webkit-backdrop-filter: blur(18px);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: clamp(16px, 2.5vw, 28px);
  }

  .reader-modal {
    position: relative;
    width: min(820px, 100%);
    max-height: calc(100vh - 36px);
    overflow-y: auto;
    border-radius: 18px;
    background: rgba(14, 14, 17, 0.94);
    box-shadow: 0 32px 90px rgba(0, 0, 0, 0.8);
    outline: none;
  }

  .reader-hero {
    position: relative;
    width: 100%;
    height: clamp(180px, 28vh, 270px);
    overflow: hidden;
    background: #101014;
  }

  .reader-hero-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .reader-hero-img.is-portrait {
    object-position: center 22%;
    filter: blur(8px) brightness(0.75);
    transform: scale(1.06);
  }

  .reader-hero-fade {
    position: absolute;
    inset: 0;
    z-index: 2;
    background: linear-gradient(180deg, rgba(14, 14, 17, 0.12) 0%, rgba(14, 14, 17, 0.96) 100%);
  }

  .reader-close {
    position: absolute;
    top: 16px;
    right: 16px;
    z-index: 5;
    width: 34px;
    height: 34px;
    border: none;
    border-radius: 10px;
    background: rgba(10, 10, 12, 0.65);
    backdrop-filter: blur(12px);
    color: rgba(255, 255, 255, 0.85);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: background-color 0.2s ease, color 0.2s ease;
  }

  .reader-close:hover {
    background: rgba(255, 255, 255, 0.16);
    color: #ffffff;
  }

  .reader-content {
    position: relative;
    z-index: 3;
    padding: clamp(18px, 2.5vw, 24px) clamp(20px, 3vw, 30px) clamp(22px, 3vw, 30px);
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .reader-title {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(20px, 2.2vw, 26px);
    font-weight: 700;
    line-height: 1.2;
    color: #ffffff;
  }

  .reader-paragraphs {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .reader-para {
    margin: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: clamp(14.5px, 1.35vw, 16px);
    line-height: 1.55;
    color: #c2c2c8;
  }

  .reader-quote {
    margin: 4px 0 0 0;
    padding: 12px 16px;
    background: rgba(255, 255, 255, 0.04);
    border-radius: 12px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 15px;
    line-height: 1.5;
    color: #9e9ea6;
  }

  .reader-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    margin-top: 6px;
  }
</style>
