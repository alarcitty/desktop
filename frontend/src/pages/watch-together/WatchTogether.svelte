<script lang="ts">
  import { onMount } from 'svelte';
  import { cubicOut } from 'svelte/easing';
  import { fade, fly, scale } from 'svelte/transition';
  import { MorphIcon } from 'morphicons/svelte';
  import {
    Check,
    Clock,
    Copy,
    Crown,
    Globe,
    Lock,
    LogOut,
    MessageSquare,
    Pause,
    Pencil,
    Play,
    Plus,
    Minus,
    Search,
    Send,
    Settings,
    Shield,
    Sparkles,
    User,
    UserPlus,
    Users,
    X
  } from 'lucide';

  import LayerBlur from '../../marlakit/blur/LayerBlur.svelte';
  import MetaLine from '../../marlakit/meta/MetaLine.svelte';
  import ScrollArea from '../../marlakit/scroll/ScrollArea.svelte';
  import { auth, loginWithGoogle } from '../../app/auth.svelte';
  import {
    friendsState,
    openFriendsPopup,
    refreshFriends
  } from '../../app/friends.svelte';
  import {
    applyRemotePlaybackSync,
    openWatch,
    player,
    selectEpisode
  } from '../../app/player.svelte';
  import { navigate } from '../../app/router.svelte';
  import { openSearch } from '../../app/search.svelte';
  import {
    connectWatchTogether,
    createWatchRoom,
    inviteFriendToWatchRoom,
    joinFriendWatchRoom,
    joinWatchRoom,
    kickWatchRoomMember,
    leaveWatchRoom,
    sendWatchRoomChat,
    setWatchRoomMedia,
    syncWatchRoomPlayback,
    toggleDraftInviteFriend,
    transferWatchRoomHost,
    updateWatchRoomMemberState,
    updateWatchRoomSettings,
    watchTogetherState
  } from '../../app/watchTogether.svelte';
  import {
    computeLiveRoomPosition,
    type RoomControlMode,
    type RoomVisibility
  } from '../../api/watchTogether';
  import type { FriendProfile } from '../../api/friends';

  const QUICK_REACTIONS = ['🔥', '😂', '😭', '❤️', '⚡'] as const;

  let editingTitle = $state(false);
  let titleInputValue = $state('');
  let settingsExpanded = $state(false);
  let joinCodeOpen = $state(false);
  let joinCodeValue = $state('');
  let codeCopied = $state(false);
  let chatInput = $state('');
  let actionBusy = $state(false);
  let hoveredFriend = $state<FriendProfile | null>(null);
  let invitedToastMap = $state<Record<string, boolean>>({});

  let lastAppliedSeq = 0;
  let suppressOutboundUntil = 0;

  const myUser = $derived(auth.user);
  const activeRoom = $derived(watchTogetherState.room);
  const draftAnime = $derived(watchTogetherState.draftAnime);

  const defaultRoomName = $derived(
    myUser?.name ? `${myUser.name}’s room` : 'Name'
  );

  const displayRoomName = $derived(
    activeRoom
      ? activeRoom.title || defaultRoomName
      : watchTogetherState.draftTitle.trim() || defaultRoomName
  );

  const amHost = $derived(
    Boolean(
      activeRoom &&
        myUser &&
        (activeRoom.host_public_id === myUser.public_id ||
          activeRoom.host_username === myUser.username)
    )
  );

  const canControlActiveRoom = $derived(
    Boolean(
      activeRoom &&
        (amHost ||
          activeRoom.control_mode === 'everyone' ||
          activeRoom.members.some(
            (m) => m.public_id === myUser?.public_id && (m.role === 'host' || m.role === 'cohost')
          ))
    )
  );

  // Lobby & active room circular slots (aligned on one row with "Create room Name")
  const maxSeats = $derived(
    activeRoom ? activeRoom.max_members : watchTogetherState.draftMaxMembers
  );

  const invitedFriendsInDraft = $derived(
    watchTogetherState.draftInvitedIds
      .map((id) => friendsState.friends.find((f) => f.public_id === id))
      .filter((f): f is FriendProfile => Boolean(f))
  );

  const occupiedCount = $derived(
    activeRoom
      ? activeRoom.members.length
      : 1 + invitedFriendsInDraft.length
  );

  const emptySlotCount = $derived(
    Math.max(0, maxSeats - occupiedCount)
  );

  // Friends sorted: in_room first (blue), then online (green), then offline (gray)
  const sortedFriends = $derived.by(() => {
    const rank = (s?: string) => (s === 'in_room' ? 0 : s === 'online' ? 1 : 2);
    return [...friendsState.friends].sort((a, b) => {
      const r = rank(a.status) - rank(b.status);
      if (r !== 0) return r;
      return a.name.localeCompare(b.name);
    });
  });

  // Friends currently inside a joinable room
  const friendsInRooms = $derived(
    sortedFriends.filter(
      (f) => f.status === 'in_room' && f.room && f.room.room_id !== activeRoom?.room_id
    )
  );

  const stageTitle = $derived(
    activeRoom?.media?.title || draftAnime?.title || ''
  );

  const stageDescription = $derived(
    draftAnime?.description ||
      (activeRoom?.media?.title
        ? `Watching Episode ${activeRoom.media.episode_ordinal || 1} together in real-time over QUIC.`
        : '')
  );

  const stageBackdrop = $derived(
    draftAnime?.backdrop ||
      draftAnime?.poster ||
      activeRoom?.media?.poster ||
      player.shikiArt?.screenshots?.[0] ||
      player.shikiArt?.posterOriginal ||
      ''
  );

  onMount(() => {
    if (auth.user) {
      void connectWatchTogether();
      void refreshFriends();
    }
  });

  // Synchronize incoming QUIC room media & playback state with Mila's player
  $effect(() => {
    const rm = activeRoom;
    if (!rm || !rm.media?.title) return;
    const pb = rm.playback;
    if (!pb || pb.seq <= lastAppliedSeq) return;
    lastAppliedSeq = pb.seq;

    if (pb.updated_by && pb.updated_by === myUser?.public_id) {
      return;
    }

    suppressOutboundUntil = Date.now() + 900;
    const targetEpOrd = rm.media.episode_ordinal || 1;
    const curTitle = player.release?.title || player.title || '';
    const sameAnime =
      (rm.media.release_alias && player.release?.alias === rm.media.release_alias) ||
      (rm.media.shiki_id && player.shikiArt?.id === rm.media.shiki_id) ||
      (curTitle && curTitle.toLowerCase() === rm.media.title.toLowerCase());

    const livePos = computeLiveRoomPosition(pb);

    if (!player.release || !sameAnime) {
      void openWatch(
        {
          title: rm.media.title,
          shikiId: rm.media.shiki_id || undefined,
          releaseAlias: rm.media.release_alias || undefined,
          searchQueries: rm.media.search_queries?.length
            ? rm.media.search_queries
            : [rm.media.title]
        },
        {
          episodeOrdinal: targetEpOrd,
          resumeTime: livePos,
          autoPlay: !pb.paused
        }
      );
      return;
    }

    const epIdx = player.release.episodes.findIndex((e) => e.ordinal === targetEpOrd);
    if (epIdx >= 0 && epIdx !== player.episodeIndex) {
      selectEpisode(epIdx);
    }
    applyRemotePlaybackSync(pb.paused, livePos);
  });

  function beginEditTitle() {
    if (activeRoom && !amHost) return;
    titleInputValue = activeRoom
      ? activeRoom.title
      : watchTogetherState.draftTitle || defaultRoomName;
    editingTitle = true;
  }

  async function commitTitleEdit(e?: Event) {
    e?.preventDefault();
    editingTitle = false;
    const clean = titleInputValue.trim();
    if (!clean) return;
    if (activeRoom && amHost) {
      await updateWatchRoomSettings({ title: clean }).catch(() => {});
    } else {
      watchTogetherState.draftTitle = clean;
    }
  }

  function cycleVisibility() {
    const order: RoomVisibility[] = ['friends', 'public', 'private'];
    const cur = activeRoom ? activeRoom.visibility : watchTogetherState.draftVisibility;
    const next = order[(order.indexOf(cur) + 1) % order.length];
    if (activeRoom && amHost) {
      void updateWatchRoomSettings({ visibility: next });
    } else {
      watchTogetherState.draftVisibility = next;
    }
  }

  function cycleControlMode() {
    const cur = activeRoom ? activeRoom.control_mode : watchTogetherState.draftControlMode;
    const next: RoomControlMode = cur === 'host_only' ? 'everyone' : 'host_only';
    if (activeRoom && amHost) {
      void updateWatchRoomSettings({ control_mode: next });
    } else {
      watchTogetherState.draftControlMode = next;
    }
  }

  function stepMaxMembers(delta: number) {
    const cur = activeRoom ? activeRoom.max_members : watchTogetherState.draftMaxMembers;
    const minAllowed = activeRoom ? Math.max(2, activeRoom.members.length) : 2;
    const next = Math.max(minAllowed, Math.min(10, cur + delta));
    if (next === cur) return;
    if (activeRoom && amHost) {
      void updateWatchRoomSettings({ max_members: next });
    } else {
      watchTogetherState.draftMaxMembers = next;
      if (watchTogetherState.draftInvitedIds.length > next - 1) {
        watchTogetherState.draftInvitedIds = watchTogetherState.draftInvitedIds.slice(0, next - 1);
      }
    }
  }

  async function handleCreateRoom() {
    if (!myUser) {
      await loginWithGoogle();
      return;
    }
    if (actionBusy) return;
    actionBusy = true;
    try {
      const anime = draftAnime;
      const room = await createWatchRoom({
        title: watchTogetherState.draftTitle.trim() || anime?.title || defaultRoomName,
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
      });
      // Send invites to all pre-selected friends in the lobby row
      const toInvite = [...watchTogetherState.draftInvitedIds];
      watchTogetherState.draftInvitedIds = [];
      for (const pubId of toInvite) {
        void inviteFriendToWatchRoom(pubId).catch(() => {});
      }
      if (room.media?.title) {
        void openWatch(
          {
            title: room.media.title,
            shikiId: room.media.shiki_id || undefined,
            releaseAlias: room.media.release_alias || undefined,
            searchQueries: room.media.search_queries?.length
              ? room.media.search_queries
              : [room.media.title]
          },
          {
            episodeOrdinal: room.media.episode_ordinal || 1,
            autoPlay: false
          }
        );
      }
    } finally {
      actionBusy = false;
    }
  }

  async function handleJoinByCode(e?: Event) {
    e?.preventDefault();
    const code = joinCodeValue.trim().toUpperCase();
    if (!code || actionBusy) return;
    actionBusy = true;
    try {
      await joinWatchRoom({ room_code: code });
      joinCodeValue = '';
      joinCodeOpen = false;
    } finally {
      actionBusy = false;
    }
  }

  async function handleFriendRailClick(friend: FriendProfile) {
    if (friend.status === 'in_room' && friend.room && friend.room.room_id !== activeRoom?.room_id) {
      await joinFriendWatchRoom(friend.public_id).catch(() => {});
      return;
    }
    if (activeRoom) {
      await inviteFriendToWatchRoom(friend.public_id).catch(() => {});
      invitedToastMap = { ...invitedToastMap, [friend.public_id]: true };
      window.setTimeout(() => {
        invitedToastMap = { ...invitedToastMap, [friend.public_id]: false };
      }, 1600);
      return;
    }
    toggleDraftInviteFriend(friend.public_id);
  }

  async function handleCopyRoomCode() {
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

  async function handleToggleRoomPlayback() {
    if (!activeRoom || !canControlActiveRoom) return;
    const nextPaused = !activeRoom.playback.paused;
    const pb = await syncWatchRoomPlayback({
      paused: nextPaused,
      position_sec: computeLiveRoomPosition(activeRoom.playback)
    });
    lastAppliedSeq = pb.seq;
  }

  async function handleStepEpisode(delta: number) {
    if (!activeRoom || !canControlActiveRoom || !activeRoom.media?.title) return;
    const nextOrd = Math.max(1, (activeRoom.media.episode_ordinal || 1) + delta);
    await setWatchRoomMedia({
      ...activeRoom.media,
      episode_ordinal: nextOrd,
      episode_index: Math.max(0, nextOrd - 1)
    });
  }

  function handleOpenSyncedPlayer() {
    if (!activeRoom?.media?.title) {
      openSearch();
      return;
    }
    const livePos = computeLiveRoomPosition(activeRoom.playback);
    void openWatch(
      {
        title: activeRoom.media.title,
        shikiId: activeRoom.media.shiki_id || undefined,
        releaseAlias: activeRoom.media.release_alias || undefined,
        searchQueries: activeRoom.media.search_queries?.length
          ? activeRoom.media.search_queries
          : [activeRoom.media.title]
      },
      {
        episodeOrdinal: activeRoom.media.episode_ordinal || 1,
        resumeTime: livePos,
        autoPlay: !activeRoom.playback.paused,
        directPlayer: true
      }
    );
  }

  async function handleSendChat(e?: Event) {
    e?.preventDefault();
    const text = chatInput.trim();
    if (!text || !activeRoom) return;
    chatInput = '';
    await sendWatchRoomChat(text, 'chat').catch(() => {});
  }

  async function handleQuickReaction(emoji: string) {
    if (!activeRoom) return;
    await sendWatchRoomChat(emoji, 'reaction').catch(() => {});
  }
</script>

<div class="wt-page">
  <!-- Full-bleed atmospheric backdrop when an anime is selected or playing -->
  {#if stageBackdrop}
    {#key stageBackdrop}
      <div class="wt-backdrop-wrap" aria-hidden="true" transition:fade={{ duration: 320, easing: cubicOut }}>
        <div class="wt-backdrop-blur">
          <LayerBlur src={stageBackdrop} blur={92} opacity={0.38} scale={1.18} />
        </div>
        <img src={stageBackdrop} alt="" class="wt-backdrop-art" decoding="async" />
        <div class="wt-backdrop-scrim"></div>
      </div>
    {/key}
  {/if}

  <!-- Top Row: "Create room Name ✎ 2/7" on the left, circular member & empty person slots on the right -->
  <header class="wt-top-bar">
    <div class="wt-heading-group">
      <span class="wt-main-heading">
        {activeRoom ? 'Room' : 'Create room'}
      </span>

      {#if editingTitle}
        <form class="wt-title-form" onsubmit={commitTitleEdit}>
          <!-- svelte-ignore a11y_autofocus -->
          <input
            type="text"
            class="wt-title-input"
            bind:value={titleInputValue}
            maxlength={40}
            autofocus
            onblur={commitTitleEdit}
          />
        </form>
      {:else}
        <button
          type="button"
          class="wt-name-trigger"
          onclick={beginEditTitle}
          title="Rename room"
        >
          <span class="wt-room-name">{displayRoomName}</span>
          {#if !activeRoom || amHost}
            <MorphIcon icon={Pencil} size={15} strokeWidth={1.8} class="wt-pencil-icon" />
          {/if}
        </button>
      {/if}

      <span class="wt-seat-counter">{occupiedCount}/{maxSeats}</span>

      {#if activeRoom}
        <button
          type="button"
          class="wt-code-chip"
          onclick={handleCopyRoomCode}
          title="Copy room invite code"
        >
          <span>#{activeRoom.room_code}</span>
          <MorphIcon icon={codeCopied ? Check : Copy} size={12} strokeWidth={2.1} />
        </button>
      {/if}
    </div>

    <!-- Top-right horizontal circle slots: Host (crown) + Joined (green) + Invited (clock) + Empty person circles -->
    <div class="wt-slots-row" aria-label="Room seats">
      {#if activeRoom}
        {#each activeRoom.members as m (m.public_id)}
          <div
            class="seat-circle is-occupied"
            title="@{m.username} ({m.role})"
          >
            {#if m.avatar_url}
              <img src={m.avatar_url} alt={m.name} class="seat-avatar" referrerpolicy="no-referrer" />
            {:else}
              <span class="seat-initials">{(m.name || 'U').slice(0, 1).toUpperCase()}</span>
            {/if}

            {#if m.role === 'host'}
              <span class="seat-badge is-crown" title="Room Host">
                <MorphIcon icon={Crown} size={9} strokeWidth={2.4} />
              </span>
            {:else}
              <span class="seat-badge is-online" class:is-buffering={m.buffering}></span>
            {/if}
          </div>
        {/each}
      {:else}
        <!-- Host seat in lobby -->
        <div class="seat-circle is-occupied" title={myUser ? `@${myUser.username || myUser.name} (Host)` : 'Host'}>
          {#if myUser?.avatar_url}
            <img src={myUser.avatar_url} alt={myUser.name} class="seat-avatar" referrerpolicy="no-referrer" />
          {:else}
            <span class="seat-initials">{(myUser?.name || 'H').slice(0, 1).toUpperCase()}</span>
          {/if}
          <span class="seat-badge is-crown" title="Host">
            <MorphIcon icon={Crown} size={9} strokeWidth={2.4} />
          </span>
        </div>

        <!-- Pre-invited friends in lobby (dimmed + clock badge, click to uninvite) -->
        {#each invitedFriendsInDraft as inv (inv.public_id)}
          <button
            type="button"
            class="seat-circle is-invited"
            title="Invited @{inv.username} (click to remove)"
            onclick={() => toggleDraftInviteFriend(inv.public_id)}
          >
            {#if inv.avatar_url}
              <img src={inv.avatar_url} alt={inv.name} class="seat-avatar is-dim" referrerpolicy="no-referrer" />
            {:else}
              <span class="seat-initials is-dim">{(inv.name || 'U').slice(0, 1).toUpperCase()}</span>
            {/if}
            <span class="seat-badge is-clock">
              <MorphIcon icon={Clock} size={9} strokeWidth={2.2} />
            </span>
          </button>
        {/each}
      {/if}

      <!-- Empty seats: person icon inside a circle -->
      {#each Array(emptySlotCount) as _, idx (idx)}
        <button
          type="button"
          class="seat-circle is-empty"
          title="Empty seat — click to invite a friend"
          aria-label="Invite friend to seat"
          onclick={() => openFriendsPopup()}
        >
          <MorphIcon icon={User} size={16} strokeWidth={1.75} />
        </button>
      {/each}
    </div>
  </header>

  <!-- Center Stage: either "Choose Anime" empty canvas OR Selected Anime Showcase / Live Room Stage -->
  <section class="wt-center-stage">
    {#if !stageTitle}
      <button
        type="button"
        class="wt-choose-prompt"
        onclick={() => openSearch()}
        aria-label="Choose anime from search"
      >
        <span class="choose-heading">Choose Anime</span>
        <span class="choose-sub">
          click on icon from search in header, in front of anime cards
        </span>
      </button>
    {:else}
      <div class="wt-anime-showcase" in:fade={{ duration: 240, easing: cubicOut }}>
        <div class="wt-anime-copy">
          <h1 class="wt-anime-title">{stageTitle}</h1>

          {#if draftAnime && (draftAnime.score || draftAnime.tags.length > 0)}
            <div class="wt-anime-meta">
              <MetaLine score={draftAnime.score} tags={draftAnime.tags} />
            </div>
          {/if}

          {#if stageDescription}
            <p class="wt-anime-desc">{stageDescription}</p>
          {/if}

          {#if activeRoom}
            <!-- Active Room Playback & Episode Sync Strip -->
            <div class="wt-live-controls">
              <button
                type="button"
                class="wt-primary-pill"
                onclick={handleOpenSyncedPlayer}
              >
                <MorphIcon icon={Play} size={15} fill="currentColor" strokeWidth={0} />
                <span>Open Player · EP {activeRoom.media.episode_ordinal || 1}</span>
              </button>

              {#if canControlActiveRoom}
                <button
                  type="button"
                  class="wt-control-chip"
                  onclick={() => void handleToggleRoomPlayback()}
                >
                  <MorphIcon
                    icon={activeRoom.playback.paused ? Play : Pause}
                    size={14}
                    fill="currentColor"
                    strokeWidth={0}
                    spring="snappy"
                  />
                  <span>{activeRoom.playback.paused ? 'Resume room' : 'Pause room'}</span>
                </button>

                <div class="wt-ep-stepper" role="group" aria-label="Episode selector">
                  <button
                    type="button"
                    class="ep-step-btn"
                    disabled={(activeRoom.media.episode_ordinal || 1) <= 1}
                    onclick={() => void handleStepEpisode(-1)}
                    aria-label="Previous episode"
                  >
                    <MorphIcon icon={Minus} size={13} strokeWidth={2.1} />
                  </button>
                  <span class="ep-step-label">EP {activeRoom.media.episode_ordinal || 1}</span>
                  <button
                    type="button"
                    class="ep-step-btn"
                    onclick={() => void handleStepEpisode(1)}
                    aria-label="Next episode"
                  >
                    <MorphIcon icon={Plus} size={13} strokeWidth={2.1} />
                  </button>
                </div>

                <button
                  type="button"
                  class="wt-control-chip is-subtle"
                  onclick={() => openSearch()}
                >
                  <MorphIcon icon={Search} size={13} strokeWidth={2} />
                  <span>Change anime</span>
                </button>
              {/if}
            </div>
          {/if}
        </div>

        {#if activeRoom}
          <!-- Sleek Glass Room Chat & Quick Reactions Panel inside Active Room -->
          <aside class="wt-chat-panel" aria-label="Room chat">
            <div class="chat-panel-head">
              <span class="chat-panel-title">
                <MorphIcon icon={MessageSquare} size={13} strokeWidth={2} />
                <span>Room Chat</span>
              </span>
              <span class="quic-badge" title="Connected over QUIC TLS 1.3">
                {watchTogetherState.transport === 'quic' ? 'QUIC' : 'LIVE'}
              </span>
            </div>

            <ScrollArea maxHeight={180} class="chat-scroll">
              {#if activeRoom.chat.length === 0}
                <div class="chat-empty">Say hi or drop a reaction while watching.</div>
              {:else}
                <div class="chat-messages">
                  {#each activeRoom.chat as msg (msg.id)}
                    {#if msg.kind === 'system'}
                      <div class="chat-line is-system">{msg.text}</div>
                    {:else}
                      <div class="chat-line" class:is-reaction={msg.kind === 'reaction'}>
                        <span class="chat-author">@{msg.sender_username}</span>
                        <span class="chat-text">{msg.text}</span>
                      </div>
                    {/if}
                  {/each}
                </div>
              {/if}
            </ScrollArea>

            <div class="chat-reactions-bar">
              {#each QUICK_REACTIONS as emoji (emoji)}
                <button
                  type="button"
                  class="reaction-chip"
                  onclick={() => void handleQuickReaction(emoji)}
                >
                  {emoji}
                </button>
              {/each}
            </div>

            <form class="chat-form" onsubmit={handleSendChat}>
              <input
                type="text"
                class="chat-input"
                bind:value={chatInput}
                placeholder="Message room..."
                maxlength={500}
              />
              <button type="submit" class="chat-send-btn" aria-label="Send message">
                <MorphIcon icon={Send} size={13} strokeWidth={2} />
              </button>
            </form>
          </aside>
        {/if}
      </div>
    {/if}
  </section>

  <!-- Right Vertical Friends Rail (exact 40px circular avatar strip with blue in_room / green online / gray offline badges) -->
  <aside class="wt-friends-rail" aria-label="Friends status rail">
    <button
      type="button"
      class="rail-add-btn"
      title="Friends & @username search"
      aria-label="Open friends popup"
      onclick={() => openFriendsPopup()}
    >
      <MorphIcon icon={UserPlus} size={16} strokeWidth={1.9} />
      {#if friendsState.incoming.length > 0}
        <span class="rail-req-dot"></span>
      {/if}
    </button>

    <div class="rail-avatars">
      {#each sortedFriends as friend (friend.public_id)}
        {@const isDraftInvited = watchTogetherState.draftInvitedIds.includes(friend.public_id)}
        {@const justInvited = invitedToastMap[friend.public_id]}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="rail-item"
          onmouseenter={() => (hoveredFriend = friend)}
          onmouseleave={() => {
            if (hoveredFriend?.public_id === friend.public_id) hoveredFriend = null;
          }}
        >
          <button
            type="button"
            class="rail-avatar-btn"
            class:is-offline={friend.status !== 'online' && friend.status !== 'in_room'}
            class:is-selected={isDraftInvited || justInvited}
            onclick={() => void handleFriendRailClick(friend)}
            aria-label="@{friend.username} ({friend.status || 'offline'})"
          >
            {#if friend.avatar_url}
              <img src={friend.avatar_url} alt={friend.name} class="rail-avatar-img" referrerpolicy="no-referrer" />
            {:else}
              <span class="rail-avatar-fallback">{(friend.name || 'U').slice(0, 1).toUpperCase()}</span>
            {/if}

            {#if friend.status === 'in_room'}
              <span class="rail-status-dot is-room" title="In a watch room">
                <span class="room-mini-play"></span>
              </span>
            {:else if friend.status === 'online'}
              <span class="rail-status-dot is-online" title="Online"></span>
            {:else}
              <span class="rail-status-dot is-offline" title="Offline"></span>
            {/if}
          </button>

          {#if hoveredFriend?.public_id === friend.public_id}
            <div class="rail-tooltip" transition:fly={{ x: 6, duration: 150, easing: cubicOut }}>
              <div class="tooltip-head">
                <span class="tooltip-name">{friend.name}</span>
                <span class="tooltip-handle">@{friend.username}</span>
              </div>
              {#if friend.status === 'in_room' && friend.room}
                <span class="tooltip-room">
                  In @{friend.room.host_username}’s room
                  {#if friend.room.anime_title}
                    · {friend.room.anime_title}
                  {/if}
                </span>
                <span class="tooltip-hint">Click to join room</span>
              {:else if friend.status === 'online'}
                <span class="tooltip-online">Online</span>
                <span class="tooltip-hint">
                  {activeRoom ? 'Click to invite to room' : isDraftInvited ? 'Invited to lobby' : 'Click to add to lobby'}
                </span>
              {:else}
                <span class="tooltip-offline">Offline</span>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
    <div class="rail-bottom-fade" aria-hidden="true"></div>
  </aside>

  <!-- Bottom Action & Settings Bar (clean Mila typography & borderless glass pills) -->
  <footer class="wt-bottom-bar">
    <div class="wt-bottom-left">
      {#if !activeRoom}
        <button
          type="button"
          class="wt-create-cta"
          disabled={actionBusy}
          onclick={() => void handleCreateRoom()}
        >
          {actionBusy ? 'Creating...' : 'Create'}
        </button>
      {:else}
        <button
          type="button"
          class="wt-leave-cta"
          onclick={() => void leaveWatchRoom()}
        >
          <MorphIcon icon={LogOut} size={16} strokeWidth={2} />
          <span>Leave room</span>
        </button>
      {/if}

      <button
        type="button"
        class="wt-gear-btn"
        class:is-open={settingsExpanded}
        aria-label="Room settings"
        title="Room settings"
        onclick={() => (settingsExpanded = !settingsExpanded)}
      >
        <MorphIcon icon={Settings} size={16} strokeWidth={1.9} />
      </button>

      <!-- Inline borderless Mila configuration pills -->
      <div class="wt-settings-strip" class:is-expanded={settingsExpanded || !activeRoom}>
        <button
          type="button"
          class="wt-setting-pill"
          disabled={Boolean(activeRoom && !amHost)}
          onclick={cycleVisibility}
          title="Room visibility"
        >
          {#if (activeRoom ? activeRoom.visibility : watchTogetherState.draftVisibility) === 'public'}
            <MorphIcon icon={Globe} size={13} strokeWidth={1.9} />
            <span>Public</span>
          {:else if (activeRoom ? activeRoom.visibility : watchTogetherState.draftVisibility) === 'private'}
            <MorphIcon icon={Lock} size={13} strokeWidth={1.9} />
            <span>Private</span>
          {:else}
            <MorphIcon icon={Users} size={13} strokeWidth={1.9} />
            <span>Friends</span>
          {/if}
        </button>

        <button
          type="button"
          class="wt-setting-pill"
          disabled={Boolean(activeRoom && !amHost)}
          onclick={cycleControlMode}
          title="Playback control permission"
        >
          <MorphIcon icon={Shield} size={13} strokeWidth={1.9} />
          <span>
            {(activeRoom ? activeRoom.control_mode : watchTogetherState.draftControlMode) === 'everyone'
              ? 'Everyone controls'
              : 'Host only'}
          </span>
        </button>

        <div class="wt-seats-pill" role="group" aria-label="Max room seats">
          <button
            type="button"
            class="seats-step"
            disabled={Boolean(activeRoom && !amHost) || maxSeats <= 2}
            onclick={() => stepMaxMembers(-1)}
            aria-label="Decrease seats"
          >
            <MorphIcon icon={Minus} size={12} strokeWidth={2.1} />
          </button>
          <span class="seats-label">{maxSeats} seats</span>
          <button
            type="button"
            class="seats-step"
            disabled={Boolean(activeRoom && !amHost) || maxSeats >= 10}
            onclick={() => stepMaxMembers(1)}
            aria-label="Increase seats"
          >
            <MorphIcon icon={Plus} size={12} strokeWidth={2.1} />
          </button>
        </div>

        {#if !activeRoom}
          {#if joinCodeOpen}
            <form class="wt-code-form" onsubmit={handleJoinByCode}>
              <!-- svelte-ignore a11y_autofocus -->
              <input
                type="text"
                class="wt-code-input"
                bind:value={joinCodeValue}
                placeholder="CODE"
                maxlength={8}
                autofocus
              />
              <button type="submit" class="wt-code-go" aria-label="Join by code">
                <MorphIcon icon={Check} size={13} strokeWidth={2.3} />
              </button>
              <button
                type="button"
                class="wt-code-go"
                onclick={() => (joinCodeOpen = false)}
                aria-label="Cancel"
              >
                <MorphIcon icon={X} size={13} strokeWidth={2} />
              </button>
            </form>
          {:else}
            <button
              type="button"
              class="wt-setting-pill is-ghost"
              onclick={() => (joinCodeOpen = true)}
            >
              <span>Join by code</span>
            </button>
          {/if}
        {/if}
      </div>
    </div>

    <!-- Incoming invites or friends actively in rooms -->
    <div class="wt-bottom-right">
      {#if watchTogetherState.invites.length > 0 && !activeRoom}
        {@const latestInv = watchTogetherState.invites[watchTogetherState.invites.length - 1]}
        <button
          type="button"
          class="wt-invite-banner"
          onclick={() => void joinWatchRoom({ room_id: latestInv.room_id, room_code: latestInv.room_code })}
        >
          <span class="inv-dot"></span>
          <span>
            <strong>@{latestInv.from_username}</strong> invited you{latestInv.anime_title ? ` · ${latestInv.anime_title}` : ''}
          </span>
          <span class="inv-join-tag">Join</span>
        </button>
      {:else if friendsInRooms.length > 0 && !activeRoom}
        {@const activeFriend = friendsInRooms[0]}
        <button
          type="button"
          class="wt-invite-banner is-friend-room"
          onclick={() => void joinFriendWatchRoom(activeFriend.public_id)}
        >
          <span class="inv-dot is-blue"></span>
          <span>
            <strong>@{activeFriend.username}</strong> is watching {activeFriend.room?.anime_title || 'in a room'}
          </span>
          <span class="inv-join-tag">Join</span>
        </button>
      {/if}
    </div>
  </footer>
</div>

<style>
  .wt-page {
    position: relative;
    width: 100%;
    min-height: 100vh;
    padding: calc(var(--header-height, 58px) + 18px) var(--page-pad-x, 38px) 34px var(--page-pad-x, 38px);
    box-sizing: border-box;
    background: var(--mila-app-canvas, #0c0c0c);
    color: var(--mila-text-primary, #ffffff);
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    overflow: hidden;
    user-select: none;
  }

  /* Full-bleed right-anchored artwork & dark vignette matching Prototype 3 */
  .wt-backdrop-wrap {
    position: absolute;
    inset: 0;
    pointer-events: none;
    z-index: 0;
    overflow: hidden;
  }

  .wt-backdrop-blur {
    position: absolute;
    inset: 0;
  }

  .wt-backdrop-art {
    position: absolute;
    top: 84px;
    right: 0;
    width: 68%;
    height: calc(100% - 84px);
    object-fit: cover;
    object-position: center 20%;
    opacity: 0.72;
    mask-image: linear-gradient(90deg, rgba(0, 0, 0, 0) 0%, rgba(0, 0, 0, 0.85) 36%, rgba(0, 0, 0, 1) 100%);
    -webkit-mask-image: linear-gradient(90deg, rgba(0, 0, 0, 0) 0%, rgba(0, 0, 0, 0.85) 36%, rgba(0, 0, 0, 1) 100%);
  }

  .wt-backdrop-scrim {
    position: absolute;
    inset: 0;
    background:
      linear-gradient(90deg, var(--mila-app-canvas, #0c0c0c) 18%, rgba(12, 12, 12, 0.72) 56%, rgba(12, 12, 12, 0.36) 86%, rgba(12, 12, 12, 0.86) 100%),
      linear-gradient(180deg, var(--mila-app-canvas, #0c0c0c) 0%, rgba(12, 12, 12, 0.25) 26%, rgba(12, 12, 12, 0.35) 74%, var(--mila-app-canvas, #0c0c0c) 100%);
  }

  /* Top bar: Create room + Name + 2/7 on the left, circular seats on the right */
  .wt-top-bar {
    position: relative;
    z-index: 3;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 20px;
    flex-wrap: wrap;
  }

  .wt-heading-group {
    display: flex;
    align-items: baseline;
    gap: 18px;
    min-width: 0;
    flex-wrap: wrap;
  }

  .wt-main-heading {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(30px, 3.5vw, 48px);
    font-weight: 700;
    line-height: 1.05;
    letter-spacing: -0.02em;
    color: var(--mila-text-primary, #ffffff);
  }

  .wt-name-trigger {
    display: inline-flex;
    align-items: baseline;
    gap: 10px;
    padding: 0;
    border: none;
    background: transparent;
    cursor: pointer;
    color: rgba(255, 255, 255, 0.24);
    transition: color 0.22s ease;
  }

  .wt-name-trigger:hover {
    color: rgba(255, 255, 255, 0.48);
  }

  .wt-room-name {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(28px, 3.3vw, 46px);
    font-weight: 700;
    line-height: 1.05;
    letter-spacing: -0.02em;
    max-width: 420px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .wt-name-trigger :global(.wt-pencil-icon) {
    opacity: 0.55;
    transform: translateY(-3px);
    transition: opacity 0.2s ease;
  }

  .wt-name-trigger:hover :global(.wt-pencil-icon) {
    opacity: 1;
  }

  .wt-title-form {
    display: inline-flex;
  }

  .wt-title-input {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(26px, 3.1vw, 42px);
    font-weight: 700;
    color: var(--mila-text-primary, #ffffff);
    background: transparent;
    border: none;
    border-bottom: 2px solid rgba(255, 255, 255, 0.22);
    outline: none;
    width: clamp(180px, 24vw, 340px);
    padding: 0 4px 2px 0;
  }

  .wt-seat-counter {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 20px;
    font-weight: 700;
    color: rgba(255, 255, 255, 0.24);
    transform: translateY(-2px);
  }

  .wt-code-chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 28px;
    padding: 0 10px;
    border: none;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.06);
    color: rgba(255, 255, 255, 0.72);
    font-family: 'Medeo Sans Display', monospace;
    font-size: 12.5px;
    font-weight: 700;
    letter-spacing: 0.04em;
    cursor: pointer;
    transform: translateY(-4px);
    transition: background-color 0.18s ease, color 0.18s ease;
  }

  .wt-code-chip:hover {
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
  }

  /* Circular member & empty person seats row on top right */
  .wt-slots-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .seat-circle {
    position: relative;
    width: 40px;
    height: 40px;
    border-radius: 50%;
    border: none;
    padding: 0;
    background: rgba(255, 255, 255, 0.045);
    color: rgba(255, 255, 255, 0.28);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    transition:
      background-color 0.2s ease,
      color 0.2s ease,
      transform 0.16s ease;
  }

  .seat-circle.is-empty,
  .seat-circle.is-invited {
    cursor: pointer;
  }

  .seat-circle.is-empty:hover {
    background: rgba(255, 255, 255, 0.09);
    color: rgba(255, 255, 255, 0.68);
    transform: translateY(-1px);
  }

  .seat-avatar {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    object-fit: cover;
    display: block;
  }

  .seat-avatar.is-dim,
  .seat-initials.is-dim {
    opacity: 0.34;
  }

  .seat-initials {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 14px;
    font-weight: 700;
    color: #ffffff;
  }

  .seat-badge {
    position: absolute;
    right: -1px;
    bottom: -1px;
    border-radius: 50%;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    box-shadow: 0 0 0 2px var(--mila-app-canvas, #0c0c0c);
  }

  .seat-badge.is-crown {
    width: 15px;
    height: 15px;
    background: #d2c852;
    color: #0b0b0c;
  }

  .seat-badge.is-online {
    width: 9px;
    height: 9px;
    right: 1px;
    bottom: 1px;
    background: #29eb36;
  }

  .seat-badge.is-online.is-buffering {
    background: #f5a623;
  }

  .seat-badge.is-clock {
    width: 14px;
    height: 14px;
    background: rgba(18, 18, 20, 0.92);
    color: #dbdbdb;
  }

  /* Center Stage */
  .wt-center-stage {
    position: relative;
    z-index: 2;
    flex: 1 1 auto;
    display: flex;
    align-items: center;
    padding: 24px 64px 24px 0;
  }

  .wt-choose-prompt {
    margin: auto;
    border: none;
    background: transparent;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    padding: 28px 42px;
    border-radius: 20px;
    transition: transform 0.24s var(--ease-out);
  }

  .wt-choose-prompt:hover {
    transform: scale(1.02);
  }

  .choose-heading {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(34px, 3.8vw, 48px);
    font-weight: 700;
    line-height: 1.08;
    color: rgba(255, 255, 255, 0.22);
    transition: color 0.22s ease;
  }

  .wt-choose-prompt:hover .choose-heading {
    color: rgba(255, 255, 255, 0.42);
  }

  .choose-sub {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    color: rgba(255, 255, 255, 0.13);
  }

  /* Selected Anime Showcase (Prototype 3 typography) + Active Room layout */
  .wt-anime-showcase {
    width: 100%;
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 32px;
    flex-wrap: wrap;
  }

  .wt-anime-copy {
    flex: 1 1 560px;
    max-width: 920px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .wt-anime-title {
    margin: 0;
    font-family: 'PP Monument Extended', 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(32px, 4.4vw, 62px);
    font-weight: 900;
    line-height: 1.05;
    letter-spacing: -0.02em;
    color: #dadada;
    display: -webkit-box;
    line-clamp: 2;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .wt-anime-desc {
    margin: 0;
    max-width: 820px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: clamp(15px, 1.4vw, 20px);
    line-height: 1.42;
    color: #8c8c8c;
    display: -webkit-box;
    line-clamp: 4;
    -webkit-line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .wt-live-controls {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
    margin-top: 8px;
  }

  .wt-primary-pill {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    height: 38px;
    padding: 0 18px;
    border: none;
    border-radius: 11px;
    background: var(--mila-accent, #d9ff70);
    color: #0b0b0c;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 13.5px;
    font-weight: 700;
    cursor: pointer;
    transition: transform 0.14s ease, filter 0.18s ease;
  }

  .wt-primary-pill:hover {
    filter: brightness(1.06);
  }

  .wt-primary-pill:active {
    transform: scale(0.97);
  }

  .wt-control-chip {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 38px;
    padding: 0 15px;
    border: none;
    border-radius: 11px;
    background: rgba(255, 255, 255, 0.08);
    backdrop-filter: blur(12px);
    color: #ffffff;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13.5px;
    font-weight: 600;
    cursor: pointer;
    transition: background-color 0.18s ease;
  }

  .wt-control-chip:hover {
    background: rgba(255, 255, 255, 0.14);
  }

  .wt-control-chip.is-subtle {
    background: rgba(255, 255, 255, 0.045);
    color: rgba(255, 255, 255, 0.68);
  }

  .wt-ep-stepper {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    height: 38px;
    padding: 0 6px;
    border-radius: 11px;
    background: rgba(255, 255, 255, 0.06);
  }

  .ep-step-btn {
    width: 26px;
    height: 26px;
    border: none;
    border-radius: 7px;
    background: transparent;
    color: rgba(255, 255, 255, 0.72);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
  }

  .ep-step-btn:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .ep-step-btn:disabled {
    opacity: 0.3;
    cursor: default;
  }

  .ep-step-label {
    padding: 0 8px;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 700;
    color: #ffffff;
  }

  /* Active Room Glass Chat Panel */
  .wt-chat-panel {
    width: min(330px, 100%);
    border-radius: 16px;
    background: rgba(14, 14, 17, 0.76);
    backdrop-filter: blur(18px);
    -webkit-backdrop-filter: blur(18px);
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .chat-panel-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .chat-panel-title {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 700;
    color: rgba(255, 255, 255, 0.86);
  }

  .quic-badge {
    padding: 2px 7px;
    border-radius: 6px;
    background: var(--mila-accent-muted, rgba(217, 255, 112, 0.14));
    color: var(--mila-accent, #d9ff70);
    font-family: 'Medeo Sans Display', monospace;
    font-size: 10.5px;
    font-weight: 700;
    letter-spacing: 0.05em;
  }

  .chat-empty {
    padding: 22px 8px;
    text-align: center;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
    color: rgba(255, 255, 255, 0.34);
  }

  .chat-messages {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .chat-line {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    line-height: 1.38;
    color: rgba(255, 255, 255, 0.86);
    word-break: break-word;
  }

  .chat-line.is-system {
    font-size: 11.5px;
    color: rgba(255, 255, 255, 0.36);
  }

  .chat-author {
    font-weight: 700;
    color: var(--mila-accent, #d9ff70);
    margin-right: 6px;
  }

  .chat-reactions-bar {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .reaction-chip {
    flex: 1 1 0;
    height: 28px;
    border: none;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.045);
    cursor: pointer;
    font-size: 14px;
    transition: background-color 0.16s ease, transform 0.12s ease;
  }

  .reaction-chip:hover {
    background: rgba(255, 255, 255, 0.1);
    transform: translateY(-1px);
  }

  .chat-form {
    display: flex;
    align-items: center;
    gap: 6px;
    background: rgba(255, 255, 255, 0.05);
    border-radius: 10px;
    padding: 4px 6px 4px 11px;
  }

  .chat-input {
    flex: 1 1 auto;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: #ffffff;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
  }

  .chat-send-btn {
    width: 28px;
    height: 28px;
    border: none;
    border-radius: 7px;
    background: rgba(255, 255, 255, 0.08);
    color: #ffffff;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
  }

  .chat-send-btn:hover {
    background: var(--mila-accent, #d9ff70);
    color: #0b0b0c;
  }

  /* Right Vertical Friends Rail (matching all 3 Figma prototypes) */
  .wt-friends-rail {
    position: fixed;
    right: var(--page-pad-x, 38px);
    bottom: 34px;
    z-index: 6;
    width: 40px;
    max-height: 390px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 13px;
  }

  .rail-add-btn {
    position: relative;
    width: 40px;
    height: 40px;
    border-radius: 50%;
    border: none;
    background: rgba(255, 255, 255, 0.055);
    color: rgba(255, 255, 255, 0.65);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    cursor: pointer;
    transition: background-color 0.2s ease, color 0.2s ease, transform 0.16s ease;
  }

  .rail-add-btn:hover {
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
    transform: scale(1.04);
  }

  .rail-req-dot {
    position: absolute;
    top: 1px;
    right: 1px;
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--mila-accent, #d9ff70);
    box-shadow: 0 0 0 2px var(--mila-app-canvas, #0c0c0c);
  }

  .rail-avatars {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 13px;
    overflow-y: auto;
    scrollbar-width: none;
    padding-bottom: 18px;
  }

  .rail-avatars::-webkit-scrollbar {
    display: none;
  }

  .rail-item {
    position: relative;
    width: 40px;
    height: 40px;
    flex-shrink: 0;
  }

  .rail-avatar-btn {
    position: relative;
    width: 40px;
    height: 40px;
    border-radius: 50%;
    border: none;
    padding: 0;
    background: rgba(255, 255, 255, 0.06);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: transform 0.16s ease, opacity 0.2s ease;
  }

  .rail-avatar-btn:hover {
    transform: scale(1.06);
  }

  .rail-avatar-btn.is-offline .rail-avatar-img {
    opacity: 0.48;
  }

  .rail-avatar-btn.is-selected {
    box-shadow: 0 0 0 2px var(--mila-accent, #d9ff70);
  }

  .rail-avatar-img {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    object-fit: cover;
    display: block;
  }

  .rail-avatar-fallback {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 14px;
    font-weight: 700;
    color: #ffffff;
  }

  /* Exact status dots from Figma prototypes: blue (#294aeb) for in_room, green (#29eb36) for online, gray (#373737) for offline */
  .rail-status-dot {
    position: absolute;
    right: 1px;
    bottom: 1px;
    width: 9px;
    height: 9px;
    border-radius: 50%;
    box-shadow: 0 0 0 2px var(--mila-app-canvas, #0c0c0c);
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .rail-status-dot.is-room {
    width: 11px;
    height: 11px;
    right: 0;
    bottom: 0;
    background: #294aeb;
  }

  .room-mini-play {
    width: 0;
    height: 0;
    border-top: 2.5px solid transparent;
    border-bottom: 2.5px solid transparent;
    border-left: 4px solid #ffffff;
    margin-left: 1px;
  }

  .rail-status-dot.is-online {
    background: #29eb36;
  }

  .rail-status-dot.is-offline {
    background: #373737;
  }

  .rail-bottom-fade {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 36px;
    pointer-events: none;
    background: linear-gradient(180deg, rgba(12, 12, 12, 0) 0%, var(--mila-app-canvas, #0c0c0c) 100%);
  }

  .rail-tooltip {
    position: absolute;
    right: 52px;
    top: 50%;
    transform: translateY(-50%);
    min-width: 185px;
    max-width: 240px;
    padding: 9px 12px;
    border-radius: 12px;
    background: rgba(14, 14, 18, 0.96);
    backdrop-filter: blur(14px);
    box-shadow: 0 14px 36px rgba(0, 0, 0, 0.65);
    display: flex;
    flex-direction: column;
    gap: 3px;
    pointer-events: none;
    white-space: nowrap;
  }

  .tooltip-head {
    display: flex;
    align-items: baseline;
    gap: 6px;
  }

  .tooltip-name {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 700;
    color: #ffffff;
  }

  .tooltip-handle {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
    color: rgba(255, 255, 255, 0.42);
  }

  .tooltip-room {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
    color: #7d96ff;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .tooltip-online {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
    color: #29eb36;
  }

  .tooltip-offline {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
    color: rgba(255, 255, 255, 0.36);
  }

  .tooltip-hint {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11px;
    color: rgba(255, 255, 255, 0.48);
  }

  /* Bottom Bar: Typographic "Create" + gear + borderless Mila setting pills */
  .wt-bottom-bar {
    position: relative;
    z-index: 3;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    flex-wrap: wrap;
    padding-right: 58px;
  }

  .wt-bottom-left {
    display: flex;
    align-items: center;
    gap: 16px;
    flex-wrap: wrap;
  }

  .wt-create-cta {
    padding: 0;
    border: none;
    background: transparent;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(32px, 3.4vw, 48px);
    font-weight: 700;
    line-height: 1;
    letter-spacing: -0.02em;
    color: #e4e4e4;
    cursor: pointer;
    transition: color 0.2s ease, transform 0.14s ease;
  }

  .wt-create-cta:hover:not(:disabled) {
    color: #ffffff;
  }

  .wt-create-cta:active:not(:disabled) {
    transform: scale(0.97);
  }

  .wt-leave-cta {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 34px;
    padding: 0 14px;
    border: none;
    border-radius: 10px;
    background: rgba(255, 107, 107, 0.12);
    color: #ff8282;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
    transition: background-color 0.18s ease, color 0.18s ease;
  }

  .wt-leave-cta:hover {
    background: rgba(255, 107, 107, 0.22);
    color: #ffffff;
  }

  .wt-gear-btn {
    width: 30px;
    height: 30px;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: rgba(231, 231, 231, 0.26);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transform: translateY(4px);
    transition: color 0.2s ease, background-color 0.2s ease;
  }

  .wt-gear-btn:hover,
  .wt-gear-btn.is-open {
    color: rgba(255, 255, 255, 0.78);
    background: rgba(255, 255, 255, 0.05);
  }

  .wt-settings-strip {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    transform: translateY(3px);
  }

  .wt-setting-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 32px;
    padding: 0 12px;
    border: none;
    border-radius: 9px;
    background: rgba(255, 255, 255, 0.048);
    color: rgba(255, 255, 255, 0.68);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 600;
    cursor: pointer;
    transition: background-color 0.18s ease, color 0.18s ease;
  }

  .wt-setting-pill:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.095);
    color: #ffffff;
  }

  .wt-setting-pill.is-ghost {
    background: rgba(255, 255, 255, 0.032);
    color: rgba(255, 255, 255, 0.46);
  }

  .wt-seats-pill {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    height: 32px;
    padding: 0 5px;
    border-radius: 9px;
    background: rgba(255, 255, 255, 0.048);
  }

  .seats-step {
    width: 22px;
    height: 22px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: rgba(255, 255, 255, 0.62);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
  }

  .seats-step:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.1);
    color: #ffffff;
  }

  .seats-step:disabled {
    opacity: 0.28;
    cursor: default;
  }

  .seats-label {
    padding: 0 6px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 600;
    color: rgba(255, 255, 255, 0.74);
  }

  .wt-code-form {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    height: 32px;
    padding: 0 6px 0 10px;
    border-radius: 9px;
    background: rgba(255, 255, 255, 0.07);
  }

  .wt-code-input {
    width: 68px;
    border: none;
    outline: none;
    background: transparent;
    color: #ffffff;
    font-family: 'Medeo Sans Display', monospace;
    font-size: 12.5px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .wt-code-go {
    width: 22px;
    height: 22px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: rgba(255, 255, 255, 0.72);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
  }

  .wt-code-go:hover {
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
  }

  /* Invite / Active Friend Room Banner */
  .wt-invite-banner {
    display: inline-flex;
    align-items: center;
    gap: 9px;
    height: 34px;
    padding: 0 12px;
    border: none;
    border-radius: 10px;
    background: rgba(255, 255, 255, 0.06);
    backdrop-filter: blur(12px);
    color: rgba(255, 255, 255, 0.85);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
    cursor: pointer;
    transition: background-color 0.18s ease, transform 0.14s ease;
  }

  .wt-invite-banner:hover {
    background: rgba(255, 255, 255, 0.11);
    transform: translateY(-1px);
  }

  .inv-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--mila-accent, #d9ff70);
  }

  .inv-dot.is-blue {
    background: #294aeb;
  }

  .inv-join-tag {
    padding: 2px 8px;
    border-radius: 6px;
    background: var(--mila-accent, #d9ff70);
    color: #0b0b0c;
    font-weight: 700;
    font-size: 11.5px;
  }

  .wt-invite-banner.is-friend-room .inv-join-tag {
    background: #294aeb;
    color: #ffffff;
  }
</style>
