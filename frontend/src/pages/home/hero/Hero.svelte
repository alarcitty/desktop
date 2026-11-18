<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import type HlsType from 'hls.js';
  import { getLoadedHls, loadHls } from '../../../app/hlsLoader';
  import { MorphIcon } from 'morphicons/svelte';
  import { ChevronLeft, ChevronRight, Play, Bookmark, Check, Volume2, VolumeX } from 'lucide';
  import MainButton from '../../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../../marlakit/button/SecondButton.svelte';
  import LikeButton from '../../../marlakit/button/LikeButton.svelte';
  import MetaLine from '../../../marlakit/meta/MetaLine.svelte';
  import LayerBlur from '../../../marlakit/blur/LayerBlur.svelte';
  import {
    getCachedHomeFeeds,
    loadHomeFeeds,
    resolveHeroOpeningPreview,
    warmHeroOpeningPreview,
    type HeroOpeningOption,
    type HeroOpeningPreview,
    type HomeSpotlightItem
  } from '../../../api/anime';
  import {
    computeAdjacentHeroIndices,
    hasEnoughHeroPreviewBuffer
  } from './heroBuffer';
  import { t } from '../../../app/i18n.svelte';
  import { isInWatchLater, isLiked, toggleLike, toggleWatchLater } from '../../../app/library.svelte';
  import { openWatch, player } from '../../../app/player.svelte';
  import { router } from '../../../app/router.svelte';
  import { search } from '../../../app/search.svelte';
  import { markHeroMediaReady } from '../../../app/boot.svelte';

  const SPOTLIGHT_INTERVAL_MS = 8500;

  let spotlight = $state<HomeSpotlightItem[]>(getCachedHomeFeeds()?.spotlight ?? []);
  let activeIndex = $state(0);
  let isPaused = $state(false);
  let isWatchLoading = $state(false);
  let isPlanLoading = $state(false);
  let planTimer: number | undefined;
  let cycleTimer: number | undefined;

  let heroBgEl = $state<HTMLDivElement | null>(null);
  let previewVideoEl = $state<HTMLVideoElement | null>(null);
  let previewClip = $state<HeroOpeningPreview | null>(null);
  let videoReady = $state(false);
  let videoMuted = $state(true);
  let isHeroVisible = $state(true);
  const AUDIO_FADE_IN_SEC = 0.4;
  const AUDIO_FADE_OUT_SEC = 1.2;
  const UNMUTE_FADE_MS = 400;
  const SWITCH_FADE_OUT_MS = 380;
  const SLIDE_FADE_OVERLAP_MS = 280;
  const BUFFER_REVEAL_FALLBACK_MS = 2200;
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

  let previewHls: HlsType | null = null;
  let previewReqToken = 0;
  let streamBufferedReady = false;
  let pendingDetachTimer: number | undefined;
  let warmNeighborsTimer: number | undefined;
  let audioFadeRaf: number | undefined;
  let audioFadeTimer: number | undefined;
  let volumePersistTimer: number | undefined;
  let unmuteFadeStartMs = 0;
  let switchFadeStartMs = 0;
  let skipNextUnmuteFade = false;
  let hasExplicitMuteChoice = false;
  let volumeTrackEl = $state<HTMLDivElement | null>(null);
  let opCtrlEl = $state<HTMLDivElement | null>(null);
  let videoVolume = $state(loadInitialVolume());
  let lastNonZeroVolume = $state(loadInitialVolume());
  let isAdjustingVolume = $state(false);
  let isSwitchingOpening = $state(false);
  let opMenuOpen = $state(false);
  let bgSlideIndex = $state(0);
  let selectedOpenings = $state<Record<number, number>>(loadSelectedOpenings());
  let effectiveVolume = $derived(videoMuted ? 0 : videoVolume);

  let activeHero = $derived(spotlight[activeIndex] ?? spotlight[0] ?? null);
  let bgScreenshots = $derived.by(() => {
    if (!activeHero) return [] as string[];
    const unique: string[] = [];
    for (const url of [activeHero.bg, ...activeHero.previews]) {
      if (url && !unique.includes(url)) {
        unique.push(url);
      }
    }
    return unique;
  });
  let displayedBgImage = $derived.by(() => {
    if (!activeHero) return '';
    if (bgSlideIndex === 0 || !previewClip) {
      const idx = bgSlideIndex % Math.max(1, bgScreenshots.length);
      return bgScreenshots[idx] ?? activeHero.bg ?? '';
    }
    const shotIdx = (bgSlideIndex - 1) % Math.max(1, bgScreenshots.length);
    return bgScreenshots[shotIdx] ?? bgScreenshots[0] ?? activeHero.bg ?? '';
  });
  let showOpeningBg = $derived(Boolean(previewClip) && bgSlideIndex === 0);
  let totalBgSlides = $derived((previewClip ? 1 : 0) + bgScreenshots.length);
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
    Boolean(previewClip) && bgSlideIndex === 0 && router.current === 'home' && isHeroVisible && !search.open
  );

  function stopAudioFadeLoop() {
    if (audioFadeRaf !== undefined) {
      window.cancelAnimationFrame(audioFadeRaf);
      audioFadeRaf = undefined;
    }
    if (audioFadeTimer !== undefined) {
      window.clearTimeout(audioFadeTimer);
      audioFadeTimer = undefined;
    }
  }

  function getPreviewEndSec(video: HTMLVideoElement, clip: HeroOpeningPreview): number {
    return clip.isDirectVideo && Number.isFinite(video.duration) && video.duration > 1
      ? video.duration
      : clip.endSec;
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

    const endSec = getPreviewEndSec(video, clip);
    const remaining = Math.max(0, endSec - 0.15 - video.currentTime);
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

  function schedulePersistVolume(val: number) {
    if (volumePersistTimer !== undefined) {
      window.clearTimeout(volumePersistTimer);
    }
    volumePersistTimer = window.setTimeout(() => {
      volumePersistTimer = undefined;
      try {
        window.localStorage.setItem(VOLUME_STORAGE_KEY, String(val));
      } catch {
        // Ignore storage write errors
      }
    }, 200);
  }

  function setPreviewVolume(nextRaw: number) {
    const clamped = Math.max(0, Math.min(1, Math.round(nextRaw * 100) / 100));
    if (clamped <= 0) {
      hasExplicitMuteChoice = true;
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

    hasExplicitMuteChoice = true;
    const wasMuted = videoMuted;
    videoVolume = clamped;
    lastNonZeroVolume = clamped;
    unmuteFadeStartMs = 0;
    if (wasMuted) {
      skipNextUnmuteFade = true;
      videoMuted = false;
    }
    schedulePersistVolume(clamped);

    if (previewVideoEl) {
      previewVideoEl.muted = false;
      if (wasMuted && shouldPlayPreview && previewVideoEl.paused) {
        previewVideoEl.play().catch(() => {});
      }
    }
    applyPreviewAudioEnvelope();
    if (shouldPlayPreview && (wasMuted || (audioFadeRaf === undefined && audioFadeTimer === undefined))) {
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
    hasExplicitMuteChoice = true;
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
      const clip = previewClip;
      if (!video || !clip || videoMuted || !shouldPlayPreview) {
        audioFadeRaf = undefined;
        return;
      }
      applyPreviewAudioEnvelope(now);

      const endSec = getPreviewEndSec(video, clip);
      const elapsedInClip = Math.max(0, video.currentTime - clip.startSec);
      const remaining = Math.max(0, endSec - 0.15 - video.currentTime);
      const inFade =
        !videoReady ||
        video.paused ||
        video.currentTime < clip.startSec - 0.15 ||
        elapsedInClip < AUDIO_FADE_IN_SEC ||
        remaining <= AUDIO_FADE_OUT_SEC + 0.12 ||
        (unmuteFadeStartMs > 0 && now - unmuteFadeStartMs < UNMUTE_FADE_MS) ||
        isSwitchingOpening;

      if (inFade) {
        audioFadeRaf = window.requestAnimationFrame(tick);
      } else {
        audioFadeRaf = undefined;
        const sleepMs = Math.max(40, (remaining - AUDIO_FADE_OUT_SEC - 0.08) * 1000);
        audioFadeTimer = window.setTimeout(() => {
          audioFadeTimer = undefined;
          audioFadeRaf = window.requestAnimationFrame(tick);
        }, sleepMs);
      }
    };
    audioFadeRaf = window.requestAnimationFrame(tick);
  }

  function clearDetachTimer() {
    if (pendingDetachTimer !== undefined) {
      window.clearTimeout(pendingDetachTimer);
      pendingDetachTimer = undefined;
    }
  }

  function destroyPreviewStream() {
    clearDetachTimer();
    stopAudioFadeLoop();
    streamBufferedReady = false;
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

  function softHideOutgoingPreview() {
    clearDetachTimer();
    stopAudioFadeLoop();
    streamBufferedReady = false;
    videoReady = false;
    if (previewVideoEl) {
      previewVideoEl.pause();
      previewVideoEl.volume = 0;
    }
    pendingDetachTimer = window.setTimeout(() => {
      pendingDetachTimer = undefined;
      destroyPreviewStream();
      previewClip = null;
    }, 560);
  }

  function pickInitialOpeningForHero(hero: HomeSpotlightItem): HeroOpeningOption | null {
    const openings = hero.openings ?? [];
    if (openings.length === 0) return null;
    const savedNum = selectedOpenings[hero.shikiId];
    const targetNum = savedNum ?? hero.defaultOpeningNumber ?? openings[0]?.number;
    return openings.find((op) => op.number === targetNum) ?? openings[0] ?? null;
  }

  function warmHeroSlideTarget(hero: HomeSpotlightItem, option: HeroOpeningOption | null) {
    warmHeroOpeningPreview(
      {
        title: hero.title,
        releaseAlias: hero.releaseAlias,
        openingAlias: option ? option.alias : hero.openingAlias,
        openingEpisode: option ? option.episode : hero.openingEpisode,
        openingStartOffsetSec: option ? option.startOffsetSec : hero.openingStartOffsetSec,
        customPreviewUrl: hero.customPreviewUrl,
        customPreviewAspect: hero.customPreviewAspect,
        shikiId: hero.shikiId,
        searchQueries: hero.searchQueries
      },
      { quality: 'hero', maxSeconds: 26 }
    );
  }

  function warmSpotlightNeighbors(currentIdx: number, hero: HomeSpotlightItem, currentOp: HeroOpeningOption | null) {
    window.clearTimeout(warmNeighborsTimer);
    const items = spotlight;
    warmNeighborsTimer = window.setTimeout(() => {
      // Warm alternate OPs of the active anime first so switching OP1/OP2 feels instant.
      for (const op of hero.openings ?? []) {
        if (!currentOp || op.number !== currentOp.number) {
          warmHeroSlideTarget(hero, op);
        }
      }
      const order = computeAdjacentHeroIndices(currentIdx, items.length);
      for (const idx of order.slice(0, 3)) {
        const neighbor = items[idx];
        if (!neighbor) continue;
        warmHeroSlideTarget(neighbor, pickInitialOpeningForHero(neighbor));
      }
    }, 220);
  }

  function applySpotlightIndex(index: number)
  {
    if (index === activeIndex)
    {
      return;
    }
    activeIndex = index;
    const selected = spotlight[index];
    if (selected?.customPreviewUrl && !hasExplicitMuteChoice && videoMuted)
    {
      videoMuted = false;
    }
  }

  function nextBgSlide() {
    if (totalBgSlides <= 1) return;
    bgSlideIndex = (bgSlideIndex + 1) % totalBgSlides;
  }

  function bgSlideIn(_node: Element) {
    return {
      duration: 540,
      easing: cubicOut,
      css: (t: number) =>
        `opacity: ${t}; transform: scale(${1.03 - t * 0.03});`
    };
  }

  function bgSlideOut(_node: Element) {
    return {
      duration: 460,
      easing: cubicOut,
      css: (t: number) =>
        `opacity: ${t}; transform: scale(${1 + (1 - t) * 0.02});`
    };
  }

  function startAutoCycle()
  {
    window.clearInterval(cycleTimer);
    cycleTimer = window.setInterval(() =>
    {
      if (isPaused || opMenuOpen || !videoMuted || bgSlideIndex > 0 || router.current !== 'home' || spotlight.length <= 1)
      {
        return;
      }
      activeIndex = (activeIndex + 1) % spotlight.length;
    }, SPOTLIGHT_INTERVAL_MS);
  }

  function resolveOpeningForHero(hero: HomeSpotlightItem, option: HeroOpeningOption | null) {
    return resolveHeroOpeningPreview({
      title: hero.title,
      releaseAlias: hero.releaseAlias,
      openingAlias: option ? option.alias : hero.openingAlias,
      openingEpisode: option ? option.episode : hero.openingEpisode,
      openingStartOffsetSec: option ? option.startOffsetSec : hero.openingStartOffsetSec,
      customPreviewUrl: hero.customPreviewUrl,
      customPreviewAspect: hero.customPreviewAspect,
      shikiId: hero.shikiId,
      searchQueries: hero.searchQueries
    });
  }

  async function selectHeroOpening(
    opNumber: number,
    options?: { triggerEl?: HTMLElement | null }
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
    bgSlideIndex = 0;
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
      previewClip = { ...clip };
    } else {
      isSwitchingOpening = false;
    }
  }

  function stepHeroOpening(delta: number, triggerEl?: HTMLElement | null) {
    if (heroOpenings.length <= 1) return;
    const nextIdx = (activeOpeningIndex + delta + heroOpenings.length) % heroOpenings.length;
    const nextOp = heroOpenings[nextIdx];
    if (nextOp) {
      void selectHeroOpening(nextOp.number, { triggerEl });
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

  function handlePreviewPlaying() {
    if (!previewClip || !previewVideoEl || !streamBufferedReady) return;
    if (previewVideoEl.currentTime >= previewClip.startSec - 0.25) {
      if (!videoReady) {
        unmuteFadeStartMs = performance.now();
        isSwitchingOpening = false;
        switchFadeStartMs = 0;
      }
      videoReady = true;
      markHeroMediaReady();
      applyPreviewAudioEnvelope();
    }
  }

  function handlePreviewTimeUpdate() {
    if (!previewClip || !previewVideoEl) return;
    const t = previewVideoEl.currentTime;
    if (!previewVideoEl.seeking && t < previewClip.startSec - 0.15) {
      previewVideoEl.volume = 0;
      previewVideoEl.currentTime = previewClip.startSec;
      return;
    }
    if (streamBufferedReady && !videoReady && t >= previewClip.startSec - 0.25) {
      videoReady = true;
      markHeroMediaReady();
      isSwitchingOpening = false;
      switchFadeStartMs = 0;
      unmuteFadeStartMs = performance.now();
    }
    const endSec = getPreviewEndSec(previewVideoEl, previewClip);
    if (t >= endSec - 0.15) {
      previewVideoEl.volume = 0;
      previewVideoEl.currentTime = previewClip.startSec;
      return;
    }
    applyPreviewAudioEnvelope();
  }

  $effect(() => {
    const hero = activeHero;
    const curIdx = activeIndex;
    const token = ++previewReqToken;
    bgSlideIndex = 0;
    isSwitchingOpening = false;
    switchFadeStartMs = 0;
    opMenuOpen = false;
    const hadVisibleVideo = untrack(() => videoReady || Boolean(previewClip));
    untrack(() => softHideOutgoingPreview());

    if (!hero) {
      untrack(() => {
        destroyPreviewStream();
        previewClip = null;
      });
      return;
    }

    const initialOp = untrack(() => pickInitialOpeningForHero(hero));
    untrack(() => warmSpotlightNeighbors(curIdx, hero, initialOp));

    Promise.all([
      resolveOpeningForHero(hero, initialOp).catch(() => null),
      hadVisibleVideo
        ? new Promise<void>((resolve) => window.setTimeout(resolve, SLIDE_FADE_OVERLAP_MS))
        : Promise.resolve()
    ])
      .then(([clip]) => {
        if (token !== previewReqToken) return;
        clearDetachTimer();
        if (!clip) {
          destroyPreviewStream();
          previewClip = null;
          markHeroMediaReady();
          return;
        }
        previewClip = { ...clip };
      })
      .catch(() => {
        if (token === previewReqToken) {
          clearDetachTimer();
          destroyPreviewStream();
          previewClip = null;
        }
        markHeroMediaReady();
      });
  });

  $effect(() => {
    const clip = previewClip;
    const video = previewVideoEl;
    untrack(() => destroyPreviewStream());
    if (!clip || !video) return;

    let cancelled = false;
    let revealed = false;
    let canPlayThrough = false;
    let seekRequested = false;
    let revealFallbackTimer: number | undefined;

    video.muted = untrack(() => videoMuted);
    video.volume = 0;
    video.pause();
    unmuteFadeStartMs = performance.now();

    const clearReadinessTimers = () => {
      if (revealFallbackTimer !== undefined) {
        window.clearTimeout(revealFallbackTimer);
        revealFallbackTimer = undefined;
      }
    };

    const detachReadinessListeners = () => {
      video.removeEventListener('loadedmetadata', onReadinessChange);
      video.removeEventListener('loadeddata', onReadinessChange);
      video.removeEventListener('seeked', onReadinessChange);
      video.removeEventListener('progress', onReadinessChange);
      video.removeEventListener('canplay', onReadinessChange);
      video.removeEventListener('canplaythrough', onCanPlayThrough);
    };

    const revealPreview = () => {
      if (cancelled || revealed) return;
      revealed = true;
      streamBufferedReady = true;
      clearReadinessTimers();
      detachReadinessListeners();

      const endSec = getPreviewEndSec(video, clip);
      if (video.currentTime < clip.startSec - 0.15 || video.currentTime >= endSec) {
        video.currentTime = clip.startSec;
      }
      videoReady = true;
      isSwitchingOpening = false;
      switchFadeStartMs = 0;
      unmuteFadeStartMs = performance.now();
      markHeroMediaReady();

      if (untrack(() => shouldPlayPreview)) {
        video.play().catch(() => {});
        if (!videoMuted) {
          startAudioFadeLoop();
        }
      }
      applyPreviewAudioEnvelope();
    };

    const evaluateReadiness = () => {
      if (cancelled || revealed) return;
      if (video.readyState < 1) return;

      const endSec = getPreviewEndSec(video, clip);
      if (!seekRequested) {
        seekRequested = true;
        if (video.currentTime < clip.startSec - 0.15 || video.currentTime >= endSec) {
          try {
            video.currentTime = clip.startSec;
          } catch {}
        }
      }

      if (
        hasEnoughHeroPreviewBuffer({
          currentTime: video.currentTime,
          targetStartSec: clip.startSec,
          targetEndSec: endSec,
          readyState: video.readyState,
          buffered: video.buffered,
          canPlayThrough
        })
      ) {
        revealPreview();
        return;
      }

      if (revealFallbackTimer === undefined && video.readyState >= 2) {
        revealFallbackTimer = window.setTimeout(() => {
          if (!cancelled && !revealed && video.readyState >= 2) {
            revealPreview();
          }
        }, BUFFER_REVEAL_FALLBACK_MS);
      }
    };

    function onReadinessChange() {
      evaluateReadiness();
    }

    function onCanPlayThrough() {
      canPlayThrough = true;
      evaluateReadiness();
    }

    video.addEventListener('loadedmetadata', onReadinessChange);
    video.addEventListener('loadeddata', onReadinessChange);
    video.addEventListener('seeked', onReadinessChange);
    video.addEventListener('progress', onReadinessChange);
    video.addEventListener('canplay', onReadinessChange);
    video.addEventListener('canplaythrough', onCanPlayThrough);

    const attachHeroHls = (HlsCtor: typeof HlsType) => {
      if (cancelled) return;
      if (HlsCtor.isSupported()) {
        const hls = new HlsCtor({
          enableWorker: true,
          lowLatencyMode: false,
          startPosition: clip.startSec,
          startFragPrefetch: true,
          maxBufferLength: 32,
          maxMaxBufferLength: 60,
          backBufferLength: 60,
          fragLoadingMaxRetry: 4,
          abrEwmaDefaultEstimate: 2_500_000
        });
        previewHls = hls;
        hls.loadSource(clip.streamUrl);
        hls.attachMedia(video);
        hls.on(HlsCtor.Events.MANIFEST_PARSED, evaluateReadiness);
        hls.on(HlsCtor.Events.FRAG_BUFFERED, evaluateReadiness);
        hls.on(HlsCtor.Events.BUFFER_APPENDED, evaluateReadiness);
        hls.on(HlsCtor.Events.ERROR, (_ev, data) => {
          if (data.fatal) {
            clearReadinessTimers();
            detachReadinessListeners();
            markHeroMediaReady();
            destroyPreviewStream();
          }
        });
      } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
        video.src = clip.streamUrl;
        video.load();
        evaluateReadiness();
      } else {
        markHeroMediaReady();
      }
    };

    if (clip.isDirectVideo) {
      video.src = clip.streamUrl;
      video.load();
      evaluateReadiness();
    } else {
      const CachedHls = getLoadedHls();
      if (CachedHls) {
        attachHeroHls(CachedHls);
      } else {
        void loadHls()
          .then(attachHeroHls)
          .catch(() => {
            if (!cancelled) markHeroMediaReady();
          });
      }
    }

    return () => {
      cancelled = true;
      clearReadinessTimers();
      detachReadinessListeners();
      destroyPreviewStream();
    };
  });

  $effect(() => {
    const video = previewVideoEl;
    const clip = previewClip;
    const play = shouldPlayPreview;
    if (!video || !clip) return;
    untrack(() => {
      if (!streamBufferedReady) {
        if (!play) {
          stopAudioFadeLoop();
          video.pause();
        }
        return;
      }
      if (play) {
        const endSec = getPreviewEndSec(video, clip);
        if (video.currentTime < clip.startSec - 0.15 || video.currentTime >= endSec) {
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
        if (shouldPlayPreview && streamBufferedReady) {
          if (video.paused) {
            video.play().catch(() => {});
          }
          startAudioFadeLoop();
        }
      }
    });
  });

  $effect(() => {
    const el = heroBgEl;
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
    window.clearTimeout(warmNeighborsTimer);
    window.clearInterval(cycleTimer);
    destroyPreviewStream();
  });

  onMount(async () =>
  {
    startAutoCycle();

    const feeds = await loadHomeFeeds();
    if (feeds.spotlight.length > 0)
    {
      spotlight = feeds.spotlight;
      activeIndex = 0;
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

  const activeHeroInPlan = $derived(
    activeHero
      ? isInWatchLater({
          shikiId: activeHero.shikiId,
          releaseAlias: activeHero.releaseAlias,
          title: activeHero.title
        })
      : false
  );

  const activeHeroLiked = $derived(
    activeHero
      ? isLiked({
          shikiId: activeHero.shikiId,
          releaseAlias: activeHero.releaseAlias,
          title: activeHero.title
        })
      : false
  );

  function buildHeroLibraryPayload(target: HomeSpotlightItem)
  {
    return {
      shikiId: target.shikiId,
      releaseAlias: target.releaseAlias,
      searchQueries: target.searchQueries,
      title: target.title,
      description: target.description,
      poster: target.poster,
      score: target.score,
      tags: target.metaTags
    };
  }

  function handlePlanClick()
  {
    if (!activeHero) return;
    isPlanLoading = true;
    toggleWatchLater(buildHeroLibraryPayload(activeHero));
    isPlanLoading = false;
  }

  function handleLikeClick()
  {
    if (!activeHero) return;
    toggleLike(buildHeroLibraryPayload(activeHero));
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="hero-bg-wrapper" bind:this={heroBgEl}>
  {#if displayedBgImage}
    {#key displayedBgImage}
      <div
        class="hero-bg-image"
        style="background-image: url('{displayedBgImage}');"
        in:bgSlideIn
        out:bgSlideOut
      ></div>
    {/key}
  {/if}

  <!-- Opening video playing full-bleed behind overlays & text -->
  <div
    class="hero-bg-video-stage"
    class:is-ready={videoReady && showOpeningBg}
    class:is-switching={isSwitchingOpening}
  >
    <video
      bind:this={previewVideoEl}
      class="hero-bg-video"
      muted={videoMuted}
      playsinline
      preload="auto"
      onplaying={handlePreviewPlaying}
      ontimeupdate={handlePreviewTimeUpdate}
    ></video>
  </div>

  <!-- 1. Dark dimming & gradients over background opening video -->
  <div class="bg-dark-overlay"></div>
  <div class="bg-gradient-overlay"></div>
  <div class="bg-fade-to-solid"></div>

  <div class="hero-content-layer">
    <section
      class="hero-container"
      onmouseenter={() => { isPaused = true; }}
      onmouseleave={() => { isPaused = false; }}
    >
      {#if totalBgSlides > 1}
        <button
          type="button"
          class="hero-bg-next-btn"
          aria-label="Next background frame"
          onclick={nextBgSlide}
        >
          <MorphIcon icon={ChevronRight} size={22} strokeWidth={1.75} />
        </button>
      {/if}

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

              <!-- Action buttons with icons + opening audio/selector controls -->
              <div class="hero-actions">
                <MainButton loading={isWatchLoading || player.loading} onclick={handleWatchClick}>
                  {#snippet icon()}
                    <MorphIcon icon={Play} size={16} fill="currentColor" strokeWidth={0} />
                  {/snippet}
                  {t('news.watch')}
                </MainButton>

                <SecondButton loading={isPlanLoading} onclick={handlePlanClick}>
                  {#snippet icon()}
                    <MorphIcon
                      icon={activeHeroInPlan ? Check : Bookmark}
                      size={16}
                      strokeWidth={activeHeroInPlan ? 2.2 : 2}
                      spring="snappy"
                    />
                  {/snippet}
                  {activeHeroInPlan ? t('anime.inPlanToWatch') : t('home.planToWatch')}
                </SecondButton>

                <LikeButton
                  variant="media"
                  liked={activeHeroLiked}
                  onclick={handleLikeClick}
                />

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
                    <button
                      type="button"
                      class="preview-sound-btn"
                      aria-label={videoMuted ? 'Unmute opening preview' : 'Mute opening preview'}
                      title={videoMuted ? 'Unmute opening' : `Volume: ${Math.round(effectiveVolume * 100)}%`}
                      onclick={togglePreviewMute}
                    >
                      <MorphIcon
                        icon={videoMuted || videoVolume <= 0 ? VolumeX : Volume2}
                        size={16}
                        strokeWidth={2}
                        spring="snappy"
                      />
                    </button>
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
                  </div>
                {/if}

                {#if heroOpenings.length > 1 && activeOpeningOption && (videoReady || isSwitchingOpening || previewClip)}
                  <div
                    bind:this={opCtrlEl}
                    class="preview-op-ctrl"
                    class:is-open={opMenuOpen}
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
                        <MorphIcon icon={ChevronLeft} size={14} strokeWidth={2.2} />
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
                        <MorphIcon icon={ChevronRight} size={14} strokeWidth={2.2} />
                      </button>
                    {/if}
                  </div>
                {/if}
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
                    class:is-paused={isPaused || opMenuOpen || bgSlideIndex > 0 || router.current !== 'home'}
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
    </section>
  </div>
</div>

<style>
  .hero-bg-wrapper {
    position: relative;
    width: 100%;
    min-height: 100vh;
    padding-top: var(--header-height, 58px);
    background-color: var(--mila-app-canvas, #0c0c0c);
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
    overflow: hidden;
    contain: layout style paint;
  }

  .hero-bg-image {
    position: absolute;
    inset: 0;
    background-size: cover;
    background-position: center center;
    background-repeat: no-repeat;
    z-index: 0;
    pointer-events: none;
    will-change: opacity;
  }

  .hero-bg-video-stage {
    position: absolute;
    inset: 0;
    z-index: 0;
    pointer-events: none;
    opacity: 0;
    transform: scale(1.022);
    filter: blur(6px);
    transition:
      opacity 0.52s cubic-bezier(0.22, 1, 0.36, 1),
      transform 0.56s cubic-bezier(0.22, 1, 0.36, 1),
      filter 0.52s cubic-bezier(0.22, 1, 0.36, 1);
    will-change: opacity, transform, filter;
  }

  .hero-bg-video-stage.is-ready {
    opacity: 1;
    transform: scale(1);
    filter: blur(0px);
  }

  .hero-bg-video-stage.is-ready.is-switching {
    opacity: 0;
    transform: scale(1.022);
    filter: blur(6px);
  }

  .hero-bg-video {
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center center;
    display: block;
  }

  /* 52% base dark dimming so moving opening stays visible yet text remains crisp */
  .bg-dark-overlay {
    position: absolute;
    inset: 0;
    background-color: rgba(0, 0, 0, 0.52);
    z-index: 1;
    pointer-events: none;
  }

  /* smooth vertical vignette */
  .bg-gradient-overlay {
    position: absolute;
    inset: 0;
    background: linear-gradient(
      180deg,
      rgba(0, 0, 0, 0.5) 0%,
      rgba(0, 0, 0, 0.1) 30%,
      rgba(0, 0, 0, 0.32) 66%,
      rgba(0, 0, 0, 0.82) 100%
    );
    z-index: 2;
    pointer-events: none;
  }

  /* bottom fade to canvas */
  .bg-fade-to-solid {
    position: absolute;
    inset: 0;
    background: linear-gradient(
      180deg,
      rgba(0, 0, 0, 0) 0%,
      rgba(var(--mila-app-canvasRgb, 12, 12, 12), 0) 40%,
      rgba(var(--mila-app-canvasRgb, 12, 12, 12), 0.6) 70%,
      rgba(var(--mila-app-canvasRgb, 12, 12, 12), 0.95) 90%,
      var(--mila-app-canvas, #0c0c0c) 100%
    );
    z-index: 3;
    pointer-events: none;
  }

  .hero-content-layer {
    position: relative;
    z-index: 4;
    width: 100%;
    flex: 1;
    display: flex;
    flex-direction: column;
    --mila-buttons-mainBg: var(--mila-media-mainBtnBg, rgba(255, 255, 255, 0.96));
    --mila-buttons-mainText: var(--mila-media-mainBtnText, #111111);
    --mila-buttons-secondBg: var(--mila-media-secondBtnBg, rgba(255, 255, 255, 0.08));
    --mila-buttons-secondText: var(--mila-media-secondBtnText, #ffffff);
    --mila-meta-tag: var(--mila-media-subtext, rgba(255, 255, 255, 0.56));
    --mila-meta-dot: var(--mila-media-muted, rgba(255, 255, 255, 0.36));
  }

  .hero-container {
    width: 100%;
    min-height: calc(100vh - var(--header-height, 58px));
    display: flex;
    flex-direction: column;
    padding: 0 var(--page-pad-x) var(--page-pad-x) var(--page-pad-x);
    box-sizing: border-box;
    position: relative;
    z-index: 2;
  }

  .hero-bg-next-btn {
    position: absolute;
    right: 6px;
    top: 40%;
    transform: translateY(-50%);
    z-index: 6;
    width: 40px;
    height: 54px;
    padding: 0;
    border: none;
    outline: none;
    border-radius: 999px;
    background: transparent;
    color: rgba(255, 255, 255, 0.28);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    opacity: 0.5;
    transition:
      opacity 0.26s var(--ease-out),
      color 0.22s var(--ease-out),
      background-color 0.22s var(--ease-out),
      transform 0.22s var(--ease-out);
  }

  .hero-bg-wrapper:hover .hero-bg-next-btn {
    opacity: 0.72;
  }

  .hero-bg-next-btn:hover,
  .hero-bg-next-btn:focus-visible {
    opacity: 1;
    color: rgba(255, 255, 255, 0.94);
    background: transparent;
    transform: translateY(-50%) translateX(2px);
  }

  .hero-bg-next-btn:active {
    transform: translateY(-50%) scale(0.94);
  }

  .preview-op-ctrl {
    position: relative;
    height: 38px;
    padding: 4px;
    border-radius: 999px;
    background: transparent;
    display: flex;
    align-items: center;
    gap: 2px;
    opacity: 0.42;
    transition: opacity 0.24s var(--ease-out);
  }

  .preview-op-ctrl:hover,
  .preview-op-ctrl:focus-within,
  .preview-op-ctrl.is-open {
    opacity: 1;
  }

  .preview-op-tag {
    padding: 0 6px 0 6px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    color: rgba(255, 255, 255, 0.48);
    user-select: none;
  }

  .preview-op-segments {
    position: relative;
    display: flex;
    align-items: center;
    height: 28px;
  }

  .preview-op-pill {
    position: absolute;
    left: 0;
    top: 0;
    width: 28px;
    height: 28px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.14);
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
    color: rgba(255, 255, 255, 0.54);
    font-size: 12px;
    font-weight: 500;
    line-height: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    user-select: none;
    transition: color 0.2s ease;
  }

  .preview-op-num:hover:not(.is-active) {
    color: rgba(255, 255, 255, 0.9);
  }

  .preview-op-num.is-active {
    color: #ffffff;
    font-weight: 700;
  }

  .preview-op-step {
    width: 24px;
    height: 28px;
    padding: 0;
    border: none;
    outline: none;
    border-radius: 999px;
    background: transparent;
    color: rgba(255, 255, 255, 0.65);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: color 0.18s ease, transform 0.16s ease;
  }

  .preview-op-step:hover {
    color: #ffffff;
  }

  .preview-op-current {
    height: 28px;
    padding: 0 8px;
    border: none;
    outline: none;
    border-radius: 999px;
    background: transparent;
    color: rgba(255, 255, 255, 0.84);
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.02em;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    user-select: none;
    transition: background-color 0.18s ease, color 0.18s ease;
  }

  .preview-op-current:hover,
  .preview-op-ctrl.is-open .preview-op-current {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .preview-op-popover {
    position: absolute;
    left: 0;
    bottom: calc(100% + 10px);
    z-index: 20;
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
    height: 38px;
    padding: 2px;
    border-radius: 999px;
    background: transparent;
    display: flex;
    align-items: center;
    opacity: 0.42;
    transition: opacity 0.24s var(--ease-out);
  }

  .preview-sound-ctrl:hover,
  .preview-sound-ctrl:focus-within,
  .preview-sound-ctrl.is-adjusting {
    opacity: 1;
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
    width: 84px;
    opacity: 1;
    margin: 0 4px 0 2px;
  }

  .preview-volume-track {
    width: 72px;
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
    height: 3px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.2);
  }

  .preview-volume-fill {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.82);
    pointer-events: none;
  }

  .preview-volume-thumb {
    position: absolute;
    top: 50%;
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.92);
    transform: translate(-50%, -50%);
    transition: transform 0.14s ease;
    pointer-events: none;
  }

  .preview-volume-track:hover .preview-volume-thumb,
  .preview-sound-ctrl.is-adjusting .preview-volume-thumb {
    transform: translate(-50%, -50%) scale(1.18);
  }

  .preview-sound-btn {
    width: 32px;
    height: 32px;
    flex-shrink: 0;
    border: none;
    outline: none;
    border-radius: 50%;
    background: transparent;
    color: rgba(255, 255, 255, 0.78);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: color 0.18s ease, transform 0.18s ease;
  }

  .preview-sound-btn:hover {
    color: #ffffff;
  }

  /* Text, actions & spotlight container */
  .hero-content {
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
    margin-top: auto;
    padding-top: clamp(78px, 11vh, 120px);
    position: relative;
    z-index: 2;
  }

  .hero-title {
    margin: 0;
    padding: 0;
    max-width: min(100%, 960px);
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(32px, 4.2vw, 64px);
    font-weight: 700;
    color: #ffffff;
    line-height: 1.06;
    letter-spacing: -0.02em;
    overflow-wrap: break-word;
    user-select: none;
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
    font-size: clamp(15px, 1.45vw, 19px);
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
    flex-wrap: wrap;
    gap: 12px;
    margin-top: 16px;
  }

  /* Bottom-right Mila-native panorama accordion strip (gap: 0, edge-to-edge posters inside rounded clip) */
  .hero-strip-wrap {
    position: relative;
    width: clamp(390px, 38vw, 520px);
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
    transition: filter 0.32s var(--ease-out);
  }

  .strip-card:not(.is-active):hover .strip-card-img {
    filter: brightness(1.09) contrast(1.04);
  }

  .strip-card::after {
    content: '';
    position: absolute;
    inset: 0;
    z-index: 2;
    background: linear-gradient(155deg, rgba(255, 255, 255, 0.1) 0%, rgba(255, 255, 255, 0) 48%);
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.28s var(--ease-out);
  }

  .strip-card:not(.is-active):hover::after {
    opacity: 1;
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
    background: var(--mila-progress-heroStripFill, rgba(255, 255, 255, 0.85));
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
      gap: 18px;
    }

    .hero-strip-wrap {
      width: 100%;
    }
  }

  @media (max-height: 740px) {
    .hero-content {
      padding-top: 56px;
    }
  }
</style>