<script lang="ts">
  import { onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Header from '../section/header/Header.svelte';
  import ScrollArea from '../marlakit/scroll/ScrollArea.svelte';
  import PageHost from './PageHost.svelte';
  import PlayerDock from './PlayerDock.svelte';
  import SearchPopup from '../section/search/SearchPopup.svelte';
  import { bootState, initBootGate } from './boot.svelte';
  import { bindings, mountBindingsController } from './bindings.svelte';
  import { getSavedPageScroll, registerScrollBridge } from './router.svelte';
  import type { PageId } from '../pages';

  let scrollArea = $state<ReturnType<typeof ScrollArea> | null>(null);

  function handlePageBeforeEnter(pageId: PageId) {
    const targetTop = getSavedPageScroll(pageId);
    scrollArea?.setScrollTop(targetTop);
  }

  onMount(() => {
    initBootGate();
    const unregisterScroll = registerScrollBridge(() => scrollArea?.getScrollTop() ?? 0);
    const unmountBindings = mountBindingsController();

    return () => {
      unregisterScroll();
      unmountBindings();
    };
  });
</script>

<div
  class="viewport"
  class:is-app={bindings.isApp}
  class:is-fullscreen={bindings.isWindowSharp}
  class:is-os-pip={bindings.osPipActive}
>
  <div class="content-layer">
    <Header />
    <ScrollArea bind:this={scrollArea}>
      <PageHost onBeforeEnter={handlePageBeforeEnter} />
    </ScrollArea>
  </div>

  <!-- global popups (above everything) -->
  <PlayerDock />
  <SearchPopup />

  {#if !bootState.ready}
    <div
      class="app-boot-screen"
      role="status"
      aria-live="polite"
      aria-label="Loading application"
      out:fade={{ duration: 360, easing: cubicOut }}
    >
      <div class="app-boot-spinner" aria-hidden="true"></div>
    </div>
  {/if}
</div>

<style>
  /* Base viewport: sharp corners (0px) when opened in a web browser */
  .viewport {
    width: 100vw;
    height: 100vh;
    min-width: 858px;
    min-height: 651px;
    border-radius: 0;
    overflow: hidden;
    position: relative;
    user-select: none;
    display: flex;
    flex-direction: column;
    background-color: #0c0c0c;
    -webkit-backface-visibility: hidden;
    backface-visibility: hidden;
    transform: translateZ(0);
  }

  /* 15px rounded corners ONLY inside the native desktop app */
  .viewport.is-app {
    border-radius: 15px;
  }

  .viewport.is-os-pip {
    min-width: 0 !important;
    min-height: 0 !important;
    border-radius: 15px !important;
  }

  .viewport.is-os-pip > .content-layer {
    opacity: 0;
    pointer-events: none;
  }

  :global(body.is-player-fullscreen) .viewport.is-app,
  :global(body.is-window-fullscreen) .viewport.is-app,
  .viewport.is-app.is-fullscreen {
    border-radius: 0 !important;
    transform: none;
  }

  .content-layer {
    position: relative;
    z-index: 2;
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    background-color: #0c0c0c;
  }

  .app-boot-screen {
    position: absolute;
    inset: 0;
    z-index: 9999;
    background: #0c0c0c;
    display: flex;
    align-items: center;
    justify-content: center;
    -webkit-app-region: drag;
  }

  .app-boot-spinner {
    width: 34px;
    height: 34px;
    border-radius: 50%;
    border: 2.5px solid rgba(255, 255, 255, 0.14);
    border-top-color: #ffffff;
    animation: app-boot-spin 0.75s linear infinite;
    -webkit-app-region: no-drag;
  }

  @keyframes app-boot-spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>