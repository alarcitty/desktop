import {
  friendsApi,
  type FriendActionResult,
  type FriendProfile
} from '../api/friends';
import { auth } from './auth.svelte';

export interface FriendsStoreState {
  friends: FriendProfile[];
  incoming: FriendProfile[];
  outgoing: FriendProfile[];
  blocked: FriendProfile[];
  loading: boolean;
  loaded: boolean;
  error: string | null;
  popupOpen: boolean;
}

export const friendsState = $state<FriendsStoreState>({
  friends: [],
  incoming: [],
  outgoing: [],
  blocked: [],
  loading: false,
  loaded: false,
  error: null,
  popupOpen: false
});

export function openFriendsPopup(): void {
  friendsState.popupOpen = true;
  void refreshFriends();
}

export function closeFriendsPopup(): void {
  friendsState.popupOpen = false;
}

export async function refreshFriends(): Promise<void> {
  if (!auth.user) {
    resetFriendsState();
    return;
  }
  friendsState.loading = true;
  friendsState.error = null;
  try {
    const snap = await friendsApi.listFriends();
    friendsState.friends = snap.friends ?? [];
    friendsState.incoming = snap.incoming ?? [];
    friendsState.outgoing = snap.outgoing ?? [];
    friendsState.blocked = snap.blocked ?? [];
    friendsState.loaded = true;
  } catch (err) {
    friendsState.error = err instanceof Error ? err.message : 'Failed to load friends';
  } finally {
    friendsState.loading = false;
  }
}

export function resetFriendsState(): void {
  friendsState.friends = [];
  friendsState.incoming = [];
  friendsState.outgoing = [];
  friendsState.blocked = [];
  friendsState.loading = false;
  friendsState.loaded = false;
  friendsState.error = null;
  friendsState.popupOpen = false;
}

export async function updateUsername(username: string) {
  const user = await friendsApi.setUsername(username);
  auth.user = user;
  return user;
}

export function searchUsersByHandle(query: string, limit = 15): Promise<FriendProfile[]> {
  return friendsApi.searchUsers(query, limit);
}

async function runFriendMutation(fn: () => Promise<FriendActionResult>): Promise<FriendActionResult> {
  const res = await fn();
  await refreshFriends();
  return res;
}

export function sendFriendRequest(target: string): Promise<FriendActionResult> {
  return runFriendMutation(() => friendsApi.sendRequest(target));
}

export function acceptFriendRequest(target: string): Promise<FriendActionResult> {
  return runFriendMutation(() => friendsApi.acceptRequest(target));
}

export function declineFriendRequest(target: string): Promise<FriendActionResult> {
  return runFriendMutation(() => friendsApi.declineRequest(target));
}

export function removeFriend(target: string): Promise<FriendActionResult> {
  return runFriendMutation(() => friendsApi.removeFriend(target));
}

export function blockUser(target: string): Promise<FriendActionResult> {
  return runFriendMutation(() => friendsApi.blockUser(target));
}

export function unblockUser(target: string): Promise<FriendActionResult> {
  return runFriendMutation(() => friendsApi.unblockUser(target));
}

export function applyFriendPresenceUpdate(updated: FriendProfile): void {
  if (!updated?.public_id) return;
  const idx = friendsState.friends.findIndex((f) => f.public_id === updated.public_id);
  if (idx >= 0) {
    friendsState.friends[idx] = {
      ...friendsState.friends[idx],
      ...updated
    };
  }
}
