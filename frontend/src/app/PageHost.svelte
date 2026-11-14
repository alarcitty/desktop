<script lang="ts">
  import { pages, getPage, type PageId } from '../pages';
  import { router } from './router.svelte';

  const CACHED_PAGES = new Set<PageId>(pages.map((p) => p.id));
  const LEAVE_MS = 280;

  // Keep visited main tabs mounted once opened so switching Home <-> Library <-> News
  // does not tear down heavy DOM trees or re-decode posters, while animating smoothly.
  let visited = $state<PageId[]>(
    CACHED_PAGES.has(router.current) ? [router.current] : ['home']
  );
  let currentTracked: PageId = router.current;
  let leavingId = $state<PageId | null>(null);
  let hasSwitched = $state(false);
  let leaveTimer: number | undefined;

  $effect(() => {
    const next = router.current;
    if (CACHED_PAGES.has(next) && !visited.includes(next)) {
      visited = [...visited, next];
    }
    if (next !== currentTracked) {
      window.clearTimeout(leaveTimer);
      leavingId = currentTracked;
      currentTracked = next;
      hasSwitched = true;
      leaveTimer = window.setTimeout(() => {
        leavingId = null;
      }, LEAVE_MS);
    }
  });

  $effect(() => () => {
    window.clearTimeout(leaveTimer);
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
      class:is-animated={active && hasSwitched}
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
      <div class="page-slot is-active is-animated">
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
    display: none;
  }

  .page-slot.is-active {
    position: relative;
    z-index: 2;
  }

  .page-slot.is-animated {
    animation: pageEnter 0.46s cubic-bezier(0.16, 1, 0.3, 1) 0.06s both;
  }

  .page-slot.is-leaving {
    position: absolute;
    inset: 0 0 auto 0;
    z-index: 1;
    pointer-events: none;
    animation: pageLeave 0.28s cubic-bezier(0.4, 0, 0.2, 1) both;
  }

  @keyframes pageEnter {
    from {
      opacity: 0;
      transform: translate3d(0, 16px, 0);
    }
    to {
      opacity: 1;
      transform: translate3d(0, 0, 0);
    }
  }

  @keyframes pageLeave {
    from {
      opacity: 1;
      transform: translate3d(0, 0, 0);
    }
    to {
      opacity: 0;
      transform: translate3d(0, -10px, 0);
    }
  }
</style>
