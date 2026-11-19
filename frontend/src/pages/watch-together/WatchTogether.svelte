<script lang="ts">
  import { onMount } from 'svelte';
  import { cubicOut } from 'svelte/easing';
  import { MorphIcon } from 'morphicons/svelte';
  import {
    Clock,
    Crown,
    LogOut,
    Pencil,
    Play,
    Settings,
    User,
    UserPlus
  } from 'lucide';

  import LayerBlur from '../../marlakit/blur/LayerBlur.svelte';
  import MainButton from '../../marlakit/button/MainButton.svelte';
  import SecondButton from '../../marlakit/button/SecondButton.svelte';
  import ComboBox from '../../marlakit/combo/ComboBox.svelte';
  import MetaLine from '../../marlakit/meta/MetaLine.svelte';
  import type { ComboSection } from '../../marlakit/combo/types';
  import { portal } from '../../marlakit/portal';
  import { ensureShikimoriArtwork, getCachedArtwork } from '../../api/anime';
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
    addDraftInviteFriend,
    cancelWatchRoomInvite,
    connectWatchTogether,
    createWatchRoom,
    inviteFriendToWatchRoom,
    joinFriendWatchRoom,
    kickWatchRoomMember,
    leaveWatchRoom,
    selectAnimeForWatchTogether,
    transferWatchRoomHost,
    updateWatchRoomSettings,
    watchTogetherState
  } from '../../app/watchTogether.svelte';

  let editingTitle = $state(false);
  let titleInput = $state('');
  let busy = $state(false);
  let brokenAvatars = $state<Record<string, boolean>>({});

  let settingsMenuOpen = $state(false);

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

  // Selected anime: inside an active room ALWAYS mirror activeRoom.media so all members see the exact same anime.
  // Outside a room (lobby draft), fall back: draftAnime > current player > most recent watchHistory.
  const selectedAnime = $derived.by(() => {
    if (activeRoom) {
      if (!activeRoom.media?.title) {
        return null;
      }
      const shikiId = activeRoom.media.shiki_id;
      const art = shikiId ? getCachedArtwork(shikiId) : undefined;
      const draftMatch =
        watchTogetherState.draftAnime &&
        ((shikiId && watchTogetherState.draftAnime.shikiId === shikiId) ||
          watchTogetherState.draftAnime.title === activeRoom.media.title)
          ? watchTogetherState.draftAnime
          : null;
      const poster =
        activeRoom.media.poster ||
        draftMatch?.poster ||
        art?.posterOriginal ||
        art?.posterMain ||
        '';
      const backdrop = art?.screenshots?.[0] || draftMatch?.backdrop || poster;
      const description = art?.description || draftMatch?.description || '';
      return {
        title: activeRoom.media.title,
        description,
        poster,
        backdrop,
        shikiId,
        releaseAlias: activeRoom.media.release_alias || draftMatch?.releaseAlias,
        score: art?.score ?? draftMatch?.score,
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
          player.shikiArt?.screenshots?.[0] || player.release?.banner || poster;
        const description = player.shikiArt?.description || '';
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
      return activeRoom.title || 'Room';
    }
    if (watchTogetherState.draftTitle.trim()) {
      return watchTogetherState.draftTitle.trim();
    }
    if (auth.user?.username) {
      return `${auth.user.username}'s room`;
    }
    return 'Room';
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

  const settingsComboSections = $derived.by<ComboSection[]>(() => {
    const visLabel =
      currentVisibility === 'public'
        ? 'Public'
        : currentVisibility === 'private'
          ? 'Private'
          : 'Friends';
    const ctrlLabel = currentControlMode === 'everyone' ? 'Everyone controls' : 'Host only';

    return [
      {
        id: 'room-settings',
        title: 'Room settings',
        options: [
          {
            value: 'cat:visibility',
            label: 'Visibility',
            sublabel: visLabel,
            subMinWidth: 192,
            children: [
              {
                value: 'vis:public',
                label: 'Public',
                sublabel: 'Anyone can join',
                selected: currentVisibility === 'public'
              },
              {
                value: 'vis:friends',
                label: 'Friends',
                sublabel: 'Friends can join',
                selected: currentVisibility === 'friends'
              },
              {
                value: 'vis:private',
                label: 'Private',
                sublabel: 'Invite only',
                selected: currentVisibility === 'private'
              }
            ]
          },
          {
            value: 'cat:control',
            label: 'Playback control',
            sublabel: ctrlLabel,
            subMinWidth: 214,
            children: [
              {
                value: 'ctrl:host_only',
                label: 'Host only',
                sublabel: 'Only host controls playback',
                selected: currentControlMode === 'host_only'
              },
              {
                value: 'ctrl:everyone',
                label: 'Everyone controls',
                sublabel: 'All members can pause & seek',
                selected: currentControlMode === 'everyone'
              }
            ]
          },
          {
            value: 'cat:seats',
            label: 'Seats',
            sublabel: `${currentMaxMembers} seats`,
            subMinWidth: 146,
            children: [2, 3, 4, 5, 6, 7, 8, 9, 10].map((n) => ({
              value: `seats:${n}`,
              label: `${n} seats`,
              selected: currentMaxMembers === n
            }))
          }
        ]
      }
    ];
  });

  function handleSettingsSelect(rawValue: string | number) {
    const val = String(rawValue);
    if (val.startsWith('vis:')) {
      void selectVisibility(val.slice('vis:'.length) as RoomVisibility);
    } else if (val.startsWith('ctrl:')) {
      void selectControlMode(val.slice('ctrl:'.length) as RoomControlMode);
    } else if (val.startsWith('seats:')) {
      const n = Number(val.slice('seats:'.length));
      if (Number.isFinite(n)) {
        void selectMaxMembers(n);
      }
    }
  }

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

    if (out.length < max) {
      const remaining = max - out.length;
      out.push({
        key: `empty-${remaining}`,
        kind: 'empty',
        name: `${remaining} open ${remaining === 1 ? 'seat' : 'seats'}`,
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
    titleInput = activeRoom ? activeRoom.title : watchTogetherState.draftTitle || displayRoomName;
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

  $effect(() => {
    const shikiId = selectedAnime?.shikiId;
    if (shikiId && shikiId > 0) {
      void ensureShikimoriArtwork([shikiId]);
    }
  });

  async function handleCreateRoom() {
    if (!auth.user) {
      await loginWithGoogle();
      return;
    }
    if (busy || activeRoom) return;
    busy = true;
    try {
      const anime = selectedAnime;
      await createWatchRoom({
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
      });

      if (watchTogetherState.draftInvitedIds.length > 0) {
        const toInvite = [...watchTogetherState.draftInvitedIds];
        watchTogetherState.draftInvitedIds = [];
        for (const friendId of toInvite) {
          void inviteFriendToWatchRoom(friendId).catch(() => {});
        }
      }
      // Stay on Room view so members can see invitations, chat, settings, and click Watch when ready.
    } finally {
      busy = false;
    }
  }

  async function handleEnterPlayer() {
    if (!auth.user) {
      await loginWithGoogle();
      return;
    }
    if (busy || !activeRoom) return;
    busy = true;
    try {
      const media = activeRoom.media;
      const anime = selectedAnime;
      if (media?.title) {
        void openWatch(
          {
            title: media.title,
            shikiId: media.shiki_id,
            releaseAlias: media.release_alias,
            searchQueries: media.search_queries ?? [media.title]
          },
          {
            targetPage: 'watch-together-player',
            autoPlay: !(activeRoom.playback?.paused ?? true),
            episodeOrdinal: media.episode_ordinal || 1,
            resumeTime: activeRoom.playback?.position_sec || 0
          }
        );
      } else if (anime && isHost) {
        await selectAnimeForWatchTogether(anime);
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
            episodeOrdinal: 1,
            resumeTime: 0
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
      x: Math.max(16, rect.left - 210),
      y: Math.max(16, Math.min(window.innerHeight - 170, rect.top - 8)),
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
        x: Math.max(16, Math.min(window.innerWidth - 216, rect.right - 196)),
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
        x: Math.max(16, Math.min(window.innerWidth - 216, rect.right - 196)),
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

  function ctxTransition(_node: Element) {
    return {
      duration: 210,
      easing: cubicOut,
      css: (t: number) => {
        const inv = 1 - t;
        const scale = 0.94 + t * 0.06;
        const y = inv * -6;
        const blur = inv * 6;
        return `opacity: ${t}; transform: translate3d(0, ${y}px, 0) scale(${scale}); filter: blur(${blur}px);`;
      }
    };
  }

  async function handleCtxInvite(targetId: string) {
    ctxMenu = null;
    if (!activeRoom) {
      // Stage locally ONLY before clicking Create (do not create room or notify friend yet)
      addDraftInviteFriend(targetId);
      return;
    }
    try {
      await inviteFriendToWatchRoom(targetId);
    } catch {
      addDraftInviteFriend(targetId);
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
      await joinFriendWatchRoom(targetId);
      // Stay in the WatchTogether Room lobby so user clicks Watch explicitly
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
    <!-- TOP BAR: Create room / Room + Name + pencil + seat count on left, Member/Invite circles on right -->
    <header class="wt-top-bar">
      <div class="wt-heading-group">
        <h1 class="wt-main-heading">
          {activeRoom ? 'Room' : 'Create room'}
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
              <span class="wt-pencil-wrap">
                <MorphIcon icon={Pencil} size={15} strokeWidth={1.8} class="wt-pencil-icon" />
              </span>
            {/if}
          </button>
        {/if}

        <span class="wt-seat-counter">{occupiedCount}/{currentMaxMembers}</span>
      </div>

      <!-- Top-right lobby member & pending invite slots (right-click ONLY on joined/pending) -->
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
                    : slot.name}
              onclick={() => {
                if (slot.kind === 'empty' && auth.user) {
                  openFriendsPopup();
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

    <!-- CENTER STAGE: Hero-like Anime Title + Description (lowered nicely above bottom controls) OR Choose Anime prompt -->
    <div class="wt-center-stage" class:has-anime={Boolean(selectedAnime)}>
      {#if selectedAnime}
        {@const metaTags = [...selectedAnime.tags, selectedAnime.episodeLabel].filter(Boolean)}
        <div
          class="wt-anime-showcase"
          in:revealBlur={{ duration: 280, y: 10, scale: 0.99, blur: 8 }}
        >
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
          <h2
            class="wt-anime-title"
            class:is-clickable={!activeRoom || isHost}
            title={!activeRoom || isHost ? 'Click to choose another anime' : selectedAnime.title}
            onclick={() => {
              if (!activeRoom || isHost) openSearch();
            }}
          >
            {selectedAnime.title}
          </h2>

          {#if (selectedAnime.score && selectedAnime.score > 0) || metaTags.length > 0}
            <div class="wt-anime-meta">
              <MetaLine score={selectedAnime.score} tags={metaTags} />
            </div>
          {/if}

          {#if selectedAnime.description}
            <p class="wt-anime-desc">{selectedAnime.description}</p>
          {/if}
        </div>
      {:else if activeRoom && !isHost}
        <div
          class="wt-choose-prompt is-waiting"
          in:revealBlur={{ duration: 260, scale: 0.97, blur: 8 }}
        >
          <span class="wt-choose-title">Waiting for host</span>
          <span class="wt-choose-sub">Host has not picked a title yet</span>
        </div>
      {:else}
        <button
          type="button"
          class="wt-choose-prompt"
          onclick={() => openSearch()}
          in:revealBlur={{ duration: 260, scale: 0.97, blur: 8 }}
        >
          <span class="wt-choose-title">Choose anime</span>
          <span class="wt-choose-sub">Search for a title to watch together</span>
        </button>
      {/if}
    </div>

    <!-- BOTTOM BAR: Create / Watch + Leave room + Settings Gear ComboBox (identical to anime player selector popup) -->
    <footer class="wt-bottom-bar">
      <div class="wt-bottom-left">
        {#if !activeRoom}
          <MainButton loading={busy} onclick={() => void handleCreateRoom()}>
            Create
          </MainButton>
        {:else}
          <MainButton
            loading={busy}
            disabled={!activeRoom.media?.title && !selectedAnime}
            onclick={() => void handleEnterPlayer()}
          >
            {#snippet icon()}
              <MorphIcon icon={Play} size={15} fill="currentColor" strokeWidth={0} />
            {/snippet}
            Watch
          </MainButton>

          <SecondButton onclick={() => void leaveWatchRoom()}>
            {#snippet icon()}
              <MorphIcon icon={LogOut} size={15} strokeWidth={1.9} />
            {/snippet}
            Leave room
          </SecondButton>
        {/if}

        <ComboBox
          bind:open={settingsMenuOpen}
          sections={settingsComboSections}
          ariaLabel="Room settings"
          placement="top-left"
          minWidth={212}
          disabled={Boolean(activeRoom && !isHost)}
          onselect={(opt) => handleSettingsSelect(opt.value)}
        >
          {#snippet trigger({ open, toggle })}
            <button
              type="button"
              class="wt-gear-btn"
              class:is-open={open}
              disabled={Boolean(activeRoom && !isHost)}
              aria-expanded={open}
              aria-label="Room settings"
              title="Room settings"
              onclick={toggle}
            >
              <span class="wt-gear-icon" class:is-spun={open}>
                <MorphIcon icon={Settings} size={17} strokeWidth={1.9} />
              </span>
            </button>
          {/snippet}
        </ComboBox>
      </div>
    </footer>

    <!-- RIGHT-SIDE VERTICAL FRIENDS RAIL (flush bottom-right, right-click ONLY opens ComboBox, left-click does nothing) -->
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

<!-- Right-click Marlakit ComboBox-identical Context Menu -->
{#if ctxMenu}
  {@const menu = ctxMenu}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    use:portal
    class="wt-kit-combo-layer"
    style:left="{menu.x}px"
    style:top="{menu.y}px"
    onclick={(e) => e.stopPropagation()}
    oncontextmenu={(e) => e.preventDefault()}
    transition:ctxTransition
  >
    <div class="wt-kit-combo-popover" role="listbox">
      <div class="wt-kit-combo-section-label">
        <span class="wt-kit-combo-user-line">
          <span>{menu.name} · @{menu.username}</span>
          <span
            class="wt-kit-status-dot"
            class:is-online={menu.statusKind === 'online'}
            class:is-room={menu.statusKind === 'in_room'}
            class:is-pending={menu.statusKind === 'pending'}
          ></span>
        </span>
        <span class="wt-kit-combo-sub-line">{menu.statusLabel}</span>
      </div>

      <div class="wt-kit-combo-list">
        {#if menu.kind === 'friend'}
          {#if menu.alreadyInvited}
            <button
              type="button"
              role="option"
              aria-selected={false}
              class="wt-kit-combo-item is-danger"
              onclick={() => void handleCtxCancelInvite(menu.id)}
            >
              <span class="wt-kit-combo-label">Cancel invite</span>
            </button>
          {:else}
            <button
              type="button"
              role="option"
              aria-selected={false}
              class="wt-kit-combo-item"
              onclick={() => void handleCtxInvite(menu.id)}
            >
              <span class="wt-kit-combo-label">Invite</span>
            </button>
          {/if}

          {#if menu.canJoinRoom}
            <button
              type="button"
              role="option"
              aria-selected={false}
              class="wt-kit-combo-item"
              onclick={() => void handleCtxJoinFriend(menu.id)}
            >
              <span class="wt-kit-combo-label">Join room</span>
            </button>
          {/if}
        {:else if menu.kind === 'slot-invited'}
          <button
            type="button"
            role="option"
            aria-selected={false}
            class="wt-kit-combo-item is-danger"
            onclick={() => void handleCtxCancelInvite(menu.id)}
          >
            <span class="wt-kit-combo-label">Cancel</span>
          </button>
        {:else if menu.kind === 'slot-member'}
          <button
            type="button"
            role="option"
            aria-selected={false}
            class="wt-kit-combo-item"
            onclick={() => void handleCtxTransferHost(menu.id)}
          >
            <span class="wt-kit-combo-label">Transfer host</span>
          </button>
          <button
            type="button"
            role="option"
            aria-selected={false}
            class="wt-kit-combo-item is-danger"
            onclick={() => void handleCtxKickMember(menu.id)}
          >
            <span class="wt-kit-combo-label">Kick</span>
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .wt-stage {
    position: relative;
    width: 100%;
    height: 100vh;
    min-height: 0;
    overflow: hidden;
    background: var(--mila-app-canvas, #0c0c0c);
    color: #ffffff;
    box-sizing: border-box;
    user-select: none;
    --mila-buttons-mainBg: var(--mila-media-mainBtnBg, rgba(255, 255, 255, 0.96));
    --mila-buttons-mainText: var(--mila-media-mainBtnText, #111111);
    --mila-buttons-secondBg: var(--mila-media-secondBtnBg, rgba(255, 255, 255, 0.08));
    --mila-buttons-secondText: var(--mila-media-secondBtnText, #ffffff);
    --mila-meta-tag: var(--mila-media-subtext, rgba(255, 255, 255, 0.56));
    --mila-meta-dot: var(--mila-media-muted, rgba(255, 255, 255, 0.36));
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
    opacity: 0.36;
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
        rgba(12, 12, 12, 0.24) 48%,
        rgba(12, 12, 12, 0.78) 84%,
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
    padding: calc(var(--header-height, 48px) + 14px) var(--page-pad-x, 16px) 12px
      var(--page-pad-x, 16px);
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
  }

  /* TOP BAR — responsive adaptive alignment */
  .wt-top-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 20px;
    flex-wrap: wrap;
    min-height: 40px;
  }

  .wt-heading-group {
    display: inline-flex;
    align-items: center;
    gap: clamp(10px, 1.1vw, 15px);
    flex-wrap: wrap;
    min-width: 0;
  }

  .wt-main-heading {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(24px, 2.5vw, 34px);
    font-weight: 800;
    letter-spacing: -0.03em;
    line-height: 1.05;
    color: #ffffff;
    display: inline-flex;
    align-items: center;
  }

  .wt-name-trigger {
    display: inline-flex;
    align-items: center;
    gap: 9px;
    padding: 0;
    border: none;
    background: transparent;
    color: rgba(255, 255, 255, 0.34);
    cursor: pointer;
    transition: color 0.18s ease;
  }

  .wt-name-trigger:hover {
    color: rgba(255, 255, 255, 0.68);
  }

  .wt-room-name {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(22px, 2.3vw, 32px);
    font-weight: 800;
    letter-spacing: -0.025em;
    line-height: 1.05;
    max-width: min(360px, 38vw);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .wt-pencil-wrap {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transform: translateY(2px);
    opacity: 0.52;
    transition: opacity 0.18s ease;
  }

  .wt-name-trigger:hover .wt-pencil-wrap {
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
    font-size: clamp(22px, 2.3vw, 32px);
    font-weight: 800;
    letter-spacing: -0.025em;
    line-height: 1.05;
    padding: 0 2px;
  }

  .wt-seat-counter {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(13.5px, 1.15vw, 17px);
    font-weight: 700;
    line-height: 1;
    color: rgba(255, 255, 255, 0.28);
    letter-spacing: -0.01em;
    display: inline-flex;
    align-items: center;
    transform: translateY(2px);
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
    background: rgba(255, 255, 255, 0.055);
    color: rgba(255, 255, 255, 0.28);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: default;
    transition:
      filter 0.18s ease,
      background-color 0.18s ease,
      color 0.18s ease;
  }

  .wt-slot-circle.is-invited,
  .wt-slot-circle.is-member {
    cursor: context-menu;
  }

  .wt-slot-circle:hover {
    filter: brightness(1.14);
    background: rgba(255, 255, 255, 0.1);
    color: rgba(255, 255, 255, 0.6);
  }

  .wt-slot-circle.is-invited {
    opacity: 0.62;
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

  .wt-slot-circle.is-empty {
    background: rgba(255, 255, 255, 0.04);
    border: none;
    color: rgba(255, 255, 255, 0.26);
    cursor: pointer;
  }

  .wt-slot-circle.is-empty:hover {
    background: rgba(255, 255, 255, 0.09);
    color: rgba(255, 255, 255, 0.65);
  }

  .wt-slot-badge.is-crown {
    background: #e5c158;
    color: #0c0c0c;
  }

  .wt-slot-badge.is-joined {
    width: 9px;
    height: 9px;
    right: 0;
    bottom: 0;
    background: #34d399;
  }

  .wt-slot-badge.is-pending {
    background: #1c1c1e;
    color: rgba(255, 255, 255, 0.78);
  }

  /* CENTER STAGE — positioned cleanly just above the bottom CTA bar (Hero-style!) */
  .wt-center-stage {
    position: relative;
    flex: 1 1 auto;
    display: flex;
    align-items: center;
    min-height: 0;
  }

  .wt-center-stage.has-anime {
    align-items: flex-end;
    padding-right: 68px;
    padding-bottom: 22px;
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

  .wt-choose-prompt.is-waiting {
    cursor: default;
  }

  .wt-choose-prompt:not(.is-waiting):hover {
    transform: scale(1.015);
  }

  .wt-choose-title {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(32px, 3.8vw, 48px);
    font-weight: 800;
    letter-spacing: -0.03em;
    color: rgba(255, 255, 255, 0.24);
    transition: color 0.22s ease;
  }

  .wt-choose-prompt:not(.is-waiting):hover .wt-choose-title {
    color: rgba(255, 255, 255, 0.44);
  }

  .wt-choose-sub {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    color: rgba(255, 255, 255, 0.34);
  }

  /* Hero-matched Anime Showcase (title + MetaLine + description) */
  .wt-anime-showcase {
    max-width: min(920px, 78vw);
    display: flex;
    flex-direction: column;
    align-items: flex-start;
  }

  .wt-anime-title {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: clamp(32px, 4.1vw, 56px);
    font-weight: 700;
    line-height: 1.06;
    letter-spacing: -0.02em;
    color: #ffffff;
    cursor: default;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    transition: color 0.18s ease;
  }

  .wt-anime-title.is-clickable {
    cursor: pointer;
  }

  .wt-anime-title.is-clickable:hover {
    color: rgba(255, 255, 255, 0.86);
  }

  .wt-anime-meta {
    margin-top: 10px;
  }

  .wt-anime-desc {
    margin: 12px 0 0 0;
    max-width: 720px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: clamp(14.5px, 1.3vw, 17px);
    font-weight: 400;
    line-height: 1.42;
    color: rgba(255, 255, 255, 0.58);
    display: -webkit-box;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  /* BOTTOM BAR — Hero-matched MainButton/SecondButton + Gear ComboBox flush to bottom-left corner */
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
    gap: 12px;
    flex-wrap: wrap;
  }

  .wt-gear-btn {
    width: 43px;
    height: 43px;
    padding: 0;
    border: none;
    outline: none;
    border-radius: 999px;
    background: var(--mila-buttons-secondBg, rgba(255, 255, 255, 0.08));
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    color: rgba(255, 255, 255, 0.82);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    flex-shrink: 0;
    transition:
      background-color 0.2s ease,
      color 0.2s ease,
      transform 0.14s ease,
      opacity 0.16s ease;
  }

  .wt-gear-btn:hover:not(:disabled),
  .wt-gear-btn.is-open:not(:disabled) {
    background: var(--mila-buttons-secondBgHover, rgba(255, 255, 255, 0.16));
    color: #ffffff;
  }

  .wt-gear-btn:active:not(:disabled) {
    transform: scale(0.95);
  }

  .wt-gear-btn:disabled {
    opacity: 0.42;
    cursor: not-allowed;
  }

  .wt-gear-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: transform 0.28s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .wt-gear-icon.is-spun {
    transform: rotate(45deg);
  }

  /* RIGHT-SIDE FRIENDS RAIL (tucked snugly into exact bottom-right corner) */
  .wt-friends-rail {
    position: absolute;
    right: 12px;
    bottom: 12px;
    z-index: 15;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
  }

  .wt-rail-add {
    position: relative;
    width: 43px;
    height: 43px;
    border-radius: 50%;
    padding: 0;
    border: none;
    background: rgba(255, 255, 255, 0.07);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    color: rgba(255, 255, 255, 0.68);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition:
      background-color 0.18s ease,
      color 0.18s ease;
  }

  .wt-rail-add:hover {
    background: rgba(255, 255, 255, 0.14);
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
    cursor: default;
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
    background: #34d399;
  }

  .wt-rail-dot.is-room {
    background: #60a5fa;
  }

  /* Marlakit Kit ComboBox-identical Context Menu */
  .wt-kit-combo-layer {
    position: fixed;
    z-index: 10000;
    transform-origin: top right;
  }

  .wt-kit-combo-popover {
    min-width: 196px;
    max-width: 248px;
    padding: 0;
    border-radius: 15px;
    background: var(--mila-popup-comboBg, rgba(10, 10, 10, 0.9));
    backdrop-filter: blur(24px);
    -webkit-backdrop-filter: blur(24px);
    border: none;
    box-shadow: var(--mila-popup-shadow, 0 18px 46px rgba(0, 0, 0, 0.56));
    display: flex;
    flex-direction: column;
    overflow: hidden;
    user-select: none;
  }

  .wt-kit-combo-section-label {
    flex-shrink: 0;
    padding: 11px 14px 7px;
    display: flex;
    flex-direction: column;
    gap: 2px;
    user-select: none;
  }

  .wt-kit-combo-user-line {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.02em;
    color: var(--mila-popup-comboSectionText, rgba(255, 255, 255, 0.34));
  }

  .wt-kit-combo-sub-line {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11px;
    font-weight: 400;
    color: var(--mila-text-muted, rgba(255, 255, 255, 0.45));
  }

  .wt-kit-status-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #48484c;
    flex-shrink: 0;
  }

  .wt-kit-status-dot.is-online {
    background: #34d399;
  }

  .wt-kit-status-dot.is-room {
    background: #60a5fa;
  }

  .wt-kit-status-dot.is-pending {
    background: #e5c158;
  }

  .wt-kit-combo-list {
    display: flex;
    flex-direction: column;
    gap: 0;
    width: 100%;
    padding-bottom: 4px;
  }

  .wt-kit-combo-item {
    flex-shrink: 0;
    width: 100%;
    min-height: 36px;
    height: 36px;
    padding: 0 14px;
    border: none;
    outline: none;
    border-radius: 0;
    background: transparent;
    color: var(--mila-popup-itemText, rgba(255, 255, 255, 0.72));
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    font-weight: 500;
    white-space: nowrap;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    cursor: pointer;
    transition:
      background-color 0.14s ease,
      color 0.14s ease;
  }

  .wt-kit-combo-item:hover {
    background: var(--mila-popup-itemHover, rgba(255, 255, 255, 0.1));
    color: var(--mila-text-primary, #ffffff);
  }

  .wt-kit-combo-item.is-danger {
    color: #ff8c8c;
  }

  .wt-kit-combo-item.is-danger:hover {
    background: rgba(255, 107, 107, 0.14);
    color: #ffb2b2;
  }

  .wt-kit-combo-label {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
