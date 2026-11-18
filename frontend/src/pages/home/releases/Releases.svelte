<script lang="ts">
  import { onMount } from 'svelte';
  import Play from '@lucide/svelte/icons/play';
  import Bookmark from '@lucide/svelte/icons/bookmark';
  import Check from '@lucide/svelte/icons/check';

  import MainButton from '../../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../../marlakit/button/SecondButton.svelte';
  import LikeButton from '../../../marlakit/button/LikeButton.svelte';
  import LayerBlur from '../../../marlakit/blur/LayerBlur.svelte';
  import MetaLine from '../../../marlakit/meta/MetaLine.svelte';
  import { getCachedHomeFeeds, loadHomeFeeds, type HomeReleaseItem } from '../../../api/anime';
  import { isInWatchLater, isLiked, toggleLike, toggleWatchLater } from '../../../app/library.svelte';
  import { openWatch } from '../../../app/player.svelte';
  import { t } from '../../../app/i18n.svelte';

  let releases = $state<HomeReleaseItem[]>(getCachedHomeFeeds()?.releases ?? []);

  let activeIndex = $state(0);
  let fallbackUsed = $state<Record<string, boolean>>({});
  let brokenImages = $state<Record<string, boolean>>({});

  function resolvePoster(item: HomeReleaseItem): string {
    return fallbackUsed[item.id] && item.fallbackPoster ? item.fallbackPoster : item.poster;
  }

  function handleImgError(item: HomeReleaseItem) {
    if (!fallbackUsed[item.id] && item.fallbackPoster) {
      fallbackUsed = { ...fallbackUsed, [item.id]: true };
    } else {
      brokenImages = { ...brokenImages, [item.id]: true };
    }
  }
  let isWatchLoading = $state(false);
  let isPlanLoading = $state(false);
  let planTimer: number | undefined;

  $effect(() => () => {
    window.clearTimeout(planTimer);
  });

  onMount(async () => {
    const feeds = await loadHomeFeeds();
    if (feeds.releases.length > 0) {
      releases = feeds.releases;
      if (activeIndex >= releases.length) {
        activeIndex = 0;
      }
    }
  });

  function setActive(index: number) {
    if (activeIndex !== index) {
      activeIndex = index;
    }
  }

  async function handleWatchClick(e: MouseEvent) {
    e.stopPropagation();
    const item = releases[activeIndex];
    if (!item) {
      return;
    }
    isWatchLoading = true;
    try {
      await openWatch({
        title: item.title,
        shikiId: item.shikiId,
        releaseAlias: item.releaseAlias,
        searchQueries: item.searchQueries
      });
    } finally {
      isWatchLoading = false;
    }
  }

  function releaseLibraryPayload(item: HomeReleaseItem) {
    return {
      shikiId: item.shikiId,
      releaseAlias: item.releaseAlias,
      searchQueries: item.searchQueries,
      title: item.title,
      description: item.description,
      poster: resolvePoster(item),
      score: item.score,
      tags: item.tags
    };
  }

  function handlePlanClick(e: MouseEvent) {
    e.stopPropagation();
    const item = releases[activeIndex];
    if (!item) return;
    isPlanLoading = true;
    toggleWatchLater(releaseLibraryPayload(item));
    isPlanLoading = false;
  }

  function handleLikeClick(e: MouseEvent, item: HomeReleaseItem) {
    e.stopPropagation();
    toggleLike(releaseLibraryPayload(item));
  }
</script>

<section class="releases-section">
  <!-- Section Title -->
  <h2 class="section-title">{t('home.newReleases')}</h2>

  <!-- Ambient Layer Blur underneath cards (GPU-accelerated cross-fade, zero layout recalculation) -->
  <div class="releases-underlay-blur" aria-hidden="true">
    {#each releases as item, index (item.id)}
      <div
        class="blur-layer"
        class:is-active={activeIndex === index}
      >
        <LayerBlur
          src={resolvePoster(item)}
          blur={60}
          opacity={0.32}
          scale={1.05}
          offsetY={0}
        />
      </div>
    {/each}
  </div>

  <!-- Full-width 4-banner panorama strip (interactive accordion) -->
  <div class="releases-strip">
    {#each releases as item, index (item.id)}
      {@const itemInPlan = isInWatchLater({
        shikiId: item.shikiId,
        releaseAlias: item.releaseAlias,
        title: item.title
      })}
      {@const itemLiked = isLiked({
        shikiId: item.shikiId,
        releaseAlias: item.releaseAlias,
        title: item.title
      })}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="card"
        class:is-active={activeIndex === index}
        onclick={() => setActive(index)}
      >
        {#if !brokenImages[item.id]}
          <img
            src={resolvePoster(item)}
            alt={item.title}
            class="card-img"
            decoding="async"
            onerror={() => handleImgError(item)}
          />
        {/if}

        <!-- Active card gradient overlays -->
        <div class="card-overlay" class:is-visible={activeIndex === index}></div>
        <div class="card-bottom-fade" class:is-visible={activeIndex === index}></div>

        <!-- Inactive side card dimming overlay -->
        <div class="side-card-overlay" class:is-dimmed={activeIndex !== index}></div>

        <!-- Foreground content with big title, description and buttons -->
        <div class="card-content">
          <h3 class="card-title">
            {#each item.titleLines as line, lineIndex}
              {line}{#if lineIndex < item.titleLines.length - 1}<br />{/if}
            {/each}
          </h3>

          <div class="card-meta">
            <MetaLine score={item.score} tags={item.tags} />
          </div>

          <p class="card-description">
            {item.description}
          </p>

          <div class="card-actions">
            <MainButton loading={isWatchLoading && activeIndex === index} onclick={handleWatchClick}>
              {#snippet icon()}
                <Play size={16} fill="currentColor" strokeWidth={0} />
              {/snippet}
              {t('news.watch')}
            </MainButton>

            <SecondButton loading={isPlanLoading && activeIndex === index} onclick={handlePlanClick}>
              {#snippet icon()}
                {#if itemInPlan}
                  <Check size={16} strokeWidth={2.2} />
                {:else}
                  <Bookmark size={16} strokeWidth={2} />
                {/if}
              {/snippet}
              {itemInPlan ? t('anime.inPlanToWatch') : t('home.planToWatch')}
            </SecondButton>

            <LikeButton
              variant="media"
              liked={itemLiked}
              onclick={(e) => handleLikeClick(e, item)}
            />
          </div>
        </div>
      </div>
    {/each}
  </div>
</section>

<style>
  .releases-section {
    position: relative;
    width: 100%;
    height: 646px;
    background: var(--mila-app-canvas, #0c0c0c);
    overflow-x: clip;
    overflow-y: visible;
    box-sizing: border-box;
    contain: layout style;
    user-select: none;
    z-index: 4;
    transition: background-color 360ms cubic-bezier(0.22, 1, 0.36, 1);
  }

  /* Section title: 48px, PP Monument Extended Black */
  .section-title {
    position: absolute;
    top: clamp(28px, 3.5vw, 44px);
    left: 50%;
    transform: translateX(-50%);
    margin: 0;
    padding: 0;
    max-width: calc(100% - var(--page-pad-x) * 2);
    font-family: 'PP Monument Extended', sans-serif;
    font-size: clamp(28px, 3.3vw, 48px);
    font-weight: 900;
    color: var(--mila-text-heading, #D4D4D4);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    line-height: 1;
    z-index: 5;
    text-shadow: none;
    transition: color 320ms ease;
  }

  /* Ambient Layer Blur underlay trailing down smoothly into the next section */
  .releases-underlay-blur {
    position: absolute;
    top: 70px;
    left: 0;
    width: 100%;
    height: 800px;
    pointer-events: none;
    user-select: none;
    z-index: 1;
    contain: layout style;
    -webkit-mask-image: linear-gradient(
      180deg,
      rgba(0, 0, 0, 0) 0%,
      rgba(0, 0, 0, 1) 15%,
      rgba(0, 0, 0, 1) 68%,
      rgba(0, 0, 0, 0) 100%
    );
    mask-image: linear-gradient(
      180deg,
      rgba(0, 0, 0, 0) 0%,
      rgba(0, 0, 0, 1) 15%,
      rgba(0, 0, 0, 1) 68%,
      rgba(0, 0, 0, 0) 100%
    );
  }

  /* Each blur layer is fixed full size, cross-fades opacity only (pure GPU compositing) */
  .blur-layer {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    opacity: 0;
    backface-visibility: hidden;
    transition: opacity 0.5s ease;
  }

  .blur-layer.is-active {
    opacity: 1;
  }

  /* 4 banners strip: edge-to-edge, 100% width, 0 gap, exact height 524px */
  .releases-strip {
    position: absolute;
    top: 122px;
    left: 0;
    width: 100%;
    height: 524px;
    display: flex;
    align-items: stretch;
    margin: 0;
    padding: 0;
    gap: 0;
    overflow: hidden;
    box-sizing: border-box;
    contain: layout style paint;
    z-index: 3;
  }

  /* Universal card styling with slightly slower (0.65s) butter-smooth flex transition */
  .card {
    position: relative;
    height: 524px;
    overflow: hidden;
    box-sizing: border-box;
    cursor: pointer;
    flex: 1 1 0;
    min-width: 0;
    contain: layout paint;
    transition: flex-grow 0.65s cubic-bezier(0.22, 1, 0.36, 1),
                flex-basis 0.65s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .card::after {
    content: '';
    position: absolute;
    inset: 0;
    z-index: 4;
    background: linear-gradient(155deg, rgba(255, 255, 255, 0.08) 0%, rgba(255, 255, 255, 0) 45%);
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.28s var(--ease-out);
  }

  .card:not(.is-active):hover::after {
    opacity: 1;
  }

  .card:not(.is-active):hover .side-card-overlay.is-dimmed {
    opacity: 0.35;
  }

  /* Active expanded card: responsive width from compact desktop (440px @ 858px) up to 840px */
  .card.is-active {
    flex: 0 0 clamp(440px, 56vw, 840px);
    cursor: default;
    z-index: 4;
  }

  /* Absolute background image behind card content */
  .card-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center center;
    display: block;
    user-select: none;
    pointer-events: none;
    z-index: 0;
  }

  /* Active card dark gradients */
  .card-overlay {
    position: absolute;
    inset: -2px;
    background:
      linear-gradient(0deg, rgba(0, 0, 0, 0.20) 0%, rgba(0, 0, 0, 0.20) 100%),
      linear-gradient(90deg, rgba(0, 0, 0, 0.74) 25%, rgba(102, 102, 102, 0.10) 100%);
    pointer-events: none;
    z-index: 1;
    opacity: 0;
    transition: opacity 0.35s ease;
  }

  .card-overlay.is-visible {
    opacity: 1;
  }

  .card-bottom-fade {
    position: absolute;
    left: -2px;
    right: -2px;
    bottom: -2px;
    height: 140px;
    background: linear-gradient(0deg, rgba(0, 0, 0, 0.35) 0%, rgba(102, 102, 102, 0) 100%);
    pointer-events: none;
    z-index: 2;
    opacity: 0;
    transition: opacity 0.35s ease;
  }

  .card-bottom-fade.is-visible {
    opacity: 1;
  }

  /* Inactive side card dimming overlay */
  .side-card-overlay {
    position: absolute;
    inset: -2px;
    background: linear-gradient(0deg, rgba(12, 12, 12, 0.40) 0%, rgba(12, 12, 12, 0.40) 100%);
    pointer-events: none;
    z-index: 2;
    opacity: 0;
    transition: opacity 0.35s ease;
  }

  .side-card-overlay.is-dimmed {
    opacity: 1;
  }

  /* Foreground content container (no transform here to keep backdrop-filter clean) */
  .card-content {
    position: absolute;
    inset: 0;
    z-index: 3;
    display: flex;
    flex-direction: column;
    padding: clamp(18px, 2vw, 24px);
    box-sizing: border-box;
    width: clamp(440px, 56vw, 840px);
    max-width: 100%;
    pointer-events: none;
    --mila-buttons-mainBg: var(--mila-media-mainBtnBg, rgba(255, 255, 255, 0.96));
    --mila-buttons-mainText: var(--mila-media-mainBtnText, #111111);
    --mila-buttons-secondBg: var(--mila-media-secondBtnBg, rgba(255, 255, 255, 0.08));
    --mila-buttons-secondText: var(--mila-media-secondBtnText, #ffffff);
    --mila-meta-tag: var(--mila-media-subtext, rgba(255, 255, 255, 0.56));
    --mila-meta-dot: var(--mila-media-muted, rgba(255, 255, 255, 0.36));
  }

  .card:not(.is-active) .card-content {
    visibility: hidden;
    transition: visibility 0s linear 0.22s;
  }

  .card.is-active .card-content {
    visibility: visible;
    transition: visibility 0s linear 0s;
    pointer-events: auto;
  }

  /* Big title: PP Monument Extended Black, #DADADA */
  .card-title {
    margin: 0 0 14px 0;
    padding: 0;
    font-family: 'PP Monument Extended', sans-serif;
    font-size: clamp(26px, 3.6vw, 60px);
    font-weight: 900;
    color: #DADADA;
    line-height: 1.05;
    word-wrap: break-word;
    text-shadow: none;
    opacity: 0;
    transform: translateY(14px);
    transition: opacity 0.2s ease-out, transform 0.2s ease-out;
  }

  .card.is-active .card-title {
    opacity: 1;
    transform: translateY(0);
    transition: opacity 0.45s cubic-bezier(0.16, 1, 0.3, 1) 0.16s,
                transform 0.45s cubic-bezier(0.16, 1, 0.3, 1) 0.16s;
  }

  .card-meta {
    margin: 0 0 12px 0;
    opacity: 0;
    transform: translateY(10px);
    transition: opacity 0.2s ease-out, transform 0.2s ease-out;
  }

  .card.is-active .card-meta {
    opacity: 1;
    transform: translateY(0);
    transition: opacity 0.45s cubic-bezier(0.16, 1, 0.3, 1) 0.19s,
                transform 0.45s cubic-bezier(0.16, 1, 0.3, 1) 0.19s;
  }

  /* Description: Medeo Sans Display Regular, #8D8D8D, directly relative to title */
  .card-description {
    margin: 0;
    padding: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: clamp(14.5px, 1.45vw, 20px);
    font-weight: 400;
    color: #8D8D8D;
    line-height: 1.35;
    word-wrap: break-word;
    text-shadow: none;
    max-width: min(480px, 92%);
    display: -webkit-box;
    line-clamp: 7;
    -webkit-line-clamp: 7;
    -webkit-box-orient: vertical;
    overflow: hidden;
    opacity: 0;
    transform: translateY(10px);
    transition: opacity 0.2s ease-out, transform 0.2s ease-out;
  }

  .card.is-active .card-description {
    opacity: 1;
    transform: translateY(0);
    transition: opacity 0.45s cubic-bezier(0.16, 1, 0.3, 1) 0.22s,
                transform 0.45s cubic-bezier(0.16, 1, 0.3, 1) 0.22s;
  }

  /* Actions: the fade lives on the buttons themselves, NOT on this container.
     An ancestor with opacity < 1 becomes a "backdrop root" and kills the
     backdrop-filter of SecondButton until the animation ends (blur pops in). */
  .card-actions {
    margin-top: auto;
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .card-actions :global(.marlakit-btn-main),
  .card-actions :global(.marlakit-btn-second),
  .card-actions :global(.marlakit-like-btn) {
    opacity: 0;
    transition: opacity 0.2s ease-out, background-color 0.15s ease, color 0.2s ease, transform 0.1s ease;
  }

  .card.is-active .card-actions :global(.marlakit-btn-main),
  .card.is-active .card-actions :global(.marlakit-btn-second),
  .card.is-active .card-actions :global(.marlakit-like-btn) {
    opacity: 1;
    transition: opacity 0.38s cubic-bezier(0.16, 1, 0.3, 1) 0.42s, background-color 0.15s ease, color 0.2s ease, transform 0.1s ease;
  }

  @media (max-width: 1120px) {
    .releases-section {
      height: 560px;
    }

    .releases-underlay-blur {
      height: 620px;
    }

    .releases-strip {
      top: 100px;
      height: 460px;
    }

    .card {
      height: 460px;
    }
  }

  @media (max-width: 960px) {
    .releases-section {
      height: 506px;
    }

    .releases-underlay-blur {
      height: 546px;
    }

    .releases-strip {
      top: 90px;
      height: 416px;
    }

    .card {
      height: 416px;
    }

    .card-description {
      line-clamp: 5;
      -webkit-line-clamp: 5;
    }
  }
</style>
