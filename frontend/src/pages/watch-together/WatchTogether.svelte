<script lang="ts">
  import { onMount } from 'svelte';
  import { MorphIcon } from 'morphicons/svelte';
  import {
    Check,
    Clock,
    Copy,
    Crown,
    Globe,
    Lock,
    LogOut,
    Pencil,
    Play,
    Shield,
    Star,
    User,
    UserPlus,
    Users,
    X
  } from 'lucide';

  import LayerBlur from '../../marlakit/blur/LayerBlur.svelte';
  import ComboBox from '../../marlakit/combo/ComboBox.svelte';
  import type { ComboOption } from '../../marlakit/combo/types';
  import { portal } from '../../marlakit/portal';
  import { getCachedArtwork } from '../../api/anime';
  import type { FriendProfile } from '../../api/friends';
  import type { RoomControlMode, RoomVisibility } from '../../api/watchTogether';
  import { auth, loginWithGoogle } from '../../app/auth.svelte';
  import { friendsState, openFriendsPopup, refreshFriends } from '../../app/friends.svelte';
  import { watchHistory } from '../../app/history.svelte';
  import { openWatch, player } from '../../app/player.svelte';
  import { revealBlur } from '../../app/reveal';
  import { navigate } from '../../app/router.svelte';
  import { openSearch } from '../../app/search.svelte';
  import {
    cancelWatchRoomInvite,
    connectWatchTogether,
    createWatchRoom,
    inviteFriendToWatchRoom,
    joinFriendWatchRoom,
    kickWatchRoomMember,
    leaveWatchRoom,
    markWatchRoomStarted,
    selectAnimeForWatchTogether,
    transferWatchRoomHost,
    updateWatchRoomSettings,
    watchTogetherState
  } from '../../app/watchTogether.svelte';

  let editingTitle = $state(false);
  let titleInput = $state('');
  let codeCopied = $state(false);
  let busy = $state(false);
  let brokenAvatars = $state<Record<string, boolean>>({});

  let visibilityMenuOpen = $state(false);
  let controlMenuOpen = $state(false);
  let seatsMenuOpen = $state(false);

  interface ContextMenuState {
    kind: 'friend' | 'slot-invited' | 'slot-member';
    x: number;
    y: number;
    id: string;
    name: string;
    username: string;
    statusLabel: string;
    statusKind: 'online' | 'in_room' | 'offline' | 'pending';
    canJoinRoom: boolean;
    alreadyInvited: boolean;
  }

  let ctxMenu = $state<ContextMenuState | null>(null);

  const activeRoom = $derived(watchTogetherState.room);
  const myPublicId = $derived(auth.user?.public_id || '');
  const isHost = $derived(
    Boolean(activeRoom && myPublicId && activeRoom.host_public_id === myPublicId)
  );

  // Selected anime: room media > explicit draftAnime > currently loaded player > most recent watchHistory
  const selectedAnime = $derived.by(() => {
    if (activeRoom && activeRoom.media?.title) {
      const shikiId = activeRoom.media.shiki_id;
      const art = shikiId ? getCachedArtwork(shikiId) : undefined;
      const draftMatch =
        watchTogetherState.draftAnime &&
        (watchTogetherState.draftAnime.shikiId === shikiId ||
          watchTogetherState.draftAnime.title === activeRoom.media.title)
          ? watchTogetherState.draftAnime
          : null;
      const poster =
        activeRoom.media.poster ||
        draftMatch?.poster ||
        art?.posterOriginal ||
        art?.posterMain ||
        '';
      const backdrop =
        draftMatch?.backdrop ||
        art?.screenshots?.[0] ||
        poster;
      const description =
        draftMatch?.description ||
        art?.description ||
        '';
      return {
        title: activeRoom.media.title,
        description,
        poster,
        backdrop,
        shikiId,
        releaseAlias: activeRoom.media.release_alias || draftMatch?.releaseAlias,
        score: draftMatch?.score ?? art?.score,
        tags: [
          art?.year ? String(art.year) : '',
          art?.episodes ? `${art.episodes} ep` : ''
        ].filter(Boolean),
        episodeLabel: `EP ${activeRoom.media.episode_ordinal || 1}`,
        searchQueries: activeRoom.media.search_queries ?? [activeRoom.media.title]
      };
    }

    if (watchTogetherState.draftAnime) {
      const d = watchTogetherState.draftAnime;
      const art = d.shikiId ? getCachedArtwork(d.shikiId) : undefined;
      return {
        ...d,
        description: d.description || art?.description || '',
        backdrop: d.backdrop || art?.screenshots?.[0] || d.poster,
        score: d.score ?? art?.score,
        tags:
          d.tags.length > 0
            ? d.tags
            : [
                art?.year ? String(art.year) : '',
                art?.episodes ? `${art.episodes} ep` : ''
              ].filter(Boolean),
        episodeLabel: ''
      };
    }

    if (player.release || player.shikiArt) {
      const title =
        player.shikiArt?.english ||
        player.shikiArt?.name ||
        player.release?.title ||
        player.title;
      if (title) {
        const poster =
          player.shikiArt?.posterOriginal ||
          player.shikiArt?.posterMain ||
          player.release?.poster ||
          '';
        const backdrop =
          player.shikiArt?.screenshots?.[0] ||
          player.release?.banner ||
          poster;
        const description =
          player.shikiArt?.description ||
          '';
        return {
          title,
          description,
          poster,
          backdrop,
          shikiId: player.shikiArt?.id ?? player.target?.shikiId,
          releaseAlias: player.release?.alias ?? player.target?.releaseAlias,
          score: player.shikiArt?.score,
          tags: [
            player.shikiArt?.year || player.release?.year
              ? String(player.shikiArt?.year || player.release?.year)
              : '',
            player.release?.episodes?.length ? `${player.release.episodes.length} ep` : ''
          ].filter(Boolean),
          episodeLabel: '',
          searchQueries: player.target?.searchQueries ?? [title]
        };
      }
    }

    const recent = watchHistory.entries[0];
    if (recent) {
      const art = recent.shikiId ? getCachedArtwork(recent.shikiId) : undefined;
      const poster = art?.posterOriginal || art?.posterMain || recent.poster || '';
      const backdrop = art?.screenshots?.[0] || poster;
      return {
        title: recent.title,
        description: art?.description || '',
        poster,
        backdrop,
        shikiId: recent.shikiId,
        releaseAlias: recent.releaseAlias,
        score: art?.score,
        tags: [
          art?.year ? String(art.year) : '',
          recent.totalEpisodes ? `${recent.totalEpisodes} ep` : ''
        ].filter(Boolean),
        episodeLabel: `Ep ${recent.episodeOrdinal}`,
        searchQueries: recent.searchQueries ?? [recent.title]
      };
    }

    return null;
  });

  const displayRoomName = $derived.by(() => {
    if (activeRoom) {
      return activeRoom.title || 'Name';
    }
    return watchTogetherState.draftTitle.trim() || 'смотрелка';
  });

  const currentVisibility = $derived<RoomVisibility>(
    activeRoom ? activeRoom.visibility : watchTogetherState.draftVisibility
  );
  const currentControlMode = $derived<RoomControlMode>(
    activeRoom ? activeRoom.control_mode : watchTogetherState.draftControlMode
  );
  const currentMaxMembers = $derived<number>(
    activeRoom ? activeRoom.max_members : watchTogetherState.draftMaxMembers
  );

  const visibilityOptions: ComboOption[] = [
    { value: 'public', label: 'Public', sublabel: 'Anyone can join' },
    { value: 'friends', label: 'Friends', sublabel: 'Friends can join' },
    { value: 'private', label: 'Private', sublabel: 'Invite only' }
  ];

  const controlOptions: ComboOption[] = [
    { value: 'host_only', label: 'Host only', sublabel: 'Only host controls playback' },
    { value: 'everyone', label: 'Everyone controls', sublabel: 'All members can pause & seek' }
  ];

  const seatsOptions: ComboOption[] = [2, 3, 4, 5, 6, 7, 8, 9, 10].map((n) => ({
    value: n,
    label: `${n} seats`
  }));

  // Pending invited friends (combines server room.pending + local draftInvitedIds)
  const pendingFriends = $derived.by<FriendProfile[]>(() => {
    const memberIds = new Set((activeRoom?.members ?? []).map((m) => m.public_id));
    const map = new Map<string, FriendProfile>();
    for (const p of activeRoom?.pending ?? []) {
      if (p.public_id && !memberIds.has(p.public_id)) {
        map.set(p.public_id, p);
      }
    }
    for (const id of watchTogetherState.draftInvitedIds) {
      if (!memberIds.has(id) && !map.has(id)) {
        const found = friendsState.friends.find((f) => f.public_id === id);
        if (found) {
          map.set(id, found);
        }
      }
    }
    return Array.from(map.values());
  });

  interface SlotCircle {
    key: string;
    kind: 'host' | 'member' | 'invited' | 'empty';
    id?: string;
    name: string;
    username: string;
    avatarUrl: string;
  }

  const slotCircles = $derived.by<SlotCircle[]>(() => {
    const max = Math.max(2, Math.min(10, currentMaxMembers));
    const out: SlotCircle[] = [];

    if (activeRoom) {
      for (const m of activeRoom.members) {
        out.push({
          key: `mem-${m.public_id}`,
          kind: m.role === 'host' ? 'host' : 'member',
          id: m.public_id,
          name: m.name,
          username: m.username,
          avatarUrl: m.avatar_url
        });
      }
    } else if (auth.user) {
      out.push({
        key: `host-${auth.user.public_id || 'me'}`,
        kind: 'host',
        id: auth.user.public_id,
        name: auth.user.name,
        username: auth.user.username || 'me',
        avatarUrl: auth.user.avatar_url
      });
    }

    for (const f of pendingFriends) {
      if (out.length >= max) break;
      out.push({
        key: `inv-${f.public_id}`,
        kind: 'invited',
        id: f.public_id,
        name: f.name,
        username: f.username,
        avatarUrl: f.avatar_url
      });
    }

    while (out.length < max) {
      out.push({
        key: `empty-${out.length}`,
        kind: 'empty',
        name: 'Empty seat',
        username: '',
        avatarUrl: ''
      });
    }

    return out.slice(0, max);
  });

  const occupiedCount = $derived(
    activeRoom
      ? activeRoom.members.length + pendingFriends.length
      : 1 + pendingFriends.length
  );

  // Friends sorted: in_room first, online second, offline last
  const sortedFriends = $derived.by(() => {
    const list = [...friendsState.friends];
    const rank = (s: FriendProfile['status']) =>
      s === 'in_room' ? 0 : s === 'online' ? 1 : 2;
    list.sort((a, b) => rank(a.status) - rank(b.status));
    return list;
  });

  onMount(() => {
    if (auth.user) {
      void connectWatchTogether();
      void refreshFriends();
    }
  });

  $effect(() => {
    if (auth.user && !watchTogetherState.connected && !watchTogetherState.connecting) {
      void connectWatchTogether();
      void refreshFriends();
    }
  });

  function beginEditTitle() {
    if (activeRoom && !isHost) return;
    titleInput = activeRoom ? activeRoom.title : watchTogetherState.draftTitle || 'смотрелка';
    editingTitle = true;
  }

  async function saveTitle(e?: Event) {
    e?.preventDefault();
    const clean = titleInput.trim();
    editingTitle = false;
    if (!clean) return;
    if (activeRoom && isHost) {
      await updateWatchRoomSettings({ title: clean });
    } else {
      watchTogetherState.draftTitle = clean;
    }
  }

  async function selectVisibility(next: RoomVisibility) {
    if (activeRoom && isHost) {
      await updateWatchRoomSettings({ visibility: next });
    } else if (!activeRoom) {
      watchTogetherState.draftVisibility = next;
    }
  }

  async function selectControlMode(next: RoomControlMode) {
    if (activeRoom && isHost) {
      await updateWatchRoomSettings({ control_mode: next });
    } else if (!activeRoom) {
      watchTogetherState.draftControlMode = next;
    }
  }

  async function selectMaxMembers(clamped: number) {
    if (activeRoom && isHost) {
      const safe = Math.max(activeRoom.members.length, clamped);
      await updateWatchRoomSettings({ max_members: safe });
    } else if (!activeRoom) {
      watchTogetherState.draftMaxMembers = clamped;
    }
  }

  async function handleStartOrContinueRoom() {
    if (!auth.user) {
      await loginWithGoogle();
      return;
    }
    if (busy) return;
    busy = true;
    try {
      const anime = selectedAnime;
      if (!activeRoom) {
        await createWatchRoom(
          {
            title:
              watchTogetherState.draftTitle.trim() ||
              anime?.title ||
              `${auth.user.username || auth.user.name}'s room`,
            visibility: watchTogetherState.draftVisibility,
            control_mode: watchTogetherState.draftControlMode,
            max_members: watchTogetherState.draftMaxMembers,
            media: anime
              ? {
                  shiki_id: anime.shikiId,
                  release_alias: anime.releaseAlias,
                  title: anime.title,
                  poster: anime.poster,
                  episode_ordinal: 1,
                  episode_index: 0,
                  search_queries: anime.searchQueries
                }
              : undefined
          },
          true
        );
      } else {
        markWatchRoomStarted();
      }

      if (watchTogetherState.draftInvitedIds.length > 0) {
        const toInvite = [...watchTogetherState.draftInvitedIds];
        watchTogetherState.draftInvitedIds = [];
        for (const friendId of toInvite) {
          void inviteFriendToWatchRoom(friendId).catch(() => {});
        }
      }

      if (anime) {
        void selectAnimeForWatchTogether(anime);
        void openWatch(
          {
            title: anime.title,
            shikiId: anime.shikiId,
            releaseAlias: anime.releaseAlias,
            searchQueries: anime.searchQueries
          },
          {
            targetPage: 'watch-together-player',
            autoPlay: true,
            episodeOrdinal: activeRoom?.media?.episode_ordinal || 1,
            resumeTime: activeRoom?.playback?.position_sec || 0
          }
        );
      } else {
        navigate('watch-together-player');
      }
    } finally {
      busy = false;
    }
  }

  function openFriendContextMenu(e: MouseEvent, friend: FriendProfile) {
    e.preventDefault();
    e.stopPropagation();
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const isAlreadyInvited = pendingFriends.some((p) => p.public_id === friend.public_id);
    const canJoin = Boolean(
      friend.status === 'in_room' &&
        friend.room &&
        friend.room.room_id !== activeRoom?.room_id
    );
    const statusLabel =
      friend.status === 'in_room' && friend.room
        ? `In @${friend.room.host_username}'s room`
        : friend.status === 'online'
          ? 'Online'
          : 'Offline';

    ctxMenu = {
      kind: 'friend',
      x: Math.max(16, rect.left - 196),
      y: Math.max(16, Math.min(window.innerHeight - 160, rect.top - 8)),
      id: friend.public_id,
      name: friend.name,
      username: friend.username,
      statusLabel,
      statusKind:
        friend.status === 'in_room'
          ? 'in_room'
          : friend.status === 'online'
            ? 'online'
            : 'offline',
      canJoinRoom: canJoin,
      alreadyInvited: isAlreadyInvited
    };
  }

  function openSlotContextMenu(e: MouseEvent, slot: SlotCircle) {
    if (!slot.id) return;
    if (slot.kind === 'invited') {
      e.preventDefault();
      e.stopPropagation();
      const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
      ctxMenu = {
        kind: 'slot-invited',
        x: Math.max(16, Math.min(window.innerWidth - 210, rect.right - 188)),
        y: rect.bottom + 8,
        id: slot.id,
        name: slot.name,
        username: slot.username,
        statusLabel: 'Pending invite',
        statusKind: 'pending',
        canJoinRoom: false,
        alreadyInvited: true
      };
    } else if (slot.kind === 'member' && isHost) {
      e.preventDefault();
      e.stopPropagation();
      const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
      ctxMenu = {
        kind: 'slot-member',
        x: Math.max(16, Math.min(window.innerWidth - 210, rect.right - 188)),
        y: rect.bottom + 8,
        id: slot.id,
        name: slot.name,
        username: slot.username,
        statusLabel: 'In room',
        statusKind: 'online',
        canJoinRoom: false,
        alreadyInvited: false
      };
    }
  }

  async function handleCtxInvite(targetId: string) {
    ctxMenu = null;
    if (!watchTogetherState.draftInvitedIds.includes(targetId)) {
      watchTogetherState.draftInvitedIds = [...watchTogetherState.draftInvitedIds, targetId];
    }
    try {
      await inviteFriendToWatchRoom(targetId);
    } catch {
      // keep in draftInvitedIds if offline
    }
  }

  async function handleCtxCancelInvite(targetId: string) {
    ctxMenu = null;
    await cancelWatchRoomInvite(targetId).catch(() => {});
  }

  async function handleCtxJoinFriend(targetId: string) {
    ctxMenu = null;
    if (busy) return;
    busy = true;
    try {
      const rm = await joinFriendWatchRoom(targetId);
      if (rm.media?.title) {
        void openWatch(
          {
            title: rm.media.title,
            shikiId: rm.media.shiki_id,
            releaseAlias: rm.media.release_alias,
            searchQueries: rm.media.search_queries ?? [rm.media.title]
          },
          {
            targetPage: 'watch-together-player',
            autoPlay: true,
            episodeOrdinal: rm.media.episode_ordinal || 1,
            resumeTime: rm.playback?.position_sec || 0
          }
        );
      }
    } finally {
      busy = false;
    }
  }

  async function handleCtxTransferHost(targetId: string) {
    ctxMenu = null;
    await transferWatchRoomHost(targetId).catch(() => {});
  }

  async function handleCtxKickMember(targetId: string) {
    ctxMenu = null;
    await kickWatchRoomMember(targetId).catch(() => {});
  }

  async function copyRoomCode() {
    if (!activeRoom?.room_code) return;
    try {
      await navigator.clipboard.writeText(activeRoom.room_code);
      codeCopied = true;
      window.setTimeout(() => {
        codeCopied = false;
      }, 1600);
    } catch {
      // ignore
    }
  }
</script>

<svelte:window
  onclick={() => {
    if (ctxMenu) ctxMenu = null;
  }}
  onkeydown={(e) => {
    if (e.key === 'Escape' && ctxMenu) ctxMenu = null;
  }}
/>

<div class="wt-stage">
  <!-- Full-bleed atmospheric backdrop of selected anime -->
  {#if selectedAnime?.backdrop || selectedAnime?.poster}
    {#key selectedAnime.backdrop || selectedAnime.poster}
      <div class="wt-backdrop" aria-hidden="true">
        <img
          src={selectedAnime.backdrop || selectedAnime.poster}
          alt=""
          class="wt-backdrop-img"
        />
        <div class="wt-backdrop-Blur">
          <LayerBlur
            src={selectedAnime.backdrop || selectedAnime.poster}
            blur={46}
            opacity={0.55}
            scale={1.2}
          />
        </div>
        <div class="wt-backdrop-vignette"></div>
      </div>
    {/key}
  {:else}
    <div class="wt-backdrop is-empty" aria-hidden="true">
      <div class="wt-backdrop-vignette"></div>
    </div>
  {/if}

  <div class="wt-canvas">
    <!-- TOP BAR: Create room / Room + Name + pencil + 1/9 + #CODE on the left, Member/Invite circles on the right -->
    <header class="wt-top-bar">
      <div class="wt-heading-group">
        <h1 class="wt-main-heading">
          {activeRoom && watchTogetherState.roomStarted ? 'Room' : 'Create room'}
        </h1>

        {#if editingTitle}
          <form
            class="wt-name-form"
            onsubmit={saveTitle}
            in:revealBlur={{ duration: 220, y: 2, scale: 0.98, blur: 6 }}
          >
            <!-- svelte-ignore a11y_autofocus -->
            <input
              type="text"
              class="wt-name-input"
              bind:value={titleInput}
              placeholder="Name"
              maxlength={36}
              autofocus
              onblur={() => void saveTitle()}
            />
          </form>
        {:else}
          <button
            type="button"
            class="wt-name-trigger"
            onclick={beginEditTitle}
            title="Rename room"
            in:revealBlur={{ duration: 200, y: -2, scale: 0.98, blur: 5 }}
          >
            <span class="wt-room-name">{displayRoomName}</span>
            {#if !activeRoom || isHost}
              <MorphIcon icon={Pencil} size={15} strokeWidth={1.7} class="wt-pencil-icon" />
            {/if}
          </button>
        {/if}

        <span class="wt-seat-counter">{occupiedCount}/{currentMaxMembers}</span>

        {#if activeRoom?.room_code}
          <button
            type="button"
            class="wt-code-chip"
            onclick={copyRoomCode}
            title="Copy room code"
          >
            <span>#{activeRoom.room_code}</span>
            {#if codeCopied}
              <MorphIcon icon={Check} size={11} strokeWidth={2.4} />
            {:else}
              <MorphIcon icon={Copy} size={11} strokeWidth={1.8} />
            {/if}
          </button>
        {/if}
      </div>

      <!-- Top-right lobby member & pending invite slots -->
      <div class="wt-slots-row" aria-label="Room seats">
        {#each slotCircles as slot (slot.key)}
          <div
            class="wt-slot-wrap"
            in:revealBlur={{ duration: 240, scale: 0.88, blur: 8 }}
            out:revealBlur={{ duration: 180, scale: 0.88, blur: 6 }}
          >
            <button
              type="button"
              class="wt-slot-circle is-{slot.kind}"
              title={slot.kind === 'host'
                ? `${slot.name} (Host)`
                : slot.kind === 'member'
                  ? isHost
                    ? `@${slot.username} (right-click to manage)`
                    : `@${slot.username}`
                  : slot.kind === 'invited'
                    ? `Pending: @${slot.username} (right-click to cancel)`
                    : 'Empty seat'}
              onclick={(e) => {
                if (slot.kind === 'empty') {
                  openFriendsPopup();
                } else if (slot.kind === 'invited' || (slot.kind === 'member' && isHost)) {
                  openSlotContextMenu(e, slot);
                }
              }}
              oncontextmenu={(e) => openSlotContextMenu(e, slot)}
            >
              {#if slot.kind === 'empty'}
                <MorphIcon icon={User} size={16} strokeWidth={1.6} />
              {:else if slot.avatarUrl && !brokenAvatars[slot.key]}
                <img
                  src={slot.avatarUrl}
                  alt={slot.name}
                  class="wt-slot-img"
                  referrerpolicy="no-referrer"
                  onerror={() => {
                    brokenAvatars = { ...brokenAvatars, [slot.key]: true };
                  }}
                />
              {:else}
                <span class="wt-slot-fallback">
                  {(slot.name || slot.username || 'U').slice(0, 1).toUpperCase()}
                </span>
              {/if}

              {#if slot.kind === 'host'}
                <span class="wt-slot-badge is-crown">
                  <MorphIcon icon={Crown} size={9} strokeWidth={2.5} />
                </span>
              {:else if slot.kind === 'member'}
                <span class="wt-slot-badge is-joined"></span>
              {:else if slot.kind === 'invited'}
                <span class="wt-slot-badge is-pending">
                  <MorphIcon icon={Clock} size={9} strokeWidth={2.4} />
                </span>
              {/if}
            </button>
          </div>
        {/each}
      </div>
    </header>

    <!-- CENTER STAGE: Choose Anime prompt OR Selected Anime Typography -->
    <div class="wt-center-stage">
      {#if selectedAnime}
        <div
          class="wt-anime-showcase"
          in:revealBlur={{ duration: 280, y: 8, scale: 0.99, blur: 8 }}
        >
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
          <h2
            class="wt-anime-title"
            title="Click to choose another anime"
            onclick={() => openSearch()}
          >
            {selectedAnime.title}
          </h2>

          {#if selectedAnime.score || selectedAnime.tags.length > 0 || selectedAnime.episodeLabel}
            <div class="wt-anime-meta">
              {#if selectedAnime.score && selectedAnime.score > 0}
                <span class="wt-meta-score">
                  <MorphIcon icon={Star} size={12} fill="currentColor" strokeWidth={0} />
                  {selectedAnime.score.toFixed(1)}
                </span>
              {/if}
              {#each selectedAnime.tags as tag}
                <span class="wt-meta-sep">·</span>
                <span class="wt-meta-tag">{tag}</span>
              {/each}
              {#if selectedAnime.episodeLabel}
                <span class="wt-meta-sep">·</span>
                <span class="wt-meta-ep">{selectedAnime.episodeLabel}</span>
              {/if}
            </div>
          {/if}

          {#if selectedAnime.description}
            <p class="wt-anime-desc">{selectedAnime.description}</p>
          {/if}
        </div>
      {:else}
        <button
          type="button"
          class="wt-choose-prompt"
          onclick={() => openSearch()}
          in:revealBlur={{ duration: 260, scale: 0.97, blur: 8 }}
        >
          <span class="wt-choose-title">Choose Anime</span>
          <span class="wt-choose-sub">
            click on icon from search in header, in front of anime cards
          </span>
        </button>
      {/if}
    </div>

    <!-- BOTTOM BAR: Create / Continue / Leave room + ComboBox Settings triggers -->
    <footer class="wt-bottom-bar">
      <div class="wt-bottom-left">
        {#if !activeRoom || !watchTogetherState.roomStarted}
          <button
            type="button"
            class="wt-create-cta"
            disabled={busy}
            onclick={() => void handleStartOrContinueRoom()}
          >
            {busy ? 'Starting…' : 'Create'}
          </button>
        {:else}
          <button
            type="button"
            class="wt-create-cta"
            disabled={busy}
            onclick={() => void handleStartOrContinueRoom()}
          >
            Continue
          </button>

          <button
            type="button"
            class="wt-setting-trigger is-danger"
            onclick={() => void leaveWatchRoom()}
          >
            <MorphIcon icon={LogOut} size={14} strokeWidth={2} />
            <span>Leave room</span>
          </button>
        {/if}

        <div class="wt-inline-settings">
          <!-- Visibility ComboBox -->
          <ComboBox
            bind:open={visibilityMenuOpen}
            options={visibilityOptions}
            value={currentVisibility}
            placement="top-left"
            minWidth={184}
            onselect={(opt) => void selectVisibility(opt.value as RoomVisibility)}
          >
            {#snippet trigger({ toggle })}
              <button
                type="button"
                class="wt-setting-trigger"
                disabled={Boolean(activeRoom && !isHost)}
                onclick={toggle}
                title="Room visibility"
              >
                <MorphIcon
                  icon={currentVisibility === 'public'
                    ? Globe
                    : currentVisibility === 'private'
                      ? Lock
                      : Users}
                  size={13}
                  strokeWidth={1.9}
                />
                <span>
                  {currentVisibility === 'public'
                    ? 'Public'
                    : currentVisibility === 'private'
                      ? 'Private'
                      : 'Friends'}
                </span>
              </button>
            {/snippet}
          </ComboBox>

          <!-- Control Mode ComboBox -->
          <ComboBox
            bind:open={controlMenuOpen}
            options={controlOptions}
            value={currentControlMode}
            placement="top-left"
            minWidth={210}
            onselect={(opt) => void selectControlMode(opt.value as RoomControlMode)}
          >
            {#snippet trigger({ toggle })}
              <button
                type="button"
                class="wt-setting-trigger"
                disabled={Boolean(activeRoom && !isHost)}
                onclick={toggle}
                title="Playback control permission"
              >
                <MorphIcon icon={Shield} size={13} strokeWidth={1.9} />
                <span>
                  {currentControlMode === 'everyone' ? 'Everyone controls' : 'Host only'}
                </span>
              </button>
            {/snippet}
          </ComboBox>

          <!-- Max Seats ComboBox -->
          <ComboBox
            bind:open={seatsMenuOpen}
            options={seatsOptions}
            value={currentMaxMembers}
            placement="top-left"
            minWidth={136}
            maxHeight={240}
            onselect={(opt) => void selectMaxMembers(Number(opt.value))}
          >
            {#snippet trigger({ toggle })}
              <button
                type="button"
                class="wt-setting-trigger"
                disabled={Boolean(activeRoom && !isHost)}
                onclick={toggle}
                title="Maximum seats"
              >
                <span>{currentMaxMembers} seats</span>
              </button>
            {/snippet}
          </ComboBox>
        </div>
      </div>
    </footer>

    <!-- RIGHT-SIDE VERTICAL FRIENDS RAIL (flush bottom-right, no clipping frame, brightness hover, right-click ComboBox) -->
    <aside class="wt-friends-rail" aria-label="Friends">
      <button
        type="button"
        class="wt-rail-add"
        title="Friends & @username"
        onclick={() => {
          if (!auth.user) {
            void loginWithGoogle();
          } else {
            openFriendsPopup();
          }
        }}
      >
        <MorphIcon icon={UserPlus} size={16} strokeWidth={1.9} />
        {#if friendsState.incoming.length > 0}
          <span class="wt-rail-req-badge">{friendsState.incoming.length}</span>
        {/if}
      </button>

      <div class="wt-rail-list">
        {#each sortedFriends as friend (friend.public_id)}
          {@const isInvited = pendingFriends.some((p) => p.public_id === friend.public_id)}
          {@const inCurrentRoom = Boolean(
            activeRoom?.members.some((m) => m.public_id === friend.public_id)
          )}
          <button
            type="button"
            class="wt-rail-avatar-btn"
            class:is-offline={friend.status === 'offline'}
            class:is-selected={isInvited || inCurrentRoom}
            onclick={(e) => openFriendContextMenu(e, friend)}
            oncontextmenu={(e) => openFriendContextMenu(e, friend)}
            title="@{friend.username} (right-click for options)"
          >
            {#if friend.avatar_url && !brokenAvatars[friend.public_id]}
              <img
                src={friend.avatar_url}
                alt={friend.name}
                class="wt-rail-avatar"
                referrerpolicy="no-referrer"
                onerror={() => {
                  brokenAvatars = { ...brokenAvatars, [friend.public_id]: true };
                }}
              />
            {:else}
              <span class="wt-rail-fallback">
                {(friend.name || friend.username || 'U').slice(0, 1).toUpperCase()}
              </span>
            {/if}

            <span
              class="wt-rail-dot"
              class:is-online={friend.status === 'online'}
              class:is-room={friend.status === 'in_room'}
            ></span>
          </button>
        {/each}
      </div>
    </aside>
  </div>
</div>

<!-- Right-click ComboBox Context Menu for Friends & Room Slots -->
{#if ctxMenu}
  {@const menu = ctxMenu}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    use:portal
    class="wt-ctx-popover"
    style:left="{menu.x}px"
    style:top="{menu.y}px"
    onclick={(e) => e.stopPropagation()}
    oncontextmenu={(e) => e.preventDefault()}
    in:revealBlur={{ duration: 180, y: 4, scale: 0.96, blur: 8 }}
  >
    <div class="wt-ctx-header">
      <div class="wt-ctx-user-row">
        <span class="wt-ctx-name">{menu.name}</span>
        <span
          class="wt-ctx-status-dot"
          class:is-online={menu.statusKind === 'online'}
          class:is-room={menu.statusKind === 'in_room'}
          class:is-pending={menu.statusKind === 'pending'}
        ></span>
      </div>
      <span class="wt-ctx-sub">@{menu.username} · {menu.statusLabel}</span>
    </div>

    <div class="wt-ctx-list">
      {#if menu.kind === 'friend'}
        {#if menu.alreadyInvited}
          <button
            type="button"
            class="wt-ctx-item is-danger"
            onclick={() => void handleCtxCancelInvite(menu.id)}
          >
            Cancel invite
          </button>
        {:else}
          <button
            type="button"
            class="wt-ctx-item"
            onclick={() => void handleCtxInvite(menu.id)}
          >
            Invite
          </button>
        {/if}

        {#if menu.canJoinRoom}
          <button
            type="button"
            class="wt-ctx-item"
            onclick={() => void handleCtxJoinFriend(menu.id)}
          >
            Join room
          </button>
        {/if}
      {:else if menu.kind === 'slot-invited'}
        <button
          type="button"
          class="wt-ctx-item is-danger"
          onclick={() => void handleCtxCancelInvite(menu.id)}
        >
          Cancel
        </button>
      {:else if menu.kind === 'slot-member'}
        <button
          type="button"
          class="wt-ctx-item"
          onclick={() => void handleCtxTransferHost(menu.id)}
        >
          Transfer host
        </button>
        <button
          type="button"
          class="wt-ctx-item is-danger"
          onclick={() => void handleCtxKickMember(menu.id)}
        >
          Kick
        </button>
      {/if}
    </div>
  </div>
{/if}

<style>
  .wt-stage {
    position: relative;
    width: 100%;
    height: 100vh;
    min-height: 620px;
    overflow: hidden;
    background: #0c0c0c;
    color: #ffffff;
    box-sizing: border-box;
    user-select: none;
  }

  /* Full-bleed atmospheric anime background */
  .wt-backdrop {
    position: absolute;
    inset: 0;
    pointer-events: none;
    z-index: 0;
    overflow: hidden;
  }

  .wt-backdrop-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    opacity: 0.34;
    filter: saturate(1.08) contrast(1.04);
    transform: scale(1.02);
  }

  .wt-backdrop-Blur {
    position: absolute;
    inset: 0;
    opacity: 0.55;
  }

  .wt-backdrop-vignette {
    position: absolute;
    inset: 0;
    background:
      linear-gradient(
        180deg,
        #0c0c0c 0%,
        rgba(12, 12, 12, 0.72) 15%,
        rgba(12, 12, 12, 0.25) 48%,
        rgba(12, 12, 12, 0.76) 84%,
        #0c0c0c 100%
      ),
      radial-gradient(
        circle at 22% 50%,
        rgba(12, 12, 12, 0.52) 0%,
        rgba(12, 12, 12, 0.16) 65%,
        rgba(12, 12, 12, 0.74) 100%
      );
  }

  .wt-canvas {
    position: relative;
    z-index: 1;
    width: 100%;
    height: 100%;
    padding: calc(var(--header-height, 48px) + 18px) var(--page-pad-x) 24px;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
  }

  /* TOP BAR */
  .wt-top-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 20px;
    flex-wrap: wrap;
  }

  .wt-heading-group {
    display: flex;
    align-items: center;
    gap: 14px;
    flex-wrap: wrap;
    min-width: 0;
  }

  .wt-main-heading {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(26px, 2.8vw, 35px);
    font-weight: 800;
    letter-spacing: -0.03em;
    line-height: 1;
    color: #ffffff;
  }

  .wt-name-trigger {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 0;
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.34);
    cursor: pointer;
    transition: color 0.18s ease;
  }

  .wt-name-trigger:hover {
    color: rgba(255, 255, 255, 0.64);
  }

  .wt-room-name {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(24px, 2.5vw, 32px);
    font-weight: 800;
    letter-spacing: -0.025em;
    line-height: 1;
    max-width: 340px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .wt-name-trigger :global(.wt-pencil-icon) {
    opacity: 0.56;
    transition: opacity 0.18s ease;
  }

  .wt-name-trigger:hover :global(.wt-pencil-icon) {
    opacity: 1;
  }

  .wt-name-form {
    display: inline-flex;
    align-items: center;
  }

  .wt-name-input {
    width: clamp(140px, 18vw, 240px);
    border: none;
    border-bottom: 1.5px solid rgba(255, 255, 255, 0.35);
    outline: none;
    background: transparent;
    color: #ffffff;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(22px, 2.3vw, 30px);
    font-weight: 800;
    letter-spacing: -0.025em;
    padding: 0 2px 2px;
  }

  .wt-seat-counter {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 15px;
    font-weight: 700;
    line-height: 1;
    color: rgba(255, 255, 255, 0.28);
    letter-spacing: -0.01em;
    display: inline-flex;
    align-items: center;
  }

  .wt-code-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 0;
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.34);
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 700;
    line-height: 1;
    letter-spacing: 0.02em;
    cursor: pointer;
    transition: color 0.18s ease;
  }

  .wt-code-chip:hover {
    color: rgba(255, 255, 255, 0.85);
  }

  /* Top-right member & empty slot circles */
  .wt-slots-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .wt-slot-wrap {
    display: inline-flex;
  }

  .wt-slot-circle {
    position: relative;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    padding: 0;
    border: none;
    background: rgba(255, 255, 255, 0.05);
    color: rgba(255, 255, 255, 0.26);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition:
      filter 0.18s ease,
      background-color 0.18s ease,
      color 0.18s ease;
  }

  .wt-slot-circle:hover {
    filter: brightness(1.15);
    background: rgba(255, 255, 255, 0.09);
    color: rgba(255, 255, 255, 0.55);
  }

  .wt-slot-circle.is-invited {
    opacity: 0.58;
  }

  .wt-slot-img {
    width: 100%;
    height: 100%;
    border-radius: 50%;
    object-fit: cover;
  }

  .wt-slot-fallback {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 12px;
    font-weight: 700;
    color: #ffffff;
  }

  .wt-slot-badge {
    position: absolute;
    right: -1px;
    bottom: -1px;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    box-shadow: 0 0 0 2px #0c0c0c;
  }

  .wt-slot-badge.is-crown {
    background: #d4b13b;
    color: #0c0c0c;
  }

  .wt-slot-badge.is-joined {
    width: 9px;
    height: 9px;
    right: 0;
    bottom: 0;
    background: #29eb36;
  }

  .wt-slot-badge.is-pending {
    background: #1c1c1e;
    color: rgba(255, 255, 255, 0.78);
  }

  /* CENTER STAGE */
  .wt-center-stage {
    position: relative;
    flex: 1 1 auto;
    display: flex;
    align-items: center;
    min-height: 0;
    padding-right: 68px;
  }

  .wt-choose-prompt {
    margin: 0 auto;
    padding: 28px 36px;
    border: none;
    border-radius: 20px;
    background: transparent;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    transition: transform 0.22s ease;
  }

  .wt-choose-prompt:hover {
    transform: scale(1.015);
  }

  .wt-choose-title {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(32px, 3.8vw, 48px);
    font-weight: 800;
    letter-spacing: -0.03em;
    color: rgba(255, 255, 255, 0.22);
    transition: color 0.22s ease;
  }

  .wt-choose-prompt:hover .wt-choose-title {
    color: rgba(255, 255, 255, 0.38);
  }

  .wt-choose-sub {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    color: rgba(255, 255, 255, 0.14);
  }

  /* Selected Anime Showcase */
  .wt-anime-showcase {
    max-width: min(860px, 78vw);
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 12px;
  }

  .wt-anime-title {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(34px, 4.4vw, 56px);
    font-weight: 900;
    line-height: 1.05;
    letter-spacing: -0.03em;
    color: rgba(255, 255, 255, 0.9);
    cursor: pointer;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    transition: color 0.18s ease;
  }

  .wt-anime-title:hover {
    color: #ffffff;
  }

  .wt-anime-meta {
    display: flex;
    align-items: center;
    gap: 7px;
    flex-wrap: wrap;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    color: rgba(255, 255, 255, 0.56);
  }

  .wt-meta-score {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: #f4c150;
    font-weight: 700;
  }

  .wt-meta-sep {
    color: rgba(255, 255, 255, 0.24);
  }

  .wt-meta-ep {
    color: rgba(255, 255, 255, 0.85);
    font-weight: 600;
  }

  .wt-anime-desc {
    margin: 0;
    max-width: 650px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: clamp(13px, 1.1vw, 14.5px);
    line-height: 1.5;
    color: rgba(255, 255, 255, 0.44);
    display: -webkit-box;
    -webkit-line-clamp: 4;
    line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  /* BOTTOM BAR */
  .wt-bottom-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding-right: 60px;
  }

  .wt-bottom-left {
    display: flex;
    align-items: center;
    gap: 18px;
    flex-wrap: wrap;
  }

  .wt-create-cta {
    padding: 0;
    border: none;
    background: transparent;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(26px, 2.6vw, 34px);
    font-weight: 800;
    letter-spacing: -0.03em;
    line-height: 1;
    color: rgba(255, 255, 255, 0.88);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    transition:
      color 0.18s ease,
      transform 0.16s ease;
  }

  .wt-create-cta:hover:not(:disabled) {
    color: #ffffff;
  }

  .wt-create-cta:disabled {
    opacity: 0.45;
    cursor: default;
  }

  .wt-inline-settings {
    display: flex;
    align-items: center;
    gap: 16px;
    flex-wrap: wrap;
  }

  /* Borderless, background-free setting triggers aligned with Create / Leave room */
  .wt-setting-trigger {
    height: 32px;
    padding: 0 2px;
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.56);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 600;
    line-height: 1;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    transition: color 0.18s ease;
  }

  .wt-setting-trigger:hover:not(:disabled) {
    color: #ffffff;
  }

  .wt-setting-trigger:disabled {
    opacity: 0.45;
    cursor: default;
  }

  .wt-setting-trigger.is-danger {
    color: rgba(255, 132, 132, 0.82);
  }

  .wt-setting-trigger.is-danger:hover {
    color: #ff7a7a;
  }

  /* RIGHT-SIDE FRIENDS RAIL (lowered flush to bottom-right corner, no clipping, brightness hover) */
  .wt-friends-rail {
    position: absolute;
    right: var(--page-pad-x, 28px);
    bottom: 24px;
    z-index: 15;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
  }

  .wt-rail-add {
    position: relative;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    padding: 0;
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.56);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: color 0.18s ease;
  }

  .wt-rail-add:hover {
    color: #ffffff;
  }

  .wt-rail-req-badge {
    position: absolute;
    top: 0;
    right: 0;
    min-width: 14px;
    height: 14px;
    padding: 0 3px;
    border-radius: 999px;
    background: #ffffff;
    color: #0c0c0c;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 9px;
    font-weight: 800;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .wt-rail-list {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    overflow: visible;
  }

  .wt-rail-avatar-btn {
    position: relative;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    padding: 0;
    border: none;
    background: rgba(255, 255, 255, 0.06);
    color: #ffffff;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    flex-shrink: 0;
    transition:
      filter 0.2s cubic-bezier(0.22, 1, 0.36, 1),
      opacity 0.2s ease;
  }

  .wt-rail-avatar-btn:hover {
    filter: brightness(1.18) contrast(1.04);
    opacity: 1;
  }

  .wt-rail-avatar-btn.is-offline {
    opacity: 0.46;
  }

  .wt-rail-avatar-btn.is-offline:hover {
    opacity: 0.85;
  }

  .wt-rail-avatar {
    width: 100%;
    height: 100%;
    border-radius: 50%;
    object-fit: cover;
  }

  .wt-rail-fallback {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 12px;
    font-weight: 700;
  }

  .wt-rail-dot {
    position: absolute;
    right: 0;
    bottom: 0;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #373737;
    box-shadow: 0 0 0 2px #0c0c0c;
  }

  .wt-rail-dot.is-online {
    background: #29eb36;
  }

  .wt-rail-dot.is-room {
    background: #294aeb;
  }

  /* Right-click ComboBox Popover */
  .wt-ctx-popover {
    position: fixed;
    z-index: 10000;
    min-width: 184px;
    max-width: 228px;
    padding: 6px;
    border-radius: 13px;
    background: var(--mila-popup-comboBg, #161616);
    box-shadow: var(--mila-popup-comboShadow, 0 18px 44px rgba(0, 0, 0, 0.62));
    color: #ffffff;
    user-select: none;
  }

  .wt-ctx-header {
    padding: 6px 9px 7px;
    margin-bottom: 4px;
    border-bottom: 1px solid rgba(255, 255, 255, 0.05);
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .wt-ctx-user-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .wt-ctx-name {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 700;
    color: #ffffff;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .wt-ctx-status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #48484c;
    flex-shrink: 0;
  }

  .wt-ctx-status-dot.is-online {
    background: #29eb36;
  }

  .wt-ctx-status-dot.is-room {
    background: #294aeb;
  }

  .wt-ctx-status-dot.is-pending {
    background: #d4b13b;
  }

  .wt-ctx-sub {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11px;
    color: rgba(255, 255, 255, 0.44);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .wt-ctx-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .wt-ctx-item {
    width: 100%;
    padding: 7px 10px;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: rgba(255, 255, 255, 0.86);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 500;
    text-align: left;
    cursor: pointer;
    transition:
      background-color 0.14s ease,
      color 0.14s ease;
  }

  .wt-ctx-item:hover {
    background: rgba(255, 255, 255, 0.08);
    color: #ffffff;
  }

  .wt-ctx-item.is-danger {
    color: #ff7e7e;
  }

  .wt-ctx-item.is-danger:hover {
    background: rgba(255, 107, 107, 0.12);
    color: #ff9494;
  }
</style>
