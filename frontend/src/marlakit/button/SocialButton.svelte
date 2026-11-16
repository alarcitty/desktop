<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    children?: Snippet;
    icon?: Snippet;
    onclick?: (e: MouseEvent) => void;
    class?: string;
    type?: 'button' | 'submit' | 'reset';
    loading?: boolean;
    disabled?: boolean;
  }

  let {
    children,
    icon,
    onclick,
    class: className = '',
    type = 'button',
    loading = false,
    disabled = false
  }: Props = $props();
</script>

<button
  {type}
  class="marlakit-btn-social {className}"
  class:loading
  disabled={disabled || loading}
  {onclick}
>
  <span class="btn-content" class:is-loading={loading}>
    {#if icon}
      <span class="btn-icon">
        {@render icon()}
      </span>
    {/if}
    {#if children}
      <span class="btn-text">
        {@render children()}
      </span>
    {/if}
  </span>

  <span class="btn-spinner" class:is-visible={loading} aria-hidden="true">
    <svg viewBox="0 0 24 24" fill="none" class="spinner-svg">
      <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="2.5" opacity="0.25"/>
      <path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"/>
    </svg>
  </span>
</button>

<style>
  .marlakit-btn-social {
    position: relative;
    height: 40px;
    background: var(--mila-buttons-socialBg, #eaeaea);
    border-radius: 12px;
    border: none;
    outline: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    --wails-draggable: no-drag;
    transition: background-color 0.15s ease, color 0.15s ease, transform 0.1s ease, opacity 0.15s ease;
    padding: 0 16px;
    box-sizing: border-box;
    user-select: none;
    white-space: nowrap;
    color: var(--mila-buttons-socialText, #1a1a1a);
  }

  .marlakit-btn-social:hover:not(:disabled) {
    background: var(--mila-buttons-socialBgHover, #ffffff);
  }

  .marlakit-btn-social:active:not(:disabled) {
    transform: scale(0.98);
  }

  .marlakit-btn-social:disabled {
    cursor: not-allowed;
  }

  .marlakit-btn-social.loading {
    cursor: default;
  }

  .btn-content {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    transition: opacity 0.2s ease, transform 0.2s ease;
  }

  .btn-content.is-loading {
    opacity: 0;
    transform: scale(0.85);
  }

  .btn-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .btn-text {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 14px;
    font-weight: 500;
    color: var(--mila-buttons-socialText, #1a1a1a);
    line-height: 1;
  }

  .btn-spinner {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    opacity: 0;
    transform: scale(0.7);
    transition: opacity 0.2s ease, transform 0.2s ease;
    pointer-events: none;
    color: #1a1a1a;
  }

  .btn-spinner.is-visible {
    opacity: 1;
    transform: scale(1);
    pointer-events: auto;
  }

  .spinner-svg {
    width: 18px;
    height: 18px;
    animation: marlakit-spin 0.75s linear infinite;
  }

  @keyframes marlakit-spin {
    0% {
      transform: rotate(0deg);
    }
    100% {
      transform: rotate(360deg);
    }
  }
</style>
