import { loadHomeFeeds } from '../api/anime';
import { router } from './router.svelte';

const IMAGE_PRELOAD_TIMEOUT_MS = 3200;
const HERO_VIDEO_TIMEOUT_MS = 4500;
const HARD_BOOT_TIMEOUT_MS = 8000;

export const bootState = $state<{ ready: boolean }>({
  ready: false
});

let bootStarted = false;
let heroMediaResolved = false;
const heroMediaWaiters: Array<() => void> = [];

export function markHeroMediaReady(): void {
  if (heroMediaResolved) return;
  heroMediaResolved = true;
  while (heroMediaWaiters.length > 0) {
    heroMediaWaiters.shift()?.();
  }
}

function waitForHeroMedia(timeoutMs: number): Promise<void> {
  if (heroMediaResolved || router.current !== 'home') {
    return Promise.resolve();
  }
  return new Promise((resolve) => {
    let settled = false;
    const done = () => {
      if (settled) return;
      settled = true;
      window.clearTimeout(timer);
      resolve();
    };
    const timer = window.setTimeout(done, timeoutMs);
    heroMediaWaiters.push(done);
  });
}

function preloadImage(url: string): Promise<void> {
  if (!url || typeof Image === 'undefined') return Promise.resolve();
  return new Promise((resolve) => {
    const img = new Image();
    img.decoding = 'async';
    img.onload = () => resolve();
    img.onerror = () => resolve();
    img.src = url;
  });
}

function preloadCriticalImages(urls: string[], timeoutMs: number): Promise<void> {
  const unique = Array.from(new Set(urls.filter(Boolean)));
  if (unique.length === 0) return Promise.resolve();
  return Promise.race([
    Promise.allSettled(unique.map(preloadImage)).then(() => undefined),
    new Promise<void>((resolve) => window.setTimeout(resolve, timeoutMs))
  ]);
}

function waitForFonts(): Promise<void> {
  if (typeof document === 'undefined' || !document.fonts?.ready) {
    return Promise.resolve();
  }
  return Promise.race([
    document.fonts.ready.then(() => undefined).catch(() => undefined),
    new Promise<void>((resolve) => window.setTimeout(resolve, 1500))
  ]);
}

export function initBootGate(): void {
  if (bootStarted || bootState.ready) return;
  bootStarted = true;

  const hardFailsafe = window.setTimeout(() => {
    bootState.ready = true;
  }, HARD_BOOT_TIMEOUT_MS);

  void (async () => {
    try {
      const fontsPromise = waitForFonts();
      const feeds = await loadHomeFeeds().catch(() => null);

      const criticalUrls: string[] = [];
      if (feeds) {
        const firstHero = feeds.spotlight[0];
        if (firstHero) {
          if (firstHero.bg) criticalUrls.push(firstHero.bg);
          if (firstHero.poster) criticalUrls.push(firstHero.poster);
        }
        for (const s of feeds.spotlight.slice(1, 5)) {
          if (s.poster) criticalUrls.push(s.poster);
        }
        for (const r of feeds.releases.slice(0, 4)) {
          if (r.poster) criticalUrls.push(r.poster);
        }
        for (const rec of feeds.recommendations.slice(0, 5)) {
          if (rec.poster) criticalUrls.push(rec.poster);
        }
      }

      await Promise.all([
        fontsPromise,
        preloadCriticalImages(criticalUrls, IMAGE_PRELOAD_TIMEOUT_MS),
        waitForHeroMedia(HERO_VIDEO_TIMEOUT_MS)
      ]);
    } finally {
      window.clearTimeout(hardFailsafe);
      bootState.ready = true;
    }
  })();
}
