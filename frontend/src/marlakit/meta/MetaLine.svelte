<script lang="ts">
  import Star from '@lucide/svelte/icons/star';

  interface Props {
    score?: number;
    tags?: readonly string[];
    size?: 'sm' | 'md';
    class?: string;
  }

  let {
    score,
    tags = [],
    size = 'md',
    class: className = ''
  }: Props = $props();

  const cleanTags = $derived(tags.filter((t) => Boolean(t && t.trim())));
  const iconSize = $derived(size === 'sm' ? 12 : 14);
</script>

{#if (score && score > 0) || cleanTags.length > 0}
  <div class="mila-meta-line is-{size} {className}">
    {#if score && score > 0}
      <span class="meta-rating">
        <Star size={iconSize} fill="currentColor" strokeWidth={0} />
        <span>{score.toFixed(1)}</span>
      </span>
    {/if}

    {#each cleanTags as tag, index (tag + index)}
      {#if (score && score > 0) || index > 0}
        <span class="meta-dot" aria-hidden="true"></span>
      {/if}
      <span class="meta-tag">{tag}</span>
    {/each}
  </div>
{/if}

<style>
  .mila-meta-line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    user-select: none;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 14px;
    line-height: 1;
  }

  .mila-meta-line.is-sm {
    gap: 8px;
    font-size: 12.5px;
  }

  .meta-rating {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-weight: 700;
    color: var(--mila-cards-rating, var(--marlakit-yellow));
    font-variant-numeric: tabular-nums;
    line-height: 1;
  }

  .meta-dot {
    width: 3px;
    height: 3px;
    border-radius: 50%;
    background: var(--mila-meta-dot, var(--mila-text-dim, rgba(255, 255, 255, 0.36)));
    flex-shrink: 0;
  }

  .meta-tag {
    color: var(--mila-meta-tag, var(--mila-text-muted, rgba(255, 255, 255, 0.56)));
    letter-spacing: 0.01em;
    line-height: 1.2;
  }
</style>
