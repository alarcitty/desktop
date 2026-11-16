import { cloudApi } from '../api/cloud';
import {
  episodeProgressMap,
  mergeCloudHistoryState,
  subscribeHistoryMutations,
  watchHistory,
  type EpisodeProgressEntry,
  type WatchHistoryEntry
} from './history.svelte';
import { registerBeforeCloseFlush } from './lifecycle';
import {
  localLibrary,
  mergeCloudLibraryItems,
  subscribeLibraryMutations,
  type LibraryItem
} from './library.svelte';

const HISTORY_CLOUD_DEBOUNCE_MS = 2500;

let syncEnabled = false;
let syncPromise: Promise<void> | null = null;
let listenersAttached = false;

let pendingHistoryEntry: WatchHistoryEntry | null = null;
let pendingEpisodeProgress: Record<string, EpisodeProgressEntry> = {};
let historySyncTimer: ReturnType<typeof globalThis.setTimeout> | undefined;

function flushPendingHistorySync(): void {
  globalThis.clearTimeout(historySyncTimer);
  historySyncTimer = undefined;
  if (!syncEnabled || !pendingHistoryEntry) {
    pendingHistoryEntry = null;
    pendingEpisodeProgress = {};
    return;
  }
  const entry = pendingHistoryEntry;
  const episodeProgress = pendingEpisodeProgress;
  pendingHistoryEntry = null;
  pendingEpisodeProgress = {};

  void cloudApi
    .put('/api/v1/user/history', {
      entry,
      episode_progress: Object.keys(episodeProgress).length > 0 ? episodeProgress : undefined
    })
    .catch(() => {
      // Offline / transient network errors stay safely in localStorage until next sync.
    });
}

function scheduleHistoryCloudUpsert(
  entry: WatchHistoryEntry,
  episodeProgress?: Record<string, EpisodeProgressEntry>
): void {
  pendingHistoryEntry = entry;
  if (episodeProgress) {
    pendingEpisodeProgress = { ...pendingEpisodeProgress, ...episodeProgress };
  }
  globalThis.clearTimeout(historySyncTimer);
  historySyncTimer = globalThis.setTimeout(flushPendingHistorySync, HISTORY_CLOUD_DEBOUNCE_MS);
}

function ensureMutationListeners(): void {
  if (listenersAttached) return;
  listenersAttached = true;

  subscribeLibraryMutations(({ type, item }) => {
    if (!syncEnabled) return;
    if (type === 'delete') {
      void cloudApi
        .del(`/api/v1/user/library?id=${encodeURIComponent(item.id)}`)
        .catch(() => {});
      return;
    }
    void cloudApi.put('/api/v1/user/library', { item }).catch(() => {});
  });

  subscribeHistoryMutations((event) => {
    if (!syncEnabled) return;
    if (event.type === 'clear') {
      globalThis.clearTimeout(historySyncTimer);
      historySyncTimer = undefined;
      pendingHistoryEntry = null;
      pendingEpisodeProgress = {};
      void cloudApi.del('/api/v1/user/history').catch(() => {});
      return;
    }
    if (event.type === 'delete') {
      if (pendingHistoryEntry?.id === event.id) {
        pendingHistoryEntry = null;
      }
      void cloudApi
        .del(`/api/v1/user/history?id=${encodeURIComponent(event.id)}`)
        .catch(() => {});
      return;
    }
    scheduleHistoryCloudUpsert(event.entry, event.episodeProgress);
  });

  if (typeof window !== 'undefined') {
    registerBeforeCloseFlush(flushPendingHistorySync);
    window.addEventListener('beforeunload', flushPendingHistorySync);
    window.addEventListener('pagehide', flushPendingHistorySync);
  }
}

export async function startCloudUserSync(): Promise<void> {
  ensureMutationListeners();
  syncEnabled = true;
  if (syncPromise) return syncPromise;

  syncPromise = (async () => {
    try {
      const [remoteLib, remoteHist] = await Promise.all([
        cloudApi.get<{ items?: LibraryItem[] }>('/api/v1/user/library'),
        cloudApi.get<{
          entries?: WatchHistoryEntry[];
          episode_progress?: Record<string, EpisodeProgressEntry>;
        }>('/api/v1/user/history')
      ]);

      const remoteItems = Array.isArray(remoteLib?.items) ? remoteLib.items : [];
      const remoteEntries = Array.isArray(remoteHist?.entries) ? remoteHist.entries : [];
      const remoteProgress =
        remoteHist?.episode_progress && typeof remoteHist.episode_progress === 'object'
          ? remoteHist.episode_progress
          : {};

      const mergedLib = mergeCloudLibraryItems(remoteItems);
      const mergedHist = mergeCloudHistoryState(remoteEntries, remoteProgress);

      const remoteLibMap = new Map(remoteItems.map((it) => [it.id, it.updatedAt ?? 0]));
      const itemsToPush = mergedLib.filter(
        (it) => !remoteLibMap.has(it.id) || (it.updatedAt ?? 0) > (remoteLibMap.get(it.id) ?? 0)
      );
      if (itemsToPush.length > 0) {
        await cloudApi.put('/api/v1/user/library', { items: itemsToPush });
      }

      const remoteHistMap = new Map(remoteEntries.map((e) => [e.id, e.updatedAt ?? 0]));
      const entriesToPush = mergedHist.entries.filter(
        (e) => !remoteHistMap.has(e.id) || (e.updatedAt ?? 0) > (remoteHistMap.get(e.id) ?? 0)
      );
      const progressToPush: Record<string, EpisodeProgressEntry> = {};
      for (const [k, v] of Object.entries(episodeProgressMap.items)) {
        const rp = remoteProgress[k];
        if (!rp || (v.updatedAt ?? 0) > (rp.updatedAt ?? 0)) {
          progressToPush[k] = v;
        }
      }
      if (entriesToPush.length > 0 || Object.keys(progressToPush).length > 0) {
        await cloudApi.put('/api/v1/user/history', {
          entries: entriesToPush,
          episode_progress: Object.keys(progressToPush).length > 0 ? progressToPush : undefined
        });
      }
    } catch {
      // Offline / temporary failure: keep local state responsive without blocking auth.
    } finally {
      syncPromise = null;
    }
  })();

  return syncPromise;
}

export function stopCloudUserSync(): void {
  syncEnabled = false;
  globalThis.clearTimeout(historySyncTimer);
  historySyncTimer = undefined;
  pendingHistoryEntry = null;
  pendingEpisodeProgress = {};
}
