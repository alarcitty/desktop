<script lang="ts">
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
  let activeTab = $state(0);
  let loadingId = $state<string | null>(null);

  function handleWatch(aId: string)
  {
    loadingId = aId;
    setTimeout(() =>
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
    <div class="library-tabs">
      {#each tabs as tab, index}
        <button
          type="button"
          class="library-tab"
          class:active={activeTab === index}
          onclick={() => (activeTab = index)}
        >
          <span class="tab-label tab-regular">{tab}</span>
          <span class="tab-label tab-black" aria-hidden="true">{tab}</span>
        </button>
      {/each}
    </div>

    <!-- View mode icons (visual only for now) -->
    <div class="library-view-modes">
      <button type="button" class="view-btn active" aria-label="List view">
        <LayoutList size={18} strokeWidth={1.7} />
      </button>
      <button type="button" class="view-btn" aria-label="Grid view">
        <LayoutGrid size={18} strokeWidth={1.7} />
      </button>
    </div>
  </div>

  <ul class="library-list">
    {#each items as item (item.id)}
      <li class="library-row">
        <img src={item.poster} alt="" class="row-bg" style:object-position={item.posterPosition} />
        <div class="row-dim"></div>

        <img
          src={item.poster}
          alt={item.title}
          class="row-poster"
          style:object-position={item.posterPosition}
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
  }

  .library-ambient {
    position: absolute;
    top: 0;
    left: 0;
    width: 100%;
    height: 700px;
    pointer-events: none;
    z-index: 0;
    -webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 1) 0%, rgba(0, 0, 0, 0) 100%);
    mask-image: linear-gradient(180deg, rgba(0, 0, 0, 1) 0%, rgba(0, 0, 0, 0) 100%);
  }

  .library-head {
    position: relative;
    z-index: 2;
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    height: 48px;
    padding: 0 var(--page-pad-x);
    box-sizing: content-box;
    margin: 30px 0 36px 0;
  }

  .library-tabs {
    display: flex;
    align-items: baseline;
    gap: 22px;
  }

  /* Two stacked labels (Regular + Black) cross-fade while the size glides,
     so the weight change never "pops". */
  .library-tab {
    display: grid;
    margin: 0;
    padding: 0;
    background: transparent;
    border: none;
    cursor: pointer;
    font-family: 'PP Monument Extended', sans-serif;
    font-size: 24px;
    line-height: 1;
    white-space: nowrap;
    transition: font-size 0.5s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .library-tab.active {
    font-size: 48px;
  }

  .tab-label {
    grid-area: 1 / 1;
    justify-self: start;
    transition:
      opacity 0.4s ease,
      color 0.4s ease;
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
    display: flex;
    align-items: center;
    gap: 14px;
    padding-bottom: 6px;
  }

  .view-btn {
    margin: 0;
    padding: 0;
    background: transparent;
    border: none;
    display: inline-flex;
    cursor: pointer;
    color: rgba(255, 255, 255, 0.18);
  }

  .view-btn.active {
    color: #ffffff;
  }

  .library-list {
    position: relative;
    z-index: 2;
    list-style: none;
    margin: 0 var(--page-pad-x);
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
  }

  /* Blurred poster as the row background */
  .row-bg {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    filter: blur(22px);
    transform: scale(1.15);
    pointer-events: none;
    user-select: none;
  }

  .row-dim {
    position: absolute;
    inset: 0;
    background: rgba(4, 4, 4, 0.78);
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
</style>
