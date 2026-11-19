<script lang="ts">
  import { MorphIcon } from 'morphicons/svelte';
  import { Check, X } from 'lucide';
  import LayerBlur from '../../marlakit/blur/LayerBlur.svelte';
  import { revealBlur } from '../../app/reveal';
  import { navigate } from '../../app/router.svelte';
  import {
    declineWatchRoomInvite,
    joinWatchRoom,
    watchTogetherState
  } from '../../app/watchTogether.svelte';
  import type { RoomInvite } from '../../api/watchTogether';

  let busyInviteId = $state<string | null>(null);
  let brokenAvatars = $state<Record<string, boolean>>({});

  const activeInvite = $derived.by<RoomInvite | null>(() => {
    const curRoomId = watchTogetherState.room?.room_id;
    const list = watchTogetherState.invites.filter((inv) => inv.room_id !== curRoomId);
    return list.length > 0 ? list[list.length - 1] : null;
  });

  const avatarStack = $derived.by<string[]>(() => {
    const inv = activeInvite;
    if (!inv) return [];
    const raw = inv.member_avatars && inv.member_avatars.length > 0
      ? inv.member_avatars
      : inv.from_avatar_url
        ? [inv.from_avatar_url]
        : [];
    return raw.slice(0, 5);
  });

  async function handleAccept(invite: RoomInvite) {
    if (busyInviteId) return;
    busyInviteId = invite.invite_id;
    try {
      await joinWatchRoom({
        room_id: invite.room_id,
        room_code: invite.room_code
      });
      navigate('watch-together');
    } catch {
      // Ignore or dismiss broken invite
      await declineWatchRoomInvite(invite);
    } finally {
      busyInviteId = null;
    }
  }

  async function handleDecline(invite: RoomInvite) {
    if (busyInviteId) return;
    await declineWatchRoomInvite(invite);
  }
</script>

{#if activeInvite}
  {@const inv = activeInvite}
  <div
    class="wt-invite-popup"
    role="dialog"
    aria-label="Watch Together room invite"
    in:revealBlur={{ duration: 280, y: 12, scale: 0.95, blur: 10 }}
    out:revealBlur={{ duration: 200, y: 8, scale: 0.96, blur: 8 }}
  >
    {#if inv.anime_poster}
      <div class="wt-invite-bg" aria-hidden="true">
        <LayerBlur
          src={inv.anime_poster}
          blur={34}
          opacity={0.42}
          scale={1.24}
        />
      </div>
    {/if}

    <div class="wt-invite-inner">
      <div class="wt-invite-top">
        <div class="wt-invite-from">
          <span class="wt-invite-kicker">@{inv.from_username || inv.from_name} invited you</span>
          <h4 class="wt-invite-room">{inv.room_title || `@${inv.from_username}'s room`}</h4>
        </div>

        <button
          type="button"
          class="wt-invite-close"
          aria-label="Dismiss invite"
          onclick={() => void handleDecline(inv)}
        >
          <MorphIcon icon={X} size={14} strokeWidth={1.8} />
        </button>
      </div>

      <div class="wt-invite-anime-row">
        <span class="wt-invite-anime">
          {inv.anime_title || 'Choosing anime…'}
        </span>
        {#if inv.anime_title && inv.episode_ordinal}
          <span class="wt-invite-ep">EP {inv.episode_ordinal}</span>
        {/if}
      </div>

      <div class="wt-invite-bottom">
        <div class="wt-invite-members">
          <div class="wt-avatar-stack">
            {#each avatarStack as avUrl, idx (`${inv.invite_id}-${idx}`)}
              <div class="wt-stack-circle" style:z-index={10 - idx}>
                {#if avUrl && !brokenAvatars[`${inv.invite_id}-${idx}`]}
                  <img
                    src={avUrl}
                    alt=""
                    onerror={() => {
                      brokenAvatars = { ...brokenAvatars, [`${inv.invite_id}-${idx}`]: true };
                    }}
                  />
                {:else}
                  <span>{(inv.from_name || inv.from_username || 'U').slice(0, 1).toUpperCase()}</span>
                {/if}
              </div>
            {/each}
          </div>
          <span class="wt-invite-count">
            {inv.member_count ?? Math.max(1, avatarStack.length)}/{inv.max_members ?? 7}
          </span>
        </div>

        <div class="wt-invite-actions">
          <button
            type="button"
            class="wt-invite-btn wt-invite-decline"
            disabled={busyInviteId === inv.invite_id}
            onclick={() => void handleDecline(inv)}
          >
            Decline
          </button>
          <button
            type="button"
            class="wt-invite-btn wt-invite-join"
            disabled={busyInviteId === inv.invite_id}
            onclick={() => void handleAccept(inv)}
          >
            <MorphIcon icon={Check} size={13} strokeWidth={2.3} />
            <span>{busyInviteId === inv.invite_id ? 'Joining…' : 'Join'}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  .wt-invite-popup {
    position: fixed;
    right: 22px;
    bottom: 22px;
    z-index: 260;
    width: 304px;
    border-radius: 18px;
    overflow: hidden;
    background: rgba(18, 18, 21, 0.88);
    backdrop-filter: blur(28px) saturate(140%);
    -webkit-backdrop-filter: blur(28px) saturate(140%);
    box-shadow: 0 22px 54px rgba(0, 0, 0, 0.68);
    color: #ffffff;
    user-select: none;
  }

  .wt-invite-bg {
    position: absolute;
    inset: 0;
    pointer-events: none;
    z-index: 0;
  }

  .wt-invite-inner {
    position: relative;
    z-index: 1;
    padding: 14px 16px 14px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .wt-invite-top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 8px;
  }

  .wt-invite-from {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .wt-invite-kicker {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 11px;
    font-weight: 500;
    color: rgba(255, 255, 255, 0.52);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .wt-invite-room {
    margin: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 15px;
    font-weight: 800;
    letter-spacing: -0.02em;
    color: #ffffff;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .wt-invite-close {
    width: 24px;
    height: 24px;
    margin: -2px -4px 0 0;
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

  .wt-invite-close:hover {
    color: #ffffff;
  }

  .wt-invite-anime-row {
    display: flex;
    align-items: center;
    gap: 7px;
    min-width: 0;
  }

  .wt-invite-anime {
    font-family: 'Medeo Sans Display', system-ui, sans-serif;
    font-size: 12.5px;
    font-weight: 600;
    color: rgba(255, 255, 255, 0.84);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .wt-invite-ep {
    flex-shrink: 0;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 10.5px;
    font-weight: 700;
    color: rgba(255, 255, 255, 0.46);
  }

  .wt-invite-bottom {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding-top: 2px;
  }

  .wt-invite-members {
    display: inline-flex;
    align-items: center;
    gap: 7px;
  }

  .wt-avatar-stack {
    display: inline-flex;
    align-items: center;
  }

  .wt-stack-circle {
    width: 26px;
    height: 26px;
    border-radius: 50%;
    overflow: hidden;
    background: rgba(255, 255, 255, 0.08);
    border: none;
    margin-left: -7px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 10px;
    font-weight: 700;
    color: #ffffff;
    box-sizing: border-box;
  }

  .wt-stack-circle:first-child {
    margin-left: 0;
  }

  .wt-stack-circle:not(:first-child) {
    -webkit-mask-image: radial-gradient(circle 15px at -6px 50%, transparent 99%, #000 100%);
    mask-image: radial-gradient(circle 15px at -6px 50%, transparent 99%, #000 100%);
  }

  .wt-stack-circle img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .wt-invite-count {
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 11px;
    font-weight: 700;
    color: rgba(255, 255, 255, 0.44);
  }

  .wt-invite-actions {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .wt-invite-btn {
    height: 28px;
    padding: 0 11px;
    border-radius: 999px;
    border: none;
    font-family: 'RF Dewi Extended', system-ui, sans-serif;
    font-size: 11.5px;
    font-weight: 700;
    display: inline-flex;
    align-items: center;
    gap: 5px;
    cursor: pointer;
    transition:
      background 0.16s ease,
      color 0.16s ease,
      opacity 0.16s ease;
  }

  .wt-invite-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .wt-invite-decline {
    background: transparent;
    color: rgba(255, 255, 255, 0.52);
    padding: 0 8px;
  }

  .wt-invite-decline:hover:not(:disabled) {
    color: #ffffff;
  }

  .wt-invite-join {
    background: #ffffff;
    color: #0c0c0e;
  }

  .wt-invite-join:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.9);
  }
</style>
