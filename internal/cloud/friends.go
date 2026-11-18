package cloud

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type FriendRoomPresence struct {
	RoomID       string `json:"room_id"`
	RoomCode     string `json:"room_code,omitempty"`
	HostPublicID string `json:"host_public_id"`
	HostUsername string `json:"host_username"`
	HostName     string `json:"host_name"`
	AnimeTitle   string `json:"anime_title,omitempty"`
	EpisodeOrd   int    `json:"episode_ordinal,omitempty"`
	IsHost       bool   `json:"is_host"`
	IsPublic     bool   `json:"is_public"`
	MemberCount  int    `json:"member_count"`
	MaxMembers   int    `json:"max_members"`
}

type FriendProfile struct {
	PublicID   string              `json:"public_id"`
	Username   string              `json:"username"`
	Name       string              `json:"name"`
	AvatarURL  string              `json:"avatar_url"`
	Relation   string              `json:"relation,omitempty"`
	Since      int64               `json:"since,omitempty"`
	UpdatedAt  int64               `json:"updated_at,omitempty"`
	Status     string              `json:"status"`
	LastSeenAt int64               `json:"last_seen_at,omitempty"`
	Activity   string              `json:"activity,omitempty"`
	Room       *FriendRoomPresence `json:"room,omitempty"`
}

type FriendsSnapshot struct {
	Friends  []FriendProfile `json:"friends"`
	Incoming []FriendProfile `json:"incoming"`
	Outgoing []FriendProfile `json:"outgoing"`
	Blocked  []FriendProfile `json:"blocked"`
}

type FriendActionResult struct {
	Relation string        `json:"relation"`
	Profile  FriendProfile `json:"profile"`
}

func (c *Client) SetUsername(ctx context.Context, username string) (*CloudUser, error) {
	var out struct {
		User CloudUser `json:"user"`
	}
	err := c.DoJSON(ctx, http.MethodPut, "/api/v1/user/username", map[string]string{
		"username": strings.TrimSpace(username),
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out.User, nil
}

func (c *Client) SearchUsers(ctx context.Context, query string, limit int) ([]FriendProfile, error) {
	params := url.Values{}
	params.Set("q", strings.TrimSpace(query))
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	var out struct {
		Users []FriendProfile `json:"users"`
	}
	if err := c.GetJSON(ctx, "/api/v1/users/search?"+params.Encode(), &out); err != nil {
		return nil, err
	}
	return out.Users, nil
}

func (c *Client) ListFriends(ctx context.Context) (*FriendsSnapshot, error) {
	var out FriendsSnapshot
	if err := c.GetJSON(ctx, "/api/v1/friends", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) postFriendTarget(ctx context.Context, path, target string) (*FriendActionResult, error) {
	var out FriendActionResult
	if err := c.PostJSON(ctx, path, map[string]string{
		"target": strings.TrimSpace(target),
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SendFriendRequest(ctx context.Context, target string) (*FriendActionResult, error) {
	return c.postFriendTarget(ctx, "/api/v1/friends/request", target)
}

func (c *Client) AcceptFriendRequest(ctx context.Context, target string) (*FriendActionResult, error) {
	return c.postFriendTarget(ctx, "/api/v1/friends/accept", target)
}

func (c *Client) DeclineFriendRequest(ctx context.Context, target string) (*FriendActionResult, error) {
	return c.postFriendTarget(ctx, "/api/v1/friends/decline", target)
}

func (c *Client) RemoveFriend(ctx context.Context, target string) (*FriendActionResult, error) {
	return c.postFriendTarget(ctx, "/api/v1/friends/remove", target)
}

func (c *Client) BlockUser(ctx context.Context, target string) (*FriendActionResult, error) {
	return c.postFriendTarget(ctx, "/api/v1/friends/block", target)
}

func (c *Client) UnblockUser(ctx context.Context, target string) (*FriendActionResult, error) {
	return c.postFriendTarget(ctx, "/api/v1/friends/unblock", target)
}
