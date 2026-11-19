import { getWailsMilaBridge } from './bridge';
import { cloudApi } from './cloud';
import type { FriendProfile } from './friends';

export type RoomVisibility = 'friends' | 'private' | 'public';
export type RoomControlMode = 'host_only' | 'everyone';
export type RoomMemberRole = 'host' | 'cohost' | 'viewer';

export interface RoomMediaState {
  shiki_id?: number;
  release_alias?: string;
  title: string;
  poster?: string;
  episode_ordinal: number;
  episode_index: number;
  provider?: string;
  voice_id?: string;
  voice_label?: string;
  dub_voice_id?: string;
  dub_offset_sec?: number;
  search_queries?: string[];
}

export interface RoomPlaybackState {
  paused: boolean;
  position_sec: number;
  playback_rate: number;
  updated_at_ms: number;
  server_now_ms: number;
  updated_by?: string;
  seq: number;
  client_received_at_ms?: number;
}

export interface RoomMember {
  public_id: string;
  username: string;
  name: string;
  avatar_url: string;
  role: RoomMemberRole;
  ready: boolean;
  buffering: boolean;
  position_sec: number;
  rtt_ms?: number;
  joined_at: number;
}

export interface RoomChatMessage {
  id: string;
  sender_public_id: string;
  sender_username: string;
  sender_name: string;
  avatar_url: string;
  kind: 'chat' | 'reaction' | 'system';
  text: string;
  created_at_ms: number;
}

export interface RoomInvite {
  invite_id: string;
  room_id: string;
  room_code: string;
  room_title?: string;
  from_public_id: string;
  from_username: string;
  from_name: string;
  from_avatar_url: string;
  anime_title?: string;
  anime_poster?: string;
  episode_ordinal?: number;
  member_count?: number;
  max_members?: number;
  member_avatars?: string[];
  created_at_ms: number;
}

export interface WatchRoomSnapshot {
  room_id: string;
  room_code: string;
  title: string;
  visibility: RoomVisibility;
  control_mode: RoomControlMode;
  host_public_id: string;
  host_username: string;
  host_name: string;
  max_members: number;
  media: RoomMediaState;
  playback: RoomPlaybackState;
  members: RoomMember[];
  pending?: FriendProfile[];
  chat: RoomChatMessage[];
  created_at: number;
  updated_at_ms: number;
}

export interface CreateRoomInput {
  title?: string;
  visibility?: RoomVisibility;
  control_mode?: RoomControlMode;
  max_members?: number;
  media?: Partial<RoomMediaState>;
}

export interface JoinRoomInput {
  room_id?: string;
  room_code?: string;
  friend_target?: string;
}

export interface SyncPlaybackInput {
  paused?: boolean;
  position_sec?: number;
  playback_rate?: number;
}

export interface MemberStateInput {
  ready?: boolean;
  buffering?: boolean;
  position_sec?: number;
  rtt_ms?: number;
}

export interface UpdateRoomSettingsInput {
  title?: string;
  visibility?: RoomVisibility;
  control_mode?: RoomControlMode;
  max_members?: number;
}

export interface WTWireEvent {
  type:
    | 'room_snapshot'
    | 'room_playback'
    | 'room_media'
    | 'room_member'
    | 'room_chat'
    | 'room_closed'
    | 'room_invite'
    | 'friend_presence'
    | string;
  server_now_ms?: number;
  text?: string;
  room?: WatchRoomSnapshot | null;
  media?: RoomMediaState;
  playback?: RoomPlaybackState;
  member?: RoomMember;
  chat?: RoomChatMessage;
  invite?: RoomInvite;
  friend?: FriendProfile;
}

export interface ConnectWatchTogetherResult {
  connected: boolean;
  transport: 'quic' | 'https';
  room?: WatchRoomSnapshot | null;
  invites?: RoomInvite[];
  presence?: FriendProfile;
}

interface DataEnvelope<T> {
  data?: T;
  error?: string;
}

function unwrapEnvelope<T>(raw: string): T {
  const parsed = JSON.parse(raw) as DataEnvelope<T>;
  if (parsed.error) {
    throw new Error(parsed.error);
  }
  return parsed.data as T;
}

export function computeLiveRoomPosition(playback: RoomPlaybackState, localRttMs = 0): number {
  const base = Number.isFinite(playback.position_sec) ? Math.max(0, playback.position_sec) : 0;
  if (playback.paused) {
    return base;
  }
  const serverDeltaMs =
    playback.server_now_ms > 0 && playback.updated_at_ms > 0
      ? Math.max(0, playback.server_now_ms - playback.updated_at_ms)
      : 0;
  const localDeltaMs =
    playback.client_received_at_ms && playback.client_received_at_ms > 0
      ? Math.max(0, Date.now() - playback.client_received_at_ms)
      : 0;
  const rttHalfMs = Math.max(0, localRttMs) * 0.5;
  const rate = Number.isFinite(playback.playback_rate) && playback.playback_rate > 0 ? playback.playback_rate : 1;
  return base + ((serverDeltaMs + localDeltaMs + rttHalfMs) / 1000) * rate;
}

export const watchTogetherApi = {
  async connect(activity = ''): Promise<ConnectWatchTogetherResult> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudConnectWatchTogether) {
      return unwrapEnvelope<ConnectWatchTogetherResult>(await bridge.CloudConnectWatchTogether(activity));
    }
    const hb = await cloudApi.post<{ presence: FriendProfile }>('/api/v1/presence/heartbeat', {
      status: 'online',
      activity
    });
    const cur = await cloudApi.get<{ room: WatchRoomSnapshot | null; invites: RoomInvite[] }>('/api/v1/rooms/current');
    return {
      connected: true,
      transport: 'https',
      presence: hb.presence,
      room: cur.room,
      invites: cur.invites ?? []
    };
  },

  async disconnect(): Promise<void> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudDisconnectWatchTogether) {
      unwrapEnvelope<{ ok: boolean }>(await bridge.CloudDisconnectWatchTogether());
      return;
    }
    await cloudApi.post('/api/v1/presence/heartbeat', { status: 'offline', activity: '' });
  },

  async heartbeat(status: 'online' | 'offline' = 'online', activity = ''): Promise<FriendProfile> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudHeartbeatPresence) {
      const res = unwrapEnvelope<{ presence: FriendProfile }>(
        await bridge.CloudHeartbeatPresence(status, activity)
      );
      return res.presence;
    }
    const res = await cloudApi.post<{ presence: FriendProfile }>('/api/v1/presence/heartbeat', {
      status,
      activity
    });
    return res.presence;
  },

  async getCurrentRoom(): Promise<{ room: WatchRoomSnapshot | null; invites: RoomInvite[] }> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudGetWatchRoom) {
      const res = unwrapEnvelope<{ room: WatchRoomSnapshot | null; invites: RoomInvite[] }>(
        await bridge.CloudGetWatchRoom()
      );
      return { room: res.room ?? null, invites: res.invites ?? [] };
    }
    const res = await cloudApi.get<{ room: WatchRoomSnapshot | null; invites: RoomInvite[] }>('/api/v1/rooms/current');
    return { room: res.room ?? null, invites: res.invites ?? [] };
  },

  async createRoom(input: CreateRoomInput = {}): Promise<WatchRoomSnapshot> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudCreateWatchRoom) {
      const res = unwrapEnvelope<{ room: WatchRoomSnapshot }>(
        await bridge.CloudCreateWatchRoom(JSON.stringify(input))
      );
      return res.room;
    }
    const res = await cloudApi.post<{ room: WatchRoomSnapshot }>('/api/v1/rooms/create', input);
    return res.room;
  },

  async joinRoom(input: JoinRoomInput): Promise<WatchRoomSnapshot> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudJoinWatchRoom) {
      const res = unwrapEnvelope<{ room: WatchRoomSnapshot }>(
        await bridge.CloudJoinWatchRoom(JSON.stringify(input))
      );
      return res.room;
    }
    const res = await cloudApi.post<{ room: WatchRoomSnapshot }>('/api/v1/rooms/join', input);
    return res.room;
  },

  async leaveRoom(): Promise<void> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudLeaveWatchRoom) {
      unwrapEnvelope<{ ok: boolean }>(await bridge.CloudLeaveWatchRoom());
      return;
    }
    await cloudApi.post('/api/v1/rooms/leave', {});
  },

  async setMedia(media: RoomMediaState): Promise<WatchRoomSnapshot> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudSetWatchRoomMedia) {
      const res = unwrapEnvelope<{ room: WatchRoomSnapshot }>(
        await bridge.CloudSetWatchRoomMedia(JSON.stringify(media))
      );
      return res.room;
    }
    const res = await cloudApi.post<{ room: WatchRoomSnapshot }>('/api/v1/rooms/media', media);
    return res.room;
  },

  async syncPlayback(input: SyncPlaybackInput): Promise<RoomPlaybackState> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudSyncWatchRoomPlayback) {
      const res = unwrapEnvelope<{ playback: RoomPlaybackState }>(
        await bridge.CloudSyncWatchRoomPlayback(JSON.stringify(input))
      );
      return res.playback;
    }
    const res = await cloudApi.post<{ playback: RoomPlaybackState }>('/api/v1/rooms/sync', input);
    return res.playback;
  },

  async updateMemberState(input: MemberStateInput): Promise<RoomMember> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudUpdateWatchRoomMember) {
      const res = unwrapEnvelope<{ member: RoomMember }>(
        await bridge.CloudUpdateWatchRoomMember(JSON.stringify(input))
      );
      return res.member;
    }
    const res = await cloudApi.post<{ member: RoomMember }>('/api/v1/rooms/member', input);
    return res.member;
  },

  async sendChat(text: string, kind: 'chat' | 'reaction' = 'chat'): Promise<RoomChatMessage> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudSendWatchRoomChat) {
      const res = unwrapEnvelope<{ message: RoomChatMessage }>(
        await bridge.CloudSendWatchRoomChat(text, kind)
      );
      return res.message;
    }
    const res = await cloudApi.post<{ message: RoomChatMessage }>('/api/v1/rooms/chat', { text, kind });
    return res.message;
  },

  async inviteFriend(target: string): Promise<RoomInvite> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudInviteFriendToWatchRoom) {
      const res = unwrapEnvelope<{ invite: RoomInvite }>(
        await bridge.CloudInviteFriendToWatchRoom(target)
      );
      return res.invite;
    }
    const res = await cloudApi.post<{ invite: RoomInvite }>('/api/v1/rooms/invite', { target });
    return res.invite;
  },

  async cancelInvite(target: string): Promise<WatchRoomSnapshot> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudCancelWatchRoomInvite) {
      const res = unwrapEnvelope<{ room: WatchRoomSnapshot }>(
        await bridge.CloudCancelWatchRoomInvite(target)
      );
      return res.room;
    }
    const res = await cloudApi.post<{ room: WatchRoomSnapshot }>('/api/v1/rooms/invite/cancel', { target });
    return res.room;
  },

  async declineInvite(target: string): Promise<void> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudDeclineWatchRoomInvite) {
      unwrapEnvelope<{ ok: boolean }>(await bridge.CloudDeclineWatchRoomInvite(target));
      return;
    }
    await cloudApi.post('/api/v1/rooms/invite/decline', { target });
  },

  async kickMember(target: string): Promise<WatchRoomSnapshot> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudKickWatchRoomMember) {
      const res = unwrapEnvelope<{ room: WatchRoomSnapshot }>(
        await bridge.CloudKickWatchRoomMember(target)
      );
      return res.room;
    }
    const res = await cloudApi.post<{ room: WatchRoomSnapshot }>('/api/v1/rooms/kick', { target });
    return res.room;
  },

  async transferHost(target: string): Promise<WatchRoomSnapshot> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudTransferWatchRoomHost) {
      const res = unwrapEnvelope<{ room: WatchRoomSnapshot }>(
        await bridge.CloudTransferWatchRoomHost(target)
      );
      return res.room;
    }
    const res = await cloudApi.post<{ room: WatchRoomSnapshot }>('/api/v1/rooms/transfer', { target });
    return res.room;
  },

  async updateSettings(input: UpdateRoomSettingsInput): Promise<WatchRoomSnapshot> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudUpdateWatchRoomSettings) {
      const res = unwrapEnvelope<{ room: WatchRoomSnapshot }>(
        await bridge.CloudUpdateWatchRoomSettings(JSON.stringify(input))
      );
      return res.room;
    }
    const res = await cloudApi.post<{ room: WatchRoomSnapshot }>('/api/v1/rooms/settings', input);
    return res.room;
  }
};
