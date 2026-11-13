<script lang="ts">
  import Header from '../section/header/Header.svelte';
  import ScrollArea from '../marlakit/scroll/ScrollArea.svelte';
  import PageHost from './PageHost.svelte';
  import SearchPopup from '../section/search/SearchPopup.svelte';
  import PlayerModal from '../section/player/PlayerModal.svelte';
  import { router } from './router.svelte';

  let scrollArea = $state<ReturnType<typeof ScrollArea> | null>(null);

  // every page starts from the top
  $effect(() => {
    router.current;
    scrollArea?.scrollToTop();
  });

  // detect if running inside native desktop app (Wails / WebView2)
  let isApp = $state(false);

  $effect(() => {
    // @ts-ignore
    isApp = typeof window !== 'undefined' && !!(window.chrome?.webview || window.runtime);
  });
</script>

<div class="viewport" class:is-app={isApp}>
  <div class="content-layer">
    <ScrollArea bind:this={scrollArea}>
      <Header />
      <PageHost />
    </ScrollArea>
  </div>

  <!-- global popups (above everything) -->
  <SearchPopup />
  <PlayerModal />
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
</style>