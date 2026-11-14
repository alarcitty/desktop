<script lang="ts">
  import { onMount } from 'svelte';
  import HlsPlayer from '../marlakit/player/HlsPlayer.svelte';
  import { episodeSources } from '../api/anime';
  import {
    closeCustomPip,
    expandCustomPip,
    fallbackVoice,
    nextEpisode,
    player,
    registerPlayerPauseControl,
    reportPlaybackProgress,
    setPipCorner,
    setPlayerPlaying,
    switchVoice,
    toggleCustomPip,
    type PipCorner
  } from './player.svelte';
  import { router } from './router.svelte';

  let hlsPlayerRef = $state<ReturnType<typeof HlsPlayer> | null>(null);
  let liveNodeEl = $state<HTMLDivElement | null>(null);
  let pipShellEl = $state<HTMLDivElement | null>(null);
  let parkingEl = $state<HTMLDivElement | null>(null);

  let isDragging = $state(false);
  let dragDx = $state(0);
  let dragDy = $state(0);

  const episodes = $derived(player.release?.episodes ?? []);
  const currentEpisode = $derived(episodes[player.episodeIndex] ?? null);
  const hasNext = $derived(Boolean(player.release && player.episodeIndex + 1 < episodes.length));

  const displayTitle = $derived(
    player.shikiArt?.english || player.shikiArt?.name || player.release?.title || player.title || 'Anime'
  );

  const posterUrl = $derived(
    player.shikiArt?.posterOriginal ||
      player.shikiArt?.posterMain ||
      player.release?.poster ||
      ''
  );

  const backdropUrl = $derived(
    player.shikiArt?.screenshots[0] ||
      player.release?.banner ||
      posterUrl
  );

  let prevPage = router.current;

  $effect.pre(() => {
    const curPage = router.current;
    if (prevPage === 'anime' && curPage !== 'anime') {
      if (currentEpisode && player.isPlaying && !player.pipDismissed) {
        player.pipMode = true;
      }
    } else if (prevPage !== 'anime' && curPage === 'anime') {
      if (!player.pipManual) {
        player.pipMode = false;
      }
    }
    prevPage = curPage;
  });

  const isPipVisible = $derived(
    Boolean(
      currentEpisode &&
        !player.loading &&
        !player.error &&
        !player.pipDismissed &&
        (player.pipMode || (router.current !== 'anime' && player.isPlaying))
    )
  );

  const isInlineVisible = $derived(
    Boolean(
      currentEpisode &&
        !player.loading &&
        !player.error &&
        router.current === 'anime' &&
        !isPipVisible &&
        player.inlineStageEl
    )
  );

  $effect.pre(() => {
    const node = liveNodeEl;
    if (!node) return;

    let targetParent: HTMLElement | null = parkingEl;
    if (isInlineVisible && player.inlineStageEl) {
      targetParent = player.inlineStageEl;
    } else if (isPipVisible && pipShellEl) {
      targetParent = pipShellEl;
    }

    if (targetParent && node.parentElement !== targetParent) {
      const wasPlaying = hlsPlayerRef?.beginReparent() ?? player.isPlaying;
      targetParent.appendChild(node);
      hlsPlayerRef?.endReparent(wasPlaying);
    }
  });

  onMount(() => {
    registerPlayerPauseControl(() => {
      hlsPlayerRef?.pause();
    });
    return () => {
      registerPlayerPauseControl(null);
    };
  });

  function handlePipDragStart(e: PointerEvent) {
    if (!pipShellEl || !isPipVisible) return;
    const startX = e.clientX;
    const startY = e.clientY;
    const initialRect = pipShellEl.getBoundingClientRect();
    let moved = false;

    const onMove = (ev: PointerEvent) => {
      const rawDx = ev.clientX - startX;
      const rawDy = ev.clientY - startY;
      if (!moved && Math.hypot(rawDx, rawDy) < 5) {
        return;
      }
      moved = true;
      isDragging = true;

      const minDx = 16 - initialRect.left;
      const maxDx = window.innerWidth - 16 - initialRect.right;
      const minDy = 16 - initialRect.top;
      const maxDy = window.innerHeight - 16 - initialRect.bottom;

      dragDx = Math.max(minDx, Math.min(maxDx, rawDx));
      dragDy = Math.max(minDy, Math.min(maxDy, rawDy));
    };

    const onUp = () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
      window.removeEventListener('pointercancel', onUp);

      if (!moved || !pipShellEl) {
        isDragging = false;
        dragDx = 0;
        dragDy = 0;
        return;
      }

      hlsPlayerRef?.notifyPipDragged();
      const finalRect = pipShellEl.getBoundingClientRect();
      const centerX = finalRect.left + finalRect.width / 2;
      const centerY = finalRect.top + finalRect.height / 2;

      const isLeft = centerX < window.innerWidth / 2;
      const isTop = centerY < window.innerHeight / 2;
      const nextCorner: PipCorner = isTop
        ? isLeft
          ? 'top-left'
          : 'top-right'
        : isLeft
          ? 'bottom-left'
          : 'bottom-right';

      setPipCorner(nextCorner);
      isDragging = false;
      dragDx = 0;
      dragDy = 0;
    };

    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    window.addEventListener('pointercancel', onUp);
  }
</script>

<!-- Hidden parking container when player is neither inline nor in PiP -->
<div class="player-parking" bind:this={parkingEl} aria-hidden="true">
  <div class="player-live-node" bind:this={liveNodeEl}>
    {#if currentEpisode}
      <HlsPlayer
        bind:this={hlsPlayerRef}
        sources={episodeSources(currentEpisode)}
        poster={currentEpisode.preview || backdropUrl}
        title={`Episode ${currentEpisode.ordinal}`}
        subtitle={isPipVisible ? displayTitle : ''}
        autoplay={player.autoplay}
        initialTime={player.resumeTime}
        voices={player.voices}
        selectedVoiceId={player.selectedVoiceId}
        preferMaxQuality={true}
        opening={currentEpisode.opening}
        ending={currentEpisode.ending}
        {hasNext}
        hasFallback={player.voices.some((v) => v.id !== player.selectedVoiceId)}
        isPip={isPipVisible}
        onnext={nextEpisode}
        ontimeprogress={(sec, dur, force) => reportPlaybackProgress(sec, dur, force)}
        onselectvoice={(voice, sec) => void switchVoice(voice, sec)}
        onfallback={() => void fallbackVoice(false)}
        onpip={toggleCustomPip}
        onexpandpip={expandCustomPip}
        onclosepip={closeCustomPip}
        onplaychange={setPlayerPlaying}
        onpipdragstart={handlePipDragStart}
        onerror={() => {
          if ((player.release?.provider ?? 'aniliberty') === 'aniliberty') {
            void fallbackVoice(true);
          }
        }}
      />
    {/if}
  </div>
</div>

<!-- Custom rounded-corner Picture-in-Picture floating dock -->
<div
  bind:this={pipShellEl}
  class="pip-floating-shell corner-{player.pipCorner}"
  class:is-visible={isPipVisible}
  class:is-dragging={isDragging}
  style:transform={isDragging
    ? `translate3d(${dragDx}px, ${dragDy}px, 0)`
    : undefined}
  aria-label="Picture in picture mini-player"
></div>

<style>
  .player-parking {
    position: fixed;
    width: 0;
    height: 0;
    overflow: hidden;
    opacity: 0;
    pointer-events: none;
    z-index: -1;
  }

  .player-live-node {
    width: 100%;
    height: 100%;
  }

  .pip-floating-shell {
    position: fixed;
    z-index: 9400;
    width: clamp(340px, 27vw, 436px);
    aspect-ratio: 16 / 9;
    border-radius: 18px;
    overflow: hidden;
    clip-path: inset(0 round 18px);
    background: #070707;
    box-shadow:
      0 24px 60px rgba(0, 0, 0, 0.78),
      0 6px 20px rgba(0, 0, 0, 0.52);
    opacity: 0;
    pointer-events: none;
    transform: translate3d(0, 14px, 0) scale(0.95);
    transition:
      opacity 0.24s cubic-bezier(0.22, 1, 0.36, 1),
      transform 0.28s cubic-bezier(0.22, 1, 0.36, 1),
      top 0.3s cubic-bezier(0.22, 1, 0.36, 1),
      right 0.3s cubic-bezier(0.22, 1, 0.36, 1),
      bottom 0.3s cubic-bezier(0.22, 1, 0.36, 1),
      left 0.3s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .pip-floating-shell.is-visible {
    opacity: 1;
    pointer-events: auto;
    transform: translate3d(0, 0, 0) scale(1);
  }

  .pip-floating-shell.is-dragging {
    transition: opacity 0.18s ease !important;
    cursor: grabbing;
  }

  .pip-floating-shell.corner-bottom-right {
    right: 26px;
    bottom: 26px;
  }

  .pip-floating-shell.corner-bottom-left {
    left: 26px;
    bottom: 26px;
  }

  .pip-floating-shell.corner-top-right {
    right: 26px;
    top: 86px;
  }

  .pip-floating-shell.corner-top-left {
    left: 26px;
    top: 86px;
  }
</style>
