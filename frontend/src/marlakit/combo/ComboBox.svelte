<script lang="ts">
  import Check from '@lucide/svelte/icons/check';
  import type { Snippet } from 'svelte';
  import { cubicOut } from 'svelte/easing';
  import ScrollArea from '../scroll/ScrollArea.svelte';
  import type { ComboOption, ComboPlacement, ComboSection, ComboValue } from './types';

  function comboTransition(node: HTMLElement, params: { dirY: number }) {
    return {
      duration: 170,
      easing: cubicOut,
      css: (t: number) => {
        const y = (1 - t) * params.dirY;
        const s = 0.96 + 0.04 * t;
        return `opacity: ${t}; transform: translate3d(0, ${y}px, 0) scale(${s});`;
      }
    };
  }

  interface Props {
    open?: boolean;
    value?: ComboValue | null;
    options?: ComboOption[];
    sections?: ComboSection[];
    label?: string;
    placeholder?: string;
    ariaLabel?: string;
    placement?: ComboPlacement;
    minWidth?: number;
    maxWidth?: number | null;
    maxHeight?: number | null;
    closeOnSelect?: boolean;
    disabled?: boolean;
    icon?: Snippet;
    trigger?: Snippet<[{ open: boolean; toggle: () => void; label: string }]>;
    footer?: Snippet;
    class?: string;
    menuClass?: string;
    onselect?: (option: ComboOption) => void;
    ontoggle?: (open: boolean) => void;
  }

  let {
    open = $bindable(false),
    value = null,
    options = [],
    sections = [],
    label = '',
    placeholder = 'Select',
    ariaLabel,
    placement = 'top-right',
    minWidth = 112,
    maxWidth = null,
    maxHeight = null,
    closeOnSelect = true,
    disabled = false,
    icon,
    trigger,
    footer,
    class: className = '',
    menuClass = '',
    onselect,
    ontoggle
  }: Props = $props();

  let wrapEl = $state<HTMLDivElement | null>(null);

  const normalizedSections = $derived.by<ComboSection[]>(() => {
    if (sections.length > 0) return sections;
    if (options.length > 0) return [{ options }];
    return [];
  });

  const flatOptions = $derived.by<ComboOption[]>(() => {
    const out: ComboOption[] = [];
    for (const sec of normalizedSections) {
      for (const opt of sec.options) {
        out.push(opt);
      }
    }
    return out;
  });

  const resolvedLabel = $derived.by(() => {
    if (label) return label;
    const found = flatOptions.find((o) => o.value === value);
    return found?.label ?? placeholder;
  });

  function setOpen(next: boolean) {
    if (open === next) return;
    open = next;
    ontoggle?.(next);
  }

  function toggle() {
    if (disabled) return;
    setOpen(!open);
  }

  function choose(opt: ComboOption) {
    if (opt.disabled) return;
    onselect?.(opt);
    if (closeOnSelect) {
      setOpen(false);
    }
  }

  $effect(() => {
    if (!open) return;

    const handlePointerDown = (e: PointerEvent) => {
      if (!wrapEl) return;
      if (e.target instanceof Node && !wrapEl.contains(e.target)) {
        setOpen(false);
      }
    };

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setOpen(false);
      }
    };

    window.addEventListener('pointerdown', handlePointerDown, true);
    window.addEventListener('keydown', handleKeyDown, true);
    return () => {
      window.removeEventListener('pointerdown', handlePointerDown, true);
      window.removeEventListener('keydown', handleKeyDown, true);
    };
  });
</script>

<div bind:this={wrapEl} class="marla-combo {className}">
  {#if trigger}
    {@render trigger({ open, toggle, label: resolvedLabel })}
  {:else}
    <button
      type="button"
      class="marla-combo-trigger"
      class:is-open={open}
      aria-haspopup="listbox"
      aria-expanded={open}
      aria-label={ariaLabel ?? resolvedLabel}
      {disabled}
      onclick={toggle}
    >
      {#if icon}
        {@render icon()}
      {/if}
      <span>{resolvedLabel}</span>
    </button>
  {/if}

  {#if open && normalizedSections.length > 0}
    <div
      class="marla-combo-popover placement-{placement} {menuClass}"
      role="listbox"
      style:min-width="{minWidth}px"
      style:max-width={maxWidth ? `${maxWidth}px` : undefined}
      transition:comboTransition={{ dirY: placement.startsWith('top') ? 6 : -6 }}
    >
      <ScrollArea maxHeight={maxHeight ?? 268}>
        <div class="marla-combo-list">
          {#each normalizedSections as sec, secIdx (sec.id ?? sec.title ?? secIdx)}
            {#if sec.title}
              <div class="marla-combo-section-label">{sec.title}</div>
            {/if}
            {#each sec.options as opt (opt.value)}
              {@const isSelected = opt.selected ?? (value !== null && value !== undefined && opt.value === value)}
              <button
                type="button"
                role="option"
                aria-selected={isSelected}
                disabled={opt.disabled}
                class="marla-combo-item"
                class:has-sub={Boolean(opt.sublabel)}
                class:active={isSelected}
                onclick={() => choose(opt)}
              >
                {#if opt.sublabel}
                  <span class="marla-combo-meta">
                    <span class="marla-combo-title">{opt.label}</span>
                    <span class="marla-combo-sub">{opt.sublabel}</span>
                  </span>
                {:else}
                  <span class="marla-combo-label">{opt.label}</span>
                {/if}
                {#if isSelected}
                  <Check size={14} strokeWidth={2.2} />
                {/if}
              </button>
            {/each}
          {/each}
        </div>
      </ScrollArea>
      {#if footer}
        <div class="marla-combo-footer">
          {@render footer()}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .marla-combo {
    position: relative;
    display: inline-flex;
  }

  .marla-combo-trigger {
    height: 28px;
    padding: 0 8px;
    border: none;
    outline: none;
    border-radius: 8px;
    background: transparent;
    color: rgba(255, 255, 255, 0.88);
    font-family: inherit;
    font-size: 12px;
    font-weight: 600;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    transition: background-color 0.15s ease, color 0.15s ease;
  }

  .marla-combo-trigger:hover:not(:disabled),
  .marla-combo-trigger.is-open:not(:disabled) {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .marla-combo-trigger:disabled {
    opacity: 0.45;
    cursor: default;
  }

  .marla-combo-popover {
    position: absolute;
    padding: 6px;
    border-radius: 12px;
    background: rgba(14, 14, 17, 0.96);
    backdrop-filter: blur(24px);
    -webkit-backdrop-filter: blur(24px);
    border: none;
    box-shadow: 0 22px 52px rgba(0, 0, 0, 0.78);
    display: flex;
    flex-direction: column;
    overflow: hidden;
    z-index: 40;
  }

  .marla-combo-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
  }

  .placement-top-right {
    right: 0;
    bottom: 36px;
    transform-origin: bottom right;
  }

  .placement-top-left {
    left: 0;
    bottom: 36px;
    transform-origin: bottom left;
  }

  .placement-bottom-right {
    right: 0;
    top: 36px;
    transform-origin: top right;
  }

  .placement-bottom-left {
    left: 0;
    top: 36px;
    transform-origin: top left;
  }

  .marla-combo-section-label {
    flex-shrink: 0;
    padding: 4px 10px 2px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: rgba(255, 255, 255, 0.38);
    user-select: none;
  }

  .marla-combo-item {
    flex-shrink: 0;
    width: 100%;
    min-height: 34px;
    height: 34px;
    padding: 0 11px;
    border: none;
    outline: none;
    border-radius: 8px;
    background: transparent;
    color: rgba(255, 255, 255, 0.78);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    white-space: nowrap;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    cursor: pointer;
    transition: background-color 0.14s ease, color 0.14s ease;
  }

  .marla-combo-label {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .marla-combo-item.has-sub {
    height: auto;
    min-height: 38px;
    padding: 6px 11px;
  }

  .marla-combo-item:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .marla-combo-item:disabled {
    opacity: 0.42;
    cursor: default;
  }

  .marla-combo-item.active {
    color: #ffffff;
    font-weight: 600;
  }

  .marla-combo-meta {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    min-width: 0;
    gap: 1px;
    text-align: left;
  }

  .marla-combo-title {
    font-size: 13px;
    line-height: 1.2;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 205px;
  }

  .marla-combo-sub {
    font-size: 11px;
    line-height: 1.25;
    font-weight: 400;
    color: rgba(255, 255, 255, 0.45);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 205px;
  }

  .marla-combo-footer {
    border-top: 1px solid rgba(255, 255, 255, 0.08);
    margin-top: 4px;
    padding: 8px 10px 4px;
  }
</style>
