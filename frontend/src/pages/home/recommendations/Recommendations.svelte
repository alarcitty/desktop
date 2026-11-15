<script lang="ts">
  import { onMount } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import LayerBlur from '../../../marlakit/blur/LayerBlur.svelte';
  import { getCachedHomeFeeds, loadHomeFeeds, type HomeRecommendationItem } from '../../../api/anime';
  import { openWatch } from '../../../app/player.svelte';

  interface Props {
    creditsOpen?: boolean;
    onToggleCredits?: () => void;
  }

  let { creditsOpen = false, onToggleCredits }: Props = $props();

  let items = $state<HomeRecommendationItem[]>(getCachedHomeFeeds()?.recommendations ?? []);
  let fallbackUsed = $state<Record<string, boolean>>({});
  let brokenImages = $state<Record<string, boolean>>({});

  function resolvePoster(item: HomeRecommendationItem): string {
    return fallbackUsed[item.id] && item.fallbackPoster ? item.fallbackPoster : item.poster;
  }

  function handleImgError(item: HomeRecommendationItem) {
    if (!fallbackUsed[item.id] && item.fallbackPoster) {
      fallbackUsed = { ...fallbackUsed, [item.id]: true };
    } else {
      brokenImages = { ...brokenImages, [item.id]: true };
    }
  }

  onMount(async () => {
    const feeds = await loadHomeFeeds();
    if (feeds.recommendations.length > 0) {
      items = feeds.recommendations;
    }
  });

  function handleRecClick(item: HomeRecommendationItem) {
    void openWatch({
      title: item.title,
      shikiId: item.shikiId,
      releaseAlias: item.releaseAlias,
      searchQueries: item.searchQueries
    });
  }
</script>

<section class="recommendations-section">
  <!-- Section Title -->
  <h2 class="section-title">You might also like</h2>

  <!-- Ambient Layer Blur underlay beneath posters -->
  <div class="recommendations-underlay-blur" aria-hidden="true">
    {#each items as item (item.id)}
      <div class="blur-col">
        <LayerBlur src={resolvePoster(item)} blur={80} opacity={0.35} scale={1.05} offsetY={0} />
      </div>
    {/each}
  </div>

  <!-- Full-width 5-poster panorama strip -->
  <div class="recommendations-strip">
    {#each items as item (item.id)}
      <button
        type="button"
        class="rec-card"
        aria-label={item.title}
        onclick={() => handleRecClick(item)}
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
        <div class="card-overlay"></div>
      </button>
    {/each}
  </div>

  <!-- Bottom tab notch with chevron -->
  <button
    type="button"
    class="bottom-tab-notch"
    class:is-open={creditsOpen}
    aria-expanded={creditsOpen}
    aria-label={creditsOpen ? 'Collapse credits' : 'Expand credits'}
    onclick={() => onToggleCredits?.()}
  >
    <ChevronDown size={18} strokeWidth={2} class="tab-arrow-icon" />
  </button>
</section>

<style>
  .recommendations-section {
    position: relative;
    width: 100%;
    height: 711px;
    background: #0c0c0c;
    overflow: hidden;
    box-sizing: border-box;
    contain: layout style paint;
    user-select: none;
    z-index: 1;
  }

  /* Section Title: PP Monument Extended Black, #D4D4D4 */
  .section-title {
    position: absolute;
    top: clamp(28px, 3.5vw, 44px);
    left: 50%;
    transform: translateX(-50%);
    margin: 0;
    padding: 0;
    max-width: calc(100% - var(--page-pad-x) * 2);
    font-family: 'PP Monument Extended', sans-serif;
    font-size: clamp(26px, 3.2vw, 48px);
    font-weight: 900;
    color: #D4D4D4;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    line-height: 1;
    z-index: 5;
    text-shadow: none;
  }

  /* Ambient Layer Blur underlay with smooth alpha gradient mask */
  .recommendations-underlay-blur {
    position: absolute;
    top: 75px;
    left: 0;
    width: 100%;
    height: 620px;
    display: flex;
    align-items: stretch;
    pointer-events: none;
    user-select: none;
    z-index: 1;
    contain: layout style paint;
    -webkit-mask-image: linear-gradient(
      180deg,
      rgba(0, 0, 0, 0) 0%,
      rgba(0, 0, 0, 1) 15%,
      rgba(0, 0, 0, 1) 65%,
      rgba(0, 0, 0, 0) 100%
    );
    mask-image: linear-gradient(
      180deg,
      rgba(0, 0, 0, 0) 0%,
      rgba(0, 0, 0, 1) 15%,
      rgba(0, 0, 0, 1) 65%,
      rgba(0, 0, 0, 0) 100%
    );
  }

  .blur-col {
    position: relative;
    flex: 1 1 0;
    height: 100%;
    overflow: visible;
  }

  /* 5-poster panorama strip: 100% width, 0 gap, exact height 589px at top: 122px */
  .recommendations-strip {
    position: absolute;
    top: 122px;
    left: 0;
    width: 100%;
    height: 589px;
    display: flex;
    align-items: stretch;
    margin: 0;
    padding: 0;
    gap: 0;
    overflow: hidden;
    box-sizing: border-box;
    z-index: 3;
  }

  /* Universal poster card styling */
  .rec-card {
    position: relative;
    flex: 1 1 0;
    height: 589px;
    padding: 0;
    border: none;
    outline: none;
    background: #0d0d10;
    overflow: hidden;
    box-sizing: border-box;
    cursor: pointer;
    contain: layout paint;
    transition: filter 0.25s ease;
  }

  .card-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center center;
    display: block;
    backface-visibility: hidden;
    user-select: none;
    pointer-events: none;
    transition: transform 0.4s var(--ease-out);
  }

  /* 46% dark overlay from Figma */
  .card-overlay {
    position: absolute;
    inset: -2px;
    background: rgba(0, 0, 0, 0.46);
    pointer-events: none;
    transition: background-color var(--hover-dur) var(--ease-out);
  }

  .rec-card:hover .card-img {
    will-change: transform;
    transform: scale(1.03);
  }

  .rec-card:hover .card-overlay {
    background: rgba(0, 0, 0, 0.22);
  }

  /* Bottom tab notch: 47x29px, #0c0c0c, rounded top corners */
  .bottom-tab-notch {
    position: absolute;
    bottom: 0;
    left: 50%;
    transform: translateX(-50%);
    width: 47px;
    height: 29px;
    padding: 0;
    border: none;
    outline: none;
    background: #0c0c0c;
    border-top-left-radius: 15px;
    border-top-right-radius: 15px;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    z-index: 5;
    transition: opacity 0.15s ease, transform 0.15s ease;
  }

  .bottom-tab-notch:hover {
    transform: translateX(-50%) translateY(-2px);
  }

  :global(.tab-arrow-icon) {
    color: #D0D0D0;
    transition: transform 0.42s cubic-bezier(0.22, 1, 0.36, 1), color 0.16s ease;
  }

  .bottom-tab-notch:hover :global(.tab-arrow-icon) {
    color: #ffffff;
  }

  .bottom-tab-notch.is-open :global(.tab-arrow-icon) {
    transform: rotate(180deg);
  }

  @media (max-width: 1120px) {
    .recommendations-section {
      height: 616px;
    }

    .recommendations-underlay-blur {
      top: 64px;
      height: 540px;
    }

    .recommendations-strip {
      top: 100px;
      height: 516px;
    }

    .rec-card {
      height: 516px;
    }
  }

  @media (max-width: 960px) {
    .recommendations-section {
      height: 536px;
    }

    .recommendations-underlay-blur {
      top: 56px;
      height: 470px;
    }

    .recommendations-strip {
      top: 90px;
      height: 446px;
    }

    .rec-card {
      height: 446px;
    }
  }
</style>

