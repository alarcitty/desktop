<script lang="ts">
  import { fade, scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import X from '@lucide/svelte/icons/x';

  import HlsPlayer from '../../marlakit/player/HlsPlayer.svelte';
  import { episodeSources } from '../../api/anime';
  import { player, closePlayer, selectEpisode, nextEpisode } from '../../app/player.svelte';

  const currentEpisode = $derived(
    player.release?.episodes[player.episodeIndex] ?? null
  );

  const hasNext = $derived(
    Boolean(player.release && player.episodeIndex + 1 < player.release.episodes.length)
  );

  const subtitle = $derived.by(() => {
    if (!currentEpisode) return '';
    const epLabel = `Episode ${currentEpisode.ordinal}`;
    const namePart = currentEpisode.name ? ` · ${currentEpisode.name}` : '';
    const notePart = player.release?.fallbackNote ? ` · ${player.release.fallbackNote}` : '';
    return `${epLabel}${namePart}${notePart}`;
  });

  function handleWindowKeydown(e: KeyboardEvent) {
    if (player.open && e.key === 'Escape') {
      e.preventDefault();
      closePlayer();
    }
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if player.open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="player-backdrop"
    transition:fade={{ duration: 220, easing: cubicOut }}
    onclick={closePlayer}
  >
    <div
      class="player-modal"
      transition:scale={{ start: 0.96, duration: 240, easing: cubicOut }}
      onclick={(e) => e.stopPropagation()}
      role="dialog"
      aria-modal="true"
      aria-label={player.title}
      tabindex="-1"
    >
      <div class="modal-head">
        <div class="modal-meta">
          <h2 class="modal-title">{player.title}</h2>
          {#if player.release?.fallbackNote}
            <span class="modal-note">{player.release.fallbackNote}</span>
          {/if}
        </div>
        <button
          type="button"
          class="close-btn"
          aria-label="Close player"
          onclick={closePlayer}
        >
          <X size={18} strokeWidth={2} />
        </button>
      </div>

      <div class="stage-wrap">
        {#if player.loading}
          <div class="stage-state">
            <div class="spinner"></div>
            <span>Loading HLS 1080p stream...</span>
          </div>
        {:else if player.error}
          <div class="stage-state stage-error">
            <span>{player.error}</span>
          </div>
        {:else if currentEpisode}
          <HlsPlayer
            sources={episodeSources(currentEpisode)}
            poster={currentEpisode.preview || player.release?.banner || player.release?.poster || ''}
            title={player.title}
            {subtitle}
            autoplay={true}
            preferMaxQuality={true}
            opening={currentEpisode.opening}
            ending={currentEpisode.ending}
            {hasNext}
            onnext={nextEpisode}
          />
        {/if}
      </div>

      {#if player.release && player.release.episodes.length > 0}
        <div class="episodes-strip" aria-label="Episodes">
          {#each player.release.episodes as ep, i (ep.id)}
            <button
              type="button"
              class="ep-chip"
              class:active={i === player.episodeIndex}
              onclick={() => selectEpisode(i)}
            >
              Ep {ep.ordinal}
            </button>
          {/each}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .player-backdrop {
    position: fixed;
    inset: 0;
    z-index: 1200;
    background: rgba(5, 5, 5, 0.78);
    backdrop-filter: blur(18px);
    -webkit-backdrop-filter: blur(18px);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 28px;
    box-sizing: border-box;
  }

  .player-modal {
    width: min(1080px, 100%);
    max-height: calc(100vh - 56px);
    background: rgba(14, 14, 14, 0.92);
    border: 1px solid rgba(255, 255, 255, 0.08);
    border-radius: 22px;
    padding: 16px;
    box-sizing: border-box;
    box-shadow: 0 32px 90px rgba(0, 0, 0, 0.7);
    display: flex;
    flex-direction: column;
    gap: 12px;
    overflow: hidden;
  }

  .modal-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 0 4px;
  }

  .modal-meta {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .modal-title {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 18px;
    font-weight: 700;
    color: #ffffff;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .modal-note {
    font-family: 'Medeo Sans', system-ui, sans-serif;
    font-size: 12px;
    color: rgba(255, 255, 255, 0.55);
  }

  .close-btn {
    width: 34px;
    height: 34px;
    flex-shrink: 0;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.06);
    color: rgba(255, 255, 255, 0.85);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: background-color 0.15s ease, color 0.15s ease;
  }

  .close-btn:hover {
    background: rgba(255, 255, 255, 0.14);
    color: #ffffff;
  }

  .stage-wrap {
    width: 100%;
    aspect-ratio: 16 / 9;
    max-height: calc(100vh - 200px);
    background: #070707;
    border-radius: 16px;
    overflow: hidden;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .stage-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    color: rgba(255, 255, 255, 0.72);
    font-family: 'Medeo Sans', system-ui, sans-serif;
    font-size: 14px;
  }

  .stage-error {
    color: #ff9c9c;
  }

  .spinner {
    width: 34px;
    height: 34px;
    border-radius: 50%;
    border: 3px solid rgba(255, 255, 255, 0.16);
    border-top-color: #ffffff;
    animation: modal-spin 0.75s linear infinite;
  }

  @keyframes modal-spin {
    to {
      transform: rotate(360deg);
    }
  }

  .episodes-strip {
    display: flex;
    align-items: center;
    gap: 6px;
    overflow-x: auto;
    padding: 2px 2px 4px;
    scrollbar-width: thin;
  }

  .ep-chip {
    flex-shrink: 0;
    height: 32px;
    padding: 0 12px;
    border-radius: 9px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.05);
    color: rgba(255, 255, 255, 0.7);
    font-family: 'Medeo Sans', system-ui, sans-serif;
    font-size: 13px;
    cursor: pointer;
    transition: background-color 0.15s ease, color 0.15s ease, border-color 0.15s ease;
  }

  .ep-chip:hover {
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
  }

  .ep-chip.active {
    background: #ffffff;
    border-color: #ffffff;
    color: #111111;
    font-weight: 600;
  }
</style>
