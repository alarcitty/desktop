<script lang="ts">
  import { onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Hero from './hero/Hero.svelte';
  import Releases from './releases/Releases.svelte';
  import Recommendations from './recommendations/Recommendations.svelte';
  import { getCachedHomeFeeds, loadHomeFeeds } from '../../api/anime';

  let bgImage = $state(getCachedHomeFeeds()?.spotlight[0]?.bg ?? '');

  onMount(async () =>
  {
    const feeds = await loadHomeFeeds();
    const firstBg = feeds.spotlight[0]?.bg;
    if (firstBg && !bgImage)
    {
      bgImage = firstBg;
    }
  });
</script>

<main class="main-wrapper">
  <!-- Hero background wrapper containing the poster and all overlays -->
  <div class="hero-bg-wrapper">
    {#if bgImage}
      {#key bgImage}
        <div
          class="hero-bg-image"
          style="background-image: url('{bgImage}');"
          transition:fade={{ duration: 420, easing: cubicOut }}
        ></div>
      {/key}
    {/if}

    <!-- 1. 60% base dark dimming -->
    <div class="bg-dark-overlay"></div>

    <!-- 2. Smooth vertical vignette -->
    <div class="bg-gradient-overlay"></div>

    <!-- 3. Gradient from 0% opacity rgba(0,0,0,0) at top to solid #0C0C0C at bottom -->
    <div class="bg-fade-to-solid"></div>

    <!-- Hero content sitting above overlays -->
    <div class="hero-content-layer">
      <Hero onselectbg={(url) => { bgImage = url; }} />
    </div>
  </div>

  <!-- New Releases section -->
  <Releases />

  <!-- You might also like section -->
  <Recommendations />
</main>

<style>
  .main-wrapper {
    width: 100%;
    margin: 0;
    padding: 0;
    box-sizing: border-box;
    position: relative;
    z-index: 2;
    display: flex;
    flex-direction: column;
    background-color: #0c0c0c;
  }

  /* Extends up behind the sticky header at scroll=0, and trails down when scrolling */
  .hero-bg-wrapper {
    position: relative;
    width: 100%;
    min-height: 112vh;
    margin-top: -72px;
    padding-top: 72px;
    background-color: #0c0c0c;
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
    overflow: hidden;
  }

  .hero-bg-image {
    position: absolute;
    inset: 0;
    background-size: cover;
    background-position: center center;
    background-repeat: no-repeat;
    z-index: 0;
    pointer-events: none;
  }

  /* 60% dark dimming */
  .bg-dark-overlay {
    position: absolute;
    inset: 0;
    background-color: rgba(0, 0, 0, 0.6);
    z-index: 1;
    pointer-events: none;
  }

  /* smooth vertical vignette */
  .bg-gradient-overlay {
    position: absolute;
    inset: 0;
    background: linear-gradient(
      180deg,
      rgba(0, 0, 0, 0.5) 0%,
      rgba(0, 0, 0, 0.1) 30%,
      rgba(0, 0, 0, 0.3) 70%,
      rgba(0, 0, 0, 0.8) 100%
    );
    z-index: 2;
    pointer-events: none;
  }

  /* user's requested gradient: from rgba(0,0,0,0) top to solid #0C0C0C bottom */
  .bg-fade-to-solid {
    position: absolute;
    inset: 0;
    background: linear-gradient(
      180deg,
      rgba(0, 0, 0, 0) 0%,
      rgba(12, 12, 12, 0) 40%,
      rgba(12, 12, 12, 0.6) 70%,
      rgba(12, 12, 12, 0.95) 90%,
      #0c0c0c 100%
    );
    z-index: 3;
    pointer-events: none;
  }

  .hero-content-layer {
    position: relative;
    z-index: 4;
    width: 100%;
    flex: 1;
    display: flex;
    flex-direction: column;
  }
</style>