<script lang="ts">
  import { onDestroy, tick } from 'svelte';
  import { fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { MorphIcon } from 'morphicons/svelte';
  import {
    ArrowLeft,
    ArrowUp,
    ArrowDown,
    Check,
    Clock,
    Copy,
    Crown,
    EyeOff,
    HardDriveDownload,
    LayoutGrid,
    LayoutList,
    ListOrdered,
    LogOut,
    MessageSquare,
    PanelRight,
    PictureInPicture2,
    RefreshCw,
    Search,
    Send,
    X
  } from 'lucide';

  import MainButton from '../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../marlakit/button/SecondButton.svelte';
  import EpisodeThumb from '../anime/EpisodeThumb.svelte';
  import TorrentPopup from '../anime/TorrentPopup.svelte';
  import type { TorrentSearchItem } from '../../api/providers/torrent';
  import { episodeProgressMap, getEpisodeProgress } from '../../app/history.svelte';
  import { auth } from '../../app/auth.svelte';
  import { revealBlur } from '../../app/reveal';
  import { navigate } from '../../app/router.svelte';
  import { openSearch } from '../../app/search.svelte';
  import {
    applyRemotePlaybackSync,
    openTorrentStream,
    openWatch,
    player,
    retryWatch,
    selectEpisode,
    setInlinePlayerStage
  } from '../../app/player.svelte';
  import {
    leaveWatchRoom,
    sendWatchRoomChat,
    setWatchRoomMedia,
    syncWatchRoomPlayback,
    watchTogetherState
  } from '../../app/watchTogether.svelte';
  import { t } from '../../app/i18n.svelte';

  const EP_PAGE_SIZE = 16;
  const EP_VIEW_MODE_KEY = 'mila.anime.episodesViewMode';
  const WT_CHAT_WIDTH_KEY = 'mila.wt.chatWidth';
  const WT_CHAT_PINNED_KEY = 'mila.wt.chatPinned';

  type EpSortKey = 'order' | 'duration' | 'unwatched';

  function readSavedViewMode(key: string, fallback: 'grid' | 'list'): 'grid' | 'list' {
    if (typeof localStorage === 'undefined') return fallback;
    return localStorage.getItem(key) === 'list' ? 'list' : 'grid';
  }

  function readSavedChatWidth(): number {
    if (typeof localStorage === 'undefined') return 320;
    const raw = Number(localStorage.getItem(WT_CHAT_WIDTH_KEY));
    return Number.isFinite(raw) && raw >= 250 && raw <= 540 ? raw : 320;
  }

  function readSavedChatPinned(): boolean {
    if (typeof localStorage === 'undefined') return true;
    return localStorage.getItem(WT_CHAT_PINNED_KEY) !== '0';
  }

  let viewportW = $state(typeof window !== 'undefined' ? window.innerWidth : 1440);
  let playerSectionEl = $state<HTMLElement | null>(null);
  let inlineStageEl = $state<HTMLDivElement | null>(null);
  let chatFeedEl = $state<HTMLDivElement | null>(null);

  let chatOpen = $state(true);
  let chatPinned = $state(readSavedChatPinned());
  let chatWidthPx = $state(readSavedChatWidth());
  let resizingChat = $state(false);
  let resizeStartX = 0;
  let resizeStartW = 320;

  let chatDraft = $state('');
  let chatSending = $state(false);
  let lastSeenChatCount = $state(0);
  let codeCopied = $state(false);
  let brokenAvatars = $state<Record<string, boolean>>({});

  let epViewMode = $state<'grid' | 'list'>(readSavedViewMode(EP_VIEW_MODE_KEY, 'grid'));
  let epSortBy = $state<EpSortKey>('order');
  let epSortDir = $state<'asc' | 'desc'>('asc');
  let epRangeIndex = $state(0);

  let torrentsOpen = $state(false);
  let torrentsLoading = $state(false);
  let torrentsError = $state('');
  let torrentResults = $state<TorrentSearchItem[]>([]);
  let torrentResultsFor = $state('');
  let torrentReqToken = 0;

  const activeRoom = $derived(watchTogetherState.room);
  const myPublicId = $derived(auth.user?.public_id || '');
  const isHost = $derived(
    Boolean(activeRoom && myPublicId && activeRoom.host_public_id === myPublicId)
  );
  const canControlPlayback = $derived(
    !activeRoom || isHost || activeRoom.control_mode === 'everyone'
  );

  const currentShikiId = $derived(player.shikiArt?.id ?? player.target?.shikiId ?? 0);
  const displayTitle = $derived(
    player.shikiArt?.english ||
      player.shikiArt?.name ||
      player.release?.title ||
      player.title ||
      activeRoom?.media?.title ||
      'Anime'
  );
  const posterUrl = $derived(
    player.shikiArt?.posterOriginal ||
      player.shikiArt?.posterMain ||
      player.release?.poster ||
      activeRoom?.media?.poster ||
      ''
  );
  const backdropUrl = $derived(
    player.shikiArt?.screenshots[0] || player.release?.banner || posterUrl
  );
  const galleryShots = $derived((player.shikiArt?.screenshots ?? []).slice(0, 6));
  const episodes = $derived(player.release?.episodes ?? []);
  const currentEpisode = $derived(episodes[player.episodeIndex] ?? null);

  const chatMessages = $derived(activeRoom?.chat ?? []);
  const unreadCount = $derived(
    chatOpen ? 0 : Math.max(0, chatMessages.length - lastSeenChatCount)
  );

  $effect(() => {
    setInlinePlayerStage(inlineStageEl);
    return () => {
      setInlinePlayerStage(null);
    };
  });

  $effect(() => {
    const count = chatMessages.length;
    if (chatOpen) {
      lastSeenChatCount = count;
      void tick().then(() => {
        if (chatFeedEl) {
          chatFeedEl.scrollTop = chatFeedEl.scrollHeight;
        }
      });
    }
  });

  // Ensure media is loaded into player if user joined a room directly
  let lastSyncedMediaKey = '';
  $effect(() => {
    const rm = activeRoom;
    if (!rm || !rm.media?.title) return;
    const mediaKey = `${rm.room_id}:${rm.media.shiki_id ?? 0}:${rm.media.release_alias ?? ''}:${rm.media.title}:${rm.media.episode_ordinal || 1}`;
    if (mediaKey === lastSyncedMediaKey) return;
    lastSyncedMediaKey = mediaKey;

    const curTitle = (player.release?.title || player.title || '').trim().toLowerCase();
    const targetTitle = rm.media.title.trim().toLowerCase();
    const sameAnime =
      (rm.media.shiki_id && currentShikiId === rm.media.shiki_id) ||
      (rm.media.release_alias && player.release?.alias === rm.media.release_alias) ||
      (curTitle && curTitle === targetTitle);

    if (!sameAnime || !player.release) {
      void openWatch(
        {
          title: rm.media.title,
          shikiId: rm.media.shiki_id,
          releaseAlias: rm.media.release_alias,
          searchQueries: rm.media.search_queries ?? [rm.media.title]
        },
        {
          targetPage: 'watch-together-player',
          autoPlay: !rm.playback?.paused,
          episodeOrdinal: rm.media.episode_ordinal || 1,
          resumeTime: rm.playback?.position_sec || 0
        }
      );
    } else if (rm.media.episode_ordinal && currentEpisode?.ordinal !== rm.media.episode_ordinal) {
      const idx = episodes.findIndex((e) => e.ordinal === rm.media.episode_ordinal);
      if (idx >= 0 && idx !== player.episodeIndex) {
        selectEpisode(idx);
      }
    }
  });

  // Sync incoming room playback state (pause/resume/seek) from other members
  let lastAppliedPlaybackSeq = 0;
  let suppressOutgoingPlaySyncUntil = 0;

  $effect(() => {
    const rm = activeRoom;
    const pb = rm?.playback;
    if (!rm || !pb) return;
    if (pb.seq <= lastAppliedPlaybackSeq) return;
    lastAppliedPlaybackSeq = pb.seq;
    if (pb.updated_by && pb.updated_by === myPublicId) return;
    suppressOutgoingPlaySyncUntil = Date.now() + 900;
    applyRemotePlaybackSync(pb.paused, pb.position_sec);
  });

  // Broadcast local play/pause changes when user can control playback
  let prevLocalPlaying = player.isPlaying;
  $effect(() => {
    const nowPlaying = player.isPlaying;
    if (nowPlaying === prevLocalPlaying) return;
    prevLocalPlaying = nowPlaying;
    if (!activeRoom || !canControlPlayback) return;
    if (Date.now() < suppressOutgoingPlaySyncUntil) return;
    void syncWatchRoomPlayback({
      paused: !nowPlaying
    }).catch(() => {});
  });

  function toggleChatPinned() {
    chatPinned = !chatPinned;
    if (typeof localStorage !== 'undefined') {
      try {
        localStorage.setItem(WT_CHAT_PINNED_KEY, chatPinned ? '1' : '0');
      } catch {
        // ignore
      }
    }
  }

  function handleResizePointerDown(e: PointerEvent) {
    if (e.button !== 0) return;
    e.preventDefault();
    resizingChat = true;
    resizeStartX = e.clientX;
    resizeStartW = chatWidthPx;
    (e.currentTarget as HTMLElement)?.setPointerCapture(e.pointerId);
  }

  function handleResizePointerMove(e: PointerEvent) {
    if (!resizingChat) return;
    const delta = resizeStartX - e.clientX;
    const maxW = Math.min(560, Math.floor(viewportW * 0.46));
    const nextW = Math.max(250, Math.min(maxW, resizeStartW + delta));
    chatWidthPx = nextW;
  }

  function handleResizePointerUp(e: PointerEvent) {
    if (!resizingChat) return;
    resizingChat = false;
    try {
      (e.currentTarget as HTMLElement)?.releasePointerCapture(e.pointerId);
      localStorage.setItem(WT_CHAT_WIDTH_KEY, String(Math.round(chatWidthPx)));
    } catch {
      // ignore
    }
  }

  async function handleSendChat(e?: Event) {
    e?.preventDefault();
    const text = chatDraft.trim();
    if (!text || !activeRoom || chatSending) return;
    chatDraft = '';
    chatSending = true;
    try {
      await sendWatchRoomChat(text, 'chat');
      await tick();
      if (chatFeedEl) {
        chatFeedEl.scrollTop = chatFeedEl.scrollHeight;
      }
    } catch {
      chatDraft = text;
    } finally {
      chatSending = false;
    }
  }

  function formatChatTime(ms: number): string {
    if (!ms) return '';
    const d = new Date(ms);
    const hh = String(d.getHours()).padStart(2, '0');
    const mm = String(d.getMinutes()).padStart(2, '0');
    return `${hh}:${mm}`;
  }

  async function copyRoomCode() {
    if (!activeRoom?.room_code) return;
    try {
      await navigator.clipboard.writeText(activeRoom.room_code);
      codeCopied = true;
      window.setTimeout(() => {
        codeCopied = false;
      }, 1500);
    } catch {
      // ignore
    }
  }

  async function handleLeaveRoom() {
    await leaveWatchRoom();
    navigate('watch-together');
  }

  function gridCornerFlags(index: number, total: number, cols: number) {
    if (total <= 0) {
      return { tl: false, tr: false, bl: false, br: false };
    }
    const safeCols = Math.max(1, cols);
    const col = index % safeCols;
    const row = Math.floor(index / safeCols);
    const lastRow = Math.floor((total - 1) / safeCols);
    return {
      tl: index === 0,
      tr: index === Math.min(safeCols - 1, total - 1),
      bl: row === lastRow && col === 0,
      br: index === total - 1
    };
  }

  const epCols = $derived(viewportW <= 960 ? 2 : viewportW <= 1220 ? 3 : 4);

  const sortedEpisodes = $derived.by(() => {
    const dir = epSortDir === 'asc' ? 1 : -1;
    const list = episodes.map((ep, idx) => ({ ep, idx }));
    if (epSortBy === 'order' && epSortDir === 'asc') {
      return list;
    }
    if (epSortBy === 'unwatched') {
      episodeProgressMap.items;
      return [...list].sort((a, b) => {
        const wa = getEpProgressInfo(a.idx, a.ep.ordinal).watched ? 1 : 0;
        const wb = getEpProgressInfo(b.idx, b.ep.ordinal).watched ? 1 : 0;
        if (wa !== wb) return (wa - wb) * dir;
        return a.ep.ordinal - b.ep.ordinal;
      });
    }
    if (epSortBy === 'duration') {
      return [...list].sort((a, b) => {
        const da = a.ep.duration || 0;
        const db = b.ep.duration || 0;
        if (da !== db) return (da - db) * dir;
        return a.ep.ordinal - b.ep.ordinal;
      });
    }
    return [...list].sort((a, b) => (a.ep.ordinal - b.ep.ordinal) * dir);
  });

  const episodeRanges = $derived.by(() => {
    if (sortedEpisodes.length <= EP_PAGE_SIZE) return [];
    const ranges: Array<{ label: string; start: number; end: number }> = [];
    for (let start = 0; start < sortedEpisodes.length; start += EP_PAGE_SIZE) {
      const end = Math.min(start + EP_PAGE_SIZE, sortedEpisodes.length);
      if (epSortBy === 'order' && epSortDir === 'asc') {
        const firstOrd = sortedEpisodes[start]?.ep.ordinal ?? start + 1;
        const lastOrd = sortedEpisodes[end - 1]?.ep.ordinal ?? end;
        ranges.push({ label: `${firstOrd}–${lastOrd}`, start, end });
      } else {
        ranges.push({ label: `${start + 1}–${end}`, start, end });
      }
    }
    return ranges;
  });

  const visibleEpisodes = $derived.by(() => {
    if (episodeRanges.length === 0) {
      return sortedEpisodes;
    }
    const range = episodeRanges[epRangeIndex] ?? episodeRanges[0];
    return sortedEpisodes.slice(range.start, range.end);
  });

  const episodeFallbackImages = $derived.by(() => {
    const out: string[] = [];
    for (const url of [...galleryShots, backdropUrl, posterUrl]) {
      const clean = url?.trim();
      if (clean && !out.includes(clean)) {
        out.push(clean);
      }
    }
    return out;
  });

  $effect(() => {
    if (episodes.length > EP_PAGE_SIZE) {
      epRangeIndex = Math.floor(player.episodeIndex / EP_PAGE_SIZE);
    } else {
      epRangeIndex = 0;
    }
  });

  onDestroy(() => {
    torrentReqToken += 1;
    torrentResults = [];
  });

  function toggleEpSort(key: EpSortKey) {
    if (epSortBy === key) {
      epSortDir = epSortDir === 'asc' ? 'desc' : 'asc';
    } else {
      epSortBy = key;
      epSortDir = 'asc';
    }
    epRangeIndex = 0;
  }

  function setEpisodeViewMode(mode: 'grid' | 'list') {
    if (epViewMode === mode) return;
    epViewMode = mode;
    if (typeof localStorage === 'undefined') return;
    try {
      localStorage.setItem(EP_VIEW_MODE_KEY, mode);
    } catch {
      // ignore
    }
  }

  function getEpProgressInfo(epIdx: number, ordinal: number) {
    episodeProgressMap.items;
    const ep = episodes[epIdx];
    const entry = getEpisodeProgress(
      {
        shikiId: currentShikiId || undefined,
        releaseAlias: player.release?.alias ?? player.target?.releaseAlias,
        title: displayTitle
      },
      ordinal
    );
    if (!entry) return { ratio: 0, watched: false, currentTime: 0 };
    const dur = entry.duration > 0 ? entry.duration : ep?.duration ?? 0;
    const ratio = dur > 0 ? Math.min(1, Math.max(0, entry.currentTime / dur)) : 0;
    return {
      ratio: entry.watched && ratio < 0.85 ? 1 : ratio,
      watched: entry.watched || ratio >= 0.85,
      currentTime: entry.currentTime
    };
  }

  function scrollToPlayer() {
    playerSectionEl?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  async function handleChooseEpisode(index: number) {
    selectEpisode(index);
    scrollToPlayer();
    const ep = episodes[index];
    if (activeRoom && canControlPlayback && ep) {
      await setWatchRoomMedia({
        ...activeRoom.media,
        shiki_id: currentShikiId || activeRoom.media.shiki_id,
        release_alias: player.release?.alias || activeRoom.media.release_alias,
        title: displayTitle,
        poster: posterUrl,
        episode_ordinal: ep.ordinal,
        episode_index: index
      }).catch(() => {});
    }
  }

  async function toggleTorrentsDrawer() {
    torrentsOpen = !torrentsOpen;
    if (!torrentsOpen) {
      torrentReqToken += 1;
      torrentsLoading = false;
      return;
    }
    const key = `${player.shikiArt?.id ?? 0}:${displayTitle}`;
    if (torrentResultsFor === key && torrentResults.length > 0) return;

    const reqToken = ++torrentReqToken;
    torrentsLoading = true;
    torrentsError = '';
    try {
      const q = (player.shikiArt?.english || player.shikiArt?.name || displayTitle || '').trim();
      const libId = player.release?.provider === 'aniliberty' ? player.release.id : 0;
      const { searchTorrents } = await import('../../api/providers/torrent');
      const items = await searchTorrents(q, libId);
      if (reqToken !== torrentReqToken) return;
      torrentResults = items;
      torrentResultsFor = key;
    } catch (err) {
      if (reqToken !== torrentReqToken) return;
      torrentsError = err instanceof Error ? err.message : 'Failed to search torrents';
    } finally {
      if (reqToken === torrentReqToken) {
        torrentsLoading = false;
      }
    }
  }

  async function handleLaunchTorrent(item: TorrentSearchItem | string) {
    torrentsOpen = false;
    await openTorrentStream(item);
    scrollToPlayer();
  }
</script>

<svelte:window bind:innerWidth={viewportW} />

<div class="wt-player-page">
  <section
    class="wt-player-section"
    bind:this={playerSectionEl}
    aria-label="Watch Together video theater"
  >
    <!-- data-player-fs-root ensures fullscreen includes BOTH the video player and the split-screen / floating chat -->
    <div
      class="wt-theater-frame"
      class:is-split-chat={chatOpen && chatPinned}
      data-player-fs-root
    >
      <div class="wt-video-stage">
        <div class="wt-top-drag" aria-hidden="true"></div>

        <!-- Floating top-right chat toggle button when chat is closed (or floating toggle) -->
        {#if !chatOpen}
          <button
            type="button"
            class="wt-chat-open-fab"
            aria-label="Open room chat"
            title="Open room chat"
            onclick={() => (chatOpen = true)}
            in:revealBlur={{ duration: 200, scale: 0.94, blur: 6 }}
          >
            <MorphIcon icon={MessageSquare} size={15} strokeWidth={1.9} />
            <span>Chat</span>
            {#if unreadCount > 0}
              <span class="wt-chat-unread">{unreadCount}</span>
            {/if}
          </button>
        {/if}

        {#if player.loading}
          <div class="stage-state">
            <div class="spinner"></div>
            <span>{t('anime.loadingRelease')}</span>
          </div>
        {:else if player.error}
          <div class="stage-state">
            <p class="error-text">{player.error}</p>
            <MainButton onclick={retryWatch}>
              {#snippet icon()}
                <MorphIcon icon={RefreshCw} size={15} strokeWidth={2} />
              {/snippet}
              {t('anime.tryAgain')}
            </MainButton>
          </div>
        {:else if !currentEpisode}
          <div class="stage-state">
            <span>{t('anime.noPlayableEpisodes')}</span>
          </div>
        {/if}

        <div
          bind:this={inlineStageEl}
          class="player-inline-slot"
          class:is-hidden={!currentEpisode || player.loading || Boolean(player.error) || player.pipMode}
        ></div>

        <!-- Floating mode chat (inside video stage so it works in fullscreen too) -->
        {#if chatOpen && !chatPinned}
          <aside
            class="wt-chat-panel is-floating"
            style:width="{chatWidthPx}px"
            in:revealBlur={{ duration: 220, y: 6, scale: 0.97, blur: 8 }}
            out:revealBlur={{ duration: 160, y: 4, scale: 0.97, blur: 6 }}
          >
            <!-- Left resize edge for floating chat -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="wt-float-resizer"
              onpointerdown={handleResizePointerDown}
              onpointermove={handleResizePointerMove}
              onpointerup={handleResizePointerUp}
              onpointercancel={handleResizePointerUp}
            ></div>

            <div class="wt-chat-head">
              <span class="wt-chat-title">{activeRoom?.title || 'Room'} chat</span>

              <div class="wt-chat-head-right">
                <div class="wt-chat-avatars" aria-label="Room members">
                  {#each (activeRoom?.members ?? []).slice(0, 5) as m, idx (m.public_id)}
                    <div
                      class="wt-chat-av-circle"
                      style:z-index={10 - idx}
                      title="@{m.username}"
                    >
                      {#if m.avatar_url && !brokenAvatars[m.public_id]}
                        <img
                          src={m.avatar_url}
                          alt={m.username}
                          referrerpolicy="no-referrer"
                          onerror={() => {
                            brokenAvatars = { ...brokenAvatars, [m.public_id]: true };
                          }}
                        />
                      {:else}
                        <span>{(m.name || m.username || 'U').slice(0, 1).toUpperCase()}</span>
                      {/if}
                    </div>
                  {/each}
                </div>

                <button
                  type="button"
                  class="wt-chat-icon-btn"
                  title="Dock chat to right split-screen"
                  aria-label="Dock chat to right split-screen"
                  onclick={toggleChatPinned}
                >
                  <MorphIcon icon={PanelRight} size={14} strokeWidth={1.8} />
                </button>
                <button
                  type="button"
                  class="wt-chat-icon-btn"
                  title="Close chat"
                  aria-label="Close chat"
                  onclick={() => (chatOpen = false)}
                >
                  <MorphIcon icon={X} size={15} strokeWidth={1.8} />
                </button>
              </div>
            </div>

            <div class="wt-chat-feed" bind:this={chatFeedEl}>
              {#if chatMessages.length === 0}
                <div class="wt-chat-empty">Say something to the room…</div>
              {:else}
                {#each chatMessages as msg (msg.id)}
                  {#if msg.kind === 'system'}
                    <div class="wt-msg-system">{msg.text}</div>
                  {:else}
                    {@const isMine = msg.sender_public_id === myPublicId}
                    <div class="wt-msg-item" class:is-mine={isMine}>
                      {#if isMine}
                        <div class="wt-bubble-mine">
                          <span class="wt-bubble-text">{msg.text}</span>
                        </div>
                      {:else}
                        <div class="wt-bubble-other">
                          <div class="wt-bubble-av" title="@{msg.sender_username}">
                            {#if msg.avatar_url && !brokenAvatars[`msg-${msg.id}`]}
                              <img
                                src={msg.avatar_url}
                                alt={msg.sender_username}
                                referrerpolicy="no-referrer"
                                onerror={() => {
                                  brokenAvatars = { ...brokenAvatars, [`msg-${msg.id}`]: true };
                                }}
                              />
                            {:else}
                              <span>{(msg.sender_name || msg.sender_username || 'U').slice(0, 1).toUpperCase()}</span>
                            {/if}
                          </div>
                          <span class="wt-bubble-text">{msg.text}</span>
                        </div>
                      {/if}
                      <span class="wt-msg-time">{formatChatTime(msg.created_at_ms)}</span>
                    </div>
                  {/if}
                {/each}
              {/if}
            </div>

            <form class="wt-chat-form" onsubmit={handleSendChat}>
              <input
                type="text"
                class="wt-chat-input"
                bind:value={chatDraft}
                placeholder="Message…"
                maxlength={400}
              />
              <button
                type="submit"
                class="wt-chat-send"
                disabled={!chatDraft.trim() || chatSending}
                aria-label="Send message"
              >
                <MorphIcon icon={Send} size={13} strokeWidth={2} />
              </button>
            </form>
          </aside>
        {/if}
      </div>

      <!-- Static Split-Screen Resizable Chat on the Right (Android split-screen style) -->
      {#if chatOpen && chatPinned}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="wt-split-resizer"
          class:is-dragging={resizingChat}
          title="Drag to resize chat"
          onpointerdown={handleResizePointerDown}
          onpointermove={handleResizePointerMove}
          onpointerup={handleResizePointerUp}
          onpointercancel={handleResizePointerUp}
        >
          <span class="wt-resizer-grip"></span>
        </div>

        <aside
          class="wt-chat-panel is-split"
          style:width="{chatWidthPx}px"
          in:revealBlur={{ duration: 220, x: 10, blur: 8 }}
          out:revealBlur={{ duration: 160, x: 10, blur: 6 }}
        >
          <div class="wt-chat-head">
            <span class="wt-chat-title">{activeRoom?.title || 'Room'} chat</span>

            <div class="wt-chat-head-right">
              <div class="wt-chat-avatars" aria-label="Room members">
                {#each (activeRoom?.members ?? []).slice(0, 5) as m, idx (m.public_id)}
                  <div
                    class="wt-chat-av-circle"
                    style:z-index={10 - idx}
                    title="@{m.username}"
                  >
                    {#if m.avatar_url && !brokenAvatars[m.public_id]}
                      <img
                        src={m.avatar_url}
                        alt={m.username}
                        referrerpolicy="no-referrer"
                        onerror={() => {
                          brokenAvatars = { ...brokenAvatars, [m.public_id]: true };
                        }}
                      />
                    {:else}
                      <span>{(m.name || m.username || 'U').slice(0, 1).toUpperCase()}</span>
                    {/if}
                  </div>
                {/each}
              </div>

              <button
                type="button"
                class="wt-chat-icon-btn"
                title="Undock to floating chat"
                aria-label="Undock to floating chat"
                onclick={toggleChatPinned}
              >
                <MorphIcon icon={PictureInPicture2} size={14} strokeWidth={1.8} />
              </button>
              <button
                type="button"
                class="wt-chat-icon-btn"
                title="Close chat"
                aria-label="Close chat"
                onclick={() => (chatOpen = false)}
              >
                <MorphIcon icon={X} size={15} strokeWidth={1.8} />
              </button>
            </div>
          </div>

          <div class="wt-chat-feed" bind:this={chatFeedEl}>
            {#if chatMessages.length === 0}
              <div class="wt-chat-empty">Say something to the room…</div>
            {:else}
              {#each chatMessages as msg (msg.id)}
                {#if msg.kind === 'system'}
                  <div class="wt-msg-system">{msg.text}</div>
                {:else}
                  {@const isMine = msg.sender_public_id === myPublicId}
                  <div class="wt-msg-item" class:is-mine={isMine}>
                    {#if isMine}
                      <div class="wt-bubble-mine">
                        <span class="wt-bubble-text">{msg.text}</span>
                      </div>
                    {:else}
                      <div class="wt-bubble-other">
                        <div class="wt-bubble-av" title="@{msg.sender_username}">
                          {#if msg.avatar_url && !brokenAvatars[`msg-${msg.id}`]}
                            <img
                              src={msg.avatar_url}
                              alt={msg.sender_username}
                              referrerpolicy="no-referrer"
                              onerror={() => {
                                brokenAvatars = { ...brokenAvatars, [`msg-${msg.id}`]: true };
                              }}
                            />
                          {:else}
                            <span>{(msg.sender_name || msg.sender_username || 'U').slice(0, 1).toUpperCase()}</span>
                          {/if}
                        </div>
                        <span class="wt-bubble-text">{msg.text}</span>
                      </div>
                    {/if}
                    <span class="wt-msg-time">{formatChatTime(msg.created_at_ms)}</span>
                  </div>
                {/if}
              {/each}
            {/if}
          </div>

          <form class="wt-chat-form" onsubmit={handleSendChat}>
            <input
              type="text"
              class="wt-chat-input"
              bind:value={chatDraft}
              placeholder="Message…"
              maxlength={400}
            />
            <button
              type="submit"
              class="wt-chat-send"
              disabled={!chatDraft.trim() || chatSending}
              aria-label="Send message"
            >
              <MorphIcon icon={Send} size={13} strokeWidth={2} />
            </button>
          </form>
        </aside>
      {/if}
    </div>
  </section>

  <!-- Below-player Watch Together Room Bar & Episode Browser -->
  <div class="wt-player-body">
    <div class="wt-room-strip">
      <div class="wt-room-strip-left">
        <button
          type="button"
          class="episodes-back-btn"
          aria-label="Back to room lobby"
          title="Back to room overview"
          onclick={() => navigate('watch-together')}
        >
          <MorphIcon icon={ArrowLeft} size={19} strokeWidth={2} />
        </button>

        <div class="wt-room-titles">
          <div class="wt-room-heading-line">
            <h1 class="wt-room-main-title">{activeRoom?.title || displayTitle}</h1>
            {#if activeRoom?.room_code}
              <button
                type="button"
                class="wt-code-inline"
                title="Copy room code"
                onclick={copyRoomCode}
              >
                <span>#{activeRoom.room_code}</span>
                {#if codeCopied}
                  <MorphIcon icon={Check} size={11} strokeWidth={2.3} />
                {:else}
                  <MorphIcon icon={Copy} size={11} strokeWidth={1.8} />
                {/if}
              </button>
            {/if}
            <span class="wt-seat-inline">
              {activeRoom?.members.length ?? 1}/{activeRoom?.max_members ?? 7}
            </span>
          </div>

          <div class="wt-room-sub-line">
            <span class="wt-anime-sub">{displayTitle}</span>
            {#if currentEpisode}
              <span class="wt-dot">·</span>
              <span class="wt-ep-sub">Episode {currentEpisode.ordinal}</span>
            {/if}
            {#if activeRoom}
              <span class="wt-dot">·</span>
              <span class="wt-mode-sub">
                {activeRoom.control_mode === 'everyone' ? 'Everyone controls' : 'Host only'}
              </span>
            {/if}
          </div>
        </div>
      </div>

      <div class="wt-room-strip-right">
        <button
          type="button"
          class="wt-strip-action"
          onclick={() => (chatOpen = !chatOpen)}
        >
          <MorphIcon icon={MessageSquare} size={14} strokeWidth={1.9} />
          <span>{chatOpen ? 'Hide chat' : 'Open chat'}</span>
        </button>

        {#if canControlPlayback}
          <button
            type="button"
            class="wt-strip-action"
            onclick={() => openSearch()}
          >
            <MorphIcon icon={Search} size={14} strokeWidth={1.9} />
            <span>Change anime</span>
          </button>
        {/if}

        {#if activeRoom}
          <button
            type="button"
            class="wt-strip-action is-danger"
            onclick={() => void handleLeaveRoom()}
          >
            <MorphIcon icon={LogOut} size={14} strokeWidth={1.9} />
            <span>Leave room</span>
          </button>
        {/if}
      </div>
    </div>

    {#if episodes.length > 0}
      <section class="episodes-section" aria-label={t('anime.episodes')}>
        <div class="section-head">
          <div class="section-head-left">
            <h2 class="section-heading">{t('anime.episodes')}</h2>
            <span class="section-count">{episodes.length}</span>
          </div>

          <div class="range-tabs">
            {#if episodeRanges.length > 1}
              {#each episodeRanges as r, idx (r.label)}
                <button
                  type="button"
                  class="range-pill"
                  class:is-active={idx === epRangeIndex}
                  onclick={() => {
                    epRangeIndex = idx;
                  }}
                >
                  {r.label}
                </button>
              {/each}
            {/if}

            <button
              type="button"
              class="range-pill torrent-toggle-pill"
              class:is-active={torrentsOpen}
              onclick={() => void toggleTorrentsDrawer()}
            >
              <MorphIcon icon={HardDriveDownload} size={14} strokeWidth={1.9} />
              <span>{t('anime.torrents1080p')}</span>
            </button>

            <div class="lib-sort-group" role="group" aria-label="Sort episodes">
              <button
                type="button"
                class="sort-chip-btn"
                class:active={epSortBy === 'order'}
                title="Episode number"
                onclick={() => toggleEpSort('order')}
              >
                <MorphIcon icon={ListOrdered} size={14} strokeWidth={1.7} />
                {#if epSortBy === 'order'}
                  <MorphIcon
                    icon={epSortDir === 'asc' ? ArrowUp : ArrowDown}
                    size={11}
                    strokeWidth={2.1}
                    spring="snappy"
                  />
                {/if}
              </button>

              <button
                type="button"
                class="sort-chip-btn"
                class:active={epSortBy === 'unwatched'}
                title="Unwatched first"
                onclick={() => toggleEpSort('unwatched')}
              >
                <MorphIcon icon={EyeOff} size={14} strokeWidth={1.7} />
                {#if epSortBy === 'unwatched'}
                  <MorphIcon
                    icon={epSortDir === 'asc' ? ArrowUp : ArrowDown}
                    size={11}
                    strokeWidth={2.1}
                    spring="snappy"
                  />
                {/if}
              </button>

              <button
                type="button"
                class="sort-chip-btn"
                class:active={epSortBy === 'duration'}
                title="Duration"
                onclick={() => toggleEpSort('duration')}
              >
                <MorphIcon icon={Clock} size={14} strokeWidth={1.7} />
                {#if epSortBy === 'duration'}
                  <MorphIcon
                    icon={epSortDir === 'asc' ? ArrowUp : ArrowDown}
                    size={11}
                    strokeWidth={2.1}
                    spring="snappy"
                  />
                {/if}
              </button>
            </div>

            <div
              class="ep-view-modes"
              class:is-grid={epViewMode === 'grid'}
              role="group"
              aria-label="Episode view mode"
            >
              <div class="ep-view-pill" aria-hidden="true"></div>
              <button
                type="button"
                class="ep-view-btn"
                class:active={epViewMode === 'list'}
                aria-label="List view"
                aria-pressed={epViewMode === 'list'}
                onclick={() => setEpisodeViewMode('list')}
              >
                <MorphIcon icon={LayoutList} size={17} strokeWidth={1.7} />
              </button>
              <button
                type="button"
                class="ep-view-btn"
                class:active={epViewMode === 'grid'}
                aria-label="Grid view"
                aria-pressed={epViewMode === 'grid'}
                onclick={() => setEpisodeViewMode('grid')}
              >
                <MorphIcon icon={LayoutGrid} size={17} strokeWidth={1.7} />
              </button>
            </div>
          </div>
        </div>

        {#if epViewMode === 'grid'}
          <div class="episodes-grid" in:fade={{ duration: 180, easing: cubicOut }}>
            {#each visibleEpisodes as entry, i (entry.ep.id)}
              {@const epProg = getEpProgressInfo(entry.idx, entry.ep.ordinal)}
              {@const corners = gridCornerFlags(i, visibleEpisodes.length, epCols)}
              <EpisodeThumb
                episode={entry.ep}
                index={entry.idx}
                variant="grid"
                isCurrent={entry.idx === player.episodeIndex}
                isWatched={epProg.watched}
                progressRatio={epProg.ratio}
                cornerTL={corners.tl}
                cornerTR={corners.tr}
                cornerBL={corners.bl}
                cornerBR={corners.br}
                fallbackImages={episodeFallbackImages}
                onchoose={() => void handleChooseEpisode(entry.idx)}
              />
            {/each}
          </div>
        {:else}
          <div class="episodes-list" in:fade={{ duration: 180, easing: cubicOut }}>
            {#each visibleEpisodes as entry (entry.ep.id)}
              {@const epProg = getEpProgressInfo(entry.idx, entry.ep.ordinal)}
              <EpisodeThumb
                episode={entry.ep}
                index={entry.idx}
                variant="list"
                isCurrent={entry.idx === player.episodeIndex}
                isWatched={epProg.watched}
                progressRatio={epProg.ratio}
                cornerTL={true}
                cornerBL={true}
                fallbackImages={episodeFallbackImages}
                onchoose={() => void handleChooseEpisode(entry.idx)}
              />
            {/each}
          </div>
        {/if}
      </section>
    {/if}
  </div>
</div>

<TorrentPopup
  open={torrentsOpen}
  title={displayTitle}
  poster={posterUrl}
  loading={torrentsLoading}
  error={torrentsError}
  results={torrentResults}
  onclose={() => {
    torrentsOpen = false;
  }}
  onselect={(item) => void handleLaunchTorrent(item)}
/>

<style>
  .wt-player-page {
    width: 100%;
    min-height: 100%;
    padding: 0;
    margin: 0;
    background: var(--mila-app-canvas, #0c0c0c);
    color: var(--mila-text-primary, #ffffff);
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
  }

  .wt-player-section {
    position: relative;
    width: 100%;
    padding: 0;
    margin: 0;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
  }

  .wt-theater-frame {
    position: relative;
    width: 100%;
    height: min(82vh, 56.25vw);
    min-height: 420px;
    background: #070709;
    display: flex;
    align-items: stretch;
    overflow: hidden;
  }

  /* When fullscreen is triggered on HlsPlayer inside data-player-fs-root, the entire theater (video + chat) fills 100vw x 100vh! */
  .wt-theater-frame:fullscreen,
  :global(.wt-theater-frame.is-fs-active) {
    position: fixed !important;
    inset: 0 !important;
    width: 100vw !important;
    height: 100vh !important;
    max-height: none !important;
    z-index: 99999 !important;
    background: #070709 !important;
  }

  .wt-video-stage {
    position: relative;
    flex: 1 1 0;
    min-width: 0;
    height: 100%;
    background: #070709;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
  }

  .wt-video-stage :global(.hls-player) {
    border-radius: 0;
    width: 100%;
    height: 100%;
    aspect-ratio: auto;
  }

  .wt-top-drag {
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 22px;
    z-index: 12;
    --wails-draggable: drag;
  }

  :global(body.is-player-fullscreen) .wt-top-drag {
    opacity: 0;
    pointer-events: none;
  }

  .wt-chat-open-fab {
    position: absolute;
    top: 16px;
    right: 16px;
    z-index: 30;
    height: 32px;
    padding: 0 12px;
    border-radius: 999px;
    border: none;
    background: rgba(16, 16, 19, 0.78);
    backdrop-filter: blur(18px);
    -webkit-backdrop-filter: blur(18px);
    color: rgba(255, 255, 255, 0.88);
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 11.5px;
    font-weight: 700;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    --wails-draggable: no-drag;
    transition:
      background 0.16s ease,
      color 0.16s ease;
  }

  .wt-chat-open-fab:hover {
    background: rgba(24, 24, 28, 0.92);
    color: #ffffff;
  }

  .wt-chat-unread {
    min-width: 16px;
    height: 16px;
    padding: 0 5px;
    border-radius: 999px;
    background: #ffffff;
    color: #0c0c0e;
    font-size: 10px;
    font-weight: 800;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  /* Split-screen draggable divider handle */
  .wt-split-resizer {
    position: relative;
    width: 10px;
    margin: 0 -4px;
    z-index: 25;
    cursor: col-resize;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    user-select: none;
    touch-action: none;
  }

  .wt-resizer-grip {
    width: 3px;
    height: 38px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.12);
    transition:
      background 0.16s ease,
      height 0.16s ease;
  }

  .wt-split-resizer:hover .wt-resizer-grip,
  .wt-split-resizer.is-dragging .wt-resizer-grip {
    height: 56px;
    background: rgba(255, 255, 255, 0.42);
  }

  /* Chat Panel (Split-Screen or Floating) */
  .wt-chat-panel {
    display: flex;
    flex-direction: column;
    background: #0c0c0e;
    color: #ffffff;
    box-sizing: border-box;
    flex-shrink: 0;
    user-select: none;
    --wails-draggable: no-drag;
  }

  .wt-chat-panel.is-split {
    height: 100%;
    padding: 10px 10px 10px 8px;
  }

  .wt-chat-panel.is-floating {
    position: absolute;
    top: 14px;
    right: 14px;
    bottom: 66px;
    z-index: 35;
    padding: 10px;
    border-radius: 16px;
    background: rgba(14, 14, 17, 0.86);
    backdrop-filter: blur(26px) saturate(140%);
    -webkit-backdrop-filter: blur(26px) saturate(140%);
    box-shadow: 0 22px 56px rgba(0, 0, 0, 0.68);
  }

  .wt-float-resizer {
    position: absolute;
    top: 12px;
    bottom: 12px;
    left: -4px;
    width: 8px;
    cursor: col-resize;
    touch-action: none;
  }

  .wt-chat-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 2px 4px 8px;
    flex-shrink: 0;
  }

  .wt-chat-title {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 800;
    letter-spacing: -0.015em;
    color: rgba(255, 255, 255, 0.9);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .wt-chat-head-right {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }

  /* Overlapping member avatars like media_1795039071518_3232e598.png */
  .wt-chat-avatars {
    display: inline-flex;
    align-items: center;
    margin-right: 2px;
  }

  .wt-chat-av-circle {
    width: 24px;
    height: 24px;
    border-radius: 50%;
    overflow: hidden;
    background: #1a1a1e;
    border: 2px solid #0c0c0e;
    margin-left: -7px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 9.5px;
    font-weight: 700;
    color: #ffffff;
    box-sizing: border-box;
  }

  .wt-chat-av-circle:first-child {
    margin-left: 0;
  }

  .wt-chat-av-circle img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .wt-chat-icon-btn {
    width: 24px;
    height: 24px;
    padding: 0;
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.42);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: color 0.16s ease;
  }

  .wt-chat-icon-btn:hover {
    color: #ffffff;
  }

  /* Chat Feed & Bubbles matching media_1795039284977_ca390929.png */
  .wt-chat-feed {
    flex: 1 1 0;
    min-height: 0;
    overflow-y: auto;
    padding: 4px 2px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    scrollbar-width: none;
  }

  .wt-chat-feed::-webkit-scrollbar {
    display: none;
  }

  .wt-chat-empty {
    margin: auto 0;
    text-align: center;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12px;
    color: rgba(255, 255, 255, 0.28);
  }

  .wt-msg-system {
    align-self: center;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 10.5px;
    color: rgba(255, 255, 255, 0.28);
    padding: 2px 6px;
  }

  .wt-msg-item {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    max-width: 88%;
  }

  .wt-msg-item.is-mine {
    align-self: flex-end;
    align-items: flex-end;
  }

  /* Incoming message: dark pill with nested avatar on the left */
  .wt-bubble-other {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 4px 12px 4px 4px;
    border-radius: 18px;
    background: #1a1a1d;
    color: rgba(255, 255, 255, 0.92);
    max-width: 100%;
    box-sizing: border-box;
  }

  .wt-bubble-av {
    width: 24px;
    height: 24px;
    border-radius: 50%;
    overflow: hidden;
    background: #27272c;
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 10px;
    font-weight: 700;
    color: #ffffff;
  }

  .wt-bubble-av img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  /* Own message: clean white pill with dark text */
  .wt-bubble-mine {
    display: inline-flex;
    align-items: center;
    padding: 6px 13px;
    border-radius: 18px;
    background: #f2f2f5;
    color: #0c0c0e;
    max-width: 100%;
    box-sizing: border-box;
  }

  .wt-bubble-text {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 500;
    line-height: 1.35;
    word-break: break-word;
    user-select: text;
  }

  .wt-msg-time {
    align-self: flex-end;
    padding-right: 4px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 9.5px;
    color: rgba(255, 255, 255, 0.28);
    line-height: 1;
  }

  /* Compact chat input bar */
  .wt-chat-form {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 32px;
    padding: 0 4px 0 11px;
    margin-top: 6px;
    border-radius: 999px;
    background: #161619;
    flex-shrink: 0;
  }

  .wt-chat-input {
    flex: 1 1 0;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: #ffffff;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12px;
  }

  .wt-chat-input::placeholder {
    color: rgba(255, 255, 255, 0.3);
  }

  .wt-chat-send {
    width: 24px;
    height: 24px;
    padding: 0;
    border: none;
    border-radius: 50%;
    background: transparent;
    color: rgba(255, 255, 255, 0.72);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition:
      color 0.16s ease,
      opacity 0.16s ease;
  }

  .wt-chat-send:disabled {
    opacity: 0.25;
    cursor: default;
  }

  .wt-chat-send:hover:not(:disabled) {
    color: #ffffff;
  }

  /* Below-theater Room Strip & Episodes */
  .wt-player-body {
    padding: clamp(14px, 1.8vw, 20px) var(--page-pad-x) var(--page-pad-x) var(--page-pad-x);
    display: flex;
    flex-direction: column;
    gap: clamp(20px, 2.4vw, 28px);
  }

  .wt-room-strip {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    flex-wrap: wrap;
  }

  .wt-room-strip-left {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }

  .wt-room-titles {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .wt-room-heading-line {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }

  .wt-room-main-title {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(18px, 1.9vw, 22px);
    font-weight: 800;
    letter-spacing: -0.02em;
    color: #ffffff;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .wt-code-inline {
    padding: 0;
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.36);
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 12px;
    font-weight: 700;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    cursor: pointer;
    transition: color 0.16s ease;
  }

  .wt-code-inline:hover {
    color: rgba(255, 255, 255, 0.82);
  }

  .wt-seat-inline {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 700;
    color: rgba(255, 255, 255, 0.34);
  }

  .wt-room-sub-line {
    display: flex;
    align-items: center;
    gap: 6px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
    color: rgba(255, 255, 255, 0.46);
  }

  .wt-anime-sub {
    color: rgba(255, 255, 255, 0.76);
    font-weight: 600;
  }

  .wt-dot {
    opacity: 0.45;
  }

  .wt-room-strip-right {
    display: flex;
    align-items: center;
    gap: 14px;
  }

  .wt-strip-action {
    padding: 0;
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.56);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 600;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    transition: color 0.16s ease;
  }

  .wt-strip-action:hover {
    color: #ffffff;
  }

  .wt-strip-action.is-danger {
    color: rgba(255, 122, 122, 0.78);
  }

  .wt-strip-action.is-danger:hover {
    color: #ff7a7a;
  }

  .episodes-back-btn {
    width: 30px;
    height: 30px;
    padding: 0;
    margin: 0 2px 0 -4px;
    border: none;
    outline: none;
    border-radius: 8px;
    background: transparent;
    color: var(--mila-text-muted, rgba(255, 255, 255, 0.48));
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition:
      color 0.2s var(--ease-out),
      transform 0.2s var(--ease-out);
  }

  .episodes-back-btn:hover {
    color: var(--mila-text-primary, #ffffff);
    transform: translateX(-2px);
  }

  .player-inline-slot {
    width: 100%;
    height: 100%;
  }

  .player-inline-slot.is-hidden {
    display: none;
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
    font-size: clamp(20px, 2vw, 24px);
    font-weight: 900;
    color: var(--mila-text-heading, #e4e4e7);
    letter-spacing: -0.01em;
  }

  .section-count {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 14px;
    color: var(--mila-text-muted, rgba(255, 255, 255, 0.42));
  }

  .range-tabs {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }

  .range-pill {
    height: 36px;
    padding: 0 13px;
    border: none;
    outline: none;
    border-radius: 11px;
    background: var(--mila-app-surfaceGlass, rgba(255, 255, 255, 0.04));
    color: var(--mila-text-secondary, rgba(255, 255, 255, 0.72));
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: background-color 0.18s var(--ease-out), color 0.18s var(--ease-out);
  }

  .range-pill:hover {
    background: var(--mila-app-surfaceHover, rgba(255, 255, 255, 0.12));
    color: var(--mila-text-primary, #ffffff);
  }

  .range-pill.is-active {
    background: var(--mila-app-surfaceActive, rgba(255, 255, 255, 0.16));
    color: var(--mila-accent-highlight, #ffffff);
    font-weight: 500;
  }

  .torrent-toggle-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .lib-sort-group {
    display: inline-flex;
    align-items: center;
    gap: 0;
    height: 34px;
    padding: 0;
    border-radius: 10px;
    overflow: hidden;
    background: var(--mila-app-surfaceGlass, rgba(255, 255, 255, 0.04));
    box-sizing: border-box;
  }

  .sort-chip-btn {
    height: 34px;
    min-width: 34px;
    padding: 0 9px;
    border: none;
    outline: none;
    border-radius: 10px;
    background: transparent;
    color: var(--mila-text-muted, rgba(255, 255, 255, 0.44));
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 3px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 500;
    cursor: pointer;
    transition: background-color 0.2s var(--ease-out), color 0.2s var(--ease-out);
    white-space: nowrap;
  }

  .sort-chip-btn:hover {
    color: var(--mila-text-secondary, rgba(255, 255, 255, 0.8));
  }

  .sort-chip-btn.active {
    background: var(--mila-app-surfaceHover, rgba(255, 255, 255, 0.1));
    color: var(--mila-accent-highlight, #ffffff);
  }

  .ep-view-modes {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 0;
    height: 34px;
    padding: 0;
    border-radius: 10px;
    overflow: hidden;
    background: var(--mila-app-surfaceGlass, rgba(255, 255, 255, 0.04));
    box-sizing: border-box;
  }

  .ep-view-pill {
    position: absolute;
    top: 0;
    left: 0;
    width: 34px;
    height: 34px;
    border-radius: 10px;
    background: var(--mila-app-surfaceHover, rgba(255, 255, 255, 0.1));
    pointer-events: none;
    transition: transform 0.32s var(--ease-out);
  }

  .ep-view-modes.is-grid .ep-view-pill {
    transform: translate3d(34px, 0, 0);
  }

  .ep-view-btn {
    position: relative;
    z-index: 1;
    width: 34px;
    height: 34px;
    padding: 0;
    border: none;
    outline: none;
    border-radius: 10px;
    background: transparent;
    color: var(--mila-text-dim, rgba(255, 255, 255, 0.42));
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: color 0.2s var(--ease-out);
  }

  .ep-view-btn:hover {
    color: var(--mila-text-secondary, rgba(255, 255, 255, 0.8));
  }

  .ep-view-btn.active {
    color: var(--mila-accent-highlight, #ffffff);
  }

  .episodes-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 0;
  }

  @media (max-width: 1220px) {
    .episodes-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (max-width: 960px) {
    .episodes-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  .episodes-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
</style>
