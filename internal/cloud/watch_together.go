package cloud

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
)

const WTQuicALPN = "mila-wt-v1"

type RoomMediaState struct {
	ShikiID        int64    `json:"shiki_id,omitempty"`
	ReleaseAlias   string   `json:"release_alias,omitempty"`
	Title          string   `json:"title"`
	Poster         string   `json:"poster,omitempty"`
	EpisodeOrdinal int      `json:"episode_ordinal"`
	EpisodeIndex   int      `json:"episode_index"`
	Provider       string   `json:"provider,omitempty"`
	VoiceID        string   `json:"voice_id,omitempty"`
	VoiceLabel     string   `json:"voice_label,omitempty"`
	DubVoiceID     string   `json:"dub_voice_id,omitempty"`
	DubOffsetSec   float64  `json:"dub_offset_sec,omitempty"`
	SearchQueries  []string `json:"search_queries,omitempty"`
}

type RoomPlaybackState struct {
	Paused       bool    `json:"paused"`
	PositionSec  float64 `json:"position_sec"`
	PlaybackRate float64 `json:"playback_rate"`
	UpdatedAtMs  int64   `json:"updated_at_ms"`
	ServerNowMs  int64   `json:"server_now_ms"`
	UpdatedBy    string  `json:"updated_by,omitempty"`
	Seq          uint64  `json:"seq"`
}

type RoomMember struct {
	PublicID    string  `json:"public_id"`
	Username    string  `json:"username"`
	Name        string  `json:"name"`
	AvatarURL   string  `json:"avatar_url"`
	Role        string  `json:"role"`
	Ready       bool    `json:"ready"`
	Buffering   bool    `json:"buffering"`
	PositionSec float64 `json:"position_sec"`
	RTTMs       int64   `json:"rtt_ms,omitempty"`
	JoinedAt    int64   `json:"joined_at"`
}

type RoomChatMessage struct {
	ID             string `json:"id"`
	SenderPublicID string `json:"sender_public_id"`
	SenderUsername string `json:"sender_username"`
	SenderName     string `json:"sender_name"`
	AvatarURL      string `json:"avatar_url"`
	Kind           string `json:"kind"`
	Text           string `json:"text"`
	CreatedAtMs    int64  `json:"created_at_ms"`
}

type RoomInvite struct {
	InviteID       string `json:"invite_id"`
	RoomID         string `json:"room_id"`
	RoomCode       string `json:"room_code"`
	FromPublicID   string `json:"from_public_id"`
	FromUsername   string `json:"from_username"`
	FromName       string `json:"from_name"`
	FromAvatarURL  string `json:"from_avatar_url"`
	AnimeTitle     string `json:"anime_title,omitempty"`
	EpisodeOrdinal int    `json:"episode_ordinal,omitempty"`
	CreatedAtMs    int64  `json:"created_at_ms"`
}

type WatchRoomSnapshot struct {
	RoomID       string            `json:"room_id"`
	RoomCode     string            `json:"room_code"`
	Title        string            `json:"title"`
	Visibility   string            `json:"visibility"`
	ControlMode  string            `json:"control_mode"`
	HostPublicID string            `json:"host_public_id"`
	HostUsername string            `json:"host_username"`
	HostName     string            `json:"host_name"`
	MaxMembers   int               `json:"max_members"`
	Media        RoomMediaState    `json:"media"`
	Playback     RoomPlaybackState `json:"playback"`
	Members      []RoomMember      `json:"members"`
	Chat         []RoomChatMessage `json:"chat"`
	CreatedAt    int64             `json:"created_at"`
	UpdatedAtMs  int64             `json:"updated_at_ms"`
}

type CreateRoomRequest struct {
	Title       string          `json:"title"`
	Visibility  string          `json:"visibility"`
	ControlMode string          `json:"control_mode"`
	MaxMembers  int             `json:"max_members"`
	Media       *RoomMediaState `json:"media,omitempty"`
}

type JoinRoomRequest struct {
	RoomID       string `json:"room_id,omitempty"`
	RoomCode     string `json:"room_code,omitempty"`
	FriendTarget string `json:"friend_target,omitempty"`
}

type SyncPlaybackRequest struct {
	Paused       *bool    `json:"paused,omitempty"`
	PositionSec  *float64 `json:"position_sec,omitempty"`
	PlaybackRate *float64 `json:"playback_rate,omitempty"`
}

type MemberStateRequest struct {
	Ready       *bool    `json:"ready,omitempty"`
	Buffering   *bool    `json:"buffering,omitempty"`
	PositionSec *float64 `json:"position_sec,omitempty"`
	RTTMs       *int64   `json:"rtt_ms,omitempty"`
}

type UpdateRoomSettingsRequest struct {
	Title       *string `json:"title,omitempty"`
	Visibility  *string `json:"visibility,omitempty"`
	ControlMode *string `json:"control_mode,omitempty"`
	MaxMembers  *int    `json:"max_members,omitempty"`
}

type WTWireMessage struct {
	Type        string                     `json:"type"`
	ReqID       string                     `json:"req_id,omitempty"`
	Token       string                     `json:"token,omitempty"`
	Error       string                     `json:"error,omitempty"`
	ServerNowMs int64                      `json:"server_now_ms,omitempty"`
	Status      string                     `json:"status,omitempty"`
	Activity    string                     `json:"activity,omitempty"`
	Target      string                     `json:"target,omitempty"`
	Text        string                     `json:"text,omitempty"`
	Kind        string                     `json:"kind,omitempty"`
	Create      *CreateRoomRequest         `json:"create,omitempty"`
	Join        *JoinRoomRequest           `json:"join,omitempty"`
	Media       *RoomMediaState            `json:"media,omitempty"`
	Playback    *RoomPlaybackState         `json:"playback,omitempty"`
	Sync        *SyncPlaybackRequest       `json:"sync,omitempty"`
	MemberState *MemberStateRequest        `json:"member_state,omitempty"`
	Settings    *UpdateRoomSettingsRequest `json:"settings,omitempty"`
	Room        *WatchRoomSnapshot         `json:"room,omitempty"`
	Member      *RoomMember                `json:"member,omitempty"`
	Chat        *RoomChatMessage           `json:"chat,omitempty"`
	Invite      *RoomInvite                `json:"invite,omitempty"`
	Friend      *FriendProfile             `json:"friend,omitempty"`
	User        *CloudUser                 `json:"user,omitempty"`
}

type WTSession struct {
	client    *Client
	conn      *quic.Conn
	stream    *quic.Stream
	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   map[string]chan WTWireMessage
	reqSeq    atomic.Uint64
	closed    atomic.Bool
	onEvent   func(WTWireMessage)
}

func (c *Client) quicTLSConfig() *tls.Config {
	var base *tls.Config
	if c != nil && c.tlsCfg != nil {
		base = c.tlsCfg.Clone()
	} else {
		base = &tls.Config{}
	}
	base.MinVersion = tls.VersionTLS13
	base.NextProtos = []string{WTQuicALPN}
	return base
}

func (c *Client) quicCandidateAddrs() []string {
	if custom := strings.TrimSpace(os.Getenv("MILA_CLOUD_QUIC_ADDR")); custom != "" {
		return []string{custom}
	}
	host := c.serverHost
	if host == "" {
		host = "50.17.87.91"
	}
	return []string{
		net.JoinHostPort(host, "443"),
		net.JoinHostPort(host, "7844"),
	}
}

func (c *Client) ConnectQUIC(ctx context.Context, activity string, onEvent func(WTWireMessage)) (*WTSession, error) {
	if c == nil {
		return nil, errors.New("cloud client is nil")
	}
	tok := c.getToken()
	if tok == "" {
		return nil, errors.New("not authenticated")
	}

	c.wtMu.Lock()
	if c.wtSession != nil && !c.wtSession.closed.Load() {
		sess := c.wtSession
		if onEvent != nil {
			sess.onEvent = onEvent
		}
		c.wtMu.Unlock()
		return sess, nil
	}
	c.wtMu.Unlock()

	var conn *quic.Conn
	var lastErr error
	for _, addr := range c.quicCandidateAddrs() {
		dialCtx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
		qConn, err := quic.DialAddr(dialCtx, addr, c.quicTLSConfig(), &quic.Config{
			MaxIdleTimeout:  60 * time.Second,
			KeepAlivePeriod: 15 * time.Second,
		})
		cancel()
		if err == nil {
			conn = qConn
			break
		}
		lastErr = err
	}
	if conn == nil {
		if lastErr == nil {
			lastErr = errors.New("failed to dial QUIC server")
		}
		return nil, lastErr
	}

	streamCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	stream, err := conn.OpenStreamSync(streamCtx)
	cancel()
	if err != nil {
		_ = conn.CloseWithError(0, "stream open failed")
		return nil, err
	}

	sess := &WTSession{
		client:  c,
		conn:    conn,
		stream:  stream,
		pending: make(map[string]chan WTWireMessage),
		onEvent: onEvent,
	}
	go sess.readLoop()

	authResp, err := sess.RoundTrip(ctx, WTWireMessage{
		Type:     "auth",
		Token:    tok,
		Activity: activity,
	})
	if err != nil {
		_ = sess.Close()
		return nil, err
	}
	if authResp.Type == "error" {
		_ = sess.Close()
		return nil, errors.New(authResp.Error)
	}

	c.wtMu.Lock()
	if c.wtSession != nil && c.wtSession != sess {
		_ = c.wtSession.Close()
	}
	c.wtSession = sess
	c.wtMu.Unlock()

	if onEvent != nil && authResp.Room != nil {
		onEvent(WTWireMessage{
			Type:        "room_snapshot",
			ServerNowMs: authResp.ServerNowMs,
			Room:        authResp.Room,
		})
	}
	return sess, nil
}

func (c *Client) DisconnectQUIC() {
	if c == nil {
		return
	}
	c.wtMu.Lock()
	sess := c.wtSession
	c.wtSession = nil
	c.wtMu.Unlock()
	if sess != nil {
		_ = sess.Close()
	}
}

func (c *Client) ActiveQUICSession() *WTSession {
	if c == nil {
		return nil
	}
	c.wtMu.Lock()
	defer c.wtMu.Unlock()
	if c.wtSession != nil && !c.wtSession.closed.Load() {
		return c.wtSession
	}
	return nil
}

func (s *WTSession) Close() error {
	if s == nil || s.closed.Swap(true) {
		return nil
	}
	_ = s.stream.Close()
	_ = s.conn.CloseWithError(0, "client closed")
	s.pendingMu.Lock()
	for id, ch := range s.pending {
		close(ch)
		delete(s.pending, id)
	}
	s.pendingMu.Unlock()
	return nil
}

func (s *WTSession) readLoop() {
	defer func() {
		_ = s.Close()
	}()
	scanner := bufio.NewScanner(s.stream)
	scanner.Buffer(make([]byte, 0, 16*1024), 128*1024)
	for scanner.Scan() {
		var msg WTWireMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.ReqID != "" {
			s.pendingMu.Lock()
			ch, ok := s.pending[msg.ReqID]
			if ok {
				delete(s.pending, msg.ReqID)
			}
			s.pendingMu.Unlock()
			if ok {
				ch <- msg
				close(ch)
				continue
			}
		}
		if s.onEvent != nil {
			s.onEvent(msg)
		}
	}
}

func (s *WTSession) RoundTrip(ctx context.Context, msg WTWireMessage) (WTWireMessage, error) {
	if s == nil || s.closed.Load() {
		return WTWireMessage{}, errors.New("quic session is closed")
	}
	reqID := fmt.Sprintf("r_%d", s.reqSeq.Add(1))
	msg.ReqID = reqID

	ch := make(chan WTWireMessage, 1)
	s.pendingMu.Lock()
	s.pending[reqID] = ch
	s.pendingMu.Unlock()

	raw, err := json.Marshal(msg)
	if err != nil {
		s.pendingMu.Lock()
		delete(s.pending, reqID)
		s.pendingMu.Unlock()
		return WTWireMessage{}, err
	}
	raw = append(raw, '\n')

	s.writeMu.Lock()
	_ = s.stream.SetWriteDeadline(time.Now().Add(6 * time.Second))
	_, wErr := s.stream.Write(raw)
	s.writeMu.Unlock()
	if wErr != nil {
		s.pendingMu.Lock()
		delete(s.pending, reqID)
		s.pendingMu.Unlock()
		_ = s.Close()
		return WTWireMessage{}, wErr
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return WTWireMessage{}, errors.New("quic stream closed before response")
		}
		if resp.Type == "error" {
			return resp, errors.New(resp.Error)
		}
		return resp, nil
	case <-ctx.Done():
		s.pendingMu.Lock()
		delete(s.pending, reqID)
		s.pendingMu.Unlock()
		return WTWireMessage{}, ctx.Err()
	}
}

// High-level Watch Together & Presence client methods (prefer active QUIC stream, fall back to HTTPS REST).

func (c *Client) HeartbeatPresence(ctx context.Context, status, activity string) (*FriendProfile, error) {
	if sess := c.ActiveQUICSession(); sess != nil {
		resp, err := sess.RoundTrip(ctx, WTWireMessage{
			Type:     "presence",
			Status:   status,
			Activity: activity,
		})
		if err == nil && resp.Friend != nil {
			return resp.Friend, nil
		}
	}
	var out struct {
		Presence FriendProfile `json:"presence"`
	}
	if err := c.PostJSON(ctx, "/api/v1/presence/heartbeat", map[string]string{
		"status":   status,
		"activity": activity,
	}, &out); err != nil {
		return nil, err
	}
	return &out.Presence, nil
}

func (c *Client) GetCurrentRoom(ctx context.Context) (*WatchRoomSnapshot, []RoomInvite, error) {
	var out struct {
		Room    *WatchRoomSnapshot `json:"room"`
		Invites []RoomInvite       `json:"invites"`
	}
	if err := c.GetJSON(ctx, "/api/v1/rooms/current", &out); err != nil {
		return nil, nil, err
	}
	return out.Room, out.Invites, nil
}

func (c *Client) CreateWatchRoom(ctx context.Context, req CreateRoomRequest) (*WatchRoomSnapshot, error) {
	if sess := c.ActiveQUICSession(); sess != nil {
		resp, err := sess.RoundTrip(ctx, WTWireMessage{
			Type:   "create_room",
			Create: &req,
		})
		if err == nil && resp.Room != nil {
			return resp.Room, nil
		}
	}
	var out struct {
		Room WatchRoomSnapshot `json:"room"`
	}
	if err := c.PostJSON(ctx, "/api/v1/rooms/create", req, &out); err != nil {
		return nil, err
	}
	return &out.Room, nil
}

func (c *Client) JoinWatchRoom(ctx context.Context, req JoinRoomRequest) (*WatchRoomSnapshot, error) {
	if sess := c.ActiveQUICSession(); sess != nil {
		resp, err := sess.RoundTrip(ctx, WTWireMessage{
			Type: "join_room",
			Join: &req,
		})
		if err == nil && resp.Room != nil {
			return resp.Room, nil
		}
	}
	var out struct {
		Room WatchRoomSnapshot `json:"room"`
	}
	if err := c.PostJSON(ctx, "/api/v1/rooms/join", req, &out); err != nil {
		return nil, err
	}
	return &out.Room, nil
}

func (c *Client) LeaveWatchRoom(ctx context.Context) error {
	if sess := c.ActiveQUICSession(); sess != nil {
		if _, err := sess.RoundTrip(ctx, WTWireMessage{Type: "leave_room"}); err == nil {
			return nil
		}
	}
	return c.PostJSON(ctx, "/api/v1/rooms/leave", map[string]any{}, nil)
}

func (c *Client) SetWatchRoomMedia(ctx context.Context, media RoomMediaState) (*WatchRoomSnapshot, error) {
	if sess := c.ActiveQUICSession(); sess != nil {
		resp, err := sess.RoundTrip(ctx, WTWireMessage{
			Type:  "set_media",
			Media: &media,
		})
		if err == nil && resp.Room != nil {
			return resp.Room, nil
		}
	}
	var out struct {
		Room WatchRoomSnapshot `json:"room"`
	}
	if err := c.PostJSON(ctx, "/api/v1/rooms/media", media, &out); err != nil {
		return nil, err
	}
	return &out.Room, nil
}

func (c *Client) SyncWatchRoomPlayback(ctx context.Context, req SyncPlaybackRequest) (*RoomPlaybackState, error) {
	if sess := c.ActiveQUICSession(); sess != nil {
		resp, err := sess.RoundTrip(ctx, WTWireMessage{
			Type: "sync_playback",
			Sync: &req,
		})
		if err == nil && resp.Playback != nil {
			return resp.Playback, nil
		}
	}
	var out struct {
		Playback RoomPlaybackState `json:"playback"`
	}
	if err := c.PostJSON(ctx, "/api/v1/rooms/sync", req, &out); err != nil {
		return nil, err
	}
	return &out.Playback, nil
}

func (c *Client) UpdateWatchRoomMemberState(ctx context.Context, req MemberStateRequest) (*RoomMember, error) {
	if sess := c.ActiveQUICSession(); sess != nil {
		resp, err := sess.RoundTrip(ctx, WTWireMessage{
			Type:        "member_state",
			MemberState: &req,
		})
		if err == nil && resp.Member != nil {
			return resp.Member, nil
		}
	}
	var out struct {
		Member RoomMember `json:"member"`
	}
	if err := c.PostJSON(ctx, "/api/v1/rooms/member", req, &out); err != nil {
		return nil, err
	}
	return &out.Member, nil
}

func (c *Client) SendWatchRoomChat(ctx context.Context, text, kind string) (*RoomChatMessage, error) {
	if sess := c.ActiveQUICSession(); sess != nil {
		resp, err := sess.RoundTrip(ctx, WTWireMessage{
			Type: "chat",
			Text: text,
			Kind: kind,
		})
		if err == nil && resp.Chat != nil {
			return resp.Chat, nil
		}
	}
	var out struct {
		Message RoomChatMessage `json:"message"`
	}
	if err := c.PostJSON(ctx, "/api/v1/rooms/chat", map[string]string{
		"text": text,
		"kind": kind,
	}, &out); err != nil {
		return nil, err
	}
	return &out.Message, nil
}

func (c *Client) InviteFriendToWatchRoom(ctx context.Context, target string) (*RoomInvite, error) {
	if sess := c.ActiveQUICSession(); sess != nil {
		resp, err := sess.RoundTrip(ctx, WTWireMessage{
			Type:   "invite_friend",
			Target: target,
		})
		if err == nil && resp.Invite != nil {
			return resp.Invite, nil
		}
	}
	var out struct {
		Invite RoomInvite `json:"invite"`
	}
	if err := c.PostJSON(ctx, "/api/v1/rooms/invite", map[string]string{"target": target}, &out); err != nil {
		return nil, err
	}
	return &out.Invite, nil
}

func (c *Client) KickWatchRoomMember(ctx context.Context, target string) (*WatchRoomSnapshot, error) {
	if sess := c.ActiveQUICSession(); sess != nil {
		resp, err := sess.RoundTrip(ctx, WTWireMessage{
			Type:   "kick_member",
			Target: target,
		})
		if err == nil && resp.Room != nil {
			return resp.Room, nil
		}
	}
	var out struct {
		Room WatchRoomSnapshot `json:"room"`
	}
	if err := c.PostJSON(ctx, "/api/v1/rooms/kick", map[string]string{"target": target}, &out); err != nil {
		return nil, err
	}
	return &out.Room, nil
}

func (c *Client) TransferWatchRoomHost(ctx context.Context, target string) (*WatchRoomSnapshot, error) {
	if sess := c.ActiveQUICSession(); sess != nil {
		resp, err := sess.RoundTrip(ctx, WTWireMessage{
			Type:   "transfer_host",
			Target: target,
		})
		if err == nil && resp.Room != nil {
			return resp.Room, nil
		}
	}
	var out struct {
		Room WatchRoomSnapshot `json:"room"`
	}
	if err := c.PostJSON(ctx, "/api/v1/rooms/transfer", map[string]string{"target": target}, &out); err != nil {
		return nil, err
	}
	return &out.Room, nil
}

func (c *Client) UpdateWatchRoomSettings(ctx context.Context, req UpdateRoomSettingsRequest) (*WatchRoomSnapshot, error) {
	if sess := c.ActiveQUICSession(); sess != nil {
		resp, err := sess.RoundTrip(ctx, WTWireMessage{
			Type:     "update_room",
			Settings: &req,
		})
		if err == nil && resp.Room != nil {
			return resp.Room, nil
		}
	}
	var out struct {
		Room WatchRoomSnapshot `json:"room"`
	}
	if err := c.PostJSON(ctx, "/api/v1/rooms/settings", req, &out); err != nil {
		return nil, err
	}
	return &out.Room, nil
}
