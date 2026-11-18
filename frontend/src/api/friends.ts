import { getWailsMilaBridge } from './bridge';
import { cloudApi, type CloudUser } from './cloud';

export type FriendRelation = 'none' | 'friend' | 'outgoing' | 'incoming' | 'blocked' | 'blocked_by';

export type FriendPresenceStatus = 'offline' | 'online' | 'in_room';

export interface FriendRoomPresence {
  room_id: string;
  room_code?: string;
  host_public_id: string;
  host_username: string;
  host_name: string;
  anime_title?: string;
  episode_ordinal?: number;
  is_host: boolean;
  is_public: boolean;
  member_count: number;
  max_members: number;
}

export interface FriendProfile {
  public_id: string;
  username: string;
  name: string;
  avatar_url: string;
  relation?: FriendRelation;
  since?: number;
  updated_at?: number;
  status?: FriendPresenceStatus;
  last_seen_at?: number;
  activity?: string;
  room?: FriendRoomPresence | null;
}

export interface FriendsSnapshot {
  friends: FriendProfile[];
  incoming: FriendProfile[];
  outgoing: FriendProfile[];
  blocked: FriendProfile[];
}

export interface FriendActionResult {
  relation: FriendRelation;
  profile: FriendProfile;
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

export const friendsApi = {
  async setUsername(username: string): Promise<CloudUser> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudSetUsername) {
      const res = unwrapEnvelope<{ user: CloudUser }>(await bridge.CloudSetUsername(username));
      return res.user;
    }
    const res = await cloudApi.put<{ user: CloudUser }>('/api/v1/user/username', { username });
    return res.user;
  },

  async searchUsers(query: string, limit = 15): Promise<FriendProfile[]> {
    const clean = query.trim();
    if (!clean) return [];
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudSearchUsers) {
      const res = unwrapEnvelope<{ users: FriendProfile[] }>(await bridge.CloudSearchUsers(clean, limit));
      return res.users ?? [];
    }
    const params = new URLSearchParams({ q: clean, limit: String(limit) });
    const res = await cloudApi.get<{ users: FriendProfile[] }>(`/api/v1/users/search?${params.toString()}`);
    return res.users ?? [];
  },

  async listFriends(): Promise<FriendsSnapshot> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudListFriends) {
      return unwrapEnvelope<FriendsSnapshot>(await bridge.CloudListFriends());
    }
    return cloudApi.get<FriendsSnapshot>('/api/v1/friends');
  },

  async sendRequest(target: string): Promise<FriendActionResult> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudSendFriendRequest) {
      return unwrapEnvelope<FriendActionResult>(await bridge.CloudSendFriendRequest(target));
    }
    return cloudApi.post<FriendActionResult>('/api/v1/friends/request', { target });
  },

  async acceptRequest(target: string): Promise<FriendActionResult> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudAcceptFriendRequest) {
      return unwrapEnvelope<FriendActionResult>(await bridge.CloudAcceptFriendRequest(target));
    }
    return cloudApi.post<FriendActionResult>('/api/v1/friends/accept', { target });
  },

  async declineRequest(target: string): Promise<FriendActionResult> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudDeclineFriendRequest) {
      return unwrapEnvelope<FriendActionResult>(await bridge.CloudDeclineFriendRequest(target));
    }
    return cloudApi.post<FriendActionResult>('/api/v1/friends/decline', { target });
  },

  async removeFriend(target: string): Promise<FriendActionResult> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudRemoveFriend) {
      return unwrapEnvelope<FriendActionResult>(await bridge.CloudRemoveFriend(target));
    }
    return cloudApi.post<FriendActionResult>('/api/v1/friends/remove', { target });
  },

  async blockUser(target: string): Promise<FriendActionResult> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudBlockUser) {
      return unwrapEnvelope<FriendActionResult>(await bridge.CloudBlockUser(target));
    }
    return cloudApi.post<FriendActionResult>('/api/v1/friends/block', { target });
  },

  async unblockUser(target: string): Promise<FriendActionResult> {
    const bridge = getWailsMilaBridge();
    if (bridge?.CloudUnblockUser) {
      return unwrapEnvelope<FriendActionResult>(await bridge.CloudUnblockUser(target));
    }
    return cloudApi.post<FriendActionResult>('/api/v1/friends/unblock', { target });
  }
};
