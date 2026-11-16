<script lang="ts">
  import Hero from './hero/Hero.svelte';
  import Releases from './releases/Releases.svelte';
  import Recommendations from './recommendations/Recommendations.svelte';
  import Credits from './credits/Credits.svelte';

  let creditsOpen = $state(false);
</script>

<main class="main-wrapper">
  <Hero />

  <!-- New Releases section -->
  <Releases />

  <!-- You might also like section -->
  <Recommendations
    {creditsOpen}
    onToggleCredits={() => {
      creditsOpen = !creditsOpen;
    }}
  />

  <!-- Smoothly expanding bottom credits section -->
  <Credits open={creditsOpen} />
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
    background-color: var(--mila-app-canvas, #0c0c0c);
    transition: background-color 360ms cubic-bezier(0.22, 1, 0.36, 1);
  }
</style>