<script lang="ts">
  import type { Snippet } from 'svelte';
  import { tick, untrack } from 'svelte';

  interface Rect
  {
    left: number;
    top: number;
    width: number;
    height: number;
  }

  interface Props
  {
    open: boolean;
    // returns the live rect of the element the popup morphs out of / back into
    getAnchor: () => Rect | null;
    width?: number;
    height?: number;
    top?: number;
    duration?: number;
    onclose?: () => void;
    onclosed?: () => void;
    children?: Snippet<[boolean]>;
  }

  let {
    open,
    getAnchor,
    width = 680,
    height = 320,
    top = 96,
    duration = 560,
    onclose,
    onclosed,
    children
  }: Props = $props();

  let mounted = $state(false);
  let expanded = $state(false);
  let from = $state<Rect>({ left: 0, top: 0, width: 320, height: 38 });
  let vw = $state(1024);

  let timer: number | undefined;
  let raf1 = 0;
  let raf2 = 0;

  const panelWidth = $derived(Math.min(width, vw - 48));

  const panelStyle = $derived(
    expanded
      ? `left:${(vw - panelWidth) / 2}px;top:${top}px;width:${panelWidth}px;height:${height}px;`
      : `left:${from.left}px;top:${from.top}px;width:${from.width}px;height:${from.height}px;`
  );

  function snapshot()
  {
    const r = getAnchor();
    if (r)
    {
      from = r;
    }
  }

  async function show()
  {
    window.clearTimeout(timer);
    cancelAnimationFrame(raf1);
    cancelAnimationFrame(raf2);
    snapshot();
    expanded = false;
    mounted = true;
    await tick();
    // two frames: make sure the collapsed state is painted so the transition runs
    raf1 = requestAnimationFrame(() =>
    {
      raf2 = requestAnimationFrame(() =>
      {
        expanded = true;
      });
    });
  }

  function hide()
  {
    if (!mounted)
    {
      return;
    }
    cancelAnimationFrame(raf1);
    cancelAnimationFrame(raf2);
    snapshot();
    expanded = false;
    window.clearTimeout(timer);
    timer = window.setTimeout(() =>
    {
      mounted = false;
      onclosed?.();
    }, duration + 40);
  }

  $effect(() =>
  {
    const isOpen = open;
    untrack(() => (isOpen ? show() : hide()));
  });

  function handleKeydown(e: KeyboardEvent)
  {
    if (open && e.key === 'Escape')
    {
      e.preventDefault();
      onclose?.();
    }
  }
</script>

<svelte:window bind:innerWidth={vw} onkeydown={handleKeydown} />

{#if mounted}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="popup-backdrop"
    class:visible={expanded}
    style:--popup-duration="{duration}ms"
    onclick={() => onclose?.()}
  ></div>

  <div
    class="popup-panel"
    class:expanded
    style={panelStyle}
    style:--popup-duration="{duration}ms"
    role="dialog"
    aria-modal="true"
  >
    {@render children?.(expanded)}
  </div>
{/if}

<style>
  /* Blur is animated through the backdrop-filter value itself (not through
     opacity), so it fades in smoothly instead of popping at the end. */
  .popup-backdrop
  {
    position: fixed;
    inset: 0;
    z-index: 1000;
    background: rgba(0, 0, 0, 0);
    -webkit-backdrop-filter: blur(0px);
    backdrop-filter: blur(0px);
    transition:
      background-color var(--popup-duration) var(--ease-out),
      -webkit-backdrop-filter var(--popup-duration) var(--ease-out),
      backdrop-filter var(--popup-duration) var(--ease-out);
  }

  .popup-backdrop.visible
  {
    background: rgba(0, 0, 0, 0.4);
    -webkit-backdrop-filter: blur(14px);
    backdrop-filter: blur(14px);
  }

  .popup-panel
  {
    position: fixed;
    z-index: 1001;
    box-sizing: border-box;
    overflow: hidden;
    padding: 0;
    border-radius: 9px;
    background: rgba(16, 16, 16, 0);
    -webkit-backdrop-filter: blur(0px);
    backdrop-filter: blur(0px);
    box-shadow: 0 0 0 1px rgba(255, 255, 255, 0), 0 30px 80px rgba(0, 0, 0, 0);
    transition:
      left var(--popup-duration) var(--ease-out),
      top var(--popup-duration) var(--ease-out),
      width var(--popup-duration) var(--ease-out),
      height var(--popup-duration) var(--ease-out),
      padding var(--popup-duration) var(--ease-out),
      border-radius var(--popup-duration) var(--ease-out),
      background-color var(--popup-duration) var(--ease-out),
      -webkit-backdrop-filter var(--popup-duration) var(--ease-out),
      backdrop-filter var(--popup-duration) var(--ease-out),
      box-shadow var(--popup-duration) var(--ease-out);
  }

  .popup-panel.expanded
  {
    padding: 14px;
    border-radius: 22px;
    background: rgba(18, 18, 18, 0.55);
    -webkit-backdrop-filter: blur(28px);
    backdrop-filter: blur(28px);
    box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.08), 0 30px 80px rgba(0, 0, 0, 0.5);
  }
</style>
