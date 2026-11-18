<script lang="ts">
  import { cubicOut } from 'svelte/easing';
  import { fade, scale } from 'svelte/transition';
  import { MorphIcon } from 'morphicons/svelte';
  import {
    Check,
    Copy,
    Pencil,
    Play,
    Search,
    UserMinus,
    UserPlus,
    Users,
    X
  } from 'lucide';

  import Input from '../../marlakit/input/Input.svelte';
  import ScrollArea from '../../marlakit/scroll/ScrollArea.svelte';
  import LayerBlur from '../../marlakit/blur/LayerBlur.svelte';
  import { portal } from '../../marlakit/portal';
  import { auth } from '../../app/auth.svelte';
  import { revealBlur } from '../../app/reveal';
  import { navigate } from '../../app/router.svelte';
  import {
    acceptFriendRequest,
    closeFriendsPopup,
    declineFriendRequest,
    friendsState,
    refreshFriends,
    removeFriend,
    searchUsersByHandle,
    sendFriendRequest,
    updateUsername
  } from '../../app/friends.svelte';
  import {
    inviteFriendToWatchRoom,
    joinFriendWatchRoom,
    watchTogetherState
  } from '../../app/watchTogether.svelte';
  import type { FriendProfile } from '../../api/friends';

  let editingHandle = $state(false);
  let handleDraft = $state('');
  let handleSaving = $state(false);
  let handleError = $state<string | null>(null);
  let idCopied = $state(false);

  let searchQuery = $state('');
  let searchResults = $state<FriendProfile[]>([]);
  let searchLoading = $state(false);
  let activeTab = $state<'friends' | 'requests'>('friends');
  let listBodyHeight = $state(0);
  let actionBusy = $state<Record<string, boolean>>({});
  let invitedNotice = $state<Record<string, boolean>>({});

  const myHandle = $derived(
    auth.user?.username ||
      auth.user?.email?.split('@')[0]?.toLowerCase().replace(/[^a-z0-9_]/g, '') ||
      'user'
  );
  const myPublicId = $derived(auth.user?.public_id || '');
  const totalRequests = $derived(friendsState.incoming.length + friendsState.outgoing.length);

  $effect(() => {
    if (friendsState.popupOpen) {
      void refreshFriends();
    } else {
      editingHandle = false;
      handleError = null;
      searchQuery = '';
      searchResults = [];
    }
  });

  $effect(() => {
    const q = searchQuery.trim();
    if (!friendsState.popupOpen || q.length < 2) {
      searchResults = [];
      searchLoading = false;
      return;
    }
    searchLoading = true;
    const timer = window.setTimeout(() => {
      void searchUsersByHandle(q, 14)
        .then((res) => {
          if (searchQuery.trim() === q) {
            searchResults = res;
          }
        })
        .catch(() => {
          if (searchQuery.trim() === q) {
            searchResults = [];
          }
        })
        .finally(() => {
          if (searchQuery.trim() === q) {
            searchLoading = false;
          }
        });
    }, 200);

    return () => window.clearTimeout(timer);
  });

  function startEditHandle() {
    handleDraft = myHandle;
    handleError = null;
    editingHandle = true;
  }

  async function saveHandle(e?: Event) {
    e?.preventDefault();
    const clean = handleDraft.trim().replace(/^@/, '').toLowerCase();
    if (!clean || clean === myHandle) {
      editingHandle = false;
      handleError = null;
      return;
    }
    handleSaving = true;
    handleError = null;
    try {
      await updateUsername(clean);
      editingHandle = false;
    } catch (err) {
      handleError = err instanceof Error ? err.message : 'Could not update username';
    } finally {
      handleSaving = false;
    }
  }

  async function copyPublicId() {
    if (!myPublicId) return;
    try {
      await navigator.clipboard.writeText(myPublicId);
      idCopied = true;
      window.setTimeout(() => {
        idCopied = false;
      }, 1500);
    } catch {
      // ignore
    }
  }

  async function runBusy(key: string, fn: () => Promise<unknown>) {
    if (actionBusy[key]) return;
    actionBusy = { ...actionBusy, [key]: true };
    try {
      await fn();
      if (searchQuery.trim().length >= 2) {
        searchResults = await searchUsersByHandle(searchQuery.trim(), 14).catch(() => searchResults);
      }
    } finally {
      actionBusy = { ...actionBusy, [key]: false };
    }
  }

  async function handleJoinRoom(friend: FriendProfile) {
    await runBusy(`join:${friend.public_id}`, async () => {
      await joinFriendWatchRoom(friend.public_id);
      closeFriendsPopup();
      navigate('watch-together');
    });
  }

  async function handleInviteToRoom(friend: FriendProfile) {
    await runBusy(`inv:${friend.public_id}`, async () => {
      await inviteFriendToWatchRoom(friend.public_id);
      invitedNotice = { ...invitedNotice, [friend.public_id]: true };
      window.setTimeout(() => {
        invitedNotice = { ...invitedNotice, [friend.public_id]: false };
      }, 1800);
    });
  }

  function handleWindowKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && friendsState.popupOpen) {
      closeFriendsPopup();
    }
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if friendsState.popupOpen && auth.user}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    use:portal
    class="friends-backdrop"
    transition:fade={{ duration: 180, easing: cubicOut }}
    onclick={(e) => {
      if (e.target === e.currentTarget) closeFriendsPopup();
    }}
  >
    <div
      class="friends-modal"
      role="dialog"
      aria-modal="true"
      aria-label="Friends & Handle"
      transition:scale={{ start: 0.96, duration: 220, easing: cubicOut }}
    >
      {#if auth.user.avatar_url}
        <div class="friends-ambient" aria-hidden="true">
          <LayerBlur src={auth.user.avatar_url} blur={88} opacity={0.3} scale={1.25} />
        </div>
      {/if}

      <!-- Top self identity bar with editable @username and subtle public_id -->
      <div class="self-head">
        <div class="self-identity">
          {#if auth.user.avatar_url}
            <img src={auth.user.avatar_url} alt={auth.user.name} class="self-avatar" referrerpolicy="no-referrer" />
          {:else}
            <div class="self-avatar is-fallback">{(auth.user.name || 'U').slice(0, 1).toUpperCase()}</div>
          {/if}

          <div class="self-meta">
            <div class="self-name-row">
              <span class="self-display-name">{auth.user.name}</span>
              {#if myPublicId}
                <button
                  type="button"
                  class="self-subtle-id"
                  title="Internal ID (click to copy)"
                  onclick={copyPublicId}
                >
                  <span>{myPublicId}</span>
                  {#if idCopied}
                    <MorphIcon icon={Check} size={9} strokeWidth={2.4} />
                  {/if}
                </button>
              {/if}
            </div>

            {#if editingHandle}
              <form
                class="handle-edit-form"
                onsubmit={saveHandle}
                in:revealBlur={{ duration: 220, y: 2, scale: 0.97, blur: 6 }}
              >
                <span class="handle-at">@</span>
                <!-- svelte-ignore a11y_autofocus -->
                <input
                  type="text"
                  class="handle-input"
                  bind:value={handleDraft}
                  placeholder="username"
                  maxlength={24}
                  autofocus
                />
                <button type="submit" class="handle-icon-btn is-save" disabled={handleSaving} aria-label="Save username">
                  <MorphIcon icon={Check} size={13} strokeWidth={2.4} />
                </button>
                <button
                  type="button"
                  class="handle-icon-btn"
                  onclick={() => {
                    editingHandle = false;
                    handleError = null;
                  }}
                  aria-label="Cancel"
                >
                  <MorphIcon icon={X} size={13} strokeWidth={2.2} />
                </button>
              </form>
            {:else}
              <button
                type="button"
                class="self-handle-btn"
                onclick={startEditHandle}
                title="Change @username"
                in:revealBlur={{ duration: 200, y: -2, scale: 0.98, blur: 5 }}
              >
                <span class="handle-text">@{myHandle}</span>
                <MorphIcon icon={Pencil} size={12} strokeWidth={2} class="handle-pencil" />
              </button>
            {/if}

            {#if handleError}
              <span class="handle-err">{handleError}</span>
            {/if}
          </div>
        </div>

        <button type="button" class="close-btn" onclick={closeFriendsPopup} aria-label="Close">
          <MorphIcon icon={X} size={17} strokeWidth={2} />
        </button>
      </div>

      <!-- Search by @username -->
      <div class="search-row">
        <Input
          bind:value={searchQuery}
          placeholder="Find friend by @username..."
          class="friends-search-input"
        >
          {#snippet icon()}
            <MorphIcon icon={Search} size={15} strokeWidth={1.9} />
          {/snippet}
          {#snippet trailingIcon()}
            {#if searchQuery.trim().length > 0}
              <button
                type="button"
                class="clear-query-btn"
                onclick={() => {
                  searchQuery = '';
                  searchResults = [];
                }}
                aria-label="Clear search"
              >
                <MorphIcon icon={X} size={13} strokeWidth={2} />
              </button>
            {/if}
          {/snippet}
        </Input>
      </div>

      {#if searchQuery.trim().length >= 2}
        <div class="list-section">
          <div class="section-caption">Search results</div>
          <ScrollArea maxHeight={290}>
            {#if searchLoading}
              <div class="empty-hint">Searching @{searchQuery.trim().replace(/^@/, '')}...</div>
            {:else if searchResults.length === 0}
              <div class="empty-hint">No users found for “{searchQuery.trim()}”</div>
            {:else}
              <div class="user-list">
                {#each searchResults as u (u.public_id)}
                  <div class="user-row">
                    <div class="avatar-wrap">
                      {#if u.avatar_url}
                        <img src={u.avatar_url} alt={u.name} class="user-avatar" referrerpolicy="no-referrer" />
                      {:else}
                        <div class="user-avatar is-fallback">{(u.name || 'U').slice(0, 1).toUpperCase()}</div>
                      {/if}
                      <span
                        class="status-dot"
                        class:is-online={u.status === 'online'}
                        class:is-room={u.status === 'in_room'}
                      ></span>
                    </div>

                    <div class="user-info">
                      <span class="user-name">{u.name}</span>
                      <span class="user-sub">@{u.username}</span>
                    </div>

                    <div class="user-actions">
                      {#if u.relation === 'friend'}
                        <span class="pill-tag is-muted">Friends</span>
                      {:else if u.relation === 'outgoing'}
                        <button
                          type="button"
                          class="pill-btn is-ghost"
                          disabled={actionBusy[u.public_id]}
                          onclick={() => runBusy(u.public_id, () => declineFriendRequest(u.public_id))}
                        >
                          Requested
                        </button>
                      {:else if u.relation === 'incoming'}
                        <button
                          type="button"
                          class="pill-btn is-accent"
                          disabled={actionBusy[u.public_id]}
                          onclick={() => runBusy(u.public_id, () => acceptFriendRequest(u.public_id))}
                        >
                          <MorphIcon icon={Check} size={13} strokeWidth={2.3} />
                          <span>Accept</span>
                        </button>
                      {:else}
                        <button
                          type="button"
                          class="pill-btn is-accent"
                          disabled={actionBusy[u.public_id]}
                          onclick={() => runBusy(u.public_id, () => sendFriendRequest(u.public_id))}
                        >
                          <MorphIcon icon={UserPlus} size={13} strokeWidth={2.1} />
                          <span>Add</span>
                        </button>
                      {/if}
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </ScrollArea>
        </div>
      {:else}
        <div class="tabs-bar" role="tablist">
          <button
            type="button"
            role="tab"
            class="tab-chip"
            class:active={activeTab === 'friends'}
            aria-selected={activeTab === 'friends'}
            onclick={() => (activeTab = 'friends')}
          >
            <span>Friends</span>
            <span class="tab-count">{friendsState.friends.length}</span>
          </button>
          <button
            type="button"
            role="tab"
            class="tab-chip"
            class:active={activeTab === 'requests'}
            aria-selected={activeTab === 'requests'}
            onclick={() => (activeTab = 'requests')}
          >
            <span>Requests</span>
            {#if totalRequests > 0}
              <span class="tab-count is-highlight">{totalRequests}</span>
            {/if}
          </button>
        </div>

        <div
          class="list-section list-section-animated"
          style:height={listBodyHeight > 0 ? `${Math.min(300, listBodyHeight)}px` : undefined}
        >
          <ScrollArea maxHeight={300}>
            <div class="list-body-measure" bind:clientHeight={listBodyHeight}>
            {#key activeTab}
              <div
                class="tab-pane-wrap"
                in:revealBlur={{ duration: 220, y: 4, scale: 0.99, blur: 6 }}
              >
            {#if activeTab === 'friends'}
              {#if friendsState.friends.length === 0}
                <div class="empty-hint">
                  No friends yet. Type a friend's <strong>@username</strong> above to add them.
                </div>
              {:else}
                <div class="user-list">
                  {#each friendsState.friends as f (f.public_id)}
                    <div class="user-row">
                      <div class="avatar-wrap">
                        {#if f.avatar_url}
                          <img src={f.avatar_url} alt={f.name} class="user-avatar" referrerpolicy="no-referrer" />
                        {:else}
                          <div class="user-avatar is-fallback">{(f.name || 'U').slice(0, 1).toUpperCase()}</div>
                        {/if}
                        <span
                          class="status-dot"
                          class:is-online={f.status === 'online'}
                          class:is-room={f.status === 'in_room'}
                        ></span>
                      </div>

                      <div class="user-info">
                        <div class="user-title-line">
                          <span class="user-name">{f.name}</span>
                          <span class="user-handle">@{f.username}</span>
                        </div>
                        {#if f.status === 'in_room' && f.room}
                          <span class="user-room-line">
                            In @{f.room.host_username}’s room{f.room.anime_title ? ` · ${f.room.anime_title}` : ''}
                            ({f.room.member_count}/{f.room.max_members})
                          </span>
                        {:else if f.status === 'online'}
                          <span class="user-status-line is-online">Online</span>
                        {:else}
                          <span class="user-status-line">Offline</span>
                        {/if}
                      </div>

                      <div class="user-actions">
                        {#if f.status === 'in_room' && f.room && f.room.room_id !== watchTogetherState.room?.room_id}
                          <button
                            type="button"
                            class="pill-btn is-room"
                            disabled={actionBusy[`join:${f.public_id}`]}
                            onclick={() => void handleJoinRoom(f)}
                          >
                            <MorphIcon icon={Play} size={11} fill="currentColor" strokeWidth={0} />
                            <span>Join</span>
                          </button>
                        {:else if watchTogetherState.room && f.status !== 'offline'}
                          <button
                            type="button"
                            class="pill-btn is-ghost"
                            disabled={actionBusy[`inv:${f.public_id}`]}
                            onclick={() => void handleInviteToRoom(f)}
                          >
                            {#if invitedNotice[f.public_id]}
                              <MorphIcon icon={Check} size={12} strokeWidth={2.3} />
                              <span>Sent</span>
                            {:else}
                              <span>Invite</span>
                            {/if}
                          </button>
                        {/if}

                        <button
                          type="button"
                          class="icon-ghost-btn"
                          title="Remove friend"
                          disabled={actionBusy[f.public_id]}
                          onclick={() => runBusy(f.public_id, () => removeFriend(f.public_id))}
                        >
                          <MorphIcon icon={UserMinus} size={14} strokeWidth={1.9} />
                        </button>
                      </div>
                    </div>
                  {/each}
                </div>
              {/if}
            {:else}
              {#if totalRequests === 0}
                <div class="empty-hint">No pending friend requests.</div>
              {:else}
                <div class="user-list">
                  {#each friendsState.incoming as req (req.public_id)}
                    <div class="user-row">
                      <div class="avatar-wrap">
                        {#if req.avatar_url}
                          <img src={req.avatar_url} alt={req.name} class="user-avatar" referrerpolicy="no-referrer" />
                        {:else}
                          <div class="user-avatar is-fallback">{(req.name || 'U').slice(0, 1).toUpperCase()}</div>
                        {/if}
                      </div>
                      <div class="user-info">
                        <span class="user-name">{req.name}</span>
                        <span class="user-sub">@{req.username} · Incoming request</span>
                      </div>
                      <div class="user-actions">
                        <button
                          type="button"
                          class="pill-btn is-accent"
                          disabled={actionBusy[req.public_id]}
                          onclick={() => runBusy(req.public_id, () => acceptFriendRequest(req.public_id))}
                        >
                          <MorphIcon icon={Check} size={13} strokeWidth={2.3} />
                          <span>Accept</span>
                        </button>
                        <button
                          type="button"
                          class="icon-ghost-btn"
                          disabled={actionBusy[req.public_id]}
                          onclick={() => runBusy(req.public_id, () => declineFriendRequest(req.public_id))}
                          title="Decline"
                        >
                          <MorphIcon icon={X} size={14} strokeWidth={2} />
                        </button>
                      </div>
                    </div>
                  {/each}

                  {#each friendsState.outgoing as req (req.public_id)}
                    <div class="user-row">
                      <div class="avatar-wrap">
                        {#if req.avatar_url}
                          <img src={req.avatar_url} alt={req.name} class="user-avatar" referrerpolicy="no-referrer" />
                        {:else}
                          <div class="user-avatar is-fallback">{(req.name || 'U').slice(0, 1).toUpperCase()}</div>
                        {/if}
                      </div>
                      <div class="user-info">
                        <span class="user-name">{req.name}</span>
                        <span class="user-sub">@{req.username} · Sent</span>
                      </div>
                      <div class="user-actions">
                        <button
                          type="button"
                          class="pill-btn is-ghost"
                          disabled={actionBusy[req.public_id]}
                          onclick={() => runBusy(req.public_id, () => declineFriendRequest(req.public_id))}
                        >
                          Cancel
                        </button>
                      </div>
                    </div>
                  {/each}
                </div>
              {/if}
            {/if}
              </div>
            {/key}
            </div>
          </ScrollArea>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .friends-backdrop {
    position: fixed;
    inset: 0;
    z-index: 260;
    background: var(--mila-popup-backdrop, rgba(4, 4, 6, 0.72));
    backdrop-filter: blur(18px);
    -webkit-backdrop-filter: blur(18px);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px;
  }

  .friends-modal {
    position: relative;
    width: min(460px, 100%);
    border-radius: 18px;
    background: var(--mila-popup-bgExpanded, rgba(14, 14, 17, 0.95));
    box-shadow: var(--mila-popup-shadow, 0 28px 80px rgba(0, 0, 0, 0.78));
    padding: 18px;
    display: flex;
    flex-direction: column;
    gap: 14px;
    overflow: hidden;
    color: var(--mila-text-primary, #ffffff);
  }

  .friends-ambient {
    position: absolute;
    inset: -24px;
    pointer-events: none;
    z-index: 0;
    opacity: 0.75;
    mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.95) 0%, rgba(0, 0, 0, 0) 75%);
    -webkit-mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.95) 0%, rgba(0, 0, 0, 0) 75%);
  }

  .self-head,
  .search-row,
  .tabs-bar,
  .list-section {
    position: relative;
    z-index: 1;
  }

  .self-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
  }

  .self-identity {
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: 0;
  }

  .self-avatar {
    width: 44px;
    height: 44px;
    border-radius: 50%;
    object-fit: cover;
    flex-shrink: 0;
    background: rgba(255, 255, 255, 0.08);
  }

  .self-avatar.is-fallback,
  .user-avatar.is-fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-weight: 700;
    font-size: 15px;
    color: var(--mila-text-primary, #ffffff);
  }

  .self-meta {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    min-width: 0;
  }

  .self-name-row {
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }

  .self-display-name {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 16px;
    font-weight: 700;
    color: var(--mila-text-primary, #ffffff);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* Subtle internal public_id that never clutters the visual design */
  .self-subtle-id {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 0;
    border: none;
    background: transparent;
    font-family: 'Medeo Sans Display', monospace;
    font-size: 10px;
    letter-spacing: 0.02em;
    color: rgba(255, 255, 255, 0.16);
    cursor: pointer;
    user-select: none;
    transition: color 0.18s ease;
  }

  .self-subtle-id:hover {
    color: rgba(255, 255, 255, 0.38);
  }

  .self-handle-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 2px 6px;
    margin-left: -6px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--mila-text-secondary, rgba(255, 255, 255, 0.62));
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    cursor: pointer;
    transition: background-color 0.18s ease, color 0.18s ease;
  }

  .self-handle-btn:hover {
    background: rgba(255, 255, 255, 0.06);
    color: var(--mila-text-primary, #ffffff);
  }

  .self-handle-btn :global(.handle-pencil) {
    opacity: 0.45;
    transition: opacity 0.18s ease;
  }

  .self-handle-btn:hover :global(.handle-pencil) {
    opacity: 0.9;
  }

  .handle-edit-form {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: rgba(255, 255, 255, 0.06);
    border-radius: 7px;
    padding: 2px 6px;
  }

  .handle-at {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
    color: rgba(255, 255, 255, 0.45);
  }

  .handle-input {
    width: 136px;
    border: none;
    outline: none;
    background: transparent;
    color: var(--mila-text-primary, #ffffff);
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13px;
  }

  .handle-icon-btn {
    width: 22px;
    height: 22px;
    border: none;
    border-radius: 5px;
    background: transparent;
    color: rgba(255, 255, 255, 0.65);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
  }

  .handle-icon-btn.is-save {
    color: var(--mila-accent, #d9ff70);
  }

  .handle-err {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
    color: #ff6b6b;
  }

  .close-btn {
    width: 30px;
    height: 30px;
    border: none;
    border-radius: 9px;
    background: transparent;
    color: rgba(255, 255, 255, 0.48);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: color 0.18s ease;
  }

  .close-btn:hover {
    background: transparent;
    color: #ffffff;
  }

  .clear-query-btn {
    width: 22px;
    height: 22px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: rgba(255, 255, 255, 0.56);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: color 0.18s ease;
  }

  .clear-query-btn:hover {
    color: #ffffff;
  }

  .tabs-bar {
    display: flex;
    align-items: baseline;
    gap: 16px;
    padding: 0 2px;
  }

  .tab-chip {
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    height: 28px;
    padding: 0;
    border: none;
    border-radius: 0;
    background: transparent;
    color: rgba(255, 255, 255, 0.36);
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 14px;
    font-weight: 400;
    letter-spacing: -0.015em;
    cursor: pointer;
    transition: color 0.18s ease, font-weight 0.18s ease;
  }

  .tab-chip:hover {
    color: rgba(255, 255, 255, 0.68);
  }

  .tab-chip.active {
    background: transparent;
    color: #ffffff;
    font-weight: 700;
  }

  .tab-count {
    font-size: 11.5px;
    font-weight: 400;
    opacity: 0.62;
  }

  .tab-chip.active .tab-count {
    font-weight: 700;
    opacity: 0.78;
  }

  .tab-count.is-highlight {
    padding: 1px 6px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.14);
    color: #ffffff;
    font-weight: 700;
    opacity: 1;
  }

  .list-section-animated {
    overflow: hidden;
    transition: height 0.28s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .section-caption {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12px;
    color: rgba(255, 255, 255, 0.42);
    margin-bottom: 8px;
  }

  .empty-hint {
    padding: 34px 14px;
    text-align: center;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 13.5px;
    color: rgba(255, 255, 255, 0.44);
  }

  .user-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .user-row {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 8px 10px;
    border-radius: 12px;
    background: rgba(255, 255, 255, 0.028);
    transition: background-color 0.18s ease;
  }

  .user-row:hover {
    background: rgba(255, 255, 255, 0.055);
  }

  .avatar-wrap {
    position: relative;
    width: 38px;
    height: 38px;
    flex-shrink: 0;
  }

  .user-avatar {
    width: 38px;
    height: 38px;
    border-radius: 50%;
    object-fit: cover;
    background: rgba(255, 255, 255, 0.07);
  }

  .status-dot {
    position: absolute;
    right: 0;
    bottom: 0;
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: #373737;
    box-shadow: 0 0 0 2px #0e0e11;
  }

  .status-dot.is-online {
    background: #29eb36;
  }

  .status-dot.is-room {
    background: #294aeb;
  }

  .user-info {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .user-title-line {
    display: flex;
    align-items: baseline;
    gap: 6px;
    min-width: 0;
  }

  .user-name {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 13.5px;
    font-weight: 700;
    color: var(--mila-text-primary, #ffffff);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .user-handle,
  .user-sub {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12px;
    color: rgba(255, 255, 255, 0.44);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .user-status-line {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
    color: rgba(255, 255, 255, 0.36);
  }

  .user-status-line.is-online {
    color: #29eb36;
  }

  .user-room-line {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11.5px;
    color: #6f8bff;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .user-actions {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }

  .pill-btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 28px;
    padding: 0 11px;
    border: none;
    border-radius: 8px;
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    transition: background-color 0.18s ease, color 0.18s ease, transform 0.12s ease;
  }

  .pill-btn:active {
    transform: scale(0.96);
  }

  .pill-btn.is-accent {
    background: var(--mila-accent, #d9ff70);
    color: #0b0b0c;
  }

  .pill-btn.is-room {
    background: rgba(41, 74, 235, 0.24);
    color: #8ea4ff;
  }

  .pill-btn.is-room:hover {
    background: #294aeb;
    color: #ffffff;
  }

  .pill-btn.is-ghost {
    background: rgba(255, 255, 255, 0.06);
    color: rgba(255, 255, 255, 0.78);
  }

  .pill-btn.is-ghost:hover {
    background: rgba(255, 255, 255, 0.12);
    color: #ffffff;
  }

  .pill-tag.is-muted {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12px;
    color: rgba(255, 255, 255, 0.42);
    padding: 0 6px;
  }

  .icon-ghost-btn {
    width: 28px;
    height: 28px;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: rgba(255, 255, 255, 0.36);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: background-color 0.18s ease, color 0.18s ease;
  }

  .icon-ghost-btn:hover {
    background: rgba(255, 107, 107, 0.14);
    color: #ff7b7b;
  }
</style>
