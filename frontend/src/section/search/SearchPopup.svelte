<script lang="ts">
  import Search from '@lucide/svelte/icons/search';

  import MorphPopup from '../../marlakit/popup/MorphPopup.svelte';
  import Input from '../../marlakit/input/Input.svelte';
  import { search, closeSearch, finishSearchClose, getSearchAnchorRect } from '../../app/search.svelte';
  import { openWatch } from '../../app/player.svelte';
  import { catalog, type AnimeEntry } from '../../data/catalog';

  const ROW_H = 68;
  const ROW_GAP = 4;
  const MAX_ROWS = 6;
  const EMPTY_H = 64;

  let selected = $state(0);

  const results = $derived.by(() =>
  {
    const q = search.query.trim().toLowerCase();
    if (!q)
    {
      return [] as AnimeEntry[];
    }
    return catalog.filter((a) => a.title.toLowerCase().includes(q));
  });

  const listHeight = $derived(
    results.length > 0
      ? Math.min(results.length, MAX_ROWS) * (ROW_H + ROW_GAP) - ROW_GAP
      : EMPTY_H
  );

  // padding 14 + input 48 + gap 12 + list + padding 14
  const popupHeight = $derived(14 + 48 + 12 + listHeight + 14);

  // keep selection inside the list when results change
  $effect(() =>
  {
    results;
    selected = 0;
  });

  // put the caret into the popup input as soon as it appears
  function focusInput(aNode: HTMLElement)
  {
    const el = aNode.querySelector('input');
    if (el)
    {
      el.focus();
      const len = el.value.length;
      el.setSelectionRange(len, len);
    }
  }

  function pick(aItem: AnimeEntry | undefined)
  {
    if (!aItem)
    {
      return;
    }
    closeSearch();
    void openWatch({
      title: aItem.title,
      shikiId: aItem.shikiId,
      releaseAlias: aItem.releaseAlias,
      searchQueries: aItem.searchQueries
    });
  }

  function handleKeydown(e: KeyboardEvent)
  {
    if (e.key === 'ArrowDown' && results.length > 0)
    {
      e.preventDefault();
      selected = (selected + 1) % results.length;
    }
    else if (e.key === 'ArrowUp' && results.length > 0)
    {
      e.preventDefault();
      selected = (selected - 1 + results.length) % results.length;
    }
    else if (e.key === 'Enter')
    {
      e.preventDefault();
      pick(results[selected]);
    }
  }
</script>

<MorphPopup
  open={search.open}
  getAnchor={getSearchAnchorRect}
  height={popupHeight}
  onclose={closeSearch}
  onclosed={finishSearchClose}
>
  {#snippet children(expanded)}
    <div class="search-body" class:expanded use:focusInput>
      <Input
        bind:value={search.query}
        onkeydown={handleKeydown}
        placeholder="Search something..."
        class="search-popup-input"
      >
        {#snippet icon()}
          <Search size={18} strokeWidth={1.8} />
        {/snippet}
        {#snippet trailingIcon()}
          <span class="esc-chip">Esc</span>
        {/snippet}
      </Input>

      <div class="search-results" style:height="{listHeight}px">
        {#if results.length > 0}
          <ul class="result-list">
            {#each results as item, i (item.id)}
              <li>
                <button
                  type="button"
                  class="result"
                  class:selected={i === selected}
                  onmouseenter={() => (selected = i)}
                  onclick={() => pick(item)}
                >
                  <img
                    src={item.poster}
                    alt=""
                    class="thumb"
                    style:object-position={item.posterPosition}
                    loading="lazy"
                    decoding="async"
                  />
                  <span class="meta">
                    <span class="name">{item.title}</span>
                    <span class="sub">Anime · {item.year}</span>
                  </span>
                </button>
              </li>
            {/each}
          </ul>
        {:else}
          <div class="empty">
            {search.query.trim() ? `Nothing found for “${search.query.trim()}”` : 'Start typing to search anime'}
          </div>
        {/if}
      </div>
    </div>
  {/snippet}
</MorphPopup>

<style>
  .search-body
  {
    display: flex;
    flex-direction: column;
    gap: 12px;
    width: 100%;
    height: 100%;
  }

  .search-body :global(.search-popup-input)
  {
    width: 100%;
    flex-shrink: 0;
    /* the panel itself is the glass; a nested backdrop-filter would be redundant */
    -webkit-backdrop-filter: none;
    backdrop-filter: none;
    transition:
      height var(--popup-duration, 560ms) var(--ease-out),
      background-color 0.15s ease;
  }

  .search-body.expanded :global(.search-popup-input)
  {
    height: 48px;
    border-radius: 12px;
  }

  .search-body :global(.search-popup-input .marlakit-input-field)
  {
    transition: font-size var(--popup-duration, 560ms) var(--ease-out);
  }

  .search-body.expanded :global(.search-popup-input .marlakit-input-field)
  {
    font-size: 17px;
  }

  .esc-chip
  {
    font-family: 'Medeo Sans', system-ui, sans-serif;
    font-size: 12px;
    color: rgba(255, 255, 255, 0.4);
    background: rgba(255, 255, 255, 0.08);
    border-radius: 6px;
    padding: 4px 8px;
    line-height: 1;
  }

  .search-results
  {
    opacity: 0;
    overflow: hidden;
    transition:
      opacity 0.35s var(--ease-out),
      height var(--popup-duration, 560ms) var(--ease-out);
  }

  .search-body.expanded .search-results
  {
    opacity: 1;
    transition-delay: 0.18s, 0s;
  }

  .result-list
  {
    list-style: none;
    margin: 0;
    padding: 0;
    height: 100%;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 4px;
    scrollbar-width: none;
  }

  .result-list::-webkit-scrollbar
  {
    display: none;
  }

  .result
  {
    width: 100%;
    height: 68px;
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 0 12px;
    margin: 0;
    box-sizing: border-box;
    background: transparent;
    border: none;
    border-radius: 12px;
    cursor: pointer;
    text-align: left;
    color: #ffffff;
    transition: background-color 0.2s var(--ease-out);
  }

  .result.selected
  {
    background: rgba(255, 255, 255, 0.07);
  }

  .thumb
  {
    flex: 0 0 auto;
    width: 40px;
    height: 54px;
    object-fit: cover;
    border-radius: 8px;
    pointer-events: none;
  }

  .meta
  {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }

  .name
  {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 16px;
    line-height: 1.2;
    color: #ffffff;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .sub
  {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    line-height: 1;
    color: var(--text-muted);
  }

  .empty
  {
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 15px;
    color: var(--text-muted);
  }
</style>
