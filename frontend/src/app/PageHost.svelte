<script lang="ts">
  import { fly } from 'svelte/transition';
  import { cubicOut, cubicIn } from 'svelte/easing';
  import { getPage } from '../pages';
  import { router } from './router.svelte';

  // Page transition controller: old page fades out, new page glides in.
  // The incoming page slides WITHOUT opacity: an ancestor with opacity < 1
  // disables backdrop-filter of buttons/inputs inside it (blur would pop in).
  const OUT_MS = 160;
  const IN_MS = 380;
  const DISTANCE = 14;
</script>

<!--
  Keyed each-block: every page lives in its own slot bound to the id it was
  created with, so the leaving page keeps its content while it fades out.
  Both slots are stacked in one grid cell to avoid layout jumps.
-->
<div class="page-host">
  {#each [router.current] as id (id)}
    {@const Page = getPage(id).component}
    <div
      class="page-slot"
      in:fly|global={{ y: DISTANCE, duration: IN_MS, delay: OUT_MS, easing: cubicOut, opacity: 1 }}
      out:fly|global={{ y: -DISTANCE / 2, duration: OUT_MS, easing: cubicIn }}
    >
      <Page />
    </div>
  {/each}
</div>

<style>
  .page-host {
    display: grid;
    width: 100%;
    min-width: 0;
  }

  .page-slot {
    grid-area: 1 / 1;
    min-width: 0;
  }
</style>
