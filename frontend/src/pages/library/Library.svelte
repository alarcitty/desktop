<script lang="ts">
  import { fly } from 'svelte/transition';
  import { cubicIn, cubicOut } from 'svelte/easing';
  import LayoutList from '@lucide/svelte/icons/layout-list';
  import LayoutGrid from '@lucide/svelte/icons/layout-grid';
  import Play from '@lucide/svelte/icons/play';

  import MainButton from '../../marlakit/button/MainButton.svelte';
  import LayerBlur from '../../marlakit/blur/LayerBlur.svelte';

  import alya from '../../assets/library/alya.png';
  import blackClover from '../../assets/library/black-clover.png';
  import tsukimichi from '../../assets/library/tsukimichi.png';

  interface LibraryItem
  {
    id: string;
    title: string;
    description: string;
    poster: string;
    posterPosition?: string;
  }

  const items: LibraryItem[] = [
    {
      id: 'alya',
      title: 'Alya Sometimes Hides Her Feelings in Russian',
      description:
        'The story centers on Alisa Mikhailovna "Alya" Kujou, a stunning and brilliant student who acts cold and distant to everyone, and her unmotivated seatmate, Masachika Kuze. To hide her true feelings of embarrassment, affection, or frustration, Alya frequently mutters sweet.',
      poster: alya
    },
    {
      id: 'black-clover',
      title: 'Black Clover',
      description:
        'is a popular fantasy anime about two orphans, Asta and Yuno, who grew up together and strive to become the Wizard King. In a world where magic is everything, Yuno is a prodigy born with a rare grimoire, while Asta is the only person completely devoid of magical power. Refusing to give up, Asta obtains a mysterious five-leaf clover grimoire that grants him Anti-Magic.',
      poster: blackClover
    },
    {
      id: 'tsukimichi',
      title: 'Tsukimichi: Moonlit Fantasy',
      description:
        'follows Makoto Misumi, an ordinary high schooler summoned to another world as a hero, only to be cast away by the goddess for his lack of beauty. Stripped of his title and sent to a barren wasteland, he discovers an overwhelming power of his own and slowly gathers allies among the monsters and outcasts of the new world.',
      poster: tsukimichi,
      posterPosition: 'left center'
    }
  ];

  const tabs = ['Likes', 'watch later', 'favorites'];
  const TAB_GAP = 22;
  const TAB_INACTIVE_SCALE = 0.5;

  let activeTab = $state(0);
  let hasSwitchedTab = $state(false);
  let tabsReady = $state(false);
  let tabWidths = $state<Array<{ active: number; inactive: number }>>([]);
  let tabBaselineY = $state(38);
  let regularRefs: Array<HTMLSpanElement | undefined> = [];
  let blackRefs: Array<HTMLSpanElement | undefined> = [];
  let probeEl: HTMLSpanElement | undefined;
  let probeMarkEl: HTMLSpanElement | undefined;

  let viewMode = $state<'list' | 'grid'>('list');
  let hasToggledView = $state(false);
  let loadingId = $state<string | null>(null);
  let loadingTimer: number | undefined;

  let tabLayout = $derived.by(() =>
  {
    if (!tabsReady || tabWidths.length !== tabs.length) {
      return tabs.map((_, i) => ({ x: 0, scale: i === activeTab ? 1 : TAB_INACTIVE_SCALE }));
    }
    let cursor = 0;
    return tabs.map((_, i) =>
    {
      const isActive = i === activeTab;
      const scale = isActive ? 1 : TAB_INACTIVE_SCALE;
      const visualWidth = isActive
        ? tabWidths[i].active
        : tabWidths[i].inactive * TAB_INACTIVE_SCALE;
      const x = cursor;
      cursor += visualWidth + TAB_GAP;
      return { x, scale };
    });
  });

  function measureTabs()
  {
    if (regularRefs.length !== tabs.length || blackRefs.length !== tabs.length) return;
    const widths: Array<{ active: number; inactive: number }> = [];
    for (let i = 0; i < tabs.length; i++) {
      const reg = regularRefs[i];
      const blk = blackRefs[i];
      if (!reg || !blk) return;
      widths.push({
        active: blk.offsetWidth,
        inactive: reg.offsetWidth
      });
    }
    if (probeEl && probeMarkEl) {
      const baseline =
        probeMarkEl.getBoundingClientRect().bottom - probeEl.getBoundingClientRect().top;
      if (baseline > 0) {
        tabBaselineY = baseline;
      }
    }
    tabWidths = widths;
    tabsReady = true;
  }

  $effect(() =>
  {
    measureTabs();
    const fonts = document.fonts;
    const onFonts = () => measureTabs();
    fonts?.ready.then(onFonts);
    fonts?.addEventListener('loadingdone', onFonts);
    return () =>
    {
      window.clearTimeout(loadingTimer);
      fonts?.removeEventListener('loadingdone', onFonts);
    };
  });

  function selectTab(index: number)
  {
    if (activeTab === index) return;
    measureTabs();
    hasSwitchedTab = true;
    activeTab = index;
  }

  function setViewMode(mode: 'list' | 'grid')
  {
    if (viewMode === mode) return;
    hasToggledView = true;
    viewMode = mode;
  }

  function cardEnter(_node: Element, params: { index: number })
  {
    if (!hasToggledView) {
      return { duration: 0 };
    }
    const stagger = Math.min(params.index, 6) * 45;
    return {
      delay: 110 + stagger,
      duration: 320,
      easing: cubicOut,
      css: (t: number) =>
        `opacity: ${t}; transform: translate3d(0, ${(1 - t) * 14}px, 0) scale(${0.96 + 0.04 * t});`
    };
  }

  function handleWatch(aId: string)
  {
    window.clearTimeout(loadingTimer);
    loadingId = aId;
    loadingTimer = window.setTimeout(() =>
    {
      loadingId = null;
    }, 2000);
  }
</script>

<div class="library-page">
  <!-- Ambient glow from the first poster, fading out downwards -->
  <div class="library-ambient" aria-hidden="true">
    <LayerBlur src={items[0].poster} blur={60} opacity={0.18} scale={1.2} />
  </div>

  <div class="library-head">
    <div
      class="library-tabs"
      class:is-ready={tabsReady}
      class:is-animated={hasSwitchedTab}
    >
      <span class="tab-baseline-probe" aria-hidden="true" bind:this={probeEl}>
        Ag<span class="probe-mark" bind:this={probeMarkEl}></span>
      </span>
      {#each tabs as tab, index}
        <button
          type="button"
          class="library-tab"
          class:active={activeTab === index}
          style:transform={`translate3d(${tabLayout[index].x}px, 0, 0) scale(${tabLayout[index].scale})`}
          style:transform-origin={`0px ${tabBaselineY}px`}
          onclick={() => selectTab(index)}
        >
          <span class="tab-label tab-regular" bind:this={regularRefs[index]}>{tab}</span>
          <span class="tab-label tab-black" aria-hidden="true" bind:this={blackRefs[index]}>{tab}</span>
        </button>
      {/each}
    </div>

    <!-- View mode switch -->
    <div class="library-view-modes" class:is-grid={viewMode === 'grid'}>
      <span class="view-pill" aria-hidden="true"></span>
      <button
        type="button"
        class="view-btn"
        class:active={viewMode === 'list'}
        aria-label="List view"
        aria-pressed={viewMode === 'list'}
        onclick={() => setViewMode('list')}
      >
        <LayoutList size={18} strokeWidth={1.7} />
      </button>
      <button
        type="button"
        class="view-btn"
        class:active={viewMode === 'grid'}
        aria-label="Grid view"
        aria-pressed={viewMode === 'grid'}
        onclick={() => setViewMode('grid')}
      >
        <LayoutGrid size={18} strokeWidth={1.7} />
      </button>
    </div>
  </div>

  <div class="library-views">
    {#if viewMode === 'list'}
      <ul
        class="library-list"
        out:fly={{ y: -10, duration: 150, easing: cubicIn }}
      >
        {#each items as item, index (item.id)}
          <li class="library-row" in:cardEnter|global={{ index }}>
            <img
              src={item.poster}
              alt=""
              class="row-bg"
              style:object-position={item.posterPosition}
              loading={index > 1 ? 'lazy' : undefined}
              decoding="async"
            />
            <div class="row-dim"></div>

            <img
              src={item.poster}
              alt={item.title}
              class="row-poster"
              style:object-position={item.posterPosition}
              loading={index > 1 ? 'lazy' : undefined}
              decoding="async"
            />

            <div class="row-info">
              <h3 class="row-title">{item.title}</h3>
              <p class="row-description">{item.description}</p>
            </div>

            <div class="row-action">
              <MainButton loading={loadingId === item.id} onclick={() => handleWatch(item.id)}>
                {#snippet icon()}
                  <Play size={16} fill="currentColor" strokeWidth={0} />
                {/snippet}
                Watch
              </MainButton>
            </div>
          </li>
        {/each}
      </ul>
    {:else}
      <ul
        class="library-grid"
        out:fly={{ y: -10, duration: 150, easing: cubicIn }}
      >
        {#each items as item, index (item.id)}
          <li class="library-tile" in:cardEnter|global={{ index }}>
            <img
              src={item.poster}
              alt={item.title}
              class="tile-poster"
              style:object-position={item.posterPosition}
              loading={index > 3 ? 'lazy' : undefined}
              decoding="async"
            />
            <div class="tile-overlay"></div>
            <div class="tile-hover-dim"></div>

            <p class="tile-description">{item.description}</p>

            <div class="tile-footer">
              <h3 class="tile-title">{item.title}</h3>
              <div class="tile-action">
                <MainButton loading={loadingId === item.id} onclick={() => handleWatch(item.id)}>
                  {#snippet icon()}
                    <Play size={16} fill="currentColor" strokeWidth={0} />
                  {/snippet}
                  Watch
                </MainButton>
              </div>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  .library-page {
    position: relative;
    width: 100%;
    min-height: 100vh;
    margin-top: -72px;
    padding: 72px 0 0 0;
    box-sizing: border-box;
    background: #070707;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    -webkit-font-smoothing: antialiased;
  }

  .library-ambient {
    position: absolute;
    top: 0;
    left: 0;
    width: 100%;
    height: 700px;
    pointer-events: none;
    z-index: 0;
    contain: strict;
    transform: translate3d(0, 0, 0);
    -webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 1) 0%, rgba(0, 0, 0, 0) 100%);
    mask-image: linear-gradient(180deg, rgba(0, 0, 0, 1) 0%, rgba(0, 0, 0, 0) 100%);
  }

  .library-head {
    position: relative;
    z-index: 3;
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    height: 48px;
    min-height: 48px;
    max-height: 48px;
    flex-shrink: 0;
    contain: layout style;
    isolation: isolate;
    padding: 0 var(--page-pad-x);
    box-sizing: content-box;
    margin: 30px 0 36px 0;
  }

  .library-tabs {
    position: relative;
    height: 48px;
    flex: 1 1 auto;
    min-width: 0;
    opacity: 0;
  }

  .library-tabs.is-ready {
    opacity: 1;
  }

  .tab-baseline-probe {
    position: absolute;
    left: 0;
    bottom: 0;
    font-family: 'PP Monument Extended', sans-serif;
    font-size: 48px;
    line-height: 1;
    white-space: nowrap;
    visibility: hidden;
    pointer-events: none;
  }

  .probe-mark {
    display: inline-block;
    width: 0;
    height: 0;
    vertical-align: baseline;
  }

  /* Two stacked labels (Regular + Black) cross-fade while GPU transform glides
     at a fixed 48px font size, eliminating reflow jitter and glyph hinting snaps. */
  .library-tab {
    position: absolute;
    left: 0;
    bottom: 0;
    display: grid;
    margin: 0;
    padding: 0;
    background: transparent;
    border: none;
    cursor: pointer;
    font-family: 'PP Monument Extended', sans-serif;
    font-size: 48px;
    line-height: 1;
    white-space: nowrap;
    text-rendering: geometricPrecision;
    -webkit-font-smoothing: antialiased;
    will-change: transform;
    backface-visibility: hidden;
  }

  .library-tabs.is-animated .library-tab {
    transition: transform 0.45s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .tab-label {
    grid-area: 1 / 1;
    justify-self: start;
    text-rendering: geometricPrecision;
    -webkit-font-smoothing: antialiased;
    transition:
      opacity 0.35s ease,
      color 0.35s ease;
  }

  .tab-regular {
    font-weight: 400;
    color: #5f5f5f;
    opacity: 1;
  }

  .tab-black {
    font-weight: 900;
    color: #ffffff;
    text-shadow: 0 4px 4px rgba(0, 0, 0, 0.25);
    opacity: 0;
    pointer-events: none;
  }

  .library-tab:hover .tab-regular {
    color: #8a8a8a;
  }

  .library-tab.active .tab-regular {
    opacity: 0;
  }

  .library-tab.active .tab-black {
    opacity: 1;
  }

  .library-view-modes {
    position: relative;
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 3px;
    border-radius: 11px;
    background: rgba(255, 255, 255, 0.04);
    margin-bottom: 2px;
  }

  .view-pill {
    position: absolute;
    top: 3px;
    left: 3px;
    width: 30px;
    height: 30px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.1);
    pointer-events: none;
    transition: transform 0.32s var(--ease-out);
  }

  .library-view-modes.is-grid .view-pill {
    transform: translate3d(34px, 0, 0);
  }

  .view-btn {
    position: relative;
    z-index: 1;
    width: 30px;
    height: 30px;
    margin: 0;
    padding: 0;
    background: transparent;
    border: none;
    border-radius: 8px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    color: rgba(255, 255, 255, 0.35);
    transition: color 0.2s ease, transform 0.1s ease;
  }

  .view-btn:hover {
    color: rgba(255, 255, 255, 0.72);
  }

  .view-btn:active {
    transform: scale(0.92);
  }

  .view-btn.active {
    color: #ffffff;
  }

  .library-views {
    position: relative;
    z-index: 2;
    display: grid;
    width: 100%;
    min-width: 0;
    contain: layout style;
    isolation: isolate;
    transform: translate3d(0, 0, 0);
  }

  .library-list {
    grid-area: 1 / 1;
    position: relative;
    list-style: none;
    margin: 0 var(--page-pad-x) 40px var(--page-pad-x);
    padding: 0;
    display: flex;
    flex-direction: column;
    border-radius: var(--radius-card);
    overflow: hidden;
  }

  .library-row {
    position: relative;
    height: 268px;
    overflow: hidden;
    display: flex;
    align-items: flex-start;
    gap: 25px;
    padding: 26px 24px 0 28px;
    box-sizing: border-box;
    flex-shrink: 0;
    contain: layout paint;
  }

  /* Blurred poster as the row background */
  .row-bg {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    filter: blur(22px);
    transform: translate3d(0, 0, 0) scale(1.15);
    will-change: transform;
    backface-visibility: hidden;
    pointer-events: none;
    user-select: none;
  }

  .row-dim {
    position: absolute;
    inset: 0;
    background: rgba(4, 4, 4, 0.78);
    transform: translate3d(0, 0, 0);
    pointer-events: none;
    transition: background-color var(--hover-dur) var(--ease-out);
  }

  .library-row:hover .row-dim {
    background: rgba(4, 4, 4, 0.62);
  }

  .row-poster {
    position: relative;
    flex: 0 0 245px;
    width: 245px;
    height: 215px;
    object-fit: cover;
    border-radius: var(--radius-card);
    user-select: none;
    pointer-events: none;
    transition: transform var(--hover-dur) var(--ease-out);
  }

  .library-row:hover .row-poster {
    transform: scale(1.02);
  }

  .row-info {
    position: relative;
    flex: 1 1 auto;
    min-width: 0;
    max-width: 741px;
  }

  .row-title {
    margin: 0;
    padding: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 36px;
    font-weight: 700;
    line-height: 1.1;
    color: #ffffff;
  }

  .row-description {
    margin: 4px 0 0 0;
    padding: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 20px;
    font-weight: 400;
    line-height: 1.3;
    color: var(--text-muted);
  }

  .row-action {
    position: absolute;
    top: 23px;
    right: 24px;
  }

  .library-grid {
    grid-area: 1 / 1;
    position: relative;
    list-style: none;
    margin: 0 var(--page-pad-x) 40px var(--page-pad-x);
    padding: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 18px;
  }

  .library-tile {
    position: relative;
    aspect-ratio: 1 / 1;
    border-radius: var(--radius-card);
    overflow: hidden;
    background: #121212;
    contain: layout style;
    isolation: isolate;
    transform: translate3d(0, 0, 0);
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
  }

  .library-tile::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: inherit;
    border: 1px solid rgba(255, 255, 255, 0.08);
    box-sizing: border-box;
    pointer-events: none;
    z-index: 4;
  }

  .tile-poster {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    border-radius: inherit;
    object-fit: cover;
    user-select: none;
    pointer-events: none;
    will-change: transform;
    transition: transform 0.4s var(--ease-out);
  }

  .library-tile:hover .tile-poster {
    transform: scale(1.04);
  }

  .tile-overlay {
    position: absolute;
    inset: 0;
    border-radius: inherit;
    background: linear-gradient(
      180deg,
      rgba(7, 7, 7, 0.06) 0%,
      rgba(7, 7, 7, 0.36) 45%,
      rgba(7, 7, 7, 0.92) 100%
    );
    transform: translate3d(0, 0, 0);
    pointer-events: none;
    transition: opacity var(--hover-dur) var(--ease-out);
  }

  .library-tile:hover .tile-overlay {
    opacity: 0.9;
  }

  .tile-hover-dim {
    position: absolute;
    inset: 0;
    border-radius: inherit;
    background: linear-gradient(
      180deg,
      rgba(7, 7, 7, 0.78) 0%,
      rgba(7, 7, 7, 0.48) 56%,
      rgba(7, 7, 7, 0.1) 88%
    );
    opacity: 0;
    transform: translate3d(0, 0, 0);
    pointer-events: none;
    transition: opacity var(--hover-dur) var(--ease-out);
  }

  .library-tile:hover .tile-hover-dim,
  .library-tile:focus-within .tile-hover-dim {
    opacity: 1;
  }

  .tile-description {
    position: absolute;
    top: 20px;
    left: 20px;
    right: 20px;
    z-index: 2;
    margin: 0;
    padding: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 16px;
    font-weight: 400;
    line-height: 1.42;
    color: rgba(255, 255, 255, 0.54);
    display: -webkit-box;
    -webkit-line-clamp: 6;
    line-clamp: 6;
    -webkit-box-orient: vertical;
    overflow: hidden;
    opacity: 0;
    transform: translate3d(0, -6px, 0);
    pointer-events: none;
    transition:
      opacity 0.28s var(--ease-out),
      transform 0.28s var(--ease-out);
  }

  .library-tile:hover .tile-description,
  .library-tile:focus-within .tile-description {
    opacity: 1;
    transform: translate3d(0, 0, 0);
  }

  .tile-footer {
    position: relative;
    z-index: 2;
    padding: 18px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .tile-title {
    margin: 0;
    padding: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 19px;
    font-weight: 700;
    line-height: 1.15;
    color: #ffffff;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .tile-action {
    display: flex;
    align-items: center;
  }
</style>
