<script lang="ts">
  import Hls, { type ErrorData } from 'hls.js';
  import Play from '@lucide/svelte/icons/play';
  import Pause from '@lucide/svelte/icons/pause';
  import Volume1 from '@lucide/svelte/icons/volume-1';
  import Volume2 from '@lucide/svelte/icons/volume-2';
  import VolumeX from '@lucide/svelte/icons/volume-x';
  import Maximize from '@lucide/svelte/icons/maximize';
  import Minimize from '@lucide/svelte/icons/minimize';
  import PictureInPicture2 from '@lucide/svelte/icons/picture-in-picture-2';
  import Settings from '@lucide/svelte/icons/settings';
  import SkipForward from '@lucide/svelte/icons/skip-forward';
  import AudioLines from '@lucide/svelte/icons/audio-lines';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import ArrowRightLeft from '@lucide/svelte/icons/arrow-right-left';
  import ArrowUpLeft from '@lucide/svelte/icons/arrow-up-left';
  import X from '@lucide/svelte/icons/x';
  import { untrack } from 'svelte';
  import { fly, scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';

  import ComboBox from '../combo/ComboBox.svelte';
  import type { ComboSection } from '../combo/types';
  import {
    createMaxQualityHlsConfig,
    formatQualityLabel,
    resolvePlaybackSource,
    type Mp4Variant
  } from './hlsConfig';
  import type {
    AudioTrackOption,
    HlsSources,
    QualityLevelOption,
    SkipRange,
    VoiceTrackOption
  } from './types';

  interface Props {
    src?: string;
    sources?: HlsSources | null;
    poster?: string;
    title?: string;
    subtitle?: string;
    autoplay?: boolean;
    initialTime?: number;
    voices?: VoiceTrackOption[];
    selectedVoiceId?: string;
    preferMaxQuality?: boolean;
    opening?: SkipRange | null;
    ending?: SkipRange | null;
    hasNext?: boolean;
    hasFallback?: boolean;
    isPip?: boolean;
    onnext?: () => void;
    onended?: () => void;
    ontimeprogress?: (currentTime: number, duration: number, force?: boolean) => void;
    onselectvoice?: (voice: VoiceTrackOption, currentTime: number) => void;
    onfallback?: () => void;
    onerror?: (message: string) => void;
    onpip?: () => void;
    onexpandpip?: () => void;
    onclosepip?: () => void;
    onplaychange?: (playing: boolean) => void;
    onpipdragstart?: (e: PointerEvent) => void;
    class?: string;
  }

  let {
    src = '',
    sources = null,
    poster = '',
    title = '',
    subtitle = '',
    autoplay = false,
    initialTime = 0,
    voices = [],
    selectedVoiceId = '',
    preferMaxQuality = true,
    opening = null,
    ending = null,
    hasNext = false,
    hasFallback = false,
    isPip = false,
    onnext,
    onended,
    ontimeprogress,
    onselectvoice,
    onfallback,
    onerror,
    onpip,
    onexpandpip,
    onclosepip,
    onplaychange,
    onpipdragstart,
    class: className = ''
  }: Props = $props();

  const VOLUME_KEY = 'mila.player.volume';
  const MUTED_KEY = 'mila.player.muted';
  const SPEED_OPTIONS = [0.5, 0.75, 1, 1.25, 1.5, 2];

  let rootEl = $state<HTMLDivElement | null>(null);
  let videoEl = $state<HTMLVideoElement | null>(null);
  let seekEl = $state<HTMLDivElement | null>(null);
  let volumeEl = $state<HTMLDivElement | null>(null);

  let hls: Hls | null = null;
  let playing = $state(false);
  let buffering = $state(false);
  let currentTime = $state(0);
  let duration = $state(0);
  let bufferedEnd = $state(0);

  let volume = $state(readSavedVolume());
  let muted = $state(readSavedMuted());
  let playbackRate = $state(1);

  let isFullscreen = $state(false);
  let controlsVisible = $state(true);
  let activeMenu = $state<'none' | 'voice' | 'quality' | 'speed'>('none');
  let errorText = $state('');

  let levels = $state<QualityLevelOption[]>([]);
  let selectedLevel = $state(-1); // -1 = Auto ABR, >=0 = pinned level index
  let activeLevelIndex = $state(-1);
  let audioTracks = $state<AudioTrackOption[]>([]);
  let activeAudioTrack = $state(-1);
  let playbackMode = $state<'hls' | 'mp4'>('hls');
  let mp4Variants = $state<Mp4Variant[]>([]);
  let reloadTick = $state(0);

  let hoverTime = $state<number | null>(null);
  let hoverPct = $state(0);
  let draggingSeek = $state(false);
  let draggingVolume = $state(false);
  let hideTimer: number | undefined;
  let networkRetries = 0;
  let reparentingGuard = false;
  let suppressNextVideoClick = false;

  export function pause(): void {
    videoEl?.pause();
  }

  export function beginReparent(): boolean {
    const wasPlaying = Boolean(videoEl && !videoEl.paused) || playing;
    reparentingGuard = true;
    return wasPlaying;
  }

  export function endReparent(wasPlaying: boolean): void {
    reparentingGuard = false;
    if (wasPlaying && videoEl && videoEl.paused) {
      void videoEl.play().catch(() => {});
    }
  }

  function readSavedVolume(): number {
    if (typeof localStorage === 'undefined') return 1;
    const raw = Number(localStorage.getItem(VOLUME_KEY));
    return Number.isFinite(raw) && raw >= 0 && raw <= 1 ? raw : 1;
  }

  function readSavedMuted(): boolean {
    if (typeof localStorage === 'undefined') return false;
    return localStorage.getItem(MUTED_KEY) === '1';
  }

  const progressPct = $derived(duration > 0 ? Math.min(100, (currentTime / duration) * 100) : 0);
  const bufferedPct = $derived(duration > 0 ? Math.min(100, (bufferedEnd / duration) * 100) : 0);
  const volumePct = $derived((muted ? 0 : volume) * 100);

  const activeVoiceLabel = $derived.by(() => {
    const activeVoice = voices.find((v) => v.id === selectedVoiceId) ?? voices[0];
    if (audioTracks.length > 1) {
      const track = audioTracks.find((t) => t.index === activeAudioTrack);
      if (track) return track.label;
    }
    return activeVoice?.label ?? '';
  });

  const activeQualityLabel = $derived.by(() => {
    const found = levels.find((l) => l.index === activeLevelIndex);
    if (found) return found.label;
    if (levels.length > 0 && selectedLevel >= 0) {
      const pinned = levels.find((l) => l.index === selectedLevel);
      if (pinned) return pinned.label;
    }
    return 'HD';
  });

  const voiceComboSections = $derived.by<ComboSection[]>(() => {
    const out: ComboSection[] = [];
    const activeId = selectedVoiceId || voices[0]?.id;

    if (voices.length > 0) {
      out.push({
        id: 'voiceover',
        options: voices.map((v) => ({
          value: `voice:${v.id}`,
          label: v.label,
          sublabel: v.sublabel,
          selected: activeId === v.id
        }))
      });
    }
    if (audioTracks.length > 1) {
      out.push({
        id: 'audio',
        title: 'Audio Track',
        options: audioTracks.map((t) => ({
          value: `audio:${t.index}`,
          label: t.label,
          selected: activeAudioTrack === t.index
        }))
      });
    }
    return out;
  });

  const speedComboOptions = SPEED_OPTIONS.map((rate) => ({
    value: rate,
    label: `${rate}x`
  }));

  const qualityComboOptions = $derived.by(() => {
    const items = levels.map((lvl) => ({
      value: lvl.index,
      label: lvl.label
    }));
    if (playbackMode === 'hls' && levels.length > 1) {
      items.push({ value: -1, label: 'Auto' });
    }
    return items;
  });

  const skipTarget = $derived.by(() => {
    if (
      opening?.start != null &&
      opening?.stop != null &&
      currentTime >= opening.start &&
      currentTime < opening.stop - 1
    ) {
      return { label: 'Skip Opening', to: opening.stop };
    }
    if (
      ending?.start != null &&
      ending?.stop != null &&
      currentTime >= ending.start &&
      currentTime < ending.stop - 1
    ) {
      return { label: 'Skip Ending', to: ending.stop };
    }
    return null;
  });

  function scheduleHideControls() {
    window.clearTimeout(hideTimer);
    controlsVisible = true;
    if (!playing || activeMenu !== 'none') return;
    hideTimer = window.setTimeout(() => {
      controlsVisible = false;
    }, 2400);
  }

  function destroyHls() {
    if (hls) {
      hls.destroy();
      hls = null;
    }
  }

  function pickHighestLevelIndex(rawLevels: Array<{ height: number; bitrate: number }>): number {
    let bestIdx = 0;
    let bestHeight = -1;
    let bestBitrate = -1;
    for (let i = 0; i < rawLevels.length; i++) {
      const h = rawLevels[i].height || 0;
      const b = rawLevels[i].bitrate || 0;
      if (h > bestHeight || (h === bestHeight && b > bestBitrate)) {
        bestHeight = h;
        bestBitrate = b;
        bestIdx = i;
      }
    }
    return bestIdx;
  }

  // Attach and configure hls.js or native MP4 whenever source props change.
  $effect(() => {
    const video = videoEl;
    const directSrc = src;
    const epSources = sources;
    const shouldAutoplay = autoplay;
    const lockMax = preferMaxQuality;
    const resumeOffset = initialTime;
    void reloadTick;

    if (!video) return;

    const resolved = resolvePlaybackSource(directSrc, epSources);
    errorText = '';
    levels = [];
    selectedLevel = -1;
    activeLevelIndex = -1;
    audioTracks = [];
    activeAudioTrack = -1;
    mp4Variants = [];
    playing = false;
    currentTime = 0;
    duration = 0;
    bufferedEnd = 0;
    networkRetries = 0;
    let appliedResumeSeek = false;

    const applyInitialResume = () => {
      if (appliedResumeSeek || resumeOffset <= 0.5 || !video) return;
      appliedResumeSeek = true;
      const targetTime =
        Number.isFinite(video.duration) && video.duration > 0
          ? Math.min(resumeOffset, Math.max(0, video.duration - 0.5))
          : resumeOffset;
      video.currentTime = targetTime;
      currentTime = targetTime;
    };

    const handleNativeLoaded = () => {
      buffering = false;
      applyInitialResume();
    };

    const handleNativeError = () => {
      buffering = false;
      errorText = 'Playback error: failed to load video stream';
      onerror?.(errorText);
    };

    if (!resolved) {
      destroyHls();
      playbackMode = 'hls';
      video.removeAttribute('src');
      video.load();
      return;
    }

    untrack(() => {
      video.volume = volume;
      video.muted = muted;
      video.playbackRate = playbackRate;
    });

    if (resolved.mode === 'mp4') {
      destroyHls();
      playbackMode = 'mp4';
      const variants = [...resolved.variants].sort(
        (a, b) => b.height - a.height || b.bitrate - a.bitrate
      );
      mp4Variants = variants;
      levels = variants.map((v, idx) => ({
        index: idx,
        height: v.height,
        bitrate: v.bitrate,
        label: v.label
      }));
      selectedLevel = 0;
      activeLevelIndex = 0;
      buffering = true;
      video.addEventListener('loadedmetadata', handleNativeLoaded);
      video.addEventListener('error', handleNativeError);
      video.src = variants[0].url;
      video.load();
      if (shouldAutoplay) {
        void video.play().catch(() => {});
      }
      return () => {
        window.clearTimeout(hideTimer);
        video.removeEventListener('loadedmetadata', handleNativeLoaded);
        video.removeEventListener('error', handleNativeError);
        video.pause();
      };
    }

    playbackMode = 'hls';
    if (Hls.isSupported()) {
      destroyHls();
      const instance = new Hls(createMaxQualityHlsConfig());
      hls = instance;
      buffering = true;

      instance.on(Hls.Events.MANIFEST_PARSED, (_event, data) => {
        const mapped: QualityLevelOption[] = data.levels.map((lvl, idx) => ({
          index: idx,
          height: lvl.height || 0,
          bitrate: lvl.bitrate || 0,
          label: formatQualityLabel(lvl.height || 0, lvl.bitrate)
        }));

        // Sort highest resolution first for the quality selector UI.
        mapped.sort((a, b) => b.height - a.height || b.bitrate - a.bitrate);
        levels = mapped;

        if (data.levels.length > 0) {
          const maxIdx = pickHighestLevelIndex(data.levels);
          instance.startLevel = maxIdx;
          if (lockMax) {
            // Lock onto highest quality (e.g. 1080p) so playback never degrades unless switched.
            instance.currentLevel = maxIdx;
            selectedLevel = maxIdx;
          } else {
            instance.currentLevel = -1;
            selectedLevel = -1;
          }
          activeLevelIndex = maxIdx;
        }

        applyInitialResume();

        if (shouldAutoplay) {
          void video.play().catch(() => {});
        }
      });

      instance.on(Hls.Events.AUDIO_TRACKS_UPDATED, (_event, data) => {
        audioTracks = (data.audioTracks ?? []).map((t, i) => ({
          index: t.id ?? i,
          label: t.name || t.lang || `Audio ${i + 1}`,
          lang: t.lang
        }));
        activeAudioTrack = instance.audioTrack;
      });

      instance.on(Hls.Events.AUDIO_TRACK_SWITCHED, (_event, data) => {
        activeAudioTrack = data.id;
      });

      instance.on(Hls.Events.LEVEL_SWITCHED, (_event, data) => {
        activeLevelIndex = data.level;
      });

      instance.on(Hls.Events.ERROR, (_event, data: ErrorData) => {
        if (!data.fatal) return;
        if (data.type === Hls.ErrorTypes.NETWORK_ERROR && networkRetries < 3) {
          networkRetries += 1;
          instance.startLoad();
          return;
        }
        if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
          instance.recoverMediaError();
          return;
        }
        const status = (data.response as { code?: number } | undefined)?.code;
        errorText = status
          ? `Playback error: CDN returned HTTP ${status}`
          : 'Playback error: failed to load HLS stream';
        buffering = false;
        onerror?.(errorText);
      });

      instance.loadSource(resolved.url);
      instance.attachMedia(video);
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      destroyHls();
      video.src = resolved.url;
      video.addEventListener('loadedmetadata', handleNativeLoaded, { once: true });
      video.addEventListener('error', handleNativeError, { once: true });
      if (shouldAutoplay) {
        void video.play().catch(() => {});
      }
    } else {
      errorText = 'HLS playback is not supported in this environment';
      onerror?.(errorText);
    }

    return () => {
      window.clearTimeout(hideTimer);
      video.removeEventListener('loadedmetadata', handleNativeLoaded);
      video.removeEventListener('error', handleNativeError);
      destroyHls();
      resolved.revoke?.();
    };
  });

  let nativeFsActive = false;

  async function setNativeWindowFullscreen(next: boolean) {
    if (typeof window === 'undefined') return;
    if (typeof document !== 'undefined') {
      document.body.classList.toggle('is-player-fullscreen', next);
      document.body.classList.toggle('is-window-fullscreen', next);
    }
    if (nativeFsActive === next) return;
    nativeFsActive = next;
    const { setAppFullscreen } = await import('../../app/bindings.svelte');
    await setAppFullscreen(next, { playerMode: true, domTarget: rootEl });
  }

  async function enterFullscreen() {
    if (!rootEl) return;
    isFullscreen = true;
    await setNativeWindowFullscreen(true);
  }

  async function exitFullscreen() {
    isFullscreen = false;
    await setNativeWindowFullscreen(false);
  }

  $effect(() => {
    function onFsChange() {
      const domFs = document.fullscreenElement === rootEl;
      if (domFs) {
        isFullscreen = true;
        setNativeWindowFullscreen(true);
      } else if (isFullscreen) {
        isFullscreen = false;
        setNativeWindowFullscreen(false);
      }
    }
    function onWinKeydown(e: KeyboardEvent) {
      if (e.key === 'Escape' && isFullscreen && document.fullscreenElement !== rootEl) {
        e.preventDefault();
        void exitFullscreen();
      }
    }
    document.addEventListener('fullscreenchange', onFsChange);
    window.addEventListener('keydown', onWinKeydown);
    return () => {
      document.removeEventListener('fullscreenchange', onFsChange);
      window.removeEventListener('keydown', onWinKeydown);
      if (nativeFsActive) {
        setNativeWindowFullscreen(false);
      }
    };
  });

  function togglePlay() {
    if (!videoEl) return;
    if (videoEl.paused) {
      void videoEl.play().catch(() => {});
    } else {
      videoEl.pause();
    }
    scheduleHideControls();
  }

  function seekTo(seconds: number) {
    if (!videoEl || !Number.isFinite(duration) || duration <= 0) return;
    const clamped = Math.max(0, Math.min(duration, seconds));
    videoEl.currentTime = clamped;
    currentTime = clamped;
    scheduleHideControls();
  }

  function setQuality(levelIdx: number) {
    selectedLevel = levelIdx;
    if (hls) {
      hls.currentLevel = levelIdx;
      if (levelIdx >= 0) {
        activeLevelIndex = levelIdx;
      }
    } else if (playbackMode === 'mp4' && videoEl) {
      const target = mp4Variants[levelIdx];
      if (target && videoEl.src !== target.url) {
        const savedTime = videoEl.currentTime || currentTime;
        const wasPlaying = !videoEl.paused;
        activeLevelIndex = levelIdx;
        buffering = true;
        const restoreSeek = () => {
          if (!videoEl) return;
          if (savedTime > 0.25 && Number.isFinite(videoEl.duration) && videoEl.duration > 0) {
            videoEl.currentTime = Math.min(savedTime, Math.max(0, videoEl.duration - 0.5));
          }
          buffering = false;
          if (wasPlaying) {
            void videoEl.play().catch(() => {});
          }
        };
        videoEl.addEventListener('loadedmetadata', restoreSeek, { once: true });
        videoEl.src = target.url;
        videoEl.load();
      }
    }
    activeMenu = 'none';
    scheduleHideControls();
  }

  function setSpeed(rate: number) {
    playbackRate = rate;
    if (videoEl) {
      videoEl.playbackRate = rate;
    }
    activeMenu = 'none';
    scheduleHideControls();
  }

  function selectVoice(voice: VoiceTrackOption) {
    activeMenu = 'none';
    scheduleHideControls();
    onselectvoice?.(voice, videoEl?.currentTime ?? currentTime);
  }

  function setAudioTrack(trackIdx: number) {
    if (hls) {
      hls.audioTrack = trackIdx;
      activeAudioTrack = trackIdx;
    }
    activeMenu = 'none';
    scheduleHideControls();
  }

  function setVolume(val: number) {
    const clamped = Math.max(0, Math.min(1, val));
    volume = clamped;
    muted = clamped === 0;
    if (videoEl) {
      videoEl.volume = clamped;
      videoEl.muted = muted;
    }
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(VOLUME_KEY, String(clamped));
      localStorage.setItem(MUTED_KEY, muted ? '1' : '0');
    }
  }

  function toggleMute() {
    muted = !muted;
    if (!muted && volume === 0) {
      volume = 0.5;
    }
    if (videoEl) {
      videoEl.muted = muted;
      videoEl.volume = volume;
    }
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(MUTED_KEY, muted ? '1' : '0');
    }
    scheduleHideControls();
  }

  async function toggleFullscreen() {
    if (!rootEl) return;
    if (isFullscreen || document.fullscreenElement === rootEl) {
      await exitFullscreen();
    } else {
      await enterFullscreen();
    }
    scheduleHideControls();
  }

  async function togglePiP() {
    if (onpip) {
      onpip();
      scheduleHideControls();
      return;
    }
    if (!videoEl || !document.pictureInPictureEnabled) return;
    if (document.pictureInPictureElement === videoEl) {
      await document.exitPictureInPicture().catch(() => {});
    } else {
      await videoEl.requestPictureInPicture().catch(() => {});
    }
  }

  function handlePipDragTrigger(e: PointerEvent) {
    if (!isPip || !onpipdragstart || e.button !== 0) return;
    const target = e.target as HTMLElement | null;
    if (target?.closest('button, .seek-track, .volume-track, .combo-root')) return;
    onpipdragstart(e);
  }

  export function notifyPipDragged(): void {
    suppressNextVideoClick = true;
    window.setTimeout(() => {
      suppressNextVideoClick = false;
    }, 80);
  }

  function seekFromPointer(clientX: number) {
    if (!seekEl || duration <= 0) return;
    const rect = seekEl.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
    hoverPct = ratio * 100;
    hoverTime = ratio * duration;
    if (draggingSeek) {
      seekTo(hoverTime);
    }
  }

  function handleSeekPointerDown(e: PointerEvent) {
    if (!seekEl) return;
    draggingSeek = true;
    seekEl.setPointerCapture(e.pointerId);
    seekFromPointer(e.clientX);
  }

  function handleSeekPointerUp(e: PointerEvent) {
    if (!draggingSeek || !seekEl) return;
    seekFromPointer(e.clientX);
    draggingSeek = false;
    seekEl.releasePointerCapture(e.pointerId);
  }

  function volumeFromPointer(clientX: number) {
    if (!volumeEl) return;
    const rect = volumeEl.getBoundingClientRect();
    if (rect.width <= 0) return;
    const ratio = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
    setVolume(ratio);
  }

  function handleVolumePointerDown(e: PointerEvent) {
    if (!volumeEl) return;
    draggingVolume = true;
    volumeEl.setPointerCapture(e.pointerId);
    volumeFromPointer(e.clientX);
  }

  function handleVolumePointerUp(e: PointerEvent) {
    if (!draggingVolume || !volumeEl) return;
    volumeFromPointer(e.clientX);
    draggingVolume = false;
    volumeEl.releasePointerCapture(e.pointerId);
  }

  function handleVolumeKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') {
      e.preventDefault();
      e.stopPropagation();
      setVolume((muted ? 0 : volume) - 0.05);
    } else if (e.key === 'ArrowRight' || e.key === 'ArrowUp') {
      e.preventDefault();
      e.stopPropagation();
      setVolume((muted ? 0 : volume) + 0.05);
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    switch (e.key) {
      case ' ':
      case 'k':
      case 'K':
        e.preventDefault();
        togglePlay();
        break;
      case 'ArrowLeft':
        e.preventDefault();
        seekTo(currentTime - 5);
        break;
      case 'ArrowRight':
        e.preventDefault();
        seekTo(currentTime + 5);
        break;
      case 'j':
      case 'J':
        e.preventDefault();
        seekTo(currentTime - 10);
        break;
      case 'l':
      case 'L':
        e.preventDefault();
        seekTo(currentTime + 10);
        break;
      case 'ArrowUp':
        e.preventDefault();
        setVolume(volume + 0.05);
        break;
      case 'ArrowDown':
        e.preventDefault();
        setVolume(volume - 0.05);
        break;
      case 'm':
      case 'M':
        e.preventDefault();
        toggleMute();
        break;
      case 'f':
      case 'F':
        e.preventDefault();
        void toggleFullscreen();
        break;
      case 'Escape':
        if (activeMenu !== 'none') {
          activeMenu = 'none';
        } else if (isFullscreen) {
          e.preventDefault();
          void exitFullscreen();
        }
        break;
    }
  }

  function updateBuffered() {
    if (!videoEl || videoEl.buffered.length === 0) {
      bufferedEnd = 0;
      return;
    }
    bufferedEnd = videoEl.buffered.end(videoEl.buffered.length - 1);
  }

  function formatTime(sec: number): string {
    if (!Number.isFinite(sec) || sec < 0) return '0:00';
    const whole = Math.floor(sec);
    const h = Math.floor(whole / 3600);
    const m = Math.floor((whole % 3600) / 60);
    const s = whole % 60;
    const mm = h > 0 ? String(m).padStart(2, '0') : String(m);
    const ss = String(s).padStart(2, '0');
    return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  bind:this={rootEl}
  class="hls-player {className}"
  class:is-fullscreen={isFullscreen}
  class:is-pip={isPip && !isFullscreen}
  class:controls-hidden={!controlsVisible && playing}
  tabindex="0"
  role="region"
  aria-label={title || 'Video player'}
  onmousemove={scheduleHideControls}
  onmouseleave={() => {
    if (playing && activeMenu === 'none') controlsVisible = false;
  }}
  onkeydown={handleKeydown}
>
  <!-- svelte-ignore a11y_media_has_caption -->
  <video
    bind:this={videoEl}
    class="hls-video"
    {poster}
    playsinline
    preload="metadata"
    onpointerdown={handlePipDragTrigger}
    onclick={() => {
      if (suppressNextVideoClick) {
        suppressNextVideoClick = false;
        return;
      }
      if (activeMenu !== 'none') {
        activeMenu = 'none';
        return;
      }
      togglePlay();
    }}
    ondblclick={() => {
      if (isPip && onexpandpip) {
        onexpandpip();
        return;
      }
      void toggleFullscreen();
    }}
    onplay={() => {
      playing = true;
      buffering = false;
      onplaychange?.(true);
      scheduleHideControls();
    }}
    onpause={() => {
      if (reparentingGuard && videoEl) {
        void videoEl.play().catch(() => {});
        return;
      }
      playing = false;
      controlsVisible = true;
      onplaychange?.(false);
      window.clearTimeout(hideTimer);
      if (videoEl) {
        ontimeprogress?.(videoEl.currentTime || 0, videoEl.duration || duration || 0, true);
      }
    }}
    onwaiting={() => (buffering = true)}
    oncanplay={() => (buffering = false)}
    onplaying={() => (buffering = false)}
    ontimeupdate={() => {
      if (!draggingSeek && videoEl) {
        currentTime = videoEl.currentTime;
        ontimeprogress?.(videoEl.currentTime || 0, videoEl.duration || duration || 0, false);
      }
    }}
    ondurationchange={() => {
      if (videoEl) duration = videoEl.duration || 0;
    }}
    onprogress={updateBuffered}
    onended={() => {
      playing = false;
      controlsVisible = true;
      onplaychange?.(false);
      if (videoEl) {
        ontimeprogress?.(videoEl.duration || videoEl.currentTime || 0, videoEl.duration || duration || 0, true);
      }
      onended?.();
    }}
  ></video>

  {#if buffering}
    <div class="center-overlay" aria-hidden="true">
      <div class="loader-ring"></div>
    </div>
  {:else if !playing && !errorText}
    <button
      type="button"
      class="center-play"
      aria-label="Play"
      onclick={togglePlay}
      transition:scale={{ start: 0.72, duration: 200, easing: cubicOut }}
    >
      <Play size={54} fill="currentColor" strokeWidth={0} />
    </button>
  {/if}

  {#if errorText}
    <div class="error-banner" role="alert">
      <span class="error-msg">{errorText}</span>
      <div class="error-actions">
        <button
          type="button"
          class="error-btn"
          onclick={() => {
            reloadTick += 1;
          }}
        >
          <RotateCcw size={14} strokeWidth={2} />
          <span>Retry</span>
        </button>
        {#if hasFallback && onfallback}
          <button
            type="button"
            class="error-btn error-btn-primary"
            onclick={() => onfallback?.()}
          >
            <ArrowRightLeft size={14} strokeWidth={2} />
            <span>Switch voice</span>
          </button>
        {/if}
      </div>
    </div>
  {/if}

  {#if skipTarget}
    <button
      type="button"
      class="skip-pill"
      class:raised={controlsVisible || !playing}
      onclick={() => seekTo(skipTarget.to)}
      transition:fly={{ y: 16, duration: 260, easing: cubicOut }}
    >
      <span>{skipTarget.label}</span>
      <SkipForward size={14} strokeWidth={2.1} class="skip-arrow" />
    </button>
  {/if}

  {#if title || subtitle || isPip}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="top-bar"
      class:visible={controlsVisible || !playing}
      onpointerdown={handlePipDragTrigger}
    >
      <div class="title-stack">
        {#if title}
          <span class="player-title">{title}</span>
        {/if}
        {#if subtitle}
          <span class="player-subtitle">{subtitle}</span>
        {/if}
      </div>
      <div class="top-bar-right">
        <span class="quality-badge">{activeQualityLabel}</span>
        {#if isPip && !isFullscreen}
          {#if onexpandpip}
            <button
              type="button"
              class="pip-top-btn"
              aria-label="Return to page"
              title="Return to page"
              onclick={(e) => {
                e.stopPropagation();
                onexpandpip?.();
              }}
            >
              <ArrowUpLeft size={15} strokeWidth={2.1} />
            </button>
          {/if}
          {#if onclosepip}
            <button
              type="button"
              class="pip-top-btn"
              aria-label="Close mini-player"
              title="Close mini-player"
              onclick={(e) => {
                e.stopPropagation();
                onclosepip?.();
              }}
            >
              <X size={15} strokeWidth={2.1} />
            </button>
          {/if}
        {/if}
      </div>
    </div>
  {/if}

  <div class="controls-dock" class:visible={controlsVisible || !playing}>
    <!-- Timeline -->
    <div
      bind:this={seekEl}
      class="seek-track"
      role="slider"
      tabindex="0"
      aria-label="Seek timeline"
      aria-valuemin={0}
      aria-valuemax={duration}
      aria-valuenow={currentTime}
      onpointerdown={handleSeekPointerDown}
      onpointermove={(e) => seekFromPointer(e.clientX)}
      onpointerup={handleSeekPointerUp}
      onpointerleave={() => {
        if (!draggingSeek) hoverTime = null;
      }}
    >
      {#if hoverTime !== null}
        <div class="seek-tooltip" style:left="{hoverPct}%">
          {formatTime(hoverTime)}
        </div>
      {/if}
      <div class="seek-rail">
        <div class="seek-buffered" style:width="{bufferedPct}%"></div>
        <div class="seek-fill" style:width="{progressPct}%"></div>
      </div>
      <div class="seek-thumb" style:left="{progressPct}%"></div>
    </div>

    <div class="controls-row">
      <div class="controls-left">
        <button
          type="button"
          class="icon-btn"
          aria-label={playing ? 'Pause' : 'Play'}
          onclick={togglePlay}
        >
          {#if playing}
            <Pause size={18} fill="currentColor" strokeWidth={0} />
          {:else}
            <Play size={18} fill="currentColor" strokeWidth={0} />
          {/if}
        </button>

        {#if hasNext}
          <button
            type="button"
            class="icon-btn"
            aria-label="Next episode"
            onclick={() => onnext?.()}
          >
            <SkipForward size={18} strokeWidth={1.8} />
          </button>
        {/if}

        <div
          class="volume-group"
          class:is-dragging={draggingVolume}
          onwheel={(e) => {
            e.preventDefault();
            setVolume((muted ? 0 : volume) + (e.deltaY < 0 ? 0.05 : -0.05));
          }}
        >
          <button
            type="button"
            class="icon-btn"
            aria-label={muted ? 'Unmute' : 'Mute'}
            onclick={toggleMute}
          >
            {#if muted || volume === 0}
              <VolumeX size={18} strokeWidth={1.8} />
            {:else if volume <= 0.5}
              <Volume1 size={18} strokeWidth={1.8} />
            {:else}
              <Volume2 size={18} strokeWidth={1.8} />
            {/if}
          </button>
          <div
            bind:this={volumeEl}
            class="volume-track"
            role="slider"
            tabindex="0"
            aria-label="Volume"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(volumePct)}
            onpointerdown={handleVolumePointerDown}
            onpointermove={(e) => {
              if (draggingVolume) volumeFromPointer(e.clientX);
            }}
            onpointerup={handleVolumePointerUp}
            onkeydown={handleVolumeKeydown}
          >
            <div class="volume-rail">
              <div class="volume-fill" style:width="{volumePct}%"></div>
            </div>
            <div class="volume-thumb" style:left="{volumePct}%"></div>
          </div>
        </div>

        <span class="time-readout">
          {formatTime(currentTime)} <span class="time-sep">/</span> {formatTime(duration)}
        </span>
      </div>

      <div class="controls-right">
        {#if !isPip || isFullscreen}
          <!-- Voiceover / Audio track selector -->
          {#if voiceComboSections.length > 0}
            <ComboBox
              open={activeMenu === 'voice'}
              sections={voiceComboSections}
              label={activeVoiceLabel || 'Audio'}
              ariaLabel="Voiceover and audio track"
              placement="top-right"
              minWidth={210}
              maxWidth={276}
              maxHeight={300}
              ontoggle={(isOpen) => {
                activeMenu = isOpen ? 'voice' : 'none';
                scheduleHideControls();
              }}
              onselect={(opt) => {
                const raw = String(opt.value);
                if (raw.startsWith('voice:')) {
                  const voiceId = raw.slice('voice:'.length);
                  const found = voices.find((v) => v.id === voiceId);
                  if (found) selectVoice(found);
                } else if (raw.startsWith('audio:')) {
                  const idx = Number(raw.slice('audio:'.length));
                  if (Number.isFinite(idx)) setAudioTrack(idx);
                }
              }}
            >
              {#snippet icon()}
                <AudioLines size={14} strokeWidth={1.9} />
              {/snippet}
            </ComboBox>
          {/if}

          <!-- Playback speed -->
          <ComboBox
            open={activeMenu === 'speed'}
            value={playbackRate}
            options={speedComboOptions}
            label={`${playbackRate}x`}
            ariaLabel="Playback speed"
            placement="top-right"
            minWidth={112}
            ontoggle={(isOpen) => {
              activeMenu = isOpen ? 'speed' : 'none';
              scheduleHideControls();
            }}
            onselect={(opt) => setSpeed(Number(opt.value))}
          />

          <!-- Quality selector (defaults to highest fixed resolution) -->
          {#if levels.length > 0}
            <ComboBox
              open={activeMenu === 'quality'}
              value={selectedLevel}
              options={qualityComboOptions}
              label={selectedLevel === -1 ? `Auto (${activeQualityLabel})` : activeQualityLabel}
              ariaLabel="Video quality"
              placement="top-right"
              minWidth={112}
              ontoggle={(isOpen) => {
                activeMenu = isOpen ? 'quality' : 'none';
                scheduleHideControls();
              }}
              onselect={(opt) => setQuality(Number(opt.value))}
            >
              {#snippet icon()}
                <Settings size={14} strokeWidth={1.9} />
              {/snippet}
            </ComboBox>
          {/if}
        {/if}

        <button
          type="button"
          class="icon-btn"
          class:is-active={isPip && !isFullscreen}
          aria-label={isPip ? 'Exit picture in picture' : 'Picture in picture'}
          title={isPip ? 'Return to page player' : 'Mini-player (Picture in picture)'}
          onclick={() => void togglePiP()}
        >
          <PictureInPicture2 size={18} strokeWidth={1.8} />
        </button>

        <button
          type="button"
          class="icon-btn"
          aria-label={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}
          onclick={() => void toggleFullscreen()}
        >
          {#if isFullscreen}
            <Minimize size={18} strokeWidth={1.8} />
          {:else}
            <Maximize size={18} strokeWidth={1.8} />
          {/if}
        </button>
      </div>
    </div>
  </div>
</div>

<style>
  .hls-player {
    position: relative;
    width: 100%;
    height: 100%;
    aspect-ratio: 16 / 9;
    background: #070707;
    border-radius: var(--radius-card, 16px);
    overflow: hidden;
    outline: none;
    user-select: none;
    font-family: 'Medeo Sans', 'Medeo Sans Display', system-ui, sans-serif;
  }

  .hls-player.controls-hidden {
    cursor: none;
  }

  .hls-player:fullscreen,
  .hls-player.is-fullscreen {
    width: 100vw !important;
    height: 100vh !important;
    max-width: none !important;
    max-height: none !important;
    aspect-ratio: auto !important;
    border-radius: 0 !important;
  }

  .hls-player.is-fullscreen:not(:fullscreen) {
    position: fixed !important;
    inset: 0 !important;
    z-index: 99999 !important;
  }

  .hls-player.is-pip {
    border-radius: 18px !important;
    clip-path: inset(0 round 18px);
  }

  .hls-player.is-pip .top-bar {
    padding: 12px 14px 24px;
    pointer-events: auto;
    cursor: grab;
    --wails-draggable: drag;
  }

  .hls-player.is-pip .top-bar:active {
    cursor: grabbing;
  }

  .hls-player.is-pip .controls-dock,
  .hls-player.is-pip .center-play,
  .hls-player.is-pip .skip-pill,
  .hls-player.is-pip .top-bar-right,
  .hls-player.is-pip .pip-top-btn {
    --wails-draggable: no-drag;
  }

  .hls-player.is-pip .player-title {
    font-size: 13.5px;
  }

  .hls-player.is-pip .player-subtitle {
    font-size: 11.5px;
  }

  .hls-player.is-pip .center-play {
    transform: translate(-50%, -50%) scale(0.76);
  }

  .hls-player.is-pip .center-play:hover {
    transform: translate(-50%, -50%) scale(0.84);
  }

  .hls-player.is-pip .center-play:active {
    transform: translate(-50%, -50%) scale(0.7);
  }

  .hls-player.is-pip .controls-dock {
    padding: 14px 12px 8px;
    gap: 2px;
  }

  .hls-player.is-pip .skip-pill {
    left: 12px;
    bottom: 12px;
    height: 30px;
    padding: 0 11px;
    font-size: 11.5px;
  }

  .hls-player.is-pip .skip-pill.raised {
    bottom: 52px;
  }

  .hls-player.is-pip .volume-track {
    width: 44px;
  }

  .hls-player.is-pip .time-readout {
    margin-left: 2px;
    font-size: 11.5px;
  }

  .top-bar-right {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
    pointer-events: auto;
  }

  .pip-top-btn {
    width: 26px;
    height: 26px;
    border: none;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.1);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
    color: rgba(255, 255, 255, 0.88);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: background-color 0.16s ease, color 0.16s ease, transform 0.16s ease;
  }

  .pip-top-btn:hover {
    background: rgba(255, 255, 255, 0.22);
    color: #ffffff;
    transform: translateY(-1px);
  }

  .hls-video {
    width: 100%;
    height: 100%;
    display: block;
    object-fit: contain;
    background: #000000;
  }

  .top-bar {
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    padding: 20px 24px 36px;
    background: linear-gradient(180deg, rgba(7, 7, 7, 0.82) 0%, rgba(7, 7, 7, 0) 100%);
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.25s var(--ease-out, ease);
  }

  .top-bar.visible {
    opacity: 1;
  }

  .title-stack {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }

  .player-title {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 18px;
    font-weight: 700;
    color: #ffffff;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .player-subtitle {
    font-size: 13px;
    color: rgba(255, 255, 255, 0.65);
  }

  .quality-badge {
    flex-shrink: 0;
    padding: 0;
    background: transparent;
    color: rgba(255, 255, 255, 0.72);
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 12px;
    font-weight: 700;
    letter-spacing: 0.04em;
  }

  .center-overlay,
  .center-play {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
  }

  .center-play {
    padding: 12px;
    border: none;
    outline: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.94);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    filter: drop-shadow(0 6px 24px rgba(0, 0, 0, 0.72));
    transition: transform 0.22s var(--ease-out, cubic-bezier(0.22, 1, 0.36, 1)), color 0.18s ease, opacity 0.18s ease;
  }

  .center-play:hover {
    transform: translate(-50%, -50%) scale(1.12);
    color: #ffffff;
  }

  .center-play:active {
    transform: translate(-50%, -50%) scale(0.92);
  }

  .loader-ring {
    width: 42px;
    height: 42px;
    border-radius: 50%;
    border: 3px solid rgba(255, 255, 255, 0.18);
    border-top-color: #ffffff;
    animation: hls-spin 0.75s linear infinite;
  }

  @keyframes hls-spin {
    to {
      transform: rotate(360deg);
    }
  }

  .error-banner {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    z-index: 5;
    padding: 14px 18px;
    border-radius: 14px;
    background: rgba(20, 12, 14, 0.92);
    backdrop-filter: blur(14px);
    -webkit-backdrop-filter: blur(14px);
    border: none;
    color: #ffb4b4;
    font-size: 14px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    text-align: center;
    max-width: min(420px, 86%);
  }

  .error-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    justify-content: center;
  }

  .error-btn {
    height: 32px;
    padding: 0 12px;
    border: none;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 500;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    transition: background-color 0.18s ease, transform 0.16s ease;
  }

  .error-btn:hover {
    background: rgba(255, 255, 255, 0.22);
    transform: translateY(-1px);
  }

  .error-btn-primary {
    background: rgba(255, 255, 255, 0.2);
  }

  .skip-pill {
    position: absolute;
    left: 24px;
    bottom: 24px;
    z-index: 4;
    height: 36px;
    padding: 0 14px;
    border-radius: 999px;
    border: none;
    outline: none;
    background: rgba(14, 14, 16, 0.54);
    backdrop-filter: blur(14px);
    -webkit-backdrop-filter: blur(14px);
    color: rgba(255, 255, 255, 0.92);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 500;
    display: inline-flex;
    align-items: center;
    gap: 7px;
    cursor: pointer;
    transition:
      bottom 0.25s var(--ease-out, cubic-bezier(0.22, 1, 0.36, 1)),
      background-color 0.2s ease,
      color 0.2s ease,
      transform 0.2s var(--ease-out, cubic-bezier(0.22, 1, 0.36, 1));
  }

  .skip-pill.raised {
    bottom: 68px;
  }

  .skip-pill :global(.skip-arrow) {
    transition: transform 0.2s var(--ease-out, cubic-bezier(0.22, 1, 0.36, 1));
  }

  .skip-pill:hover {
    background: rgba(255, 255, 255, 0.18);
    color: #ffffff;
    transform: translateY(-1px);
  }

  .skip-pill:hover :global(.skip-arrow) {
    transform: translateX(2px);
  }

  .controls-dock {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    padding: 20px 20px 10px;
    background: linear-gradient(0deg, rgba(7, 7, 7, 0.88) 0%, rgba(7, 7, 7, 0) 100%);
    display: flex;
    flex-direction: column;
    gap: 4px;
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.25s var(--ease-out, ease);
  }

  .controls-dock.visible {
    opacity: 1;
    pointer-events: auto;
  }

  .seek-track {
    position: relative;
    width: 100%;
    height: 12px;
    display: flex;
    align-items: center;
    cursor: pointer;
    touch-action: none;
  }

  .seek-rail {
    position: relative;
    width: 100%;
    height: 3px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.18);
    overflow: hidden;
    transition: height 0.15s ease;
  }

  .seek-track:hover .seek-rail {
    height: 5px;
  }

  .seek-buffered {
    position: absolute;
    inset: 0 auto 0 0;
    background: rgba(255, 255, 255, 0.3);
  }

  .seek-fill {
    position: absolute;
    inset: 0 auto 0 0;
    background: #ffffff;
  }

  .seek-thumb {
    position: absolute;
    top: 50%;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: #ffffff;
    transform: translate(-50%, -50%) scale(0);
    transition: transform 0.15s ease;
    pointer-events: none;
  }

  .seek-track:hover .seek-thumb {
    transform: translate(-50%, -50%) scale(1);
  }

  .seek-tooltip {
    position: absolute;
    bottom: 22px;
    transform: translateX(-50%);
    padding: 4px 8px;
    border-radius: 6px;
    background: rgba(16, 16, 16, 0.9);
    color: #ffffff;
    font-size: 12px;
    pointer-events: none;
    white-space: nowrap;
  }

  .controls-row {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .controls-left,
  .controls-right {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .icon-btn {
    width: 30px;
    height: 30px;
    border: none;
    border-radius: 9px;
    background: transparent;
    color: rgba(255, 255, 255, 0.88);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: background-color 0.15s ease, color 0.15s ease;
  }

  .icon-btn:hover,
  .icon-btn.is-active {
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
  }

  .volume-group {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .volume-track {
    position: relative;
    width: 60px;
    height: 24px;
    margin-right: 4px;
    display: flex;
    align-items: center;
    cursor: pointer;
    touch-action: none;
    outline: none;
  }

  .volume-rail {
    position: relative;
    width: 100%;
    height: 3px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.22);
    overflow: hidden;
    transition: height 0.16s ease, background-color 0.16s ease;
  }

  .volume-track:hover .volume-rail,
  .volume-group.is-dragging .volume-rail {
    height: 4px;
    background: rgba(255, 255, 255, 0.3);
  }

  .volume-fill {
    position: absolute;
    inset: 0 auto 0 0;
    border-radius: 999px;
    background: #ffffff;
  }

  .volume-thumb {
    position: absolute;
    top: 50%;
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: #ffffff;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.45);
    transform: translate(-50%, -50%) scale(0);
    transition: transform 0.16s var(--ease-out, ease);
    pointer-events: none;
  }

  .volume-track:hover .volume-thumb,
  .volume-track:focus-visible .volume-thumb,
  .volume-group.is-dragging .volume-thumb {
    transform: translate(-50%, -50%) scale(1);
  }

  .time-readout {
    margin-left: 6px;
    font-size: 13px;
    color: #ffffff;
    font-variant-numeric: tabular-nums;
  }

  .time-sep {
    color: rgba(255, 255, 255, 0.45);
  }
</style>
