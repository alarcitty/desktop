<script lang="ts">
  import { onMount } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import Play from '@lucide/svelte/icons/play';
  import Bookmark from '@lucide/svelte/icons/bookmark';

  import MainButton from '../../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../../marlakit/button/SecondButton.svelte';
  import LayerBlur from '../../../marlakit/blur/LayerBlur.svelte';
  import MetaLine from '../../../marlakit/meta/MetaLine.svelte';
  import { buildMetaTags, ensureShikimoriArtwork, truncateDescription } from '../../../api/anime';
  import { openWatch } from '../../../app/player.svelte';

  const RELEASE_DESC_LIMIT = 155;

  interface ReleaseItem {
    id: string;
    shikiId: number;
    releaseAlias?: string;
    searchQueries?: string[];
    title: string;
    titleLines: string[];
    description: string;
    poster: string;
    score?: number;
    tags: string[];
  }

  let releases = $state<ReleaseItem[]>([
    {
      id: 'mushoku',
      shikiId: 39535,
      searchQueries: ['Mushoku Tensei', 'Реинкарнация безработного'],
      title: 'Mushoku Tensei: Jobless Reincarnation',
      titleLines: ['Mushoku', 'Tensei: Jobless', 'Reincarnation'],
      description:
        'A 34-year-old unemployed recluse is killed after saving strangers from a speeding truck and reincarnated in a magical fantasy world as an infant named Rudeus Greyrat. Retaining his memories and regrets from his past life, he vows to overcome his past trauma and live his new life to the fullest without regrets.',
      poster: 'https://shikimori.io/uploads/poster/animes/39535/f66d91c3fdc65449f4bb3568e4d36fec.jpeg',
      score: 8.4,
      tags: ['2021', '11 ep', 'Adventure', 'Drama', 'Fantasy']
    },
    {
      id: 'black-clover',
      shikiId: 34572,
      releaseAlias: 'black-clover-special',
      searchQueries: ['Black Clover', 'Чёрный клевер'],
      title: 'Black Clover',
      titleLines: ['Black', 'Clover', 'Sword of the King'],
      description:
        'In a world where magic is everything, Asta, a boy born without magic, aims to become the Wizard King to overcome adversity and prove his power to the entire kingdom.',
      poster: 'https://shikimori.io/uploads/poster/animes/34572/d4c5bf9cac831c03fc5ebed2676b7b10.jpeg',
      score: 8.1,
      tags: ['2017', '170 ep', 'Action', 'Comedy', 'Fantasy']
    },
    {
      id: 'frieren',
      shikiId: 52991,
      releaseAlias: 'sousou-no-frieren',
      searchQueries: ['Sousou no Frieren', 'Провожающая в последний путь Фрирен'],
      title: "Frieren: Beyond Journey's End",
      titleLines: ['Frieren:', "Beyond Journey's", 'End'],
      description:
        "An elf mage reflects on life and mortality as she embarks on a journey to fulfill her former party's wishes after defeating the Demon King and wandering through centuries.",
      poster: 'https://shikimori.io/uploads/poster/animes/52991/dc841cc9fce2aa1e9907a4b61c5d1d92.jpeg',
      score: 9.3,
      tags: ['2023', '28 ep', 'Adventure', 'Drama', 'Fantasy']
    },
    {
      id: 'tsukimichi',
      shikiId: 43523,
      releaseAlias: 'tsuki-ga-michibiku-isekai-douchuu',
      searchQueries: ['Tsuki ga Michibiku Isekai Douchuu', 'Лунное путешествие'],
      title: 'Tsukimichi: Moonlit Fantasy',
      titleLines: ['Tsukimichi:', 'Moonlit', 'Fantasy'],
      description:
        'Makoto Misumi is transported to another world to be a hero, only to be cast away by the goddess for not meeting her standards of beauty, surviving in the wasteland.',
      poster: 'https://shikimori.io/uploads/poster/animes/43523/9f56a98052f05ad81a43452ecacf657d.jpeg',
      score: 7.7,
      tags: ['2021', '12 ep', 'Action', 'Adventure', 'Fantasy']
    }
  ]);

  let activeIndex = $state(0);
  let isWatchLoading = $state(false);
  let isPlanLoading = $state(false);
  let planTimer: number | undefined;

  $effect(() => () => {
    window.clearTimeout(planTimer);
  });

  onMount(async () => {
    const map = await ensureShikimoriArtwork(releases.map((r) => r.shikiId));
    releases = releases.map((r) => {
      const shiki = map.get(r.shikiId);
      if (!shiki) {
        return r;
      }
      const nextTags = buildMetaTags(shiki);
      return {
        ...r,
        poster: shiki.posterOriginal || shiki.posterMain || r.poster,
        description: shiki.description ? truncateDescription(shiki.description, RELEASE_DESC_LIMIT) : r.description,
        score: shiki.score > 0 ? Number(shiki.score.toFixed(1)) : r.score,
        tags: nextTags.length > 0 ? nextTags : r.tags
      };
    });
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

  function handlePlanClick(e: MouseEvent) {
    e.stopPropagation();
    window.clearTimeout(planTimer);
    isPlanLoading = true;
    planTimer = window.setTimeout(() => {
      isPlanLoading = false;
    }, 2000);
  }
</script>

<section class="releases-section">
  <!-- Section Title -->
  <h2 class="section-title">New Releases</h2>

  <!-- Ambient Layer Blur underneath cards (GPU-accelerated cross-fade, zero layout recalculation) -->
  <div class="releases-underlay-blur" aria-hidden="true">
    {#each releases as item, index (item.id)}
      <div
        class="blur-layer"
        class:is-active={activeIndex === index}
      >
        <LayerBlur
          src={item.poster}
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
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="card"
        class:is-active={activeIndex === index}
        onclick={() => setActive(index)}
      >
        <img
          src={item.poster}
          alt={item.id}
          class="card-img"
          loading="lazy"
          decoding="async"
        />

        <!-- Active card gradient overlays -->
        <div class="card-overlay" class:is-visible={activeIndex === index}></div>
        <div class="card-bottom-fade" class:is-visible={activeIndex === index}></div>

        <!-- Inactive side card dimming overlay -->
        <div class="side-card-overlay" class:is-dimmed={activeIndex !== index}></div>

        <!-- Foreground content with big title, description and buttons -->
        <div class="card-content">
          <h3 class="card-title">
            {#each item.titleLines as line, lineIndex}
              {#if lineIndex === 1 && line.includes('Jobless')}
                <span class="title-nowrap">{line}</span>
              {:else}
                {line}
              {/if}
              {#if lineIndex < item.titleLines.length - 1}<br />{/if}
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
              Watch
            </MainButton>

            <SecondButton loading={isPlanLoading && activeIndex === index} onclick={handlePlanClick}>
              {#snippet icon()}
                <Bookmark size={16} strokeWidth={2} />
              {/snippet}
              Plan to Watch
            </SecondButton>
          </div>
        </div>
      </div>
    {/each}
  </div>

  <!-- Bottom Chevron Down -->
  <div class="releases-bottom-arrow">
    <ChevronDown size={18} strokeWidth={2} class="arrow-icon" />
  </div>
</section>

<style>
  .releases-section {
    position: relative;
    width: 100%;
    height: 740px;
    background: #0c0c0c;
    overflow-x: clip;
    overflow-y: visible;
    box-sizing: border-box;
    user-select: none;
    z-index: 4;
  }

  /* Section title: 48px, #D4D4D4, PP Monument Extended Black */
  .section-title {
    position: absolute;
    top: 44px;
    left: 50%;
    transform: translateX(-50%);
    margin: 0;
    padding: 0;
    font-family: 'PP Monument Extended', sans-serif;
    font-size: 48px;
    font-weight: 900;
    color: #D4D4D4;
    white-space: nowrap;
    line-height: 1;
    z-index: 5;
    text-shadow: none;
  }

  /* Ambient Layer Blur underlay trailing down over the next section */
  .releases-underlay-blur {
    position: absolute;
    top: 70px;
    left: 0;
    width: 100%;
    height: 800px;
    pointer-events: none;
    user-select: none;
    z-index: 1;
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
    will-change: opacity;
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
    will-change: flex-grow, flex-basis;
    contain: layout paint;
    transition: flex-grow 0.65s cubic-bezier(0.22, 1, 0.36, 1),
                flex-basis 0.65s cubic-bezier(0.22, 1, 0.36, 1),
                filter var(--hover-dur) var(--ease-out);
  }

  .card:not(.is-active):hover {
    filter: brightness(1.15);
  }

  /* Active expanded card: wide clamp 690px - 840px */
  .card.is-active {
    flex: 0 0 clamp(690px, 58vw, 840px);
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
    inset: 0;
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
    left: 0;
    bottom: 0;
    width: 100%;
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
    inset: 0;
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
    padding: 24px;
    box-sizing: border-box;
    width: clamp(690px, 58vw, 840px);
    pointer-events: none;
  }

  .card.is-active .card-content {
    pointer-events: auto;
  }

  /* Big title: exact 64px, PP Monument Extended Black, #DADADA */
  .card-title {
    margin: 0 0 16px 0;
    padding: 0;
    font-family: 'PP Monument Extended', sans-serif;
    font-size: 64px;
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

  .title-nowrap {
    white-space: nowrap;
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

  /* Description: Medeo Sans Display Regular 20px, #8D8D8D, directly relative to title */
  .card-description {
    margin: 0;
    padding: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 20px;
    font-weight: 400;
    color: #8D8D8D;
    line-height: 1.35;
    word-wrap: break-word;
    text-shadow: none;
    max-width: 480px;
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
  .card-actions :global(.marlakit-btn-second) {
    opacity: 0;
    transition: opacity 0.2s ease-out, background-color 0.15s ease, transform 0.1s ease;
  }

  .card.is-active .card-actions :global(.marlakit-btn-main),
  .card.is-active .card-actions :global(.marlakit-btn-second) {
    opacity: 1;
    transition: opacity 0.38s cubic-bezier(0.16, 1, 0.3, 1) 0.42s, background-color 0.15s ease, transform 0.1s ease;
  }

  /* Centered bottom arrow */
  .releases-bottom-arrow {
    position: absolute;
    top: 676px;
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    opacity: 0.65;
    transition: opacity 0.15s ease, transform 0.15s ease;
    z-index: 5;
  }

  .releases-bottom-arrow:hover {
    opacity: 1;
    transform: translateX(-50%) translateY(2px);
  }

  :global(.arrow-icon) {
    color: #D0D0D0;
  }
</style>
