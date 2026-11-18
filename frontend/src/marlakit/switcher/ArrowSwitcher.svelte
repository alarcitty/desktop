<script lang="ts">
  import { cubicOut } from 'svelte/easing';
  import { ChevronLeft, ChevronRight } from 'lucide';
  import { MorphIcon } from 'morphicons/svelte';

  export interface ArrowSwitcherOption {
    value: string | number;
    label: string;
  }

  interface Props {
    value: string | number;
    options: readonly ArrowSwitcherOption[];
    ariaLabel?: string;
    compact?: boolean;
    disabled?: boolean;
    class?: string;
    onchange?: (value: string | number, index: number) => void;
  }

  let {
    value,
    options,
    ariaLabel = 'Switch option',
    compact = false,
    disabled = false,
    class: className = '',
    onchange
  }: Props = $props();

  let slideDir = $state<1 | -1>(1);

  const currentIndex = $derived.by(() => {
    const idx = options.findIndex((o) => o.value === value);
    return idx >= 0 ? idx : 0;
  });

  const currentOption = $derived(options[currentIndex] ?? { value, label: String(value) });

  function slideBlur(
    _node: HTMLElement,
    params: { dir: 1 | -1; mode: 'in' | 'out'; duration?: number }
  ) {
    const duration = params.duration ?? 250;
    const offset = (params.mode === 'in' ? 1 : -1) * params.dir * 20;
    const startScale = 0.86;
    const maxBlur = 6;
    return {
      duration,
      easing: cubicOut,
      css: (t: number) => {
        const inv = 1 - t;
        const x = offset * inv;
        const s = startScale + (1 - startScale) * t;
        const b = maxBlur * inv;
        return `opacity: ${t}; transform: translate3d(${x.toFixed(2)}px, 0, 0) scale(${s.toFixed(
          4
        )}); filter: blur(${b.toFixed(2)}px);`;
      }
    };
  }

  function step(delta: 1 | -1) {
    if (disabled || options.length <= 1) return;
    slideDir = delta;
    const nextIdx = (currentIndex + delta + options.length) % options.length;
    const next = options[nextIdx];
    if (next) {
      onchange?.(next.value, nextIdx);
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (disabled || options.length <= 1) return;
    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      e.stopPropagation();
      step(-1);
    } else if (e.key === 'ArrowRight') {
      e.preventDefault();
      e.stopPropagation();
      step(1);
    }
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="marla-arrow-switcher {className}"
  class:is-compact={compact}
  class:is-disabled={disabled || options.length <= 1}
  role="group"
  aria-label={ariaLabel}
  tabindex={disabled ? -1 : 0}
  onkeydown={handleKeydown}
>
  <button
    type="button"
    class="switcher-chevron"
    aria-label="Previous"
    disabled={disabled || options.length <= 1}
    onclick={(e) => {
      e.stopPropagation();
      step(-1);
    }}
  >
    <MorphIcon icon={ChevronLeft} size={compact ? 15 : 17} strokeWidth={2.1} spring="snappy" />
  </button>

  <div class="switcher-value-slot" aria-live="polite">
    {#key `${currentIndex}:${currentOption.value}`}
      <span
        class="switcher-value"
        in:slideBlur={{ dir: slideDir, mode: 'in' }}
        out:slideBlur={{ dir: slideDir, mode: 'out' }}
      >
        {currentOption.label}
      </span>
    {/key}
  </div>

  <button
    type="button"
    class="switcher-chevron"
    aria-label="Next"
    disabled={disabled || options.length <= 1}
    onclick={(e) => {
      e.stopPropagation();
      step(1);
    }}
  >
    <MorphIcon icon={ChevronRight} size={compact ? 15 : 17} strokeWidth={2.1} spring="snappy" />
  </button>
</div>

<style>
  .marla-arrow-switcher {
    width: 100%;
    height: 36px;
    padding: 0 6px;
    border-radius: 10px;
    background: var(--mila-app-surfaceGlass, rgba(255, 255, 255, 0.04));
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 4px;
    box-sizing: border-box;
    outline: none;
    user-select: none;
    -webkit-app-region: no-drag;
    transition: background-color 160ms ease;
  }

  .marla-arrow-switcher:hover:not(.is-disabled) {
    background: var(--mila-popup-itemHover, rgba(255, 255, 255, 0.08));
  }

  .marla-arrow-switcher.is-compact {
    height: 32px;
    border-radius: 9px;
  }

  .switcher-chevron {
    width: 26px;
    height: 26px;
    padding: 0;
    margin: 0;
    border: none;
    outline: none;
    border-radius: 7px;
    background: transparent;
    color: var(--mila-setup-chevron, rgba(255, 255, 255, 0.46));
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    cursor: pointer;
    transition:
      color 150ms ease,
      transform 130ms cubic-bezier(0.22, 1, 0.36, 1);
  }

  .switcher-chevron:hover:not(:disabled) {
    color: var(--mila-setup-chevronHover, #ffffff);
  }

  .switcher-chevron:active:not(:disabled) {
    transform: scale(0.84);
  }

  .switcher-chevron:disabled {
    opacity: 0.35;
    cursor: default;
  }

  .switcher-value-slot {
    flex: 1 1 auto;
    min-width: 0;
    height: 26px;
    display: grid;
    place-items: center;
    overflow: hidden;
  }

  .switcher-value {
    grid-area: 1 / 1;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: 'RF Dewi Extended', 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 700;
    letter-spacing: -0.015em;
    color: var(--mila-setup-valueText, #ffffff);
    text-align: center;
  }

  .marla-arrow-switcher.is-compact .switcher-value {
    font-size: 12.5px;
  }
</style>
