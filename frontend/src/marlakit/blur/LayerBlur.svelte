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

  // Downsample image blurs to 1/4 linear size (1/16th pixel area) and GPU-upscale back.
  // Keeps large blur radii smooth on Windows WebView2 without 100%-scale Gaussian pass overhead.
  const sampledBlur = $derived(src ? Math.max(2, Number((blur / 4).toFixed(2))) : blur);
  const sampledTransform = $derived(
    src
      ? `translate3d(calc(-50% + ${offsetX}px), calc(-50% + ${offsetY}px), 0) scale(${Number((scale * 4).toFixed(3))})`
      : `translate3d(${offsetX}px, ${offsetY}px, 0) scale(${scale})`
  );
</script>

{#if children}
  <div class="marlakit-layer-blur-wrapper {className}" {style}>
    <!-- Underlay blur -->
    <div
      class="layer-blur-underlay"
      class:is-downsampled={Boolean(src)}
      style="
        filter: blur({sampledBlur}px);
        -webkit-filter: blur({sampledBlur}px);
        opacity: {opacity};
        transform: {sampledTransform};
      "
      aria-hidden="true"
    >
      {#if src}
        <img {src} alt="" class="blur-img" decoding="async" referrerpolicy="no-referrer" />
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
    class:is-downsampled={Boolean(src)}
    style="
      filter: blur({sampledBlur}px);
      -webkit-filter: blur({sampledBlur}px);
      opacity: {opacity};
      transform: {sampledTransform};
      {style}
    "
    aria-hidden="true"
  >
    {#if src}
      <img {src} alt="" class="blur-img" decoding="async" referrerpolicy="no-referrer" />
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
    backface-visibility: hidden;
  }

  .layer-blur-underlay.is-downsampled,
  .marlakit-layer-blur-standalone.is-downsampled {
    inset: auto;
    top: 50%;
    left: 50%;
    width: 25%;
    height: 25%;
    transform-origin: center center;
  }

  .blur-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    border-radius: inherit;
  }
</style>
