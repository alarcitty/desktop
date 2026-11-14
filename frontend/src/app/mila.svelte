<script lang="ts">
  import { onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Header from '../section/header/Header.svelte';
  import ScrollArea from '../marlakit/scroll/ScrollArea.svelte';
  import PageHost from './PageHost.svelte';
  import SearchPopup from '../section/search/SearchPopup.svelte';
  import { bootState, initBootGate } from './boot.svelte';
  import { router } from './router.svelte';
  import type { PageId } from '../pages';

  let scrollArea = $state<ReturnType<typeof ScrollArea> | null>(null);
  const scrollByPage = new Map<PageId, number>();
  let activePage: PageId = router.current;

  // Save outgoing tab scroll position cleanly; restore incoming tab scroll only after
  // the outgoing fade-out completes so leaving content never jumps mid-animation.
  $effect(() => {
    const next = router.current;
    if (next !== activePage) {
      if (activePage !== 'anime' && scrollArea) {
        scrollByPage.set(activePage, scrollArea.getScrollTop());
      }
      activePage = next;
    }
  });

  function handlePageBeforeEnter(pageId: PageId) {
    const targetTop = pageId === 'anime' ? 0 : (scrollByPage.get(pageId) ?? 0);
    scrollArea?.setScrollTop(targetTop);
  }

  // detect if running inside native desktop app (Wails / WebView2)
  let isApp = $state(false);

  $effect(() => {
    // @ts-ignore
    isApp = typeof window !== 'undefined' && !!(window.chrome?.webview || window.runtime);
  });

  onMount(() => {
    initBootGate();
  });
</script>

<div class="viewport" class:is-app={isApp}>
  <div class="content-layer">
    <Header />
    <ScrollArea bind:this={scrollArea}>
      <PageHost onBeforeEnter={handlePageBeforeEnter} />
    </ScrollArea>
  </div>

  <!-- global popups (above everything) -->
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

  :global(body.is-player-fullscreen) .viewport.is-app {
    border-radius: 0;
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