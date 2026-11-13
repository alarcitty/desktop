<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    children?: Snippet;
    src?: string;
    blur?: number;
    opacity?: number;
    scale?: number;
    offsetY?: number;
    offsetX?: number;
    class?: string;
    style?: string;
  }

  let {
    children,
    src,
    blur = 40,
    opacity = 0.6,
    scale = 1.05,
    offsetY = 0,
    offsetX = 0,
    class: className = '',
    style = ''
  }: Props = $props();
</script>

{#if children}
  <div class="marlakit-layer-blur-wrapper {className}" {style}>
    <!-- Underlay blur -->
    <div
      class="layer-blur-underlay"
      style="
        filter: blur({blur}px);
        -webkit-filter: blur({blur}px);
        opacity: {opacity};
        transform: translate3d({offsetX}px, {offsetY}px, 0) scale({scale});
      "
      aria-hidden="true"
    >
      {#if src}
        <img {src} alt="" class="blur-img" decoding="async" />
      {:else}
        {@render children()}
      {/if}
    </div>

    <!-- Foreground -->
    <div class="layer-blur-foreground">
      {@render children()}
    </div>
  </div>
{:else}
  <!-- Standalone blur layer -->
  <div
    class="marlakit-layer-blur-standalone {className}"
    style="
      filter: blur({blur}px);
      -webkit-filter: blur({blur}px);
      opacity: {opacity};
      transform: translate3d({offsetX}px, {offsetY}px, 0) scale({scale});
      {style}
    "
    aria-hidden="true"
  >
    {#if src}
      <img {src} alt="" class="blur-img" decoding="async" />
    {/if}
  </div>
{/if}

<style>
  .marlakit-layer-blur-wrapper {
    position: relative;
    box-sizing: border-box;
  }

  .layer-blur-underlay {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    pointer-events: none;
    user-select: none;
    z-index: 0;
    will-change: transform;
    backface-visibility: hidden;
  }

  .layer-blur-foreground {
    position: relative;
    z-index: 1;
    width: 100%;
    height: 100%;
  }

  .marlakit-layer-blur-standalone {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    pointer-events: none;
    user-select: none;
    z-index: 0;
    will-change: transform;
    backface-visibility: hidden;
  }

  .blur-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    border-radius: inherit;
  }
</style>
