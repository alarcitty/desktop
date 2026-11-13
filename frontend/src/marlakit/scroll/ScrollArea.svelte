<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    children?: Snippet;
    class?: string;
  }

  let { children, class: className = '' }: Props = $props();

  let viewportRef = $state<HTMLDivElement | null>(null);
  let thumbHeight = $state(0);
  let thumbTop = $state(0);
  let isVisible = $state(false);
  let isDragging = $state(false);

  let startY = 0;
  let startScrollTop = 0;
  let hideTimer: number | undefined;

  export function scrollToTop() {
    if (viewportRef) {
      viewportRef.scrollTop = 0;
    }
  }

  function updateThumb() {
    if (!viewportRef) return;
    const { scrollTop, scrollHeight, clientHeight } = viewportRef;

    if (scrollHeight <= clientHeight) {
      thumbHeight = 0;
      return;
    }

    const ratio = clientHeight / scrollHeight;
    thumbHeight = Math.max(ratio * clientHeight, 36);
    const availableTrack = clientHeight - thumbHeight;
    const maxScroll = scrollHeight - clientHeight;
    thumbTop = (scrollTop / maxScroll) * availableTrack;

    isVisible = true;
    clearTimeout(hideTimer);
    hideTimer = window.setTimeout(() => {
      if (!isDragging) {
        isVisible = false;
      }
    }, 1200);
  }

  function handleThumbMouseDown(e: MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    isDragging = true;
    startY = e.clientY;
    if (viewportRef) {
      startScrollTop = viewportRef.scrollTop;
    }
    window.addEventListener('mousemove', handleWindowMouseMove);
    window.addEventListener('mouseup', handleWindowMouseUp);
  }

  function handleWindowMouseMove(e: MouseEvent) {
    if (!isDragging || !viewportRef) return;
    const deltaY = e.clientY - startY;
    const { scrollHeight, clientHeight } = viewportRef;
    const availableTrack = clientHeight - thumbHeight;
    const maxScroll = scrollHeight - clientHeight;
    viewportRef.scrollTop = startScrollTop + (deltaY / availableTrack) * maxScroll;
  }

  function handleWindowMouseUp() {
    isDragging = false;
    window.removeEventListener('mousemove', handleWindowMouseMove);
    window.removeEventListener('mouseup', handleWindowMouseUp);
    clearTimeout(hideTimer);
    hideTimer = window.setTimeout(() => {
      isVisible = false;
    }, 800);
  }

  function handleTrackClick(e: MouseEvent) {
    if (!viewportRef || e.target !== e.currentTarget) return;
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const clickY = e.clientY - rect.top;
    const ratio = clickY / rect.height;
    viewportRef.scrollTop = ratio * (viewportRef.scrollHeight - viewportRef.clientHeight);
  }
</script>

<div
  class="marlakit-scroll-area {className}"
  onmouseenter={() => { if (thumbHeight > 0) isVisible = true; }}
  onmouseleave={() => { if (!isDragging) isVisible = false; }}
  role="region"
  aria-label="Scrollable content"
>
  <!-- Scrollable viewport hiding native scrollbar -->
  <div
    bind:this={viewportRef}
    class="marlakit-scroll-viewport"
    onscroll={updateThumb}
  >
    {#if children}
      {@render children()}
    {/if}
  </div>

  <!-- Custom floating minimalist scrollbar -->
  {#if thumbHeight > 0}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="marlakit-scroll-track"
      class:visible={isVisible || isDragging}
      onclick={handleTrackClick}
    >
      <div
        class="marlakit-scroll-thumb"
        class:dragging={isDragging}
        style="height: {thumbHeight}px; transform: translateY({thumbTop}px);"
        onmousedown={handleThumbMouseDown}
      ></div>
    </div>
  {/if}
</div>

<style>
  .marlakit-scroll-area {
    position: relative;
    width: 100%;
    height: 100%;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .marlakit-scroll-viewport {
    width: 100%;
    height: 100%;
    overflow-y: auto;
    overflow-x: hidden;
    scrollbar-width: none; /* Firefox */
    -ms-overflow-style: none; /* IE/Edge */
  }

  .marlakit-scroll-viewport::-webkit-scrollbar {
    display: none; /* Chrome, Safari, WebView2 */
  }

  .marlakit-scroll-track {
    position: absolute;
    top: 4px;
    right: 4px;
    bottom: 4px;
    width: 8px;
    background: transparent;
    border-radius: 999px;
    opacity: 0;
    transition: opacity 0.25s ease, width 0.15s ease;
    pointer-events: none;
    z-index: 50;
    --wails-draggable: no-drag;
  }

  .marlakit-scroll-track.visible {
    opacity: 1;
    pointer-events: auto;
  }

  .marlakit-scroll-thumb {
    width: 5px;
    margin-left: auto;
    background: rgba(255, 255, 255, 0.22);
    border-radius: 999px;
    cursor: pointer;
    transition: background 0.15s ease, width 0.15s ease;
    user-select: none;
  }

  .marlakit-scroll-track:hover .marlakit-scroll-thumb,
  .marlakit-scroll-thumb.dragging {
    background: rgba(255, 255, 255, 0.45);
    width: 6px;
  }
</style>
