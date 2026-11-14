<script lang="ts">
  import Hero from './hero/Hero.svelte';
  import Releases from './releases/Releases.svelte';
  import Recommendations from './recommendations/Recommendations.svelte';
</script>

<main class="main-wrapper">
  <Hero />

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
</style>