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
    width = 760,
    height = 320,
    top = 96,
    duration = 480,
    onclose,
    onclosed,
    children
  }: Props = $props();

  let mounted = $state(false);
  let expanded = $state(false);
  let fullyExpanded = $state(false);
  let from = $state<Rect>({ left: 0, top: 0, width: 320, height: 38 });
  let vw = $state(1024);
  let panelEl = $state<HTMLDivElement | null>(null);

  let timer: number | undefined;
  let raf1 = 0;

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
    snapshot();
    fullyExpanded = false;
    expanded = false;
    mounted = true;
    await tick();
    // Force synchronous layout flush on the collapsed anchor geometry so the morph transition starts on the very next frame
    void panelEl?.getBoundingClientRect();
    raf1 = requestAnimationFrame(() =>
    {
      expanded = true;
      timer = window.setTimeout(() =>
      {
        if (expanded)
        {
          fullyExpanded = true;
        }
      }, duration);
    });
  }

  function hide()
  {
    if (!mounted)
    {
      return;
    }
    cancelAnimationFrame(raf1);
    snapshot();
    fullyExpanded = false;
    expanded = false;
    window.clearTimeout(timer);
    timer = window.setTimeout(() =>
    {
      mounted = false;
      onclosed?.();
    }, duration + 16);
  }

  $effect(() =>
  {
    const isOpen = open;
    untrack(() => (isOpen ? show() : hide()));
  });

  $effect(() =>
  {
    return () =>
    {
      window.clearTimeout(timer);
      cancelAnimationFrame(raf1);
    };
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
    bind:this={panelEl}
    class="popup-panel"
    class:expanded
    class:fully-expanded={fullyExpanded}
    style={panelStyle}
    style:--popup-duration="{duration}ms"
    role="dialog"
    aria-modal="true"
  >
    {@render children?.(expanded)}
  </div>
{/if}

<style>
  /* Fixed backdrop-filter + opacity fade avoids rebuilding fullscreen Gaussian blur every frame. */
  .popup-backdrop
  {
    position: fixed;
    inset: 0;
    z-index: 1000;
    background: var(--mila-popup-backdrop, rgba(5, 5, 7, 0.56));
    -webkit-backdrop-filter: blur(24px) saturate(1.15);
    backdrop-filter: blur(24px) saturate(1.15);
    opacity: 0;
    will-change: opacity;
    transition: opacity var(--popup-duration) cubic-bezier(0.22, 1, 0.36, 1);
  }

  .popup-backdrop.visible
  {
    opacity: 1;
  }

  .popup-panel
  {
    position: fixed;
    z-index: 1001;
    box-sizing: border-box;
    overflow: hidden;
    contain: layout style;
    will-change: left, top, width, height, padding, border-radius, background-color;
    padding: 0;
    border: none;
    border-radius: 9px;
    opacity: 1;
    background: var(--mila-popup-bg, rgba(18, 18, 22, 0.94));
    color: var(--mila-text-primary, #ffffff);
    box-shadow: 0 36px 100px rgba(0, 0, 0, 0);
    transition:
      left var(--popup-duration) cubic-bezier(0.22, 1, 0.36, 1),
      top var(--popup-duration) cubic-bezier(0.22, 1, 0.36, 1),
      width var(--popup-duration) cubic-bezier(0.22, 1, 0.36, 1),
      height var(--popup-duration) cubic-bezier(0.22, 1, 0.36, 1),
      padding var(--popup-duration) cubic-bezier(0.22, 1, 0.36, 1),
      border-radius var(--popup-duration) cubic-bezier(0.22, 1, 0.36, 1),
      background-color var(--popup-duration) cubic-bezier(0.22, 1, 0.36, 1),
      box-shadow var(--popup-duration) cubic-bezier(0.22, 1, 0.36, 1);
  }

  .popup-panel.expanded
  {
    padding: 16px;
    border-radius: var(--radius-card, 16px);
    background: var(--mila-popup-bgExpanded, rgba(12, 12, 15, 0.74));
    -webkit-backdrop-filter: blur(36px) saturate(1.28);
    backdrop-filter: blur(36px) saturate(1.28);
    box-shadow: var(--mila-popup-shadow, 0 36px 100px rgba(0, 0, 0, 0.85));
  }

  .popup-panel.fully-expanded
  {
    overflow: visible;
  }
</style>
