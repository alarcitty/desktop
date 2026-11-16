<script lang="ts">
  interface Props {
    hue?: number;
    ariaLabel?: string;
    disabled?: boolean;
    class?: string;
    onchange?: (hue: number) => void;
    oncommit?: (hue: number) => void;
  }

  let {
    hue = $bindable(215),
    ariaLabel = 'Custom color hue',
    disabled = false,
    class: className = '',
    onchange,
    oncommit
  }: Props = $props();

  let trackEl = $state<HTMLDivElement | null>(null);
  let dragging = $state(false);

  const clampedHue = $derived(Math.max(0, Math.min(360, Number.isFinite(hue) ? hue : 0)));
  const pct = $derived((clampedHue / 360) * 100);

  function applyHue(raw: number) {
    const next = Math.round(Math.max(0, Math.min(360, raw)));
    if (next !== hue) {
      hue = next;
    }
    onchange?.(next);
  }

  function updateFromClientX(clientX: number) {
    if (!trackEl || disabled) return;
    const rect = trackEl.getBoundingClientRect();
    if (rect.width <= 0) return;
    const ratio = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
    applyHue(ratio * 360);
  }

  function handlePointerDown(e: PointerEvent) {
    if (disabled || !trackEl || e.button !== 0) return;
    e.stopPropagation();
    dragging = true;
    trackEl.setPointerCapture(e.pointerId);
    updateFromClientX(e.clientX);
  }

  function handlePointerMove(e: PointerEvent) {
    if (!dragging) return;
    updateFromClientX(e.clientX);
  }

  function handlePointerUp(e: PointerEvent) {
    if (!dragging || !trackEl) return;
    updateFromClientX(e.clientX);
    dragging = false;
    try {
      trackEl.releasePointerCapture(e.pointerId);
    } catch {
      // Ignore if pointer capture already released
    }
    oncommit?.(hue);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (disabled) return;
    if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') {
      e.preventDefault();
      e.stopPropagation();
      applyHue(clampedHue - 6);
      oncommit?.(hue);
    } else if (e.key === 'ArrowRight' || e.key === 'ArrowUp') {
      e.preventDefault();
      e.stopPropagation();
      applyHue(clampedHue + 6);
      oncommit?.(hue);
    }
  }
</script>

<div
  bind:this={trackEl}
  class="marla-rgb-line {className}"
  class:is-dragging={dragging}
  class:is-disabled={disabled}
  role="slider"
  tabindex={disabled ? -1 : 0}
  aria-label={ariaLabel}
  aria-valuemin={0}
  aria-valuemax={360}
  aria-valuenow={clampedHue}
  aria-disabled={disabled}
  onpointerdown={handlePointerDown}
  onpointermove={handlePointerMove}
  onpointerup={handlePointerUp}
  onpointercancel={handlePointerUp}
  onkeydown={handleKeydown}
>
  <div class="marla-rgb-rail"></div>
  <div class="marla-rgb-thumb" style:left="{pct}%"></div>
</div>

<style>
  .marla-rgb-line {
    position: relative;
    width: 100%;
    height: 28px;
    display: flex;
    align-items: center;
    cursor: pointer;
    touch-action: none;
    outline: none;
    user-select: none;
    -webkit-app-region: no-drag;
  }

  .marla-rgb-line.is-disabled {
    opacity: 0.45;
    cursor: default;
  }

  .marla-rgb-rail {
    position: relative;
    width: 100%;
    height: 12px;
    border-radius: 999px;
    background: linear-gradient(
      90deg,
      #ff2d2d 0%,
      #ffea2d 17%,
      #2dff4e 33%,
      #2dffff 50%,
      #2d5bff 67%,
      #ff2df0 83%,
      #ff2d2d 100%
    );
  }

  .marla-rgb-thumb {
    position: absolute;
    top: 50%;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: #ffffff;
    transform: translate3d(-50%, -50%, 0) scale(1);
    transition: transform 140ms cubic-bezier(0.22, 1, 0.36, 1);
    pointer-events: none;
  }

  .marla-rgb-line:hover:not(.is-disabled) .marla-rgb-thumb,
  .marla-rgb-line.is-dragging .marla-rgb-thumb {
    transform: translate3d(-50%, -50%, 0) scale(1.12);
  }
</style>
