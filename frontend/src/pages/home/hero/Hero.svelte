<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import Hls from 'hls.js';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Play from '@lucide/svelte/icons/play';
  import Bookmark from '@lucide/svelte/icons/bookmark';
  import Volume2 from '@lucide/svelte/icons/volume-2';
  import VolumeX from '@lucide/svelte/icons/volume-x';
  import MainButton from '../../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../../marlakit/button/SecondButton.svelte';
  import MetaLine from '../../../marlakit/meta/MetaLine.svelte';
  import LayerBlur from '../../../marlakit/blur/LayerBlur.svelte';
  import {
    getCachedHomeFeeds,
    loadHomeFeeds,
    resolveHeroOpeningPreview,
    type HeroOpeningOption,
    type HeroOpeningPreview,
    type HomeSpotlightItem
  } from '../../../api/anime';
  import { openWatch, player } from '../../../app/player.svelte';
  import { router } from '../../../app/router.svelte';
  import { search } from '../../../app/search.svelte';

  interface Props {
    onselectbg?: (url: string) => void;
  }

  const SPOTLIGHT_INTERVAL_MS = 8500;

  let { onselectbg }: Props = $props();

  let spotlight = $state<HomeSpotlightItem[]>(getCachedHomeFeeds()?.spotlight ?? []);
  let activeIndex = $state(0);
  let isPaused = $state(false);
  let isWatchLoading = $state(false);
  let isPlanLoading = $state(false);
  let planTimer: number | undefined;
  let cycleTimer: number | undefined;

  let previewStripEl = $state<HTMLDivElement | null>(null);
  let previewVideoEl = $state<HTMLVideoElement | null>(null);
  let previewClip = $state<HeroOpeningPreview | null>(null);
  let videoReady = $state(false);
  let videoMuted = $state(true);
  let isHeroVisible = $state(true);
  const AUDIO_FADE_IN_SEC = 0.4;
  const AUDIO_FADE_OUT_SEC = 1.2;
  const UNMUTE_FADE_MS = 400;
  const SWITCH_FADE_OUT_MS = 220;
  const VOLUME_STORAGE_KEY = 'mila.hero.previewVolume';
  const OPENINGS_STORAGE_KEY = 'mila.hero.selectedOpenings';

  function loadInitialVolume(): number {
    try {
      const saved = window.localStorage.getItem(VOLUME_STORAGE_KEY);
      if (saved !== null) {
        const parsed = Number(saved);
        if (Number.isFinite(parsed) && parsed > 0 && parsed <= 1) {
          return parsed;
        }
      }
    } catch {
      // Ignore storage access errors
    }
    return 0.7;
  }

  function loadSelectedOpenings(): Record<number, number> {
    try {
      const raw = window.localStorage.getItem(OPENINGS_STORAGE_KEY);
      if (!raw) return {};
      const parsed = JSON.parse(raw) as Record<string, unknown>;
      const out: Record<number, number> = {};
      for (const [k, v] of Object.entries(parsed)) {
        const id = Number(k);
        const num = Number(v);
        if (Number.isFinite(id) && Number.isFinite(num) && num >= 1) {
          out[id] = num;
        }
      }
      return out;
    } catch {
      return {};
    }
  }

  let previewHls: Hls | null = null;
  let previewReqToken = 0;
  let audioFadeRaf: number | undefined;
  let unmuteFadeStartMs = 0;
  let switchFadeStartMs = 0;
  let skipNextUnmuteFade = false;
  let volumeTrackEl = $state<HTMLDivElement | null>(null);
  let opCtrlEl = $state<HTMLDivElement | null>(null);
  let videoVolume = $state(loadInitialVolume());
  let lastNonZeroVolume = $state(loadInitialVolume());
  let isAdjustingVolume = $state(false);
  let isSwitchingOpening = $state(false);
  let opMenuOpen = $state(false);
  let opBarDismissed = $state(false);
  let selectedOpenings = $state<Record<number, number>>(loadSelectedOpenings());
  let effectiveVolume = $derived(videoMuted ? 0 : videoVolume);

  let activeHero = $derived(spotlight[activeIndex] ?? spotlight[0] ?? null);
  let heroOpenings = $derived(activeHero?.openings ?? []);
  let activeOpeningOption = $derived.by((): HeroOpeningOption | null => {
    if (!activeHero || heroOpenings.length === 0) return null;
    const savedNum = selectedOpenings[activeHero.shikiId];
    const targetNum = savedNum ?? activeHero.defaultOpeningNumber ?? heroOpenings[0]?.number;
    return heroOpenings.find((op) => op.number === targetNum) ?? heroOpenings[0] ?? null;
  });
  let activeOpeningIndex = $derived(
    activeOpeningOption ? Math.max(0, heroOpenings.findIndex((op) => op.number === activeOpeningOption.number)) : 0
  );
  let shouldPlayPreview = $derived(
    Boolean(previewClip) && router.current === 'home' && isHeroVisible && !search.open
  );

  function stopAudioFadeLoop() {
    if (audioFadeRaf !== undefined) {
      window.cancelAnimationFrame(audioFadeRaf);
      audioFadeRaf = undefined;
    }
  }

  function applyPreviewAudioEnvelope(now = performance.now()) {
    const video = previewVideoEl;
    const clip = previewClip;
    if (!video || !clip) return;

    if (videoMuted || !videoReady || video.currentTime < clip.startSec - 0.15) {
      video.volume = 0;
      return;
    }

    const elapsedInClip = Math.max(0, video.currentTime - clip.startSec);
    const clipInLinear = Math.min(1, elapsedInClip / AUDIO_FADE_IN_SEC);
    const clipIn = clipInLinear * clipInLinear;

    const remaining = Math.max(0, clip.endSec - 0.15 - video.currentTime);
    const clipOutLinear = remaining < AUDIO_FADE_OUT_SEC ? Math.max(0, remaining / AUDIO_FADE_OUT_SEC) : 1;
    const clipOut = clipOutLinear * clipOutLinear;

    const unmuteLinear =
      unmuteFadeStartMs > 0 ? Math.min(1, Math.max(0, now - unmuteFadeStartMs) / UNMUTE_FADE_MS) : 1;
    const unmuteFactor = unmuteLinear * unmuteLinear;

    const switchOutLinear =
      isSwitchingOpening && switchFadeStartMs > 0
        ? Math.max(0, 1 - Math.max(0, now - switchFadeStartMs) / SWITCH_FADE_OUT_MS)
        : 1;
    const switchOutFactor = switchOutLinear * switchOutLinear;

    video.volume = Math.max(0, Math.min(1, clipIn * clipOut * unmuteFactor * switchOutFactor * videoVolume));
  }

  function setPreviewVolume(nextRaw: number) {
    const clamped = Math.max(0, Math.min(1, Math.round(nextRaw * 100) / 100));
    if (clamped <= 0) {
      skipNextUnmuteFade = false;
      videoVolume = 0;
      videoMuted = true;
      if (previewVideoEl) {
        previewVideoEl.muted = true;
        previewVideoEl.volume = 0;
      }
      stopAudioFadeLoop();
      return;
    }

    const wasMuted = videoMuted;
    videoVolume = clamped;
    lastNonZeroVolume = clamped;
    unmuteFadeStartMs = 0;
    if (wasMuted) {
      skipNextUnmuteFade = true;
      videoMuted = false;
    }
    try {
      window.localStorage.setItem(VOLUME_STORAGE_KEY, String(clamped));
    } catch {
      // Ignore storage write errors
    }

    if (previewVideoEl) {
      previewVideoEl.muted = false;
      if (wasMuted && shouldPlayPreview && previewVideoEl.paused) {
        previewVideoEl.play().catch(() => {});
      }
    }
    applyPreviewAudioEnvelope();
    if (shouldPlayPreview && (wasMuted || audioFadeRaf === undefined)) {
      startAudioFadeLoop();
    }
  }

  function setVolumeFromPointer(clientX: number) {
    if (!volumeTrackEl) return;
    const rect = volumeTrackEl.getBoundingClientRect();
    if (rect.width <= 0) return;
    setPreviewVolume((clientX - rect.left) / rect.width);
  }

  function togglePreviewMute() {
    skipNextUnmuteFade = false;
    if (videoMuted || videoVolume <= 0) {
      if (videoVolume <= 0) {
        videoVolume = lastNonZeroVolume > 0 ? lastNonZeroVolume : 0.7;
      }
      videoMuted = false;
      return;
    }
    videoMuted = true;
  }

  function startAudioFadeLoop() {
    stopAudioFadeLoop();
    const tick = (now: number) => {
      const video = previewVideoEl;
      if (!video || !previewClip || videoMuted || !shouldPlayPreview) {
        audioFadeRaf = undefined;
        return;
      }
      applyPreviewAudioEnvelope(now);
      audioFadeRaf = window.requestAnimationFrame(tick);
    };
    audioFadeRaf = window.requestAnimationFrame(tick);
  }

  function destroyPreviewStream() {
    stopAudioFadeLoop();
    if (previewHls) {
      previewHls.destroy();
      previewHls = null;
    }
    if (previewVideoEl) {
      previewVideoEl.pause();
      previewVideoEl.volume = 0;
      previewVideoEl.removeAttribute('src');
      previewVideoEl.load();
    }
    videoReady = false;
  }

  function applySpotlightIndex(index: number)
  {
    if (index === activeIndex)
    {
      return;
    }
    activeIndex = index;
    const selected = spotlight[index];
    if (selected?.bg)
    {
      onselectbg?.(selected.bg);
    }
  }

  function startAutoCycle()
  {
    window.clearInterval(cycleTimer);
    cycleTimer = window.setInterval(() =>
    {
      if (isPaused || opMenuOpen || !videoMuted || router.current !== 'home' || spotlight.length <= 1)
      {
        return;
      }
      const next = (activeIndex + 1) % spotlight.length;
      activeIndex = next;
      const selected = spotlight[next];
      if (selected?.bg)
      {
        onselectbg?.(selected.bg);
      }
    }, SPOTLIGHT_INTERVAL_MS);
  }

  function resolveOpeningForHero(hero: HomeSpotlightItem, option: HeroOpeningOption | null) {
    return resolveHeroOpeningPreview({
      title: hero.title,
      releaseAlias: hero.releaseAlias,
      openingAlias: option ? option.alias : hero.openingAlias,
      openingEpisode: option ? option.episode : hero.openingEpisode,
      openingStartOffsetSec: option ? option.startOffsetSec : hero.openingStartOffsetSec,
      shikiId: hero.shikiId,
      searchQueries: hero.searchQueries
    });
  }

  async function selectHeroOpening(
    opNumber: number,
    options?: { dismiss?: boolean; triggerEl?: HTMLElement | null }
  ) {
    options?.triggerEl?.blur();
    if (document.activeElement instanceof HTMLElement && opCtrlEl?.contains(document.activeElement)) {
      document.activeElement.blur();
    }

    const hero = activeHero;
    if (!hero || heroOpenings.length === 0) return;
    const targetOp = heroOpenings.find((op) => op.number === opNumber);
    if (!targetOp || targetOp.number === activeOpeningOption?.number) {
      opMenuOpen = false;
      if (options?.dismiss !== false) {
        opBarDismissed = true;
      }
      return;
    }

    const nextMap = { ...selectedOpenings, [hero.shikiId]: targetOp.number };
    selectedOpenings = nextMap;
    try {
      window.localStorage.setItem(OPENINGS_STORAGE_KEY, JSON.stringify(nextMap));
    } catch {
      // Ignore storage write errors
    }

    opMenuOpen = false;
    if (options?.dismiss !== false) {
      opBarDismissed = true;
    }
    const token = ++previewReqToken;
    switchFadeStartMs = performance.now();
    isSwitchingOpening = true;
    if (!videoMuted && shouldPlayPreview) {
      startAudioFadeLoop();
    }

    const [clip] = await Promise.all([
      resolveOpeningForHero(hero, targetOp).catch(() => null),
      new Promise<void>((resolve) => window.setTimeout(resolve, SWITCH_FADE_OUT_MS))
    ]);

    if (token !== previewReqToken) return;

    switchFadeStartMs = 0;
    if (clip) {
      previewClip = clip;
    } else {
      isSwitchingOpening = false;
    }
  }

  function stepHeroOpening(delta: number, triggerEl?: HTMLElement | null) {
    if (heroOpenings.length <= 1) return;
    const nextIdx = (activeOpeningIndex + delta + heroOpenings.length) % heroOpenings.length;
    const nextOp = heroOpenings[nextIdx];
    if (nextOp) {
      void selectHeroOpening(nextOp.number, { dismiss: false, triggerEl });
    }
  }

  $effect(() => {
    if (!opMenuOpen) return;
    const onPointerDown = (e: PointerEvent) => {
      if (opCtrlEl && e.target instanceof Node && !opCtrlEl.contains(e.target)) {
        opMenuOpen = false;
      }
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        opMenuOpen = false;
      }
    };
    window.addEventListener('pointerdown', onPointerDown);
    window.addEventListener('keydown', onKeyDown);
    return () => {
      window.removeEventListener('pointerdown', onPointerDown);
      window.removeEventListener('keydown', onKeyDown);
    };
  });

  $effect(() => {
    const hero = activeHero;
    const token = ++previewReqToken;
    previewClip = null;
    isSwitchingOpening = false;
    switchFadeStartMs = 0;
    opMenuOpen = false;
    opBarDismissed = false;
    untrack(() => destroyPreviewStream());

    if (!hero) return;

    const initialOp = untrack(() => {
      const openings = hero.openings ?? [];
      if (openings.length === 0) return null;
      const savedNum = selectedOpenings[hero.shikiId];
      const targetNum = savedNum ?? hero.defaultOpeningNumber ?? openings[0]?.number;
      return openings.find((op) => op.number === targetNum) ?? openings[0] ?? null;
    });

    resolveOpeningForHero(hero, initialOp)
      .then((clip) => {
        if (token !== previewReqToken || !clip) return;
        previewClip = clip;
      })
      .catch(() => {});
  });

  $effect(() => {
    const clip = previewClip;
    const video = previewVideoEl;
    untrack(() => destroyPreviewStream());
    if (!clip || !video) return;

    let cancelled = false;
    video.muted = untrack(() => videoMuted);
    video.volume = 0;
    unmuteFadeStartMs = performance.now();

    const trySeekAndPlay = () => {
      if (cancelled || !video) return;
      if (video.currentTime < clip.startSec - 0.15 || video.currentTime >= clip.endSec) {
        video.currentTime = clip.startSec;
      }
      if (untrack(() => shouldPlayPreview)) {
        video.play().catch(() => {});
      }
    };

    if (Hls.isSupported()) {
      const hls = new Hls({
        enableWorker: true,
        lowLatencyMode: false,
        startPosition: clip.startSec,
        maxBufferLength: 14,
        maxMaxBufferLength: 28,
        backBufferLength: 8
      });
      previewHls = hls;
      hls.loadSource(clip.streamUrl);
      hls.attachMedia(video);
      hls.on(Hls.Events.MANIFEST_PARSED, trySeekAndPlay);
      hls.on(Hls.Events.ERROR, (_ev, data) => {
        if (data.fatal) {
          destroyPreviewStream();
        }
      });
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = clip.streamUrl;
      video.addEventListener('loadedmetadata', trySeekAndPlay, { once: true });
    }

    return () => {
      cancelled = true;
      destroyPreviewStream();
    };
  });

  $effect(() => {
    const video = previewVideoEl;
    const clip = previewClip;
    const play = shouldPlayPreview;
    if (!video || !clip) return;
    untrack(() => {
      if (play) {
        if (video.currentTime < clip.startSec - 0.15 || video.currentTime >= clip.endSec) {
          video.currentTime = clip.startSec;
        }
        unmuteFadeStartMs = performance.now();
        applyPreviewAudioEnvelope(unmuteFadeStartMs);
        video.play().catch(() => {});
        if (!videoMuted) {
          startAudioFadeLoop();
        }
      } else {
        stopAudioFadeLoop();
        video.pause();
      }
    });
  });

  $effect(() => {
    const video = previewVideoEl;
    const muted = videoMuted;
    if (!video) return;
    untrack(() => {
      video.muted = muted;
      if (muted) {
        stopAudioFadeLoop();
        video.volume = 0;
      } else {
        if (skipNextUnmuteFade) {
          skipNextUnmuteFade = false;
          unmuteFadeStartMs = 0;
        } else {
          unmuteFadeStartMs = performance.now();
        }
        applyPreviewAudioEnvelope();
        if (shouldPlayPreview) {
          if (video.paused) {
            video.play().catch(() => {});
          }
          startAudioFadeLoop();
        }
      }
    });
  });

  $effect(() => {
    const el = previewStripEl;
    if (!el || typeof IntersectionObserver === 'undefined') return;
    const io = new IntersectionObserver(
      (entries) => {
        isHeroVisible = entries[0]?.isIntersecting ?? true;
      },
      { threshold: 0.15 }
    );
    io.observe(el);
    return () => {
      io.disconnect();
    };
  });

  $effect(() => () =>
  {
    window.clearTimeout(planTimer);
    window.clearInterval(cycleTimer);
    destroyPreviewStream();
  });

  onMount(async () =>
  {
    if (spotlight.length > 0 && spotlight[activeIndex]?.bg)
    {
      onselectbg?.(spotlight[activeIndex].bg);
    }
    startAutoCycle();

    const feeds = await loadHomeFeeds();
    if (feeds.spotlight.length > 0)
    {
      spotlight = feeds.spotlight;
      activeIndex = 0;
      if (feeds.spotlight[0]?.bg)
      {
        onselectbg?.(feeds.spotlight[0].bg);
      }
    }
  });

  async function handleWatchClick()
  {
    if (!activeHero) return;
    const target = activeHero;
    isWatchLoading = true;
    try
    {
      await openWatch({
        title: target.title,
        releaseAlias: target.releaseAlias,
        shikiId: target.shikiId,
        searchQueries: target.searchQueries
      });
    }
    finally
    {
      isWatchLoading = false;
    }
  }

  function handlePlanClick()
  {
    window.clearTimeout(planTimer);
    isPlanLoading = true;
    planTimer = window.setTimeout(() =>
    {
      isPlanLoading = false;
    }, 2000);
  }

  function scrollBelowHero(e: MouseEvent)
  {
    const btn = e.currentTarget as HTMLElement | null;
    const viewport = btn?.closest('.marlakit-scroll-viewport') as HTMLElement | null;
    const releases = document.querySelector('.releases-section');
    if (viewport && releases instanceof HTMLElement)
    {
      const targetTop = viewport.scrollTop + releases.getBoundingClientRect().top - 42;
      viewport.scrollTo({
        top: Math.max(0, targetTop),
        behavior: 'smooth'
      });
      return;
    }
    if (releases instanceof HTMLElement)
    {
      releases.scrollIntoView({ behavior: 'smooth', block: 'start' });
      return;
    }
    viewport?.scrollBy({
      top: Math.round(window.innerHeight * 0.88),
      behavior: 'smooth'
    });
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<section
  class="hero-container"
  onmouseenter={() => { isPaused = true; }}
  onmouseleave={() => { isPaused = false; }}
>
  <!-- 3-screenshot horizontal gallery strip with stretched full-width opening video preview -->
  <div
    class="preview-strip"
    bind:this={previewStripEl}
    onmouseleave={() => { opBarDismissed = false; }}
  >
    {#if activeHero}
      {#key activeHero.shikiId}
        <div
          class="preview-set"
          transition:fade={{ duration: 280, easing: cubicOut }}
        >
          {#if activeHero.previews.length >= 3}
            <img src={activeHero.previews[0]} alt="{activeHero.title} preview 1" class="preview-img img-1" decoding="async" />
            <img src={activeHero.previews[1]} alt="{activeHero.title} preview 2" class="preview-img img-2" decoding="async" />
            <img src={activeHero.previews[2]} alt="{activeHero.title} preview 3" class="preview-img img-3" decoding="async" />
          {/if}
        </div>
      {/key}
    {/if}

    <div
      class="preview-video-stage"
      class:is-ready={videoReady}
      class:is-switching={isSwitchingOpening}
    >
      <video
        bind:this={previewVideoEl}
        class="preview-video"
        muted={videoMuted}
        playsinline
        preload="auto"
        onplaying={() => {
          if (previewClip && previewVideoEl && previewVideoEl.currentTime >= previewClip.startSec - 0.2) {
            if (!videoReady) {
              unmuteFadeStartMs = performance.now();
              isSwitchingOpening = false;
              switchFadeStartMs = 0;
            }
            videoReady = true;
            applyPreviewAudioEnvelope();
          }
        }}
        ontimeupdate={() => {
          if (!previewClip || !previewVideoEl) return;
          const t = previewVideoEl.currentTime;
          if (!previewVideoEl.seeking && t < previewClip.startSec - 0.15) {
            previewVideoEl.volume = 0;
            previewVideoEl.currentTime = previewClip.startSec;
            return;
          }
          if (!videoReady && t >= previewClip.startSec - 0.2) {
            videoReady = true;
            isSwitchingOpening = false;
            switchFadeStartMs = 0;
            unmuteFadeStartMs = performance.now();
          }
          if (t >= previewClip.endSec - 0.15) {
            previewVideoEl.volume = 0;
            previewVideoEl.currentTime = previewClip.startSec;
            return;
          }
          applyPreviewAudioEnvelope();
        }}
      ></video>
    </div>

    {#if videoReady && previewClip}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="preview-sound-ctrl"
        class:is-adjusting={isAdjustingVolume}
        onwheel={(e) => {
          e.preventDefault();
          const delta = e.deltaY < 0 ? 0.05 : -0.05;
          setPreviewVolume((videoMuted ? 0 : videoVolume) + delta);
        }}
      >
        <div class="preview-volume-wrap">
          <div
            bind:this={volumeTrackEl}
            class="preview-volume-track"
            role="slider"
            tabindex="0"
            aria-label="Opening preview volume"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(effectiveVolume * 100)}
            onpointerdown={(e) => {
              e.preventDefault();
              e.stopPropagation();
              isAdjustingVolume = true;
              e.currentTarget.setPointerCapture(e.pointerId);
              setVolumeFromPointer(e.clientX);
            }}
            onpointermove={(e) => {
              if (!isAdjustingVolume) return;
              setVolumeFromPointer(e.clientX);
            }}
            onpointerup={(e) => {
              if (!isAdjustingVolume) return;
              isAdjustingVolume = false;
              if (e.currentTarget.hasPointerCapture(e.pointerId)) {
                e.currentTarget.releasePointerCapture(e.pointerId);
              }
            }}
            onpointercancel={(e) => {
              if (!isAdjustingVolume) return;
              isAdjustingVolume = false;
              if (e.currentTarget.hasPointerCapture(e.pointerId)) {
                e.currentTarget.releasePointerCapture(e.pointerId);
              }
            }}
            onkeydown={(e) => {
              if (e.key === 'ArrowRight' || e.key === 'ArrowUp') {
                e.preventDefault();
                setPreviewVolume((videoMuted ? 0 : videoVolume) + 0.05);
              } else if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') {
                e.preventDefault();
                setPreviewVolume((videoMuted ? 0 : videoVolume) - 0.05);
              }
            }}
          >
            <div class="preview-volume-bar">
              <div
                class="preview-volume-fill"
                style="width: {Math.round(effectiveVolume * 100)}%"
              ></div>
              <div
                class="preview-volume-thumb"
                style="left: {Math.round(effectiveVolume * 100)}%"
              ></div>
            </div>
          </div>
        </div>
        <button
          type="button"
          class="preview-sound-btn"
          aria-label={videoMuted ? 'Unmute opening preview' : 'Mute opening preview'}
          title={videoMuted ? 'Unmute opening' : `Volume: ${Math.round(effectiveVolume * 100)}%`}
          onclick={togglePreviewMute}
        >
          {#if videoMuted || videoVolume <= 0}
            <VolumeX size={15} strokeWidth={2} />
          {:else}
            <Volume2 size={15} strokeWidth={2} />
          {/if}
        </button>
      </div>
    {/if}

    {#if heroOpenings.length > 1 && activeOpeningOption && (videoReady || isSwitchingOpening || previewClip)}
      <div
        class="preview-op-dock"
        onmouseenter={() => { opBarDismissed = false; }}
      >
        <div
          bind:this={opCtrlEl}
          class="preview-op-ctrl"
          class:is-open={opMenuOpen}
          class:is-dismissed={opBarDismissed}
        >
          {#if opMenuOpen && heroOpenings.length > 6}
            <div
              class="preview-op-popover"
              role="listbox"
              aria-label="Choose anime opening"
              transition:fly={{ y: 6, duration: 200, easing: cubicOut }}
            >
              <div class="preview-op-popover-head">
                <span>Opening</span>
                <span class="preview-op-popover-title">{activeOpeningOption.title}</span>
              </div>
              <div class="preview-op-grid">
                {#each heroOpenings as op (op.number)}
                  <button
                    type="button"
                    role="option"
                    aria-selected={op.number === activeOpeningOption.number}
                    class="preview-op-grid-btn"
                    class:is-active={op.number === activeOpeningOption.number}
                    title={`${op.label} · ${op.title}`}
                    onclick={(e) => selectHeroOpening(op.number, { triggerEl: e.currentTarget })}
                  >
                    {op.number}
                  </button>
                {/each}
              </div>
            </div>
          {/if}

          {#if heroOpenings.length <= 6}
            <span class="preview-op-tag" title={activeOpeningOption.title}>OP</span>
            <div class="preview-op-segments" role="tablist" aria-label="Opening selector">
              <span
                class="preview-op-pill"
                aria-hidden="true"
                style="transform: translateX({activeOpeningIndex * 28}px);"
              ></span>
              {#each heroOpenings as op (op.number)}
                <button
                  type="button"
                  role="tab"
                  aria-selected={op.number === activeOpeningOption.number}
                  class="preview-op-num"
                  class:is-active={op.number === activeOpeningOption.number}
                  title={`${op.label} · ${op.title}`}
                  onclick={(e) => selectHeroOpening(op.number, { triggerEl: e.currentTarget })}
                >
                  {op.number}
                </button>
              {/each}
            </div>
          {:else}
            <button
              type="button"
              class="preview-op-step"
              aria-label="Previous opening"
              title="Previous opening"
              onclick={(e) => stepHeroOpening(-1, e.currentTarget)}
            >
              <ChevronLeft size={14} strokeWidth={2.2} />
            </button>
            <button
              type="button"
              class="preview-op-current"
              aria-expanded={opMenuOpen}
              title={activeOpeningOption.title}
              onclick={() => {
                opMenuOpen = !opMenuOpen;
              }}
            >
              <span>{activeOpeningOption.label}</span>
            </button>
            <button
              type="button"
              class="preview-op-step"
              aria-label="Next opening"
              title="Next opening"
              onclick={(e) => stepHeroOpening(1, e.currentTarget)}
            >
              <ChevronRight size={14} strokeWidth={2.2} />
            </button>
          {/if}
        </div>
      </div>
    {/if}
  </div>

  <!-- Hero title + symmetric two-column body -->
  <div class="hero-content">
    {#if activeHero}
      <h1 class="hero-title">{activeHero.title}</h1>

      <div class="hero-body">
        <div class="hero-main-col">
          <div class="hero-meta">
            <MetaLine score={activeHero.score} tags={activeHero.metaTags} />
          </div>
          <p class="hero-description">{activeHero.description}</p>

          <!-- Action buttons with icons -->
          <div class="hero-actions">
            <MainButton loading={isWatchLoading || player.loading} onclick={handleWatchClick}>
              {#snippet icon()}
                <Play size={16} fill="currentColor" strokeWidth={0} />
              {/snippet}
              Watch
            </MainButton>

            <SecondButton loading={isPlanLoading} onclick={handlePlanClick}>
              {#snippet icon()}
                <Bookmark size={16} strokeWidth={2} />
              {/snippet}
              Plan to Watch
            </SecondButton>
          </div>
        </div>

        <!-- Bottom-right Mila-native panorama accordion strip -->
        <aside class="hero-strip-wrap" aria-label="Featured spotlight switcher">
          <div class="hero-strip-blur" aria-hidden="true">
            {#key activeHero.shikiId}
              {#if activeHero.poster}
                <div class="strip-blur-layer" transition:fade={{ duration: 420, easing: cubicOut }}>
                  <LayerBlur src={activeHero.poster} blur={46} opacity={0.32} scale={1.1} offsetY={0} />
                </div>
              {/if}
            {/key}
          </div>

          <div class="hero-strip">
            {#each spotlight as item, idx (item.shikiId)}
              <button
                type="button"
                class="strip-card"
                class:is-active={idx === activeIndex}
                aria-label={item.title}
                onclick={() => applySpotlightIndex(idx)}
              >
                {#if item.poster}
                  <img src={item.poster} alt={item.title} class="strip-card-img" decoding="async" />
                {/if}
                <div class="strip-card-overlay"></div>
                <div class="strip-card-bottom-fade"></div>

                {#if idx === activeIndex}
                  <span
                    class="strip-progress"
                    class:is-paused={isPaused}
                    style="animation-duration: {SPOTLIGHT_INTERVAL_MS}ms;"
                  ></span>
                {/if}
              </button>
            {/each}
          </div>
        </aside>
      </div>
    {/if}
  </div>

  <!-- Bottom scroll indicator chevron -->
  <button type="button" class="hero-bottom-arrow" aria-label="Scroll to releases" onclick={scrollBelowHero}>
    <ChevronDown size={18} strokeWidth={2} class="arrow-icon" />
  </button>
</section>

<style>
  .hero-container {
    width: 100%;
    min-height: calc(100vh - 72px);
    display: flex;
    flex-direction: column;
    padding: 0 var(--page-pad-x) 14px var(--page-pad-x);
    box-sizing: border-box;
    position: relative;
    z-index: 2;
  }

  /* 3-screenshot horizontal gallery strip */
  .preview-strip {
    position: relative;
    width: 100%;
    height: clamp(260px, 38vh, 310px);
    min-height: 250px;
    border-radius: var(--radius-card);
    overflow: hidden;
    box-sizing: border-box;
    flex-shrink: 0;
    contain: layout style paint;
    background: rgba(12, 12, 12, 0.45);
  }

  .preview-set {
    position: absolute;
    inset: 0;
    display: flex;
    gap: 8px;
    width: 100%;
    height: 100%;
    background: #0c0c0c;
  }

  .preview-img {
    flex: 1 1 0;
    height: 100%;
    min-width: 0;
    border-radius: calc(var(--radius-card) - 2px);
    object-fit: cover;
    object-position: center 25%;
    display: block;
    user-select: none;
    pointer-events: none;
  }

  .preview-video-stage {
    position: absolute;
    inset: 0;
    z-index: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    background: #0c0c0c;
    pointer-events: none;
    opacity: 0;
    transform: scale(1.022);
    filter: blur(6px);
    transition:
      opacity 0.42s cubic-bezier(0.22, 1, 0.36, 1),
      transform 0.48s cubic-bezier(0.22, 1, 0.36, 1),
      filter 0.42s cubic-bezier(0.22, 1, 0.36, 1);
    will-change: opacity, transform, filter;
  }

  .preview-video-stage.is-ready {
    opacity: 1;
    transform: scale(1);
    filter: blur(0px);
  }

  .preview-video-stage.is-ready.is-switching {
    opacity: 0;
    transform: scale(1.022);
    filter: blur(6px);
  }

  .preview-video {
    width: 100%;
    height: 100%;
    object-fit: fill;
    display: block;
  }

  .preview-op-dock {
    position: absolute;
    left: 0;
    bottom: 0;
    z-index: 3;
    padding: 14px 18px 12px 12px;
    display: flex;
    align-items: flex-end;
  }

  .preview-op-ctrl {
    position: relative;
    height: 36px;
    padding: 3px;
    border-radius: 999px;
    background: rgba(12, 12, 14, 0.72);
    backdrop-filter: blur(8px);
    display: flex;
    align-items: center;
    gap: 2px;
    opacity: 0;
    transform: translateY(16px);
    pointer-events: none;
    transition:
      opacity 0.28s cubic-bezier(0.22, 1, 0.36, 1),
      transform 0.34s cubic-bezier(0.22, 1, 0.36, 1),
      background-color 0.18s ease;
  }

  .preview-strip:hover .preview-op-ctrl:not(.is-dismissed),
  .preview-op-dock:hover .preview-op-ctrl:not(.is-dismissed),
  .preview-op-ctrl:has(:focus-visible),
  .preview-op-ctrl.is-open {
    opacity: 1;
    transform: translateY(0);
    pointer-events: auto;
    transition-delay: 0s;
  }

  .preview-op-ctrl.is-dismissed:not(.is-open) {
    opacity: 0;
    transform: translateY(16px);
    pointer-events: none;
    transition-delay: 0.12s;
  }

  .preview-op-tag {
    padding: 0 7px 0 9px;
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.06em;
    color: rgba(255, 255, 255, 0.6);
    user-select: none;
  }

  .preview-op-segments {
    position: relative;
    display: flex;
    align-items: center;
    height: 30px;
  }

  .preview-op-pill {
    position: absolute;
    left: 0;
    top: 1px;
    width: 28px;
    height: 28px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.92);
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.35);
    transition: transform 0.32s cubic-bezier(0.22, 1, 0.36, 1);
    pointer-events: none;
  }

  .preview-op-num {
    position: relative;
    z-index: 1;
    width: 28px;
    height: 28px;
    padding: 0;
    border: none;
    outline: none;
    border-radius: 999px;
    background: transparent;
    color: rgba(255, 255, 255, 0.78);
    font-size: 12px;
    font-weight: 600;
    line-height: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    user-select: none;
    transition: color 0.2s ease;
  }

  .preview-op-num:hover:not(.is-active) {
    color: #ffffff;
  }

  .preview-op-num.is-active {
    color: #0e0e11;
    font-weight: 700;
  }

  .preview-op-step {
    width: 26px;
    height: 30px;
    padding: 0;
    border: none;
    outline: none;
    border-radius: 999px;
    background: transparent;
    color: rgba(255, 255, 255, 0.78);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: background-color 0.18s ease, color 0.18s ease;
  }

  .preview-op-step:hover {
    background: rgba(255, 255, 255, 0.14);
    color: #ffffff;
  }

  .preview-op-current {
    height: 28px;
    padding: 0 10px;
    border: none;
    outline: none;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.14);
    color: #ffffff;
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.02em;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    user-select: none;
    transition: background-color 0.18s ease, transform 0.16s ease;
  }

  .preview-op-current:hover,
  .preview-op-ctrl.is-open .preview-op-current {
    background: rgba(255, 255, 255, 0.24);
  }

  .preview-op-popover {
    position: absolute;
    left: 0;
    bottom: calc(100% + 8px);
    padding: 10px;
    border-radius: 14px;
    background: rgba(14, 14, 18, 0.9);
    backdrop-filter: blur(14px);
    border: 1px solid rgba(255, 255, 255, 0.1);
    box-shadow: 0 10px 28px rgba(0, 0, 0, 0.45);
    min-width: 196px;
  }

  .preview-op-popover-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    margin-bottom: 8px;
    font-size: 11px;
    font-weight: 600;
    color: rgba(255, 255, 255, 0.55);
  }

  .preview-op-popover-title {
    color: rgba(255, 255, 255, 0.9);
    max-width: 126px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .preview-op-grid {
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    gap: 5px;
  }

  .preview-op-grid-btn {
    width: 32px;
    height: 28px;
    padding: 0;
    border: none;
    outline: none;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.08);
    color: rgba(255, 255, 255, 0.84);
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    transition: background-color 0.16s ease, color 0.16s ease, transform 0.16s ease;
  }

  .preview-op-grid-btn:hover:not(.is-active) {
    background: rgba(255, 255, 255, 0.2);
    color: #ffffff;
  }

  .preview-op-grid-btn.is-active {
    background: rgba(255, 255, 255, 0.92);
    color: #0e0e11;
    font-weight: 700;
  }

  .preview-sound-ctrl {
    position: absolute;
    right: 12px;
    bottom: 12px;
    z-index: 2;
    height: 36px;
    padding: 3px;
    border-radius: 999px;
    background: rgba(12, 12, 14, 0.72);
    backdrop-filter: blur(8px);
    display: flex;
    align-items: center;
    opacity: 0;
    transform: translateY(4px);
    transition:
      opacity 0.22s var(--ease-out),
      transform 0.22s var(--ease-out),
      background-color 0.18s ease;
  }

  .preview-strip:hover .preview-sound-ctrl,
  .preview-sound-ctrl:focus-within,
  .preview-sound-ctrl.is-adjusting {
    opacity: 1;
    transform: translateY(0);
  }

  .preview-volume-wrap {
    width: 0;
    opacity: 0;
    margin: 0;
    overflow: hidden;
    display: flex;
    align-items: center;
    transition:
      width 0.24s var(--ease-out),
      opacity 0.2s var(--ease-out),
      margin 0.24s var(--ease-out);
  }

  .preview-sound-ctrl:hover .preview-volume-wrap,
  .preview-sound-ctrl:focus-within .preview-volume-wrap,
  .preview-sound-ctrl.is-adjusting .preview-volume-wrap {
    width: 88px;
    opacity: 1;
    margin: 0 2px 0 6px;
  }

  .preview-volume-track {
    width: 76px;
    height: 22px;
    margin: 0 6px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    cursor: pointer;
    outline: none;
    touch-action: none;
    user-select: none;
  }

  .preview-volume-bar {
    position: relative;
    width: 100%;
    height: 4px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.24);
  }

  .preview-volume-fill {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.92);
    pointer-events: none;
  }

  .preview-volume-thumb {
    position: absolute;
    top: 50%;
    width: 11px;
    height: 11px;
    border-radius: 50%;
    background: #ffffff;
    transform: translate(-50%, -50%);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.45);
    transition: transform 0.14s ease;
    pointer-events: none;
  }

  .preview-volume-track:hover .preview-volume-thumb,
  .preview-sound-ctrl.is-adjusting .preview-volume-thumb {
    transform: translate(-50%, -50%) scale(1.15);
  }

  .preview-sound-btn {
    width: 30px;
    height: 30px;
    flex-shrink: 0;
    border: none;
    outline: none;
    border-radius: 50%;
    background: transparent;
    color: rgba(255, 255, 255, 0.88);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition:
      background-color 0.18s ease,
      color 0.18s ease;
  }

  .preview-sound-btn:hover {
    background: rgba(255, 255, 255, 0.16);
    color: #ffffff;
  }

  /* Text, actions & spotlight container */
  .hero-content {
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
    margin-top: 16px;
    position: relative;
    z-index: 2;
  }

  .hero-title {
    margin: 0 0 -0.08em 0;
    padding: 0 0 0.08em 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(34px, 4.4vw, 68px);
    font-weight: 700;
    color: #ffffff;
    line-height: 1.16;
    letter-spacing: -0.02em;
    user-select: none;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .hero-body {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 36px;
    margin-top: 10px;
  }

  .hero-main-col {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-width: 0;
  }

  .hero-meta {
    margin-top: 2px;
  }

  .hero-description {
    margin: 12px 0 0 0;
    padding: 0;
    max-width: 760px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 19px;
    font-weight: 400;
    color: #8f8f94;
    line-height: 1.36;
    user-select: none;
    display: -webkit-box;
    line-clamp: 4;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .hero-actions {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 16px;
  }

  /* Bottom-right Mila-native panorama accordion strip (gap: 0, edge-to-edge posters inside rounded clip) */
  .hero-strip-wrap {
    position: relative;
    width: clamp(360px, 34vw, 470px);
    height: 158px;
    flex-shrink: 0;
  }

  .hero-strip-blur {
    position: absolute;
    inset: -16px;
    pointer-events: none;
    z-index: 0;
    overflow: hidden;
    border-radius: calc(var(--radius-card) + 12px);
    -webkit-mask-image: radial-gradient(ellipse at center, rgba(0, 0, 0, 1) 42%, rgba(0, 0, 0, 0) 76%);
    mask-image: radial-gradient(ellipse at center, rgba(0, 0, 0, 1) 42%, rgba(0, 0, 0, 0) 76%);
  }

  .strip-blur-layer {
    position: absolute;
    inset: 0;
  }

  .hero-strip {
    position: relative;
    z-index: 1;
    width: 100%;
    height: 100%;
    display: flex;
    align-items: stretch;
    gap: 0;
    border-radius: var(--radius-card);
    overflow: hidden;
    transform: translateZ(0);
    background: #101012;
  }

  .strip-card {
    position: relative;
    flex: 1 1 0;
    min-width: 0;
    height: 100%;
    padding: 0;
    border: none;
    outline: none;
    background: transparent;
    cursor: pointer;
    overflow: hidden;
    will-change: flex-grow;
    contain: layout paint;
    transition: flex-grow 0.65s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .strip-card.is-active {
    flex: 2.35 1 0;
    cursor: default;
  }

  .strip-card-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center 22%;
    display: block;
    backface-visibility: hidden;
    pointer-events: none;
    user-select: none;
    transition: transform 0.45s var(--ease-out);
  }

  .strip-card:not(.is-active):hover .strip-card-img {
    transform: scale(1.04);
  }

  .strip-card-overlay {
    position: absolute;
    inset: -2px;
    background: rgba(8, 8, 10, 0.56);
    pointer-events: none;
    transition: background-color 0.35s var(--ease-out);
  }

  .strip-card:not(.is-active):hover .strip-card-overlay {
    background: rgba(8, 8, 10, 0.28);
  }

  .strip-card.is-active .strip-card-overlay {
    background: rgba(8, 8, 10, 0.08);
  }

  .strip-card-bottom-fade {
    position: absolute;
    left: -2px;
    right: -2px;
    bottom: -2px;
    height: 46%;
    background: linear-gradient(180deg, rgba(8, 8, 10, 0) 0%, rgba(8, 8, 10, 0.65) 100%);
    pointer-events: none;
  }

  .strip-progress {
    position: absolute;
    left: 0;
    bottom: 0;
    height: 2px;
    width: 100%;
    z-index: 3;
    background: rgba(255, 255, 255, 0.85);
    transform-origin: left center;
    animation-name: spotlightSweep;
    animation-timing-function: linear;
    animation-fill-mode: forwards;
    pointer-events: none;
  }

  .strip-progress.is-paused {
    animation-play-state: paused;
  }

  @keyframes spotlightSweep {
    from {
      transform: scaleX(0);
    }
    to {
      transform: scaleX(1);
    }
  }

  @media (max-width: 980px) {
    .hero-body {
      flex-direction: column;
      align-items: flex-start;
      gap: 20px;
    }

    .hero-strip-wrap {
      width: 100%;
    }
  }

  .hero-bottom-arrow {
    margin-top: auto;
    margin-bottom: 2px;
    padding: 4px 12px;
    border: none;
    background: transparent;
    align-self: center;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    opacity: 0.65;
    transition: opacity 0.15s ease, transform 0.15s ease;
    position: relative;
    z-index: 2;
  }

  .hero-bottom-arrow:hover {
    opacity: 1;
    transform: translateY(2px);
  }

  :global(.arrow-icon) {
    color: #ffffff;
  }
</style>