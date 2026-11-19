import { ensureShikimoriArtwork, getCachedArtwork } from '../api/anime';
import { getWailsRuntimeBridge } from '../api/bridge';
import type { FriendProfile } from '../api/friends';
import {
  watchTogetherApi,
  type CreateRoomInput,
  type JoinRoomInput,
  type MemberStateInput,
  type RoomChatMessage,
  type RoomControlMode,
  type RoomInvite,
  type RoomMediaState,
  type RoomPlaybackState,
  type RoomVisibility,
  type SyncPlaybackInput,
  type UpdateRoomSettingsInput,
  type WatchRoomSnapshot,
  type WTWireEvent
} from '../api/watchTogether';
import { auth } from './auth.svelte';
import { applyFriendPresenceUpdate, refreshFriends } from './friends.svelte';

export interface WatchTogetherDraftAnime {
  title: string;
  description: string;
  poster: string;
  backdrop: string;
  shikiId?: number;
  releaseAlias?: string;
  score?: number;
  tags: string[];
  searchQueries: string[];
}

export interface WatchTogetherStoreState {
  connected: boolean;
  connecting: boolean;
  transport: 'quic' | 'https' | 'none';
  room: WatchRoomSnapshot | null;
  invites: RoomInvite[];
  error: string | null;
  draftTitle: string;
  draftVisibility: RoomVisibility;
  draftControlMode: RoomControlMode;
  draftMaxMembers: number;
  draftAnime: WatchTogetherDraftAnime | null;
  draftInvitedIds: string[];
}

export const watchTogetherState = $state<WatchTogetherStoreState>({
  connected: false,
  connecting: false,
  transport: 'none',
  room: null,
  invites: [],
  error: null,
  draftTitle: '',
  draftVisibility: 'friends',
  draftControlMode: 'host_only',
  draftMaxMembers: 7,
  draftAnime: null,
  draftInvitedIds: []
});

let unbindEvents: (() => void) | null = null;
let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
let syncPollTimer: ReturnType<typeof setInterval> | null = null;
let syncInFlight = false;
let syncTickCount = 0;
let mutationSeq = 0;
let lastMembersSignature = '';
const declinedInviteIds = new Set<string>();

function computeMembersSignature(room: WatchRoomSnapshot | null): string {
  if (!room) return '';
  const m = (room.members ?? []).map((x) => `${x.public_id}:${x.role}`).join(',');
  const p = (room.pending ?? []).map((x) => x.public_id).join(',');
  return `${room.room_id}|${room.host_public_id}|${m}|${p}`;
}

function ensureEventListeners(): void {
  if (unbindEvents) return;
  const runtime = getWailsRuntimeBridge();
  if (!runtime?.EventsOn) return;

  const offWT = runtime.EventsOn('mila:watch-together', (raw: unknown) => {
    const ev = raw as WTWireEvent | undefined;
    if (!ev || typeof ev.type !== 'string') return;
    handleIncomingWTEvent(ev);
  });
  const offPresence = runtime.EventsOn('mila:friend-presence', (raw: unknown) => {
    const friend = raw as FriendProfile | undefined;
    if (friend?.public_id) {
      applyFriendPresenceUpdate(friend);
    }
  });
  unbindEvents = () => {
    offWT?.();
    offPresence?.();
  };
}

function appendUniqueChatMessage(list: RoomChatMessage[], msg: RoomChatMessage): RoomChatMessage[] {
  if (list.some((m) => m.id === msg.id)) {
    return list;
  }
  return [...list, msg].slice(-50);
}

function stampPlayback(pb: RoomPlaybackState | undefined): RoomPlaybackState | undefined {
  if (!pb) return undefined;
  return {
    ...pb,
    client_received_at_ms: pb.client_received_at_ms || Date.now()
  };
}

function applyRoomSnapshot(next: WatchRoomSnapshot | null): void {
  if (next?.playback) {
    next = {
      ...next,
      playback: stampPlayback(next.playback)!
    };
  }
  const sig = computeMembersSignature(next);
  const membershipChanged = sig !== lastMembersSignature;
  lastMembersSignature = sig;
  watchTogetherState.room = next;
  if (membershipChanged) {
    void refreshFriends();
  }
}

export function handleIncomingWTEvent(ev: WTWireEvent): void {
  switch (ev.type) {
    case 'room_snapshot':
      applyRoomSnapshot(ev.room ?? null);
      break;
    case 'room_media':
      if (ev.room) {
        applyRoomSnapshot(ev.room);
      } else if (watchTogetherState.room && ev.media) {
        watchTogetherState.room.media = ev.media;
        if (ev.playback) {
          watchTogetherState.room.playback = stampPlayback(ev.playback)!;
        }
      }
      break;
    case 'room_playback':
      if (watchTogetherState.room && ev.playback) {
        if (ev.playback.seq >= (watchTogetherState.room.playback?.seq ?? 0)) {
          watchTogetherState.room.playback = stampPlayback(ev.playback)!;
        }
      }
      break;
    case 'room_member':
      if (watchTogetherState.room && ev.member) {
        const idx = watchTogetherState.room.members.findIndex(
          (m) => m.public_id === ev.member!.public_id
        );
        if (idx >= 0) {
          watchTogetherState.room.members[idx] = ev.member;
        } else {
          watchTogetherState.room.members = [...watchTogetherState.room.members, ev.member];
        }
      }
      break;
    case 'room_chat':
      if (watchTogetherState.room && ev.chat) {
        watchTogetherState.room.chat = appendUniqueChatMessage(
          watchTogetherState.room.chat ?? [],
          ev.chat
        );
      }
      break;
    case 'room_closed':
      applyRoomSnapshot(null);
      break;
    case 'room_invite':
      if (ev.invite && !declinedInviteIds.has(ev.invite.invite_id)) {
        const filtered = watchTogetherState.invites.filter(
          (i) => i.room_id !== ev.invite!.room_id
        );
        watchTogetherState.invites = [...filtered, ev.invite];
      }
      break;
    case 'friend_presence':
      if (ev.friend) {
        applyFriendPresenceUpdate(ev.friend);
      }
      break;
  }
}

async function pollRoomAndInvitesOnce(): Promise<void> {
  if (!auth.user || syncInFlight) return;
  syncInFlight = true;
  const startGen = mutationSeq;
  try {
    const res = await watchTogetherApi.getCurrentRoom();
    if (startGen === mutationSeq) {
      applyRoomSnapshot(res.room ?? null);
      const freshInvites = (res.invites ?? []).filter(
        (inv) => !declinedInviteIds.has(inv.invite_id)
      );
      watchTogetherState.invites = freshInvites;
    }

    syncTickCount += 1;
    if (syncTickCount % 2 === 0) {
      void refreshFriends();
    }
  } catch {
    // Transient network error
  } finally {
    syncInFlight = false;
  }
}

function startBackgroundSyncLoop(): void {
  if (syncPollTimer) clearInterval(syncPollTimer);
  syncPollTimer = setInterval(() => {
    void pollRoomAndInvitesOnce();
  }, 1000);
}

export async function connectWatchTogether(activity = ''): Promise<void> {
  if (!auth.user || watchTogetherState.connecting) return;
  ensureEventListeners();
  startBackgroundSyncLoop();
  watchTogetherState.connecting = true;
  watchTogetherState.error = null;
  const startGen = ++mutationSeq;
  try {
    const res = await watchTogetherApi.connect(activity);
    watchTogetherState.connected = res.connected;
    watchTogetherState.transport = res.transport;
    if (startGen === mutationSeq) {
      applyRoomSnapshot(res.room ?? null);
      watchTogetherState.invites = (res.invites ?? []).filter(
        (inv) => !declinedInviteIds.has(inv.invite_id)
      );
    }
    if (heartbeatTimer) clearInterval(heartbeatTimer);
    heartbeatTimer = setInterval(() => {
      if (auth.user) {
        void watchTogetherApi.heartbeat('online').catch(() => {});
      }
    }, 15_000);
    void refreshFriends();
  } catch (err) {
    watchTogetherState.error = err instanceof Error ? err.message : 'Failed to connect Watch Together';
  } finally {
    watchTogetherState.connecting = false;
  }
}

export async function disconnectWatchTogether(): Promise<void> {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }
  if (syncPollTimer) {
    clearInterval(syncPollTimer);
    syncPollTimer = null;
  }
  unbindEvents?.();
  unbindEvents = null;
  mutationSeq += 1;
  watchTogetherState.connected = false;
  watchTogetherState.transport = 'none';
  applyRoomSnapshot(null);
  watchTogetherState.invites = [];
  await watchTogetherApi.disconnect().catch(() => {});
}

export async function createWatchRoom(input: CreateRoomInput = {}): Promise<WatchRoomSnapshot> {
  mutationSeq += 1;
  const room = await watchTogetherApi.createRoom(input);
  mutationSeq += 1;
  applyRoomSnapshot(room);
  return room;
}

export async function joinWatchRoom(input: JoinRoomInput): Promise<WatchRoomSnapshot> {
  mutationSeq += 1;
  const room = await watchTogetherApi.joinRoom(input);
  mutationSeq += 1;
  applyRoomSnapshot(room);
  watchTogetherState.invites = watchTogetherState.invites.filter((i) => i.room_id !== room.room_id);
  return room;
}

export async function joinFriendWatchRoom(friendHandleOrPublicId: string): Promise<WatchRoomSnapshot> {
  return joinWatchRoom({ friend_target: friendHandleOrPublicId });
}

export async function leaveWatchRoom(): Promise<void> {
  mutationSeq += 1;
  applyRoomSnapshot(null);
  watchTogetherState.draftInvitedIds = [];
  try {
    await watchTogetherApi.leaveRoom();
  } finally {
    mutationSeq += 1;
    void refreshFriends();
  }
}

export async function setWatchRoomMedia(media: RoomMediaState): Promise<WatchRoomSnapshot> {
  mutationSeq += 1;
  const room = await watchTogetherApi.setMedia(media);
  mutationSeq += 1;
  applyRoomSnapshot(room);
  return room;
}

export async function syncWatchRoomPlayback(input: SyncPlaybackInput): Promise<RoomPlaybackState> {
  const raw = await watchTogetherApi.syncPlayback(input);
  const pb = stampPlayback(raw)!;
  if (watchTogetherState.room) {
    watchTogetherState.room.playback = pb;
  }
  return pb;
}

export async function updateWatchRoomMemberState(input: MemberStateInput): Promise<void> {
  await watchTogetherApi.updateMemberState(input);
}

export async function sendWatchRoomChat(
  text: string,
  kind: 'chat' | 'reaction' = 'chat'
): Promise<RoomChatMessage> {
  const msg = await watchTogetherApi.sendChat(text, kind);
  if (watchTogetherState.room && msg) {
    watchTogetherState.room.chat = appendUniqueChatMessage(
      watchTogetherState.room.chat ?? [],
      msg
    );
  }
  return msg;
}

export async function inviteFriendToWatchRoom(target: string): Promise<RoomInvite> {
  if (!watchTogetherState.room) {
    throw new Error('Create the room before sending invites');
  }
  mutationSeq += 1;
  const inv = await watchTogetherApi.inviteFriend(target);
  mutationSeq += 1;
  void pollRoomAndInvitesOnce();
  return inv;
}

export async function cancelWatchRoomInvite(target: string): Promise<void> {
  watchTogetherState.draftInvitedIds = watchTogetherState.draftInvitedIds.filter(
    (id) => id !== target
  );
  if (watchTogetherState.room) {
    mutationSeq += 1;
    const room = await watchTogetherApi.cancelInvite(target);
    mutationSeq += 1;
    applyRoomSnapshot(room);
  }
}

export async function declineWatchRoomInvite(invite: RoomInvite): Promise<void> {
  declinedInviteIds.add(invite.invite_id);
  watchTogetherState.invites = watchTogetherState.invites.filter(
    (i) => i.invite_id !== invite.invite_id && i.room_id !== invite.room_id
  );
  await watchTogetherApi.declineInvite(invite.room_id || invite.invite_id).catch(() => {});
}

export async function kickWatchRoomMember(target: string): Promise<WatchRoomSnapshot> {
  mutationSeq += 1;
  const room = await watchTogetherApi.kickMember(target);
  mutationSeq += 1;
  applyRoomSnapshot(room);
  return room;
}

export async function transferWatchRoomHost(target: string): Promise<WatchRoomSnapshot> {
  mutationSeq += 1;
  const room = await watchTogetherApi.transferHost(target);
  mutationSeq += 1;
  applyRoomSnapshot(room);
  return room;
}

export async function updateWatchRoomSettings(
  input: UpdateRoomSettingsInput
): Promise<WatchRoomSnapshot> {
  mutationSeq += 1;
  const room = await watchTogetherApi.updateSettings(input);
  mutationSeq += 1;
  applyRoomSnapshot(room);
  return room;
}

export function addDraftInviteFriend(publicId: string): void {
  if (!publicId || watchTogetherState.draftInvitedIds.includes(publicId)) return;
  const maxInvites = Math.max(1, watchTogetherState.draftMaxMembers - 1);
  if (watchTogetherState.draftInvitedIds.length >= maxInvites) return;
  watchTogetherState.draftInvitedIds = [...watchTogetherState.draftInvitedIds, publicId];
}

export function toggleDraftInviteFriend(publicId: string): void {
  if (!publicId) return;
  const idx = watchTogetherState.draftInvitedIds.indexOf(publicId);
  if (idx >= 0) {
    watchTogetherState.draftInvitedIds = watchTogetherState.draftInvitedIds.filter(
      (id) => id !== publicId
    );
  } else {
    addDraftInviteFriend(publicId);
  }
}

export async function selectAnimeForWatchTogether(item: {
  title: string;
  description?: string;
  poster?: string;
  shikiId?: number;
  releaseAlias?: string;
  score?: number;
  tags?: string[];
  searchQueries?: string[];
}): Promise<void> {
  const cachedArt = item.shikiId ? getCachedArtwork(item.shikiId) : undefined;
  const poster =
    cachedArt?.posterOriginal || cachedArt?.posterMain || item.poster || '';
  const backdrop = cachedArt?.screenshots?.[0] || poster;
  const description =
    item.description?.trim() ||
    cachedArt?.description?.trim() ||
    '';

  watchTogetherState.draftAnime = {
    title: item.title,
    description,
    poster,
    backdrop,
    shikiId: item.shikiId,
    releaseAlias: item.releaseAlias,
    score: item.score ?? cachedArt?.score,
    tags: item.tags ?? [],
    searchQueries: item.searchQueries ?? [item.title]
  };

  if (item.shikiId && (!cachedArt || !cachedArt.description)) {
    void ensureShikimoriArtwork([item.shikiId])
      .then((map) => {
        const art = map.get(item.shikiId!);
        const prev = watchTogetherState.draftAnime;
        if (!art || !prev || prev.shikiId !== item.shikiId) return;
        watchTogetherState.draftAnime = {
          ...prev,
          title: art.english || art.name || prev.title,
          description: art.description || prev.description,
          poster: art.posterOriginal || art.posterMain || prev.poster,
          backdrop:
            art.screenshots?.[0] ||
            art.posterOriginal ||
            art.posterMain ||
            prev.backdrop,
          score: art.score ?? prev.score
        };
      })
      .catch(() => {});
  }

  if (watchTogetherState.room) {
    try {
      await setWatchRoomMedia({
        shiki_id: item.shikiId,
        release_alias: item.releaseAlias,
        title: item.title,
        poster,
        episode_ordinal: 1,
        episode_index: 0,
        search_queries: item.searchQueries ?? [item.title]
      });
    } catch {
      // Non-host or offline: keep in draftAnime
    }
  }
}
