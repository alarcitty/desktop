<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    children?: Snippet;
    icon?: Snippet;
    onclick?: (e: MouseEvent) => void;
    class?: string;
    type?: 'button' | 'submit' | 'reset';
    loading?: boolean;
    progress?: number | null;
    disabled?: boolean;
    fullWidth?: boolean;
    ariaLabel?: string;
  }

  let {
    children,
    icon,
    onclick,
    class: className = '',
    type = 'button',
    loading = false,
    progress = null,
    disabled = false,
    fullWidth = false,
    ariaLabel
  }: Props = $props();

  const hasProgress = $derived(typeof progress === 'number' && Number.isFinite(progress));
  const clampedProgress = $derived(
    hasProgress ? Math.max(0, Math.min(100, Number(progress))) : 0
  );
  const isChecking = $derived(loading && !hasProgress);
  const isBusy = $derived(isChecking || hasProgress);
</script>

<button
  {type}
  class="marlakit-btn-git {className}"
  class:is-full={fullWidth}
  class:loading={isBusy}
  disabled={disabled || isBusy}
  aria-label={ariaLabel}
  aria-busy={isBusy}
  {onclick}
>
  <span class="btn-content" class:is-hidden={isBusy}>
    <span class="btn-icon">
      {#if icon}
        {@render icon()}
      {:else}
        <svg viewBox="0 0 24 24" width="15" height="15" fill="currentColor" aria-hidden="true">
          <path
            d="M12 1.27a11 11 0 0 0-3.48 21.46c.55.1.73-.24.73-.53v-1.85c-3.03.66-3.67-1.45-3.67-1.45-.5-1.26-1.21-1.6-1.21-1.6-.99-.68.07-.67.07-.67 1.09.08 1.67 1.12 1.67 1.12.97 1.66 2.54 1.18 3.16.9.1-.7.38-1.18.69-1.45-2.42-.27-4.96-1.21-4.96-5.38 0-1.19.42-2.16 1.12-2.92-.11-.28-.49-1.39.11-2.89 0 0 .92-.29 3 1.12a10.4 10.4 0 0 1 5.46 0c2.08-1.41 3-1.12 3-1.12.6 1.5.22 2.61.11 2.89.7.76 1.12 1.73 1.12 2.92 0 4.18-2.54 5.1-4.97 5.37.39.34.74 1.01.74 2.03v3.01c0 .29.19.63.74.53A11 11 0 0 0 12 1.27Z"
          />
        </svg>
      {/if}
    </span>
    {#if children}
      <span class="btn-text">
        {@render children()}
      </span>
    {/if}
  </span>

  <span class="btn-spinner" class:is-visible={isChecking} aria-hidden="true">
    <svg viewBox="0 0 24 24" fill="none" class="spinner-svg">
      <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="2.5" opacity="0.25" />
      <path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" />
    </svg>
  </span>

  <span
    class="btn-progress"
    class:is-visible={hasProgress}
    role={hasProgress ? 'progressbar' : undefined}
    aria-valuemin={hasProgress ? 0 : undefined}
    aria-valuemax={hasProgress ? 100 : undefined}
    aria-valuenow={hasProgress ? Math.round(clampedProgress) : undefined}
  >
    <span class="btn-progress-track">
      <span class="btn-progress-fill" style:width={`${clampedProgress}%`}></span>
    </span>
  </span>
</button>

<style>
  .marlakit-btn-git {
    position: relative;
    height: 36px;
    padding: 0 14px;
    border-radius: 10px;
    border: none;
    outline: none;
    background: var(--mila-buttons-mainBg, rgba(255, 255, 255, 0.96));
    color: var(--mila-buttons-mainText, #111111);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    --wails-draggable: no-drag;
    user-select: none;
    white-space: nowrap;
    box-sizing: border-box;
    transition:
      background-color 0.15s ease,
      color 0.15s ease,
      transform 0.1s ease,
      opacity 0.15s ease;
  }

  .marlakit-btn-git.is-full {
    width: 100%;
  }

  .marlakit-btn-git:hover:not(:disabled) {
    background: var(--mila-buttons-mainBgHover, #ffffff);
  }

  .marlakit-btn-git:active:not(:disabled) {
    transform: scale(0.98);
  }

  .marlakit-btn-git:disabled {
    cursor: not-allowed;
  }

  .marlakit-btn-git.loading {
    cursor: default;
  }

  .btn-content {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    transition: opacity 0.2s ease, transform 0.2s ease;
  }

  .btn-content.is-hidden {
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
    font-size: 13px;
    font-weight: 500;
    line-height: 1;
    color: currentColor;
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
    color: currentColor;
  }

  .btn-spinner.is-visible {
    opacity: 1;
    transform: scale(1);
    pointer-events: auto;
  }

  .spinner-svg {
    width: 17px;
    height: 17px;
    animation: marlakit-spin 0.75s linear infinite;
  }

  .btn-progress {
    position: absolute;
    inset: 0;
    padding: 0 14px;
    display: flex;
    align-items: center;
    justify-content: center;
    opacity: 0;
    transform: scale(0.92);
    transition: opacity 0.2s ease, transform 0.2s ease;
    pointer-events: none;
  }

  .btn-progress.is-visible {
    opacity: 1;
    transform: scale(1);
  }

  .btn-progress-track {
    width: 100%;
    height: 5px;
    border-radius: 999px;
    overflow: hidden;
    background: color-mix(in srgb, currentColor 20%, transparent);
  }

  .btn-progress-fill {
    height: 100%;
    border-radius: 999px;
    background: currentColor;
    transition: width 160ms cubic-bezier(0.22, 1, 0.36, 1);
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
