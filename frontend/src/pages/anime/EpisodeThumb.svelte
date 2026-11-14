<script lang="ts">
  import { onDestroy } from 'svelte';
  import Hls from 'hls.js';
  import Play from '@lucide/svelte/icons/play';
  import type { PlayableEpisode } from '../../api/anime';

  interface Props {
    episode: PlayableEpisode;
    index: number;
    isCurrent?: boolean;
    fallbackImages?: string[];
    onchoose?: () => void;
  }

  let {
    episode,
    index,
    isCurrent = false,
    fallbackImages = [],
    onchoose
  }: Props = $props();

  let hoverTimer: number | undefined;
  let hoverActive = $state(false);
  let videoReady = $state(false);
  let videoEl = $state<HTMLVideoElement | null>(null);
  let hlsInstance: Hls | null = null;
  let capturedFrameUrl = $state<string | null>(null);
  let failedUrls = $state<Record<string, boolean>>({});
  let excerptStart = $state(0);

  function formatDuration(sec: number): string {
    if (!sec || sec <= 0) return '';
    const mins = Math.round(sec / 60);
    return mins > 0 ? `${mins}m` : `${sec}s`;
  }

  const coverUrl = $derived.by(() => {
    const candidates: string[] = [];
    if (capturedFrameUrl) candidates.push(capturedFrameUrl);
    if (episode.preview?.trim()) candidates.push(episode.preview.trim());

    if (fallbackImages.length > 0) {
      const len = fallbackImages.length;
      for (let offset = 0; offset < len; offset++) {
        const u = fallbackImages[(index + offset) % len];
        if (u && !candidates.includes(u)) {
          candidates.push(u);
        }
      }
    }

    for (const url of candidates) {
      if (!failedUrls[url]) return url;
    }
    return '';
  });

  const previewStream = $derived.by(() => {
    const hlsUrl = episode.hls_480 || episode.hls_720 || episode.hls_1080;
    if (hlsUrl?.trim()) return { kind: 'hls' as const, url: hlsUrl.trim() };
    const mp4Url = episode.mp4_480 || episode.mp4_720 || episode.mp4_1080;
    if (mp4Url?.trim()) return { kind: 'mp4' as const, url: mp4Url.trim() };
    return null;
  });

  function computeExcerptStart(totalSec: number): number {
    const base =
      episode.opening?.stop != null
        ? Math.round(episode.opening.stop + 45)
        : episode.duration > 180
          ? Math.round(episode.duration * 0.25)
          : 130;
    if (totalSec > 20) {
      return Math.max(0, Math.min(base, Math.floor(totalSec - 12)));
    }
    return Math.max(0, base);
  }

  function isSameOriginUrl(rawUrl: string): boolean {
    if (typeof window === 'undefined') return false;
    try {
      const parsed = new URL(rawUrl, window.location.href);
      return parsed.origin === window.location.origin;
    } catch {
      return false;
    }
  }

  function tryCaptureVideoFrame(video: HTMLVideoElement, streamUrl: string) {
    if (capturedFrameUrl || episode.preview || !isSameOriginUrl(streamUrl)) return;
    if (!video.videoWidth || !video.videoHeight) return;
    try {
      const canvas = document.createElement('canvas');
      canvas.width = 480;
      canvas.height = 270;
      const ctx = canvas.getContext('2d');
      if (!ctx) return;
      ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
      const dataUrl = canvas.toDataURL('image/jpeg', 0.82);
      if (dataUrl.startsWith('data:image/')) {
        capturedFrameUrl = dataUrl;
      }
    } catch {
      // Ignore tainted canvas errors on third-party media CDNs
    }
  }

  function cleanupPreview() {
    window.clearTimeout(hoverTimer);
    hoverTimer = undefined;
    videoReady = false;
    if (hlsInstance) {
      hlsInstance.destroy();
      hlsInstance = null;
    }
    if (videoEl) {
      videoEl.pause();
      videoEl.removeAttribute('src');
      videoEl.load();
    }
  }

  function handleMouseEnter() {
    if (typeof window === 'undefined' || !previewStream) return;
    if (window.matchMedia && !window.matchMedia('(hover: hover)').matches) return;
    window.clearTimeout(hoverTimer);
    hoverTimer = window.setTimeout(() => {
      hoverActive = true;
    }, 180);
  }

  function handleMouseLeave() {
    window.clearTimeout(hoverTimer);
    hoverTimer = undefined;
    hoverActive = false;
    cleanupPreview();
  }

  $effect(() => {
    const active = hoverActive;
    const video = videoEl;
    const stream = previewStream;
    const epId = episode.id;

    void epId;
    if (!active || !video || !stream) {
      return;
    }

    let cancelled = false;
    const startSec = computeExcerptStart(episode.duration);
    excerptStart = startSec;
    let initialSeekApplied = false;

    const applyInitialSeek = () => {
      if (cancelled || initialSeekApplied) return;
      initialSeekApplied = true;
      const clamped = computeExcerptStart(Number.isFinite(video.duration) ? video.duration : episode.duration);
      excerptStart = clamped;
      if (clamped > 1 && Math.abs(video.currentTime - clamped) > 3) {
        try {
          video.currentTime = clamped;
        } catch {
          // Seek deferred
        }
      }
      void video.play().catch(() => {});
    };

    const handlePlaying = () => {
      if (cancelled) return;
      videoReady = true;
      tryCaptureVideoFrame(video, stream.url);
    };

    const handleTimeUpdate = () => {
      if (cancelled) return;
      if (video.currentTime >= excerptStart + 10) {
        try {
          video.currentTime = excerptStart;
        } catch {
          // Ignore seek failures during teardown
        }
      }
    };

    video.muted = true;
    video.playsInline = true;
    video.addEventListener('loadedmetadata', applyInitialSeek);
    video.addEventListener('playing', handlePlaying);
    video.addEventListener('canplay', handlePlaying);
    video.addEventListener('timeupdate', handleTimeUpdate);

    if (stream.kind === 'hls' && Hls.isSupported()) {
      const hls = new Hls({
        enableWorker: true,
        lowLatencyMode: false,
        capLevelToPlayerSize: true,
        startPosition: startSec,
        maxBufferLength: 10,
        maxMaxBufferLength: 16,
        fragLoadingMaxRetry: 1,
        manifestLoadingMaxRetry: 1
      });
      hlsInstance = hls;
      hls.loadSource(stream.url);
      hls.attachMedia(video);
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        if (!cancelled) {
          void video.play().catch(() => {});
        }
      });
    } else {
      video.src = stream.url;
      video.load();
    }

    return () => {
      cancelled = true;
      video.removeEventListener('loadedmetadata', applyInitialSeek);
      video.removeEventListener('playing', handlePlaying);
      video.removeEventListener('canplay', handlePlaying);
      video.removeEventListener('timeupdate', handleTimeUpdate);
      cleanupPreview();
    };
  });

  onDestroy(() => {
    hoverActive = false;
    cleanupPreview();
  });
</script>

<button
  type="button"
  class="ep-card"
  class:is-active={isCurrent}
  class:is-previewing={hoverActive && videoReady}
  onmouseenter={handleMouseEnter}
  onmouseleave={handleMouseLeave}
  onclick={() => onchoose?.()}
>
  <div class="ep-thumb-wrap">
    {#if coverUrl}
      <img
        src={coverUrl}
        alt="Episode {episode.ordinal}"
        class="ep-thumb"
        loading="lazy"
        decoding="async"
        onerror={() => {
          failedUrls = { ...failedUrls, [coverUrl]: true };
        }}
      />
    {:else}
      <div class="ep-thumb-fallback" aria-hidden="true">
        <Play size={20} fill="currentColor" strokeWidth={0} />
      </div>
    {/if}

    <!-- svelte-ignore a11y_media_has_caption -->
    <video
      bind:this={videoEl}
      class="ep-preview-video"
      class:is-visible={hoverActive && videoReady}
      muted
      playsinline
      preload="none"
      aria-hidden="true"
    ></video>

    <div class="ep-thumb-fade" aria-hidden="true"></div>
    <div class="ep-overlay-bottom">
      <div class="ep-title-row">
        {#if episode.name}
          <span class="ep-num">EP {episode.ordinal}</span>
          <span class="ep-name">{episode.name}</span>
        {:else}
          <span class="ep-num">Episode {episode.ordinal}</span>
        {/if}
      </div>
      {#if episode.duration > 0}
        <span class="ep-duration">{formatDuration(episode.duration)}</span>
      {/if}
    </div>
  </div>
</button>

<style>
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

  .ep-preview-video {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.34s var(--ease-out);
    z-index: 1;
  }

  .ep-preview-video.is-visible {
    opacity: 1;
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
</style>
