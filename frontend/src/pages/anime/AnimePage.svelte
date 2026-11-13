<script lang="ts">
  import { fade, scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import Play from '@lucide/svelte/icons/play';
  import Bookmark from '@lucide/svelte/icons/bookmark';
  import Check from '@lucide/svelte/icons/check';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import X from '@lucide/svelte/icons/x';

  import MainButton from '../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../marlakit/button/SecondButton.svelte';
  import MetaLine from '../../marlakit/meta/MetaLine.svelte';
  import LayerBlur from '../../marlakit/blur/LayerBlur.svelte';
  import HlsPlayer from '../../marlakit/player/HlsPlayer.svelte';
  import { buildMetaTags, episodeSources, type AnimeSeasonEntry } from '../../api/anime';
  import { isInWatchLater, toggleWatchLater } from '../../app/library.svelte';
  import { goBack } from '../../app/router.svelte';
  import {
    fallbackVoice,
    nextEpisode,
    player,
    reportPlaybackProgress,
    retryWatch,
    selectEpisode,
    switchSeason,
    switchVoice
  } from '../../app/player.svelte';

  const EP_PAGE_SIZE = 16;

  let playerSectionEl = $state<HTMLElement | null>(null);
  let descTextEl = $state<HTMLParagraphElement | null>(null);
  let descExpanded = $state(false);
  let descCollapsedH = $state(78);
  let descExpandedH = $state(0);
  let isPlanLoading = $state(false);
  let epRangeIndex = $state(0);
  let lightboxUrl = $state<string | null>(null);
  let failedThumbs = $state<Record<string, boolean>>({});

  const displayTitle = $derived(
    player.shikiArt?.english || player.shikiArt?.name || player.release?.title || player.title || 'Anime'
  );

  const subTitle = $derived.by(() => {
    const romaji = player.shikiArt?.name;
    const ru = player.shikiArt?.russian;
    if (romaji && romaji !== displayTitle) return romaji;
    if (ru && ru !== displayTitle) return ru;
    return '';
  });

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

  const score = $derived(
    player.shikiArt?.score && player.shikiArt.score > 0
      ? Number(player.shikiArt.score.toFixed(1))
      : undefined
  );

  const metaTags = $derived.by(() => {
    const tags = buildMetaTags(player.shikiArt ?? undefined);
    const epCount = player.release?.episodes.length ?? 0;
    if (tags.length > 0) {
      if (
        epCount > 0 &&
        player.shikiArt?.status !== 'ongoing' &&
        player.shikiArt?.episodes &&
        player.shikiArt.episodes !== epCount
      ) {
        return tags.map((tag) => (/^\d+ ep$/.test(tag) ? `${epCount} ep` : tag));
      }
      return tags;
    }
    return epCount > 0 ? [`${epCount} ep`] : [];
  });

  function formatSingleSeasonLabel(typeLabel?: string): string {
    switch ((typeLabel ?? '').toUpperCase()) {
      case 'MOVIE':
        return 'Movie';
      case 'OVA':
        return 'OVA';
      case 'SPECIAL':
        return 'Special';
      default:
        return 'Season 1';
    }
  }

  const displaySeasons = $derived.by((): AnimeSeasonEntry[] => {
    if (player.seasons.length > 0) {
      return player.seasons;
    }
    if (!player.release) {
      return [];
    }
    return [
      {
        id: player.release.id,
        alias: player.release.alias,
        shikiId: player.shikiArt?.id,
        title: displayTitle,
        shortLabel: formatSingleSeasonLabel(player.release.typeLabel),
        year: player.release.year ?? player.shikiArt?.year,
        episodesTotal: player.release.episodes.length,
        poster: posterUrl
      }
    ];
  });

  const fullDescription = $derived(
    player.shikiArt?.description?.trim() || ''
  );

  const isLongDescription = $derived(
    descExpandedH > 0 ? descExpandedH > descCollapsedH + 8 : fullDescription.length > 290
  );

  $effect(() => {
    void fullDescription;
    descExpanded = false;
  });

  $effect(() => {
    const el = descTextEl;
    if (!el || !fullDescription) {
      descExpandedH = 0;
      return;
    }

    const measure = () => {
      const lh = parseFloat(getComputedStyle(el).lineHeight) || 26;
      descCollapsedH = Math.round(lh * 3);
      descExpandedH = Math.ceil(el.scrollHeight);
    };

    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => {
      ro.disconnect();
    };
  });

  const episodes = $derived(player.release?.episodes ?? []);

  const currentEpisode = $derived(episodes[player.episodeIndex] ?? null);

  const hasNext = $derived(
    Boolean(player.release && player.episodeIndex + 1 < episodes.length)
  );

  const episodeRanges = $derived.by(() => {
    if (episodes.length <= EP_PAGE_SIZE) return [];
    const ranges: Array<{ label: string; start: number; end: number }> = [];
    for (let start = 0; start < episodes.length; start += EP_PAGE_SIZE) {
      const end = Math.min(start + EP_PAGE_SIZE, episodes.length);
      const firstOrd = episodes[start]?.ordinal ?? start + 1;
      const lastOrd = episodes[end - 1]?.ordinal ?? end;
      ranges.push({ label: `${firstOrd}–${lastOrd}`, start, end });
    }
    return ranges;
  });

  const visibleEpisodes = $derived.by(() => {
    if (episodeRanges.length === 0) {
      return episodes.map((ep, idx) => ({ ep, idx }));
    }
    const range = episodeRanges[epRangeIndex] ?? episodeRanges[0];
    return episodes.slice(range.start, range.end).map((ep, offset) => ({
      ep,
      idx: range.start + offset
    }));
  });

  const galleryShots = $derived((player.shikiArt?.screenshots ?? []).slice(0, 6));

  $effect(() => {
    void player.release?.id;
    failedThumbs = {};
  });

  $effect(() => {
    // Keep active episode range synced when auto-advancing to the next episode.
    if (episodes.length > EP_PAGE_SIZE) {
      epRangeIndex = Math.floor(player.episodeIndex / EP_PAGE_SIZE);
    } else {
      epRangeIndex = 0;
    }
  });

  const inLibrary = $derived(
    isInWatchLater({
      shikiId: player.shikiArt?.id ?? player.target?.shikiId,
      releaseAlias: player.release?.alias ?? player.target?.releaseAlias,
      title: displayTitle
    })
  );

  function formatDuration(sec: number): string {
    if (!sec || sec <= 0) return '';
    const mins = Math.round(sec / 60);
    return mins > 0 ? `${mins}m` : `${sec}s`;
  }

  function scrollToPlayer() {
    playerSectionEl?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }

  function handleChooseEpisode(index: number) {
    selectEpisode(index);
    scrollToPlayer();
  }

  function handlePlanToggle() {
    if (!displayTitle) return;
    isPlanLoading = true;
    toggleWatchLater({
      shikiId: player.shikiArt?.id ?? player.target?.shikiId,
      releaseAlias: player.release?.alias ?? player.target?.releaseAlias,
      searchQueries: player.target?.searchQueries,
      title: displayTitle,
      description: fullDescription,
      poster: posterUrl,
      score,
      tags: metaTags
    });
    isPlanLoading = false;
  }

  function handleWindowKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && lightboxUrl) {
      lightboxUrl = null;
    }
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

<div class="anime-page">
  <!-- Spacious asymmetrical cinema showcase hero -->
  <div class="anime-hero-bg">
    {#if backdropUrl}
      <img src={backdropUrl} alt="" aria-hidden="true" class="backdrop-img" decoding="async" />
    {/if}
    <div class="backdrop-dim" aria-hidden="true"></div>
    <div class="backdrop-fade" aria-hidden="true"></div>

    <div class="anime-top-inner">
      <div class="anime-overview">
        <div class="overview-meta-col">
          {#if subTitle}
            <span class="top-origin-title">{subTitle}</span>
          {/if}
          <h1 class="anime-title">{displayTitle}</h1>

          {#if score || metaTags.length > 0}
            <div class="anime-meta-line">
              <MetaLine {score} tags={metaTags} />
            </div>
          {/if}

          {#if fullDescription}
            {#key fullDescription}
              <div class="desc-block">
                <div
                  class="desc-viewport"
                  class:is-collapsible={isLongDescription}
                  class:is-expanded={descExpanded}
                  style:max-height={isLongDescription
                    ? `${descExpanded ? (descExpandedH || 600) : descCollapsedH}px`
                    : 'none'}
                >
                  <p
                    bind:this={descTextEl}
                    class="anime-description"
                    class:is-expanded={descExpanded || !isLongDescription}
                  >
                    {fullDescription}
                  </p>
                </div>
                {#if isLongDescription}
                  <button
                    type="button"
                    class="desc-toggle"
                    aria-expanded={descExpanded}
                    onclick={() => { descExpanded = !descExpanded; }}
                  >
                    {descExpanded ? 'Show less' : 'Read more'}
                  </button>
                {/if}
              </div>
            {/key}
          {/if}

          <div class="overview-actions">
            <MainButton loading={player.loading} onclick={scrollToPlayer}>
              {#snippet icon()}
                <Play size={16} fill="currentColor" strokeWidth={0} />
              {/snippet}
              Watch Ep {currentEpisode?.ordinal ?? 1}
            </MainButton>

            <SecondButton loading={isPlanLoading} onclick={handlePlanToggle}>
              {#snippet icon()}
                {#if inLibrary}
                  <Check size={16} strokeWidth={2.2} />
                {:else}
                  <Bookmark size={16} strokeWidth={2} />
                {/if}
              {/snippet}
              {inLibrary ? 'In Plan to Watch' : 'Plan to Watch'}
            </SecondButton>

            <button
              type="button"
              class="back-btn"
              onclick={goBack}
              aria-label="Go back"
              title="Go back"
            >
              <ArrowLeft size={16} strokeWidth={2} />
              <span>Back</span>
            </button>
          </div>
        </div>

        {#if posterUrl}
          <div class="showcase-art">
            <div class="poster-glow" aria-hidden="true">
              <LayerBlur src={posterUrl} blur={72} opacity={0.54} scale={1.24} offsetY={0} />
            </div>
            <div class="poster-wrap">
              <img src={posterUrl} alt={displayTitle} class="poster-img" decoding="async" />
              <div class="poster-edge-vignette" aria-hidden="true"></div>
            </div>
          </div>
        {/if}
      </div>

      {#if displaySeasons.length > 0}
        <nav class="seasons-bar" aria-label="Seasons">
          {#each displaySeasons as season (season.id)}
            {@const isActive = player.release
              ? player.release.id === season.id || player.release.alias === season.alias
              : player.target?.releaseAlias === season.alias ||
                (player.target?.shikiId != null && player.target.shikiId === season.shikiId)}
            {@const isInteractive = displaySeasons.length > 1 && !isActive}
            <button
              type="button"
              class="season-tab"
              class:active={isActive}
              class:is-static={displaySeasons.length === 1}
              aria-current={isActive ? 'page' : undefined}
              onclick={() => {
                if (isInteractive) void switchSeason(season);
              }}
            >
              <span>{season.shortLabel}</span>
              {#if season.year}
                <span class="season-year">{season.year}</span>
              {/if}
            </button>
          {/each}
        </nav>
      {/if}
    </div>
  </div>

  <!-- Full-bleed edge-to-edge player section (no side padding, no border-radius) -->
  <section class="player-section" bind:this={playerSectionEl} aria-label="Video player">
    <div class="player-stage">
      {#if player.loading}
        <div class="stage-state" role="status" aria-label="Loading" transition:fade={{ duration: 180 }}>
          <div class="spinner" aria-hidden="true"></div>
        </div>
      {:else if player.error}
        <div class="stage-state stage-error">
          <p class="error-text">{player.error}</p>
          <MainButton onclick={retryWatch}>
            {#snippet icon()}
              <RefreshCw size={15} strokeWidth={2} />
            {/snippet}
            Retry
          </MainButton>
        </div>
      {:else if currentEpisode}
        <HlsPlayer
          sources={episodeSources(currentEpisode)}
          poster={currentEpisode.preview || backdropUrl}
          title={`Episode ${currentEpisode.ordinal}`}
          subtitle=""
          autoplay={player.autoplay}
          initialTime={player.resumeTime}
          voices={player.voices}
          selectedVoiceId={player.selectedVoiceId}
          preferMaxQuality={true}
          opening={currentEpisode.opening}
          ending={currentEpisode.ending}
          {hasNext}
          hasFallback={player.voices.some((v) => v.id !== player.selectedVoiceId)}
          onnext={nextEpisode}
          ontimeprogress={(sec, dur, force) => reportPlaybackProgress(sec, dur, force)}
          onselectvoice={(voice, sec) => void switchVoice(voice, sec)}
          onfallback={() => void fallbackVoice(false)}
          onerror={() => {
            if ((player.release?.provider ?? 'aniliberty') === 'aniliberty') {
              void fallbackVoice(true);
            }
          }}
        />
      {:else}
        <div class="stage-state">
          <span>Select a title with Watch to begin playback.</span>
        </div>
      {/if}
    </div>
  </section>

  <!-- Episode picker & frames gallery -->
  <div class="anime-body">

    {#if episodes.length > 0}
      <section class="episodes-section" aria-label="Episodes">
        <div class="section-head">
          <div class="section-head-left">
            <h2 class="section-heading">Episodes</h2>
            <span class="section-count">{episodes.length}</span>
          </div>

          {#if episodeRanges.length > 1}
            <div class="range-tabs" role="tablist" aria-label="Episode ranges">
              {#each episodeRanges as range, i (range.label)}
                <button
                  type="button"
                  class="range-pill"
                  class:is-active={epRangeIndex === i}
                  onclick={() => { epRangeIndex = i; }}
                >
                  {range.label}
                </button>
              {/each}
            </div>
          {/if}
        </div>

        <div class="episodes-grid">
          {#each visibleEpisodes as entry (entry.ep.id)}
            {@const ep = entry.ep}
            {@const isCurrent = entry.idx === player.episodeIndex}
            <button
              type="button"
              class="ep-card"
              class:is-active={isCurrent}
              onclick={() => handleChooseEpisode(entry.idx)}
            >
              <div class="ep-thumb-wrap">
                {#if ep.preview && !failedThumbs[ep.id]}
                  <img
                    src={ep.preview}
                    alt="Episode {ep.ordinal}"
                    class="ep-thumb"
                    loading="lazy"
                    decoding="async"
                    onerror={() => {
                      failedThumbs = { ...failedThumbs, [ep.id]: true };
                    }}
                  />
                {:else}
                  <div class="ep-thumb-fallback" aria-hidden="true">
                    <Play size={20} fill="currentColor" strokeWidth={0} />
                  </div>
                {/if}
                <div class="ep-thumb-fade" aria-hidden="true"></div>
                <div class="ep-overlay-bottom">
                  <div class="ep-title-row">
                    {#if ep.name}
                      <span class="ep-num">EP {ep.ordinal}</span>
                      <span class="ep-name">{ep.name}</span>
                    {:else}
                      <span class="ep-num">Episode {ep.ordinal}</span>
                    {/if}
                  </div>
                  {#if ep.duration > 0}
                    <span class="ep-duration">{formatDuration(ep.duration)}</span>
                  {/if}
                </div>
              </div>
            </button>
          {/each}
        </div>
      </section>
    {/if}

    {#if galleryShots.length > 0}
      <section class="frames-section" aria-label="Frames">
        <div class="section-head">
          <h2 class="section-heading">Frames</h2>
        </div>

        <div class="frames-grid">
          {#each galleryShots as shot, i (shot)}
            <button
              type="button"
              class="frame-card"
              onclick={() => { lightboxUrl = shot; }}
              aria-label="Open frame {i + 1}"
            >
              <img src={shot} alt="{displayTitle} frame {i + 1}" class="frame-img" loading="lazy" decoding="async" />
            </button>
          {/each}
        </div>
      </section>
    {/if}
  </div>
</div>

{#if lightboxUrl}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="lightbox-backdrop"
    transition:fade={{ duration: 180, easing: cubicOut }}
    onclick={(e) => {
      if (e.target === e.currentTarget) {
        lightboxUrl = null;
      }
    }}
  >
    <button
      type="button"
      class="lightbox-close"
      aria-label="Close frame preview"
      onclick={() => { lightboxUrl = null; }}
    >
      <X size={20} strokeWidth={2} />
    </button>
    <img
      src={lightboxUrl}
      alt={displayTitle}
      class="lightbox-img"
      transition:scale={{ start: 0.96, duration: 200, easing: cubicOut }}
    />
  </div>
{/if}

<style>
  .anime-page {
    width: 100%;
    min-height: 100%;
    background: #0c0c0c;
    color: #ffffff;
    display: flex;
    flex-direction: column;
    padding-bottom: 64px;
    box-sizing: border-box;
  }

  .anime-hero-bg {
    position: relative;
    width: 100%;
    min-height: clamp(500px, 68vh, 650px);
    padding-top: 72px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    contain: layout style paint;
  }

  .backdrop-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center 22%;
    opacity: 0.56;
    pointer-events: none;
    user-select: none;
    z-index: 0;
  }

  .backdrop-dim {
    position: absolute;
    inset: 0;
    background: linear-gradient(
      90deg,
      rgba(12, 12, 12, 0.9) 0%,
      rgba(12, 12, 12, 0.56) 52%,
      rgba(12, 12, 12, 0.44) 100%
    );
    pointer-events: none;
    z-index: 1;
  }

  .backdrop-fade {
    position: absolute;
    inset: 0;
    background: linear-gradient(
      180deg,
      rgba(12, 12, 12, 0.42) 0%,
      rgba(12, 12, 12, 0.22) 38%,
      rgba(12, 12, 12, 0.78) 78%,
      #0c0c0c 100%
    );
    pointer-events: none;
    z-index: 2;
  }

  .anime-top-inner {
    position: relative;
    z-index: 3;
    flex: 1 1 auto;
    padding: 28px var(--page-pad-x) 24px var(--page-pad-x);
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
  }

  .top-origin-title {
    margin-bottom: 12px;
    font-family: 'PP Monument Extended', system-ui, sans-serif;
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: rgba(255, 255, 255, 0.42);
    user-select: none;
  }

  .back-btn {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 43px;
    padding: 0 16px;
    border: none;
    outline: none;
    border-radius: 21px;
    background: transparent;
    color: rgba(255, 255, 255, 0.48);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
    transition: background-color 0.2s var(--ease-out), color 0.2s var(--ease-out), transform 0.2s var(--ease-out);
  }

  .back-btn:hover {
    background: rgba(255, 255, 255, 0.06);
    color: rgba(255, 255, 255, 0.92);
    transform: translateX(-2px);
  }

  .anime-overview {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 48px;
    margin-top: auto;
  }

  .overview-meta-col {
    display: flex;
    flex-direction: column;
    min-width: 0;
    flex: 1 1 auto;
    max-width: 780px;
  }

  .anime-title {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(36px, 4.6vw, 66px);
    font-weight: 700;
    line-height: 1.02;
    letter-spacing: -0.02em;
    color: #ffffff;
  }

  .anime-meta-line {
    margin-top: 16px;
  }

  .desc-block {
    margin-top: 16px;
    max-width: 740px;
    animation: descReveal 0.36s cubic-bezier(0.22, 1, 0.36, 1) both;
  }

  @keyframes descReveal {
    from {
      opacity: 0;
      transform: translate3d(0, 6px, 0);
    }
    to {
      opacity: 1;
      transform: translate3d(0, 0, 0);
    }
  }

  .desc-viewport {
    overflow: hidden;
    transition:
      max-height 0.44s cubic-bezier(0.22, 1, 0.36, 1),
      -webkit-mask-position 0.38s cubic-bezier(0.22, 1, 0.36, 1),
      mask-position 0.38s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .desc-viewport.is-collapsible {
    -webkit-mask-image: linear-gradient(to bottom, #000 0%, #000 80%, transparent 100%);
    mask-image: linear-gradient(to bottom, #000 0%, #000 80%, transparent 100%);
    -webkit-mask-size: 100% 210%;
    mask-size: 100% 210%;
    -webkit-mask-repeat: no-repeat;
    mask-repeat: no-repeat;
    -webkit-mask-position: 0% 100%;
    mask-position: 0% 100%;
  }

  .desc-viewport.is-collapsible.is-expanded {
    -webkit-mask-position: 0% 0%;
    mask-position: 0% 0%;
  }

  .anime-description {
    margin: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 18px;
    line-height: 1.44;
    color: rgba(255, 255, 255, 0.62);
    transition: color 0.36s var(--ease-out);
  }

  .anime-description.is-expanded {
    color: rgba(255, 255, 255, 0.72);
  }

  .desc-toggle {
    margin-top: 8px;
    padding: 0;
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.65);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    cursor: pointer;
    transition: color 0.16s ease;
  }

  .desc-toggle:hover {
    color: #ffffff;
  }

  .overview-actions {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 22px;
  }

  .seasons-bar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 18px;
    margin-top: 24px;
  }

  .season-tab {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    border: 0;
    border-radius: 0;
    padding: 4px 0;
    background: transparent;
    color: rgba(255, 255, 255, 0.5);
    font-family: inherit;
    font-size: 13.5px;
    font-weight: 500;
    letter-spacing: -0.01em;
    cursor: pointer;
    transition:
      color 180ms var(--ease-out),
      transform 180ms var(--ease-out);
  }

  .season-tab:hover {
    background: transparent;
    color: rgba(255, 255, 255, 0.86);
  }

  .season-tab:active {
    transform: scale(0.97);
  }

  .season-tab.active {
    background: transparent;
    color: #ffffff;
    font-weight: 600;
  }

  .season-tab.is-static {
    cursor: default;
  }

  .season-tab.is-static:hover {
    background: transparent;
    color: #ffffff;
  }

  .season-tab.is-static:active {
    transform: none;
  }

  .season-year {
    font-size: 12px;
    font-weight: 500;
    opacity: 0.56;
  }

  .season-tab.active .season-year {
    opacity: 0.65;
  }

  /* Atmospheric borderless poster showcase on right side of hero */
  .showcase-art {
    position: relative;
    width: clamp(280px, 24vw, 350px);
    flex-shrink: 0;
    display: flex;
    align-items: flex-end;
    justify-content: flex-end;
  }

  .poster-glow {
    position: absolute;
    inset: -32px;
    pointer-events: none;
    overflow: hidden;
    -webkit-mask-image: radial-gradient(ellipse at center, rgba(0, 0, 0, 1) 38%, rgba(0, 0, 0, 0) 74%);
    mask-image: radial-gradient(ellipse at center, rgba(0, 0, 0, 1) 38%, rgba(0, 0, 0, 0) 74%);
  }

  .poster-wrap {
    position: relative;
    width: 100%;
    aspect-ratio: 2 / 3;
    z-index: 2;
    overflow: hidden;
    -webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.55) 0%, #000 10%, #000 58%, transparent 100%);
    mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.55) 0%, #000 10%, #000 58%, transparent 100%);
  }

  .poster-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    user-select: none;
    pointer-events: none;
  }

  .poster-edge-vignette {
    position: absolute;
    inset: 0;
    background: radial-gradient(
      88% 84% at 52% 42%,
      rgba(12, 12, 12, 0) 50%,
      rgba(12, 12, 12, 0.42) 76%,
      rgba(12, 12, 12, 0.92) 100%
    );
    pointer-events: none;
  }

  /* Full-width edge-to-edge HLS Player Section (zero horizontal padding, zero radius) */
  .player-section {
    width: 100%;
    display: flex;
    flex-direction: column;
  }

  .player-stage {
    position: relative;
    width: 100%;
    aspect-ratio: 16 / 9;
    border-radius: 0;
    clip-path: none;
    overflow: hidden;
    background: #070709;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .player-stage :global(.hls-player) {
    border-radius: 0;
  }

  .anime-body {
    padding: 36px var(--page-pad-x) 0 var(--page-pad-x);
    display: flex;
    flex-direction: column;
    gap: 40px;
  }

  .stage-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 14px;
    padding: 24px;
    text-align: center;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 15px;
    color: rgba(255, 255, 255, 0.6);
  }

  .error-text {
    margin: 0;
    color: #f87171;
  }

  .spinner {
    width: 30px;
    height: 30px;
    border-radius: 50%;
    border: 2.5px solid rgba(255, 255, 255, 0.14);
    border-top-color: #ffffff;
    animation: spin 0.75s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  /* Episodes Section */
  .section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 18px;
  }

  .section-head-left {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }

  .section-heading {
    margin: 0;
    font-family: 'PP Monument Extended', system-ui, sans-serif;
    font-size: 24px;
    font-weight: 900;
    color: #e4e4e7;
    letter-spacing: -0.01em;
  }

  .section-count {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 14px;
    color: rgba(255, 255, 255, 0.42);
  }

  .range-tabs {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }

  .range-pill {
    height: 32px;
    padding: 0 13px;
    border: none;
    outline: none;
    border-radius: 10px;
    background: rgba(255, 255, 255, 0.06);
    color: rgba(255, 255, 255, 0.7);
    font-family: inherit;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: background-color 0.18s var(--ease-out), color 0.18s var(--ease-out);
  }

  .range-pill:hover {
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
  }

  .range-pill.is-active {
    background: rgba(255, 255, 255, 0.16);
    color: #ffffff;
    font-weight: 600;
  }

  .episodes-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 16px;
  }

  @media (max-width: 1180px) {
    .episodes-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (max-width: 820px) {
    .episodes-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .anime-overview {
      flex-direction: column;
      align-items: flex-start;
    }

    .showcase-art {
      width: 210px;
    }

    .poster-wrap {
      width: 100%;
    }
  }

  .ep-card {
    position: relative;
    display: block;
    padding: 0;
    border: none;
    outline: none;
    border-radius: var(--radius-card);
    overflow: hidden;
    background: #0c0c0c;
    isolation: isolate;
    contain: layout style paint;
    transform: translate3d(0, 0, 0);
    cursor: pointer;
    text-align: left;
    transition: transform 0.24s var(--ease-out);
  }

  .ep-card::after {
    content: '';
    position: absolute;
    inset: -3px;
    border-radius: calc(var(--radius-card) + 3px);
    border: 4px solid #0c0c0c;
    transform: translateZ(0);
    pointer-events: none;
    z-index: 6;
  }

  .ep-card:hover {
    transform: translate3d(0, -2px, 0);
  }

  .ep-thumb-wrap {
    position: relative;
    width: 100%;
    aspect-ratio: 16 / 9;
    overflow: hidden;
    background: #0c0c0c;
  }

  .ep-thumb {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    backface-visibility: hidden;
    transition: transform 0.45s var(--ease-out);
  }

  .ep-card:hover .ep-thumb {
    transform: scale(1.04);
  }

  .ep-thumb-fallback {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    color: rgba(255, 255, 255, 0.24);
    background: radial-gradient(circle at 50% 42%, rgba(255, 255, 255, 0.06) 0%, #0c0c0c 78%);
    transition: color 0.25s var(--ease-out);
  }

  .ep-card:hover .ep-thumb-fallback {
    color: rgba(255, 255, 255, 0.55);
  }

  .ep-thumb-fade {
    position: absolute;
    inset: -4px;
    z-index: 2;
    background: linear-gradient(
      180deg,
      rgba(12, 12, 12, 0.14) 0%,
      rgba(12, 12, 12, 0.2) 44%,
      rgba(12, 12, 12, 0.84) 76%,
      rgba(12, 12, 12, 0.96) 89%,
      #0c0c0c 95%,
      #0c0c0c 100%
    );
    pointer-events: none;
  }

  .ep-overlay-bottom {
    position: absolute;
    z-index: 3;
    left: 12px;
    right: 12px;
    bottom: 10px;
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
    min-width: 0;
  }

  .ep-title-row {
    display: flex;
    align-items: baseline;
    gap: 7px;
    min-width: 0;
    flex: 1 1 auto;
  }

  .ep-num {
    flex-shrink: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 700;
    letter-spacing: -0.01em;
    color: #ffffff;
  }

  .ep-name {
    min-width: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 400;
    line-height: 1.3;
    color: rgba(255, 255, 255, 0.74);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .ep-card.is-active .ep-name {
    color: rgba(255, 255, 255, 0.92);
  }

  .ep-duration {
    flex-shrink: 0;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12px;
    font-weight: 500;
    color: rgba(255, 255, 255, 0.56);
    font-variant-numeric: tabular-nums;
  }

  /* Frames Gallery */
  .frames-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 16px;
  }

  @media (max-width: 960px) {
    .frames-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  .frame-card {
    position: relative;
    padding: 0;
    border: none;
    outline: none;
    border-radius: var(--radius-card);
    overflow: hidden;
    aspect-ratio: 16 / 9;
    background: #0c0c0c;
    isolation: isolate;
    transform: translate3d(0, 0, 0);
    cursor: pointer;
  }

  .frame-card::after {
    content: '';
    position: absolute;
    inset: -3px;
    border-radius: calc(var(--radius-card) + 3px);
    border: 4px solid #0c0c0c;
    transform: translateZ(0);
    pointer-events: none;
    z-index: 3;
  }

  .frame-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    backface-visibility: hidden;
    transition: transform 0.45s var(--ease-out);
  }

  .frame-card:hover .frame-img {
    transform: scale(1.04);
  }

  .lightbox-backdrop {
    position: fixed;
    inset: 0;
    z-index: 900;
    background: rgba(6, 6, 8, 0.88);
    backdrop-filter: blur(18px);
    -webkit-backdrop-filter: blur(18px);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 36px;
  }

  .lightbox-close {
    position: absolute;
    top: 24px;
    right: 28px;
    width: 40px;
    height: 40px;
    border: none;
    outline: none;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
  }

  .lightbox-img {
    max-width: 92vw;
    max-height: 86vh;
    border-radius: var(--radius-card);
    object-fit: contain;
    display: block;
  }
</style>
