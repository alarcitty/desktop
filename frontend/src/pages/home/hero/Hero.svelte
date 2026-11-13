<script lang="ts">
  import { onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import Play from '@lucide/svelte/icons/play';
  import Bookmark from '@lucide/svelte/icons/bookmark';
  import MainButton from '../../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../../marlakit/button/SecondButton.svelte';
  import MetaLine from '../../../marlakit/meta/MetaLine.svelte';
  import LayerBlur from '../../../marlakit/blur/LayerBlur.svelte';
  import { getCachedHomeFeeds, loadHomeFeeds, type HomeSpotlightItem } from '../../../api/anime';
  import { openWatch, player } from '../../../app/player.svelte';

  interface Props {
    onselectbg?: (url: string) => void;
  }

  const SPOTLIGHT_INTERVAL_MS = 8500;

  let { onselectbg }: Props = $props();

  let spotlight = $state<HomeSpotlightItem[]>(getCachedHomeFeeds()?.spotlight ?? []);
  let activeIndex = $state(0);
  let isPaused = $state(false);
  let isWatchLoading = $state(false);
  let isPlanLoading = $state(false);
  let planTimer: number | undefined;
  let cycleTimer: number | undefined;

  let activeHero = $derived(spotlight[activeIndex] ?? spotlight[0] ?? null);

  function applySpotlightIndex(index: number)
  {
    if (index === activeIndex)
    {
      return;
    }
    activeIndex = index;
    const selected = spotlight[index];
    if (selected?.bg)
    {
      onselectbg?.(selected.bg);
    }
  }

  function startAutoCycle()
  {
    window.clearInterval(cycleTimer);
    cycleTimer = window.setInterval(() =>
    {
      if (isPaused || spotlight.length <= 1)
      {
        return;
      }
      const next = (activeIndex + 1) % spotlight.length;
      activeIndex = next;
      const selected = spotlight[next];
      if (selected?.bg)
      {
        onselectbg?.(selected.bg);
      }
    }, SPOTLIGHT_INTERVAL_MS);
  }

  $effect(() => () =>
  {
    window.clearTimeout(planTimer);
    window.clearInterval(cycleTimer);
  });

  onMount(async () =>
  {
    if (spotlight.length > 0 && spotlight[activeIndex]?.bg)
    {
      onselectbg?.(spotlight[activeIndex].bg);
    }
    startAutoCycle();

    const feeds = await loadHomeFeeds();
    if (feeds.spotlight.length > 0)
    {
      spotlight = feeds.spotlight;
      activeIndex = 0;
      if (feeds.spotlight[0]?.bg)
      {
        onselectbg?.(feeds.spotlight[0].bg);
      }
    }
  });

  async function handleWatchClick()
  {
    if (!activeHero) return;
    const target = activeHero;
    isWatchLoading = true;
    try
    {
      await openWatch({
        title: target.title,
        releaseAlias: target.releaseAlias,
        shikiId: target.shikiId,
        searchQueries: target.searchQueries
      });
    }
    finally
    {
      isWatchLoading = false;
    }
  }

  function handlePlanClick()
  {
    window.clearTimeout(planTimer);
    isPlanLoading = true;
    planTimer = window.setTimeout(() =>
    {
      isPlanLoading = false;
    }, 2000);
  }

  function scrollBelowHero(e: MouseEvent)
  {
    const btn = e.currentTarget as HTMLElement | null;
    const viewport = btn?.closest('.scroll-viewport') as HTMLElement | null;
    const releases = document.querySelector('.releases-section');
    if (viewport && releases instanceof HTMLElement)
    {
      const targetTop = viewport.scrollTop + releases.getBoundingClientRect().top - 42;
      viewport.scrollTo({
        top: Math.max(0, targetTop),
        behavior: 'smooth'
      });
      return;
    }
    if (releases instanceof HTMLElement)
    {
      releases.scrollIntoView({ behavior: 'smooth', block: 'start' });
      return;
    }
    viewport?.scrollBy({
      top: Math.round(window.innerHeight * 0.88),
      behavior: 'smooth'
    });
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<section
  class="hero-container"
  onmouseenter={() => { isPaused = true; }}
  onmouseleave={() => { isPaused = false; }}
>
  <!-- 3-screenshot horizontal gallery strip -->
  <div class="preview-strip">
    {#if activeHero}
      {#key activeHero.shikiId}
        <div class="preview-set" transition:fade={{ duration: 280, easing: cubicOut }}>
          {#if activeHero.previews.length >= 3}
            <img src={activeHero.previews[0]} alt="{activeHero.title} preview 1" class="preview-img img-1" decoding="async" />
            <img src={activeHero.previews[1]} alt="{activeHero.title} preview 2" class="preview-img img-2" decoding="async" />
            <img src={activeHero.previews[2]} alt="{activeHero.title} preview 3" class="preview-img img-3" decoding="async" />
          {/if}
        </div>
      {/key}
    {/if}
  </div>

  <!-- Hero title + symmetric two-column body -->
  <div class="hero-content">
    {#if activeHero}
      <h1 class="hero-title">{activeHero.title}</h1>

      <div class="hero-body">
        <div class="hero-main-col">
          <div class="hero-meta">
            <MetaLine score={activeHero.score} tags={activeHero.metaTags} />
          </div>
          <p class="hero-description">{activeHero.description}</p>

          <!-- Action buttons with icons -->
          <div class="hero-actions">
            <MainButton loading={isWatchLoading || player.loading} onclick={handleWatchClick}>
              {#snippet icon()}
                <Play size={16} fill="currentColor" strokeWidth={0} />
              {/snippet}
              Watch
            </MainButton>

            <SecondButton loading={isPlanLoading} onclick={handlePlanClick}>
              {#snippet icon()}
                <Bookmark size={16} strokeWidth={2} />
              {/snippet}
              Plan to Watch
            </SecondButton>
          </div>
        </div>

        <!-- Bottom-right Mila-native panorama accordion strip -->
        <aside class="hero-strip-wrap" aria-label="Featured spotlight switcher">
          <div class="hero-strip-blur" aria-hidden="true">
            {#key activeHero.shikiId}
              {#if activeHero.poster}
                <div class="strip-blur-layer" transition:fade={{ duration: 420, easing: cubicOut }}>
                  <LayerBlur src={activeHero.poster} blur={46} opacity={0.32} scale={1.1} offsetY={0} />
                </div>
              {/if}
            {/key}
          </div>

          <div class="hero-strip">
            {#each spotlight as item, idx (item.shikiId)}
              <button
                type="button"
                class="strip-card"
                class:is-active={idx === activeIndex}
                aria-label={item.title}
                onclick={() => applySpotlightIndex(idx)}
              >
                {#if item.poster}
                  <img src={item.poster} alt={item.title} class="strip-card-img" decoding="async" />
                {/if}
                <div class="strip-card-overlay"></div>
                <div class="strip-card-bottom-fade"></div>

                {#if idx === activeIndex}
                  <span
                    class="strip-progress"
                    class:is-paused={isPaused}
                    style="animation-duration: {SPOTLIGHT_INTERVAL_MS}ms;"
                  ></span>
                {/if}
              </button>
            {/each}
          </div>
        </aside>
      </div>
    {/if}
  </div>

  <!-- Bottom scroll indicator chevron -->
  <button type="button" class="hero-bottom-arrow" aria-label="Scroll to releases" onclick={scrollBelowHero}>
    <ChevronDown size={18} strokeWidth={2} class="arrow-icon" />
  </button>
</section>

<style>
  .hero-container {
    width: 100%;
    min-height: calc(100vh - 72px);
    display: flex;
    flex-direction: column;
    padding: 0 var(--page-pad-x) 14px var(--page-pad-x);
    box-sizing: border-box;
    position: relative;
    z-index: 2;
  }

  /* 3-screenshot horizontal gallery strip */
  .preview-strip {
    position: relative;
    width: 100%;
    height: clamp(260px, 38vh, 310px);
    min-height: 250px;
    border-radius: var(--radius-card);
    overflow: hidden;
    box-sizing: border-box;
    flex-shrink: 0;
    background: rgba(12, 12, 12, 0.45);
  }

  .preview-set {
    position: absolute;
    inset: 0;
    display: flex;
    width: 100%;
    height: 100%;
  }

  .preview-img {
    height: 100%;
    min-width: 0;
    object-fit: cover;
    display: block;
    user-select: none;
    pointer-events: none;
  }

  .img-1 {
    flex: 540;
  }

  .img-2 {
    flex: 385;
  }

  .img-3 {
    flex: 373;
  }

  /* Text, actions & spotlight container */
  .hero-content {
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
    margin-top: 16px;
    position: relative;
    z-index: 2;
  }

  .hero-title {
    margin: 0 0 -0.08em 0;
    padding: 0 0 0.08em 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(34px, 4.4vw, 68px);
    font-weight: 700;
    color: #ffffff;
    line-height: 1.16;
    letter-spacing: -0.02em;
    user-select: none;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .hero-body {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 36px;
    margin-top: 10px;
  }

  .hero-main-col {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-width: 0;
  }

  .hero-meta {
    margin-top: 2px;
  }

  .hero-description {
    margin: 12px 0 0 0;
    padding: 0;
    max-width: 760px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 19px;
    font-weight: 400;
    color: #8f8f94;
    line-height: 1.36;
    user-select: none;
    display: -webkit-box;
    line-clamp: 4;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .hero-actions {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 16px;
  }

  /* Bottom-right Mila-native panorama accordion strip (gap: 0, edge-to-edge posters inside rounded clip) */
  .hero-strip-wrap {
    position: relative;
    width: clamp(340px, 31vw, 430px);
    height: 158px;
    flex-shrink: 0;
  }

  .hero-strip-blur {
    position: absolute;
    inset: -16px;
    pointer-events: none;
    z-index: 0;
    overflow: hidden;
    border-radius: calc(var(--radius-card) + 12px);
    -webkit-mask-image: radial-gradient(ellipse at center, rgba(0, 0, 0, 1) 42%, rgba(0, 0, 0, 0) 76%);
    mask-image: radial-gradient(ellipse at center, rgba(0, 0, 0, 1) 42%, rgba(0, 0, 0, 0) 76%);
  }

  .strip-blur-layer {
    position: absolute;
    inset: 0;
  }

  .hero-strip {
    position: relative;
    z-index: 1;
    width: 100%;
    height: 100%;
    display: flex;
    align-items: stretch;
    gap: 0;
    border-radius: var(--radius-card);
    overflow: hidden;
    transform: translateZ(0);
    background: #101012;
  }

  .strip-card {
    position: relative;
    flex: 1 1 0;
    min-width: 0;
    height: 100%;
    padding: 0;
    border: none;
    outline: none;
    background: transparent;
    cursor: pointer;
    overflow: hidden;
    will-change: flex-grow;
    contain: layout paint;
    transition: flex-grow 0.65s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .strip-card.is-active {
    flex: 2.35 1 0;
    cursor: default;
  }

  .strip-card-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center 22%;
    display: block;
    backface-visibility: hidden;
    pointer-events: none;
    user-select: none;
    transition: transform 0.45s var(--ease-out);
  }

  .strip-card:not(.is-active):hover .strip-card-img {
    transform: scale(1.04);
  }

  .strip-card-overlay {
    position: absolute;
    inset: -2px;
    background: rgba(8, 8, 10, 0.56);
    pointer-events: none;
    transition: background-color 0.35s var(--ease-out);
  }

  .strip-card:not(.is-active):hover .strip-card-overlay {
    background: rgba(8, 8, 10, 0.28);
  }

  .strip-card.is-active .strip-card-overlay {
    background: rgba(8, 8, 10, 0.08);
  }

  .strip-card-bottom-fade {
    position: absolute;
    left: -2px;
    right: -2px;
    bottom: -2px;
    height: 46%;
    background: linear-gradient(180deg, rgba(8, 8, 10, 0) 0%, rgba(8, 8, 10, 0.65) 100%);
    pointer-events: none;
  }

  .strip-progress {
    position: absolute;
    left: 0;
    bottom: 0;
    height: 2px;
    width: 100%;
    z-index: 3;
    background: rgba(255, 255, 255, 0.85);
    transform-origin: left center;
    animation-name: spotlightSweep;
    animation-timing-function: linear;
    animation-fill-mode: forwards;
    pointer-events: none;
  }

  .strip-progress.is-paused {
    animation-play-state: paused;
  }

  @keyframes spotlightSweep {
    from {
      transform: scaleX(0);
    }
    to {
      transform: scaleX(1);
    }
  }

  @media (max-width: 980px) {
    .hero-body {
      flex-direction: column;
      align-items: flex-start;
      gap: 20px;
    }

    .hero-strip-wrap {
      width: 100%;
    }
  }

  .hero-bottom-arrow {
    margin-top: auto;
    margin-bottom: 2px;
    padding: 4px 12px;
    border: none;
    background: transparent;
    align-self: center;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    opacity: 0.65;
    transition: opacity 0.15s ease, transform 0.15s ease;
    position: relative;
    z-index: 2;
  }

  .hero-bottom-arrow:hover {
    opacity: 1;
    transform: translateY(2px);
  }

  :global(.arrow-icon) {
    color: #ffffff;
  }
</style>