<script lang="ts">
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import Play from '@lucide/svelte/icons/play';
  import Bookmark from '@lucide/svelte/icons/bookmark';
  import preview1 from '../../../assets/preview_1.png';
  import preview2 from '../../../assets/preview_2.png';
  import preview3 from '../../../assets/preview_3.png';
  import MainButton from '../../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../../marlakit/button/SecondButton.svelte';

  let isWatchLoading = $state(false);
  let isPlanLoading = $state(false);
  let watchTimer: number | undefined;
  let planTimer: number | undefined;

  $effect(() => () => {
    window.clearTimeout(watchTimer);
    window.clearTimeout(planTimer);
  });

  function handleWatchClick() {
    window.clearTimeout(watchTimer);
    isWatchLoading = true;
    watchTimer = window.setTimeout(() => {
      isWatchLoading = false;
    }, 2000);
  }

  function handlePlanClick() {
    window.clearTimeout(planTimer);
    isPlanLoading = true;
    planTimer = window.setTimeout(() => {
      isPlanLoading = false;
    }, 2000);
  }
</script>

<section class="hero-container">
  <!-- 3-screenshot horizontal gallery strip -->
  <div class="preview-strip">
    <img src={preview1} alt="Preview 1" class="preview-img img-1" decoding="async" />
    <img src={preview2} alt="Preview 2" class="preview-img img-2" decoding="async" />
    <img src={preview3} alt="Preview 3" class="preview-img img-3" decoding="async" />
  </div>

  <!-- Hero text and action buttons with uniform 16px spacing -->
  <div class="hero-content">
    <h1 class="hero-title">The Apothecary Diaries</h1>
    <p class="hero-description">
      follows Maomao, a young woman trained as an apothecary in the red-light district, who is kidnapped and forced to work as a lowly servant in the Emperor's inner palace. Possessing an eccentric obsession with poisons, medical herbs, and biological experiments, Maomao tries to keep a low profile until a series of mysterious illnesses threatens the Emperor's heirs. Using her sharp intellect and deep knowledge of pharmacology, she secretly solves the puzzles, catches the attention of the influential eunuch Jinshi, and gets pulled into a web of court intrigue, political conspiracies, and palace mysteries...
    </p>

    <!-- Action buttons with icons -->
    <div class="hero-actions">
      <MainButton loading={isWatchLoading} onclick={handleWatchClick}>
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

  <!-- Bottom scroll indicator chevron -->
  <div class="hero-bottom-arrow">
    <ChevronDown size={18} strokeWidth={2} class="arrow-icon" />
  </div>
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
    display: flex;
    width: 100%;
    height: clamp(260px, 38vh, 310px);
    min-height: 250px;
    border-radius: var(--radius-card);
    overflow: hidden;
    box-sizing: border-box;
    flex-shrink: 0;
  }

  .preview-img {
    height: 100%;
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

  /* Text & actions container with uniform 16px spacing */
  .hero-content {
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
    margin-top: 16px;
    position: relative;
    z-index: 2;
  }

  .hero-title {
    margin: 0;
    padding: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(40px, 5.2vw, 76px);
    font-weight: 700;
    color: #ffffff;
    line-height: 1;
    letter-spacing: -0.02em;
    user-select: none;
    white-space: nowrap;
  }

  .hero-description {
    margin: 16px 0 0 0;
    padding: 0;
    max-width: 940px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 20px;
    font-weight: 400;
    color: #808080;
    line-height: 1.35;
    user-select: none;
  }

  .hero-actions {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 16px;
  }

  .hero-bottom-arrow {
    margin-top: auto;
    margin-bottom: 2px;
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