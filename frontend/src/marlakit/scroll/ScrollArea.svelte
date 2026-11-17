<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    children?: Snippet;
    class?: string;
    maxHeight?: number | null;
    onscroll?: (scrollTop: number) => void;
  }

  let { children, class: className = '', maxHeight = null, onscroll }: Props = $props();

  let viewportRef = $state<HTMLDivElement | null>(null);
  let contentRef = $state<HTMLDivElement | null>(null);
  let trackRef = $state<HTMLDivElement | null>(null);
  let thumbRef = $state<HTMLDivElement | null>(null);
  let hasOverflow = $state(false);
  let isVisible = $state(false);
  let isDragging = $state(false);

  let cachedScrollHeight = 0;
  let cachedClientHeight = 0;
  let thumbHeight = 0;
  let startY = 0;
  let startScrollTop = 0;
  let hideTimer: number | undefined;
  let rafId = 0;
  let pendingShow = false;

  let wheelRafId = 0;
  let smoothTop = 0;
  let targetTop = 0;
  let lastFrameTs = 0;

  function stopWheelAnimation() {
    if (wheelRafId) {
      cancelAnimationFrame(wheelRafId);
      wheelRafId = 0;
    }
    lastFrameTs = 0;
  }

  export function getScrollTop(): number {
    return viewportRef?.scrollTop ?? 0;
  }

  export function setScrollTop(top: number) {
    if (!viewportRef) return;
    stopWheelAnimation();
    const clamped = Math.max(0, top);
    viewportRef.scrollTop = clamped;
    smoothTop = clamped;
    targetTop = clamped;
    syncMetrics();
    onscroll?.(viewportRef.scrollTop);
  }

  export function scrollToTop() {
    setScrollTop(0);
  }

  export function syncMetrics() {
    if (!viewportRef) return;
    cachedScrollHeight = viewportRef.scrollHeight;
    cachedClientHeight = viewportRef.clientHeight;
    scheduleThumbUpdate(false);
  }

  function hasInnerScrollable(target: EventTarget | null, deltaY: number): boolean {
    if (!viewportRef || !(target instanceof Element)) return false;
    let el: Element | null = target;
    while (el && el !== viewportRef) {
      if (el instanceof HTMLElement && el.scrollHeight > el.clientHeight + 1) {
        const style = getComputedStyle(el);
        const oy = style.overflowY;
        if (oy === 'auto' || oy === 'scroll') {
          const atTop = el.scrollTop <= 0;
          const atBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 1;
          if ((deltaY < 0 && !atTop) || (deltaY > 0 && !atBottom)) {
            return true;
          }
        }
      }
      el = el.parentElement;
    }
    return false;
  }

  function stepWheelScroll(now: number) {
    if (!viewportRef) {
      stopWheelAnimation();
      return;
    }
    const maxScroll = Math.max(0, viewportRef.scrollHeight - viewportRef.clientHeight);
    if (targetTop > maxScroll) targetTop = maxScroll;
    if (targetTop < 0) targetTop = 0;

    const dt = lastFrameTs > 0 ? Math.min(0.05, (now - lastFrameTs) / 1000) : 1 / 60;
    lastFrameTs = now;

    // Frame-rate independent exponential ease-out (~250ms silky settle across 60-144Hz)
    const alpha = 1 - Math.exp(-12 * dt);
    smoothTop += (targetTop - smoothTop) * alpha;

    if (Math.abs(targetTop - smoothTop) < 0.5) {
      smoothTop = targetTop;
      viewportRef.scrollTop = smoothTop;
      updateThumb(true);
      stopWheelAnimation();
      return;
    }

    viewportRef.scrollTop = smoothTop;
    updateThumb(true);
    wheelRafId = requestAnimationFrame(stepWheelScroll);
  }

  function handleWheel(e: WheelEvent) {
    if (!viewportRef || e.ctrlKey || e.metaKey || e.deltaY === 0) return;
    if (hasInnerScrollable(e.target, e.deltaY)) return;

    const maxScroll = Math.max(0, viewportRef.scrollHeight - viewportRef.clientHeight);
    if (maxScroll <= 0) return;

    e.preventDefault();

    let delta = e.deltaY;
    if (e.deltaMode === 1) delta *= 40;
    else if (e.deltaMode === 2) delta *= viewportRef.clientHeight;

    if (!wheelRafId) {
      smoothTop = viewportRef.scrollTop;
      targetTop = smoothTop;
    }
    targetTop = Math.max(0, Math.min(maxScroll, targetTop + delta));

    if (!wheelRafId) {
      lastFrameTs = 0;
      wheelRafId = requestAnimationFrame(stepWheelScroll);
    }
  }

  function armHideTimer(delayMs: number) {
    window.clearTimeout(hideTimer);
    hideTimer = window.setTimeout(() => {
      if (!isDragging) {
        isVisible = false;
      }
    }, delayMs);
  }

  function updateThumb(showIndicator: boolean) {
    if (!viewportRef) return;
    if (cachedClientHeight <= 0 || cachedScrollHeight <= 0) {
      cachedScrollHeight = viewportRef.scrollHeight;
      cachedClientHeight = viewportRef.clientHeight;
    }

    const scrollHeight = cachedScrollHeight;
    const clientHeight = cachedClientHeight;

    if (scrollHeight <= clientHeight) {
      thumbHeight = 0;
      if (hasOverflow) hasOverflow = false;
      if (isVisible) isVisible = false;
      return;
    }

    if (!hasOverflow) hasOverflow = true;

    const trackHeight = trackRef?.clientHeight || clientHeight;
    const nextThumbH = Math.max((clientHeight / scrollHeight) * trackHeight, 36);
    const availableTrack = Math.max(0, trackHeight - nextThumbH);
    const maxScroll = scrollHeight - clientHeight;
    const nextTop = maxScroll > 0 ? (viewportRef.scrollTop / maxScroll) * availableTrack : 0;

    if (thumbRef) {
      if (nextThumbH !== thumbHeight) {
        thumbRef.style.height = `${nextThumbH}px`;
      }
      thumbRef.style.transform = `translate3d(0, ${nextTop}px, 0)`;
    }
    thumbHeight = nextThumbH;

    if (showIndicator) {
      if (!isVisible) isVisible = true;
      armHideTimer(1200);
    }
  }

  function scheduleThumbUpdate(showIndicator: boolean) {
    if (showIndicator) pendingShow = true;
    if (rafId) return;
    rafId = requestAnimationFrame(() => {
      rafId = 0;
      const shouldShow = pendingShow;
      pendingShow = false;
      updateThumb(shouldShow);
    });
  }

  $effect(() => {
    const viewport = viewportRef;
    const content = contentRef;
    if (!viewport) return;

    const ro = new ResizeObserver(() => {
      cachedScrollHeight = viewport.scrollHeight;
      cachedClientHeight = viewport.clientHeight;
      scheduleThumbUpdate(false);
    });
    ro.observe(viewport);
    if (content) {
      ro.observe(content);
    }
    cachedScrollHeight = viewport.scrollHeight;
    cachedClientHeight = viewport.clientHeight;
    scheduleThumbUpdate(false);
    viewport.addEventListener('wheel', handleWheel, { passive: false });

    return () => {
      viewport.removeEventListener('wheel', handleWheel);
      stopWheelAnimation();
      ro.disconnect();
      cancelAnimationFrame(rafId);
      rafId = 0;
      window.clearTimeout(hideTimer);
      window.removeEventListener('mousemove', handleWindowMouseMove);
      window.removeEventListener('mouseup', handleWindowMouseUp);
    };
  });

  $effect(() => {
    if (hasOverflow && thumbRef) {
      scheduleThumbUpdate(false);
    }
  });

  function handleThumbMouseDown(e: MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    stopWheelAnimation();
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
    const trackHeight = trackRef?.clientHeight || clientHeight;
    const availableTrack = trackHeight - thumbHeight;
    if (availableTrack <= 0) return;
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
    stopWheelAnimation();
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const clickY = e.clientY - rect.top;
    const ratio = clickY / rect.height;
    const next = ratio * (viewportRef.scrollHeight - viewportRef.clientHeight);
    targetTop = next;
    smoothTop = next;
    viewportRef.scrollTop = next;
  }
</script>

<div
  class="marlakit-scroll-area {className}"
  style:max-height={maxHeight ? `${maxHeight}px` : undefined}
  role="region"
  aria-label="Scrollable content"
>
  <!-- Scrollable viewport hiding native scrollbar -->
  <div
    bind:this={viewportRef}
    class="marlakit-scroll-viewport"
    style:max-height={maxHeight ? `${maxHeight}px` : undefined}
    onscroll={() => {
      if (!wheelRafId) scheduleThumbUpdate(true);
      if (viewportRef) onscroll?.(viewportRef.scrollTop);
    }}
  >
    <div bind:this={contentRef} class="marlakit-scroll-content">
      {#if children}
        {@render children()}
      {/if}
    </div>
  </div>

  <!-- Custom floating minimalist scrollbar -->
  {#if hasOverflow}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      bind:this={trackRef}
      class="marlakit-scroll-track"
      class:visible={isVisible || isDragging}
      onmouseenter={() => { if (thumbHeight > 0) isVisible = true; }}
      onmouseleave={() => {
        if (!isDragging) {
          clearTimeout(hideTimer);
          hideTimer = window.setTimeout(() => {
            isVisible = false;
          }, 500);
        }
      }}
      onclick={handleTrackClick}
    >
      <div
        bind:this={thumbRef}
        class="marlakit-scroll-thumb"
        class:dragging={isDragging}
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
    overscroll-behavior: contain;
    will-change: scroll-position;
    scrollbar-width: none; /* Firefox */
    -ms-overflow-style: none; /* IE/Edge */
  }

  .marlakit-scroll-content {
    width: 100%;
    display: flex;
    flex-direction: column;
  }

  .marlakit-scroll-viewport::-webkit-scrollbar {
    display: none; /* Chrome, Safari, WebView2 */
  }

  .marlakit-scroll-track {
    position: absolute;
    top: 10px;
    right: 5px;
    bottom: 10px;
    width: 8px;
    background: transparent;
    border-radius: 999px;
    opacity: 0;
    transition: opacity 0.3s ease, width 0.15s ease;
    pointer-events: auto;
    z-index: 50;
    --wails-draggable: no-drag;
  }

  .marlakit-scroll-track.visible {
    opacity: 0.48;
  }

  .marlakit-scroll-track:hover,
  .marlakit-scroll-track:has(.dragging) {
    opacity: 0.92;
  }

  .marlakit-scroll-thumb {
    width: 3px;
    margin-left: auto;
    background: var(--mila-app-scrollbarThumb, rgba(255, 255, 255, 0.22));
    border-radius: 999px;
    cursor: pointer;
    will-change: transform;
    transition: background 0.15s ease, width 0.15s ease;
    user-select: none;
  }

  .marlakit-scroll-track:hover .marlakit-scroll-thumb,
  .marlakit-scroll-thumb.dragging {
    background: var(--mila-app-scrollbarThumbHover, rgba(255, 255, 255, 0.45));
    width: 5px;
  }
</style>
