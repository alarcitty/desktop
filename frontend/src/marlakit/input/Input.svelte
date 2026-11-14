<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    value?: string;
    placeholder?: string;
    type?: string;
    disabled?: boolean;
    class?: string;
    icon?: Snippet;
    trailingIcon?: Snippet;
    oninput?: (e: Event) => void;
    onchange?: (e: Event) => void;
    onkeydown?: (e: KeyboardEvent) => void;
  }

  let {
    value = $bindable(''),
    placeholder = '',
    type = 'text',
    disabled = false,
    class: className = '',
    icon,
    trailingIcon,
    oninput,
    onchange,
    onkeydown,
    ...rest
  }: Props = $props();
</script>

<div class="marlakit-input-wrap {className}" class:disabled>
  {#if icon}
    <div class="input-icon leading-icon">
      {@render icon()}
    </div>
  {/if}

  <input
    {type}
    {placeholder}
    {disabled}
    bind:value
    {oninput}
    {onchange}
    {onkeydown}
    spellcheck="false"
    class="marlakit-input-field"
    {...rest}
  />

  {#if trailingIcon}
    <div class="input-icon trailing-icon">
      {@render trailingIcon()}
    </div>
  {/if}
</div>

<style>
  .marlakit-input-wrap {
    display: inline-flex;
    align-items: center;
    width: 320px;
    height: 38px;
    background: rgba(255, 255, 255, 0.08);
    border-radius: 9px;
    padding: 0 12px;
    box-sizing: border-box;
    gap: 10px;
    transition: background-color 0.15s ease, box-shadow 0.15s ease;
    --wails-draggable: no-drag;
    user-select: text;
  }

  .marlakit-input-wrap:hover {
    background: rgba(255, 255, 255, 0.09);
  }

  .marlakit-input-wrap:focus-within {
    background: rgba(255, 255, 255, 0.12);
  }

  .marlakit-input-wrap.disabled {
    opacity: 0.5;
    pointer-events: none;
  }

  .input-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    color: rgba(255, 255, 255, 0.35);
  }

  .marlakit-input-field {
    flex: 1;
    width: 100%;
    height: 100%;
    background: transparent;
    border: none;
    outline: none;
    color: #ffffff;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 15px;
    font-weight: 400;
    padding: 0;
    margin: 0;
  }

  .marlakit-input-field::placeholder {
    color: rgba(255, 255, 255, 0.3);
  }
</style>
