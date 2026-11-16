<script lang="ts">
  import Heart from '@lucide/svelte/icons/heart';

  interface Props {
    liked?: boolean;
    variant?: 'media' | 'surface';
    ariaLabel?: string;
    title?: string;
    class?: string;
    onclick?: (e: MouseEvent) => void;
  }

  let {
    liked = false,
    variant = 'media',
    ariaLabel = liked ? 'Remove from Likes' : 'Add to Likes',
    title = ariaLabel,
    class: className = '',
    onclick
  }: Props = $props();
</script>

<button
  type="button"
  class="marlakit-like-btn variant-{variant} {className}"
  class:is-liked={liked}
  aria-label={ariaLabel}
  aria-pressed={liked}
  {title}
  {onclick}
>
  <span class="like-heart-wrap" class:is-popped={liked}>
    <Heart size={18} strokeWidth={2} fill={liked ? 'currentColor' : 'none'} />
  </span>
</button>

<style>
  .marlakit-like-btn {
    position: relative;
    width: 40px;
    height: 40px;
    padding: 0;
    margin: 0;
    border: none;
    outline: none;
    background: transparent;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    flex-shrink: 0;
    -webkit-tap-highlight-color: transparent;
    --wails-draggable: no-drag;
    transition: color 0.2s cubic-bezier(0.22, 1, 0.36, 1), transform 0.16s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .marlakit-like-btn.variant-media {
    color: rgba(255, 255, 255, 0.62);
  }

  .marlakit-like-btn.variant-media:hover {
    color: #ffffff;
    transform: translateY(-1px);
  }

  .marlakit-like-btn.variant-surface {
    color: var(--mila-text-muted, rgba(255, 255, 255, 0.56));
  }

  .marlakit-like-btn.variant-surface:hover {
    color: var(--mila-text-primary, #ffffff);
    transform: translateY(-1px);
  }

  .marlakit-like-btn:active {
    transform: scale(0.9);
  }

  .marlakit-like-btn.is-liked,
  .marlakit-like-btn.is-liked:hover {
    color: #ff4d5a;
  }

  .like-heart-wrap {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: transform 0.24s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .like-heart-wrap.is-popped {
    animation: likePop 320ms cubic-bezier(0.22, 1, 0.36, 1);
  }

  @keyframes likePop {
    0% {
      transform: scale(0.84);
    }
    55% {
      transform: scale(1.22);
    }
    100% {
      transform: scale(1);
    }
  }
</style>
