<script lang="ts">
  import { untrack, type Component } from 'svelte';
  import { pages, getPage, loadAnimePage, loadPlayerPage, type PageId } from '../pages';
  import { router } from './router.svelte';

  interface Props {
    onBeforeEnter?: (pageId: PageId) => void;
  }

  let { onBeforeEnter }: Props = $props();

  const CACHED_PAGES = new Set<PageId>(pages.map((p) => p.id));
  const DETAIL_PAGES: PageId[] = ['anime', 'player'];
  let animePageComponent = $state<Component | null>(loadAnimePage.getLoaded());
  let playerPageComponent = $state<Component | null>(loadPlayerPage.getLoaded());
  const LEAVE_MS = 130;
  const MORPH_MS = 560;
  const ENTER_SETTLE_MS = 300;

  // Pre-mount all main tabs at startup (hidden tabs use visibility/height:0 rather than
  // display:none) so Blink/WebView2 retains layout boxes and GPU image layers with zero tab-switch stutter.
  let visited = $state<PageId[]>(Array.from(CACHED_PAGES));
  let currentTracked: PageId = router.current;
  let currentRouteKeyTracked: string = router.routeKey;
  let leavingId = $state<PageId | null>(null);
  let morphMode = $state<'forward' | 'back' | null>(null);
  let hasSwitched = $state(false);
  let enterReady = $state(true);
  let isTransitioning = $state(false);
  let leaveTimer: number | undefined;
  let settleTimer: number | undefined;
  let enterRaf = 0;
  let switchToken = 0;

  function clearTimers() {
    window.clearTimeout(leaveTimer);
    window.clearTimeout(settleTimer);
    if (enterRaf) {
      window.cancelAnimationFrame(enterRaf);
      enterRaf = 0;
    }
  }

  $effect(() => {
    const next = router.current;
    const nextKey = router.routeKey;
    if (next === 'anime' && !animePageComponent) {
      void loadAnimePage.load().then((Comp) => {
        animePageComponent = Comp;
      });
    }
    if (next === 'player' && !playerPageComponent) {
      void loadPlayerPage.load().then((Comp) => {
        playerPageComponent = Comp;
      });
    }
    if (CACHED_PAGES.has(next) && !visited.includes(next)) {
      visited = [...visited, next];
    }
    if (next !== currentTracked) {
      const from = currentTracked;
      const isAnimeToPlayer = from === 'anime' && next === 'player';
      const isPlayerToAnime = from === 'player' && next === 'anime';
      const nextMorph = isAnimeToPlayer ? 'forward' : isPlayerToAnime ? 'back' : null;

      const token = ++switchToken;
      clearTimers();
      leavingId = from;
      morphMode = nextMorph;
      currentTracked = next;
      currentRouteKeyTracked = nextKey;
      hasSwitched = true;
      enterReady = false;
      isTransitioning = true;

      if (nextMorph) {
        untrack(() => onBeforeEnter?.(next));
        enterReady = true;
        settleTimer = window.setTimeout(() => {
          if (token === switchToken) {
            leavingId = null;
            morphMode = null;
            isTransitioning = false;
          }
        }, MORPH_MS);
      } else {
        leaveTimer = window.setTimeout(() => {
          if (token !== switchToken) return;
          leavingId = null;
          untrack(() => onBeforeEnter?.(next));
          enterRaf = window.requestAnimationFrame(() => {
            enterRaf = 0;
            if (token !== switchToken) return;
            enterReady = true;
            settleTimer = window.setTimeout(() => {
              if (token === switchToken) {
                isTransitioning = false;
              }
            }, ENTER_SETTLE_MS);
          });
        }, LEAVE_MS);
      }
    } else if (nextKey !== currentRouteKeyTracked) {
      currentRouteKeyTracked = nextKey;
      untrack(() => onBeforeEnter?.(next));
    }
  });

  $effect(() => () => {
    clearTimers();
  });
</script>

<div class="page-host">
  {#each visited as id (id)}
    {@const Page = getPage(id).component}
    {@const active = router.current === id}
    {@const leaving = leavingId === id && !active}
    <div
      class="page-slot"
      class:is-active={active}
      class:is-animating={active && hasSwitched && isTransitioning}
      class:is-pre-enter={active && hasSwitched && !enterReady}
      class:is-entered={active && (!hasSwitched || enterReady)}
      class:is-leaving={leaving}
      class:is-hidden={!active && !leaving}
      aria-hidden={!active}
    >
      <Page />
    </div>
  {/each}

  {#each DETAIL_PAGES as detailId (detailId)}
    {@const active = router.current === detailId}
    {@const leaving = leavingId === detailId && !active}
    {@const DetailPage = detailId === 'anime' ? animePageComponent : playerPageComponent}
    {#if (active || leaving) && DetailPage}
      <div
        class="page-slot"
        class:is-active={active}
        class:is-animating={active && hasSwitched && isTransitioning}
        class:is-pre-enter={active && hasSwitched && !enterReady}
        class:is-entered={active && (!hasSwitched || enterReady)}
        class:is-leaving={leaving}
        class:is-morph-forward={morphMode === 'forward'}
        class:is-morph-back={morphMode === 'back'}
        aria-hidden={!active}
      >
        <DetailPage />
      </div>
    {/if}
  {/each}
</div>

<style>
  .page-host {
    position: relative;
    width: 100%;
    min-width: 0;
  }

  .page-slot {
    min-width: 0;
  }

  .page-slot.is-hidden {
    position: absolute;
    inset: 0 0 auto 0;
    height: 0;
    overflow: hidden;
    visibility: hidden;
    opacity: 0;
    pointer-events: none;
    contain: strict;
  }

  .page-slot.is-active {
    position: relative;
    z-index: 2;
  }

  .page-slot.is-animating {
    will-change: opacity, transform, filter;
    backface-visibility: hidden;
  }

  .page-slot.is-pre-enter {
    opacity: 0;
    transform: translate3d(0, 10px, 0);
    pointer-events: none;
  }

  .page-slot.is-pre-enter.is-morph-forward,
  .page-slot.is-pre-enter.is-morph-back {
    opacity: 0;
    transform: none;
  }

  .page-slot.is-entered {
    opacity: 1;
    filter: blur(0px);
    transform: none;
  }

  .page-slot.is-animating.is-entered:not(.is-morph-forward):not(.is-morph-back) {
    transform: translate3d(0, 0, 0);
    transition:
      opacity 0.26s cubic-bezier(0.22, 1, 0.36, 1),
      transform 0.3s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .page-slot.is-active.is-animating.is-morph-forward {
    animation: pageMorphEnterUp 0.56s cubic-bezier(0.16, 1, 0.3, 1) both;
  }

  .page-slot.is-active.is-animating.is-morph-back {
    animation: pageMorphEnterDown 0.52s cubic-bezier(0.16, 1, 0.3, 1) both;
  }

  .page-slot.is-leaving {
    position: absolute;
    inset: 0 0 auto 0;
    z-index: 1;
    pointer-events: none;
    will-change: opacity, transform, filter;
    backface-visibility: hidden;
    animation: pageLeave 0.13s cubic-bezier(0.4, 0, 1, 1) both;
  }

  .page-slot.is-leaving.is-morph-forward {
    animation: pageMorphLeaveUp 0.48s cubic-bezier(0.16, 1, 0.3, 1) both;
  }

  .page-slot.is-leaving.is-morph-back {
    animation: pageMorphLeaveDown 0.48s cubic-bezier(0.16, 1, 0.3, 1) both;
  }

  @keyframes pageLeave {
    from {
      opacity: 1;
      transform: translate3d(0, 0, 0);
    }
    to {
      opacity: 0;
      transform: translate3d(0, -6px, 0);
    }
  }

  @keyframes pageMorphEnterUp {
    from {
      opacity: 0;
      filter: blur(20px);
      transform: translate3d(0, 44px, 0) scale(0.985);
    }
    to {
      opacity: 1;
      filter: blur(0px);
      transform: translate3d(0, 0, 0) scale(1);
    }
  }

  @keyframes pageMorphEnterDown {
    from {
      opacity: 0;
      filter: blur(18px);
      transform: translate3d(0, -32px, 0) scale(0.988);
    }
    to {
      opacity: 1;
      filter: blur(0px);
      transform: translate3d(0, 0, 0) scale(1);
    }
  }

  @keyframes pageMorphLeaveUp {
    from {
      opacity: 1;
      filter: blur(0px);
      transform: translate3d(0, 0, 0) scale(1);
    }
    to {
      opacity: 0;
      filter: blur(20px);
      transform: translate3d(0, -44px, 0) scale(0.985);
    }
  }

  @keyframes pageMorphLeaveDown {
    from {
      opacity: 1;
      filter: blur(0px);
      transform: translate3d(0, 0, 0) scale(1);
    }
    to {
      opacity: 0;
      filter: blur(18px);
      transform: translate3d(0, 38px, 0) scale(0.988);
    }
  }
</style>
