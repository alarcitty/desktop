<script lang="ts">
  import { untrack } from 'svelte';
  import { pages, getPage, type PageId } from '../pages';
  import { router } from './router.svelte';

  interface Props {
    onBeforeEnter?: (pageId: PageId) => void;
  }

  let { onBeforeEnter }: Props = $props();

  const CACHED_PAGES = new Set<PageId>(pages.map((p) => p.id));
  const LEAVE_MS = 130;
  const ENTER_SETTLE_MS = 300;

  // Pre-mount all main tabs at startup (hidden tabs use visibility/height:0 rather than
  // display:none) so Blink/WebView2 retains layout boxes and GPU image layers with zero tab-switch stutter.
  let visited = $state<PageId[]>(Array.from(CACHED_PAGES));
  let currentTracked: PageId = router.current;
  let currentRouteKeyTracked: string = router.routeKey;
  let leavingId = $state<PageId | null>(null);
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
    if (CACHED_PAGES.has(next) && !visited.includes(next)) {
      visited = [...visited, next];
    }
    if (next !== currentTracked) {
      const token = ++switchToken;
      clearTimers();
      leavingId = currentTracked;
      currentTracked = next;
      currentRouteKeyTracked = nextKey;
      hasSwitched = true;
      enterReady = false;
      isTransitioning = true;

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

  {#if !CACHED_PAGES.has(router.current)}
    {#key router.current}
      {@const Page = getPage(router.current).component}
      <div
        class="page-slot is-active is-animating"
        class:is-pre-enter={!enterReady}
        class:is-entered={enterReady}
      >
        <Page />
      </div>
    {/key}
  {/if}
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
    will-change: opacity, transform;
    backface-visibility: hidden;
  }

  .page-slot.is-pre-enter {
    opacity: 0;
    transform: translate3d(0, 10px, 0);
    pointer-events: none;
  }

  .page-slot.is-entered {
    opacity: 1;
    transform: translate3d(0, 0, 0);
    transition:
      opacity 0.26s cubic-bezier(0.22, 1, 0.36, 1),
      transform 0.3s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .page-slot.is-leaving {
    position: absolute;
    inset: 0 0 auto 0;
    z-index: 1;
    pointer-events: none;
    will-change: opacity, transform;
    backface-visibility: hidden;
    animation: pageLeave 0.13s cubic-bezier(0.4, 0, 1, 1) both;
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
</style>
