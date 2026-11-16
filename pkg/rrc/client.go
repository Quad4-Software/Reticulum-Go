// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// ClientState is the client session state machine (4-RRC).
type ClientState int

const (
	ClientConnected ClientState = iota
	ClientAwaitingWelcome
	ClientActive
	ClientDisconnected
)

// ClientConfig configures Dial / client HELLO metadata.
type ClientConfig struct {
	Nick           string
	Name           string
	Version        string
	Capabilities   map[uint64]any
	DialTimeout    time.Duration
	WelcomeTimeout time.Duration
	Handlers       ClientHandlers
}

// Client is an RRC client session over a single Link.
type Client struct {
	mu       sync.Mutex
	tr       *transport.Transport
	id       *identity.Identity
	sender   []byte
	cfg      ClientConfig
	state    ClientState
	sess     *session
	rooms    map[string]struct{}
	pending  map[string]struct{}
	welcome  *WelcomeBody
	handlers ClientHandlers
	lastErr  error
}

// Dial establishes a Link to hubHash, sends HELLO, and waits for WELCOME.
func Dial(tr *transport.Transport, id *identity.Identity, hubHash []byte, cfg ClientConfig) (*Client, error) {
	if tr == nil || id == nil {
		return nil, ErrNilArgument
	}
	if len(hubHash) != IdentityLength {
		return nil, fmt.Errorf("%w: hub hash", ErrBadFieldLength)
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 45 * time.Second
	}
	if cfg.WelcomeTimeout <= 0 {
		cfg.WelcomeTimeout = 30 * time.Second
	}

	c := &Client{
		tr:       tr,
		id:       id,
		sender:   append([]byte(nil), id.Hash()...),
		cfg:      cfg,
		state:    ClientConnected,
		rooms:    make(map[string]struct{}),
		pending:  make(map[string]struct{}),
		handlers: cfg.Handlers,
	}

	if !tr.HasPath(hubHash) {
		_ = tr.RequestPath(hubHash, "", nil, true)
	}
	if !waitUntil(cfg.DialTimeout, 50*time.Millisecond, func() bool { return tr.HasPath(hubHash) }) {
		return nil, fmt.Errorf("%w: no path", ErrDialTimeout)
	}

	remote, err := identity.Recall(hubHash)
	if err != nil || remote == nil {
		if err != nil {
			return nil, fmt.Errorf("%w: recall: %v", ErrDialTimeout, err)
		}
		return nil, fmt.Errorf("%w: recall", ErrDialTimeout)
	}
	destOut, err := destination.FromHash(hubHash, remote, destination.Single, tr)
	if err != nil {
		return nil, err
	}

	welcomeCh := make(chan *Envelope, 1)
	closedCh := make(chan struct{})
	var closeOnce sync.Once

	lnk := link.NewLink(destOut, tr, nil, nil, func(*link.Link) {
		closeOnce.Do(func() { close(closedCh) })
		c.onLinkClosed()
	})

	c.sess = newSession(lnk, c.sender, true, func(env *Envelope) {
		c.dispatch(env, welcomeCh)
	}, func() {
		c.onLinkClosed()
	})
	if cfg.Nick != "" {
		c.sess.setNick(cfg.Nick)
	}

	if err := lnk.Establish(); err != nil {
		return nil, fmt.Errorf("establish: %w", err)
	}
	lnk.Start()

	if !waitUntil(cfg.DialTimeout, 25*time.Millisecond, lnk.IsActive) {
		lnk.Teardown()
		return nil, fmt.Errorf("%w: link", ErrDialTimeout)
	}

	if err := lnk.Identify(id); err != nil {
		lnk.Teardown()
		return nil, fmt.Errorf("identify: %w", err)
	}

	c.mu.Lock()
	c.state = ClientAwaitingWelcome
	c.mu.Unlock()
	if err := c.sendHello(); err != nil {
		lnk.Teardown()
		return nil, err
	}

	welcomeTimer := time.NewTimer(cfg.WelcomeTimeout)
	defer welcomeTimer.Stop()
	select {
	case env := <-welcomeCh:
		body, err := ParseWelcomeBody(env.Body)
		if err != nil {
			lnk.Teardown()
			return nil, err
		}
		c.mu.Lock()
		c.welcome = body
		c.state = ClientActive
		h := c.handlers.OnWelcome
		c.mu.Unlock()
		if h != nil {
			h(body, env)
		}
	case <-welcomeTimer.C:
		lnk.Teardown()
		return nil, ErrWelcomeTimeout
	case <-closedCh:
		return nil, ErrSessionClosed
	}
	return c, nil
}

func DialHash(tr *transport.Transport, id *identity.Identity, hubHex string, cfg ClientConfig) (*Client, error) {
	hubHash, err := ParseHash(hubHex)
	if err != nil {
		return nil, err
	}
	return Dial(tr, id, hubHash, cfg)
}

func (c *Client) sendHello() error {
	body := &HelloBody{}
	if c.cfg.Name != "" {
		body.ClientName = c.cfg.Name
		body.HasName = true
	}
	if c.cfg.Version != "" {
		body.ClientVersion = c.cfg.Version
		body.HasVersion = true
	}
	if c.cfg.Capabilities != nil {
		body.Capabilities = c.cfg.Capabilities
		body.HasCaps = true
	}
	var payload any
	if m := body.ToMap(); m != nil {
		payload = m
	}
	return c.sess.sendType(TypeHello, "", payload, c.sess.getNick())
}

func (c *Client) dispatch(env *Envelope, welcomeCh chan<- *Envelope) {
	c.mu.Lock()
	state := c.state
	c.mu.Unlock()

	switch env.Type {
	case TypeWelcome:
		if state == ClientConnected || state == ClientAwaitingWelcome {
			select {
			case welcomeCh <- env:
			default:
			}
		}
	case TypeJoined:
		room := NormalizeRoom(env.Room)
		c.mu.Lock()
		c.rooms[room] = struct{}{}
		h := c.handlers.OnJoined
		c.mu.Unlock()
		members, _ := ParseJoinedMembers(env.Body)
		if h != nil {
			h(room, members, env)
		}
	case TypeParted:
		room := NormalizeRoom(env.Room)
		c.mu.Lock()
		_, selfPart := c.pending[room]
		if selfPart {
			delete(c.pending, room)
			delete(c.rooms, room)
		}
		h := c.handlers.OnParted
		c.mu.Unlock()
		if h != nil {
			h(room, env)
		}
	case TypeMsg:
		if h := c.handlers.OnMsg; h != nil {
			h(env)
		}
	case TypeNotice:
		if h := c.handlers.OnNotice; h != nil {
			h(env)
		}
	case TypeAction:
		if h := c.handlers.OnAction; h != nil {
			h(env)
		}
	case TypeError:
		c.setErr(fmt.Errorf("%w: %s", ErrHub, FormatError(env)))
		if h := c.handlers.OnError; h != nil {
			h(env)
		}
	case TypePong:
		if h := c.handlers.OnPong; h != nil {
			h(env)
		}
	case TypeResourceEnvelope:
		if h := c.handlers.OnResource; h != nil {
			h(env)
		}
	}
}

func (c *Client) onLinkClosed() {
	c.mu.Lock()
	if c.state == ClientDisconnected {
		c.mu.Unlock()
		return
	}
	c.state = ClientDisconnected
	c.rooms = make(map[string]struct{})
	c.pending = make(map[string]struct{})
	h := c.handlers.OnClose
	c.mu.Unlock()
	if h != nil {
		h()
	}
}

// State returns the current client session state.
func (c *Client) State() ClientState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Client) LastError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

func (c *Client) setErr(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	c.lastErr = err
	c.mu.Unlock()
}

// Welcome returns the WELCOME body if received.
func (c *Client) Welcome() *WelcomeBody {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.welcome
}

// Rooms returns a snapshot of joined room names (normalized).
func (c *Client) Rooms() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.rooms))
	for r := range c.rooms {
		out = append(out, r)
	}
	return out
}

func (c *Client) requireActive() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != ClientActive {
		return ErrNotWelcome
	}
	return nil
}

// Join requests membership in room.
func (c *Client) Join(room string) error {
	return c.JoinKeyed(room, "")
}

// JoinKeyed requests membership in room, sending key as the JOIN body when set.
// rrcd keyed rooms (+k) expect the room key in the JOIN body.
func (c *Client) JoinKeyed(room, key string) error {
	if err := c.requireActive(); err != nil {
		return err
	}
	room = NormalizeRoom(room)
	if room == "" {
		return fmt.Errorf("%w: empty room", ErrInvalidEnvelope)
	}
	var body any
	if strings.TrimSpace(key) != "" {
		body = key
	}
	return c.sess.sendType(TypeJoin, room, body, c.sess.getNick())
}

// SendSlash sends a hub slash command as a NOTICE with no room.
func (c *Client) SendSlash(cmd string) error {
	if err := c.requireActive(); err != nil {
		return err
	}
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("%w: empty command", ErrInvalidEnvelope)
	}
	return c.sess.sendType(TypeNotice, "", cmd, c.sess.getNick())
}

// Part leaves room.
func (c *Client) Part(room string) error {
	if err := c.requireActive(); err != nil {
		return err
	}
	room = NormalizeRoom(room)
	c.mu.Lock()
	if c.pending == nil {
		c.pending = make(map[string]struct{})
	}
	c.pending[room] = struct{}{}
	c.mu.Unlock()
	return c.sess.sendType(TypePart, room, nil, c.sess.getNick())
}

// SendMsg sends a chat MSG to room.
func (c *Client) SendMsg(room, text string) error {
	return c.sendRoomContent(TypeMsg, room, text)
}

// SendNotice sends a NOTICE to room.
func (c *Client) SendNotice(room, text string) error {
	return c.sendRoomContent(TypeNotice, room, text)
}

// SendAction sends an ACTION to room.
func (c *Client) SendAction(room, text string) error {
	return c.sendRoomContent(TypeAction, room, text)
}

func (c *Client) sendRoomContent(msgType uint64, room, text string) error {
	if err := c.requireActive(); err != nil {
		return err
	}
	room = NormalizeRoom(room)
	c.mu.Lock()
	_, member := c.rooms[room]
	c.mu.Unlock()
	if !member {
		// Client may still send if it believes it joined; hub will correct.
		// Spec allows sending only for rooms it believes joined - we track JOINED.
		return ErrNotMember
	}
	return c.sess.sendType(msgType, room, text, c.sess.getNick())
}

// SendDirectNotice sends a NOTICE to destHash without a room (rrcd K_DST).
func (c *Client) SendDirectNotice(destHash []byte, text string) error {
	if err := c.requireActive(); err != nil {
		return err
	}
	if len(destHash) != IdentityLength {
		return fmt.Errorf("%w: destination identity", ErrBadFieldLength)
	}
	env, err := NewEnvelope(TypeNotice, c.sender)
	if err != nil {
		return err
	}
	env.Destination = append([]byte(nil), destHash...)
	env.HasDestination = true
	env.Body = text
	env.HasBody = true
	if nick := c.sess.getNick(); nick != "" {
		env.Nick = nick
		env.HasNick = true
	}
	return c.sess.sendEnvelope(env)
}

// SendResourceEnvelope sends TypeResourceEnvelope metadata to the hub.
func (c *Client) SendResourceEnvelope(room string, body *ResourceEnvelopeBody) error {
	if err := c.requireActive(); err != nil {
		return err
	}
	env, err := NewEnvelope(TypeResourceEnvelope, c.sender)
	if err != nil {
		return err
	}
	if room != "" {
		env.Room = NormalizeRoom(room)
		env.HasRoom = true
	}
	if body != nil {
		if m := body.ToMap(); m != nil {
			env.Body = m
			env.HasBody = true
		}
	}
	if nick := c.sess.getNick(); nick != "" {
		env.Nick = nick
		env.HasNick = true
	}
	return c.sess.sendEnvelope(env)
}

// Ping sends a PING with optional body.
func (c *Client) Ping(body any) error {
	if err := c.requireActive(); err != nil {
		return err
	}
	return c.sess.sendType(TypePing, "", body, "")
}

// SetNick updates the advisory nickname for subsequent outbound messages.
func (c *Client) SetNick(nick string) {
	if c.sess != nil {
		c.sess.setNick(nick)
	}
}

// Close tears down the Link and ends the session.
func (c *Client) Close() {
	if c.sess != nil {
		c.sess.close()
	}
	c.onLinkClosed()
}

func waitUntil(timeout, interval time.Duration, ok func() bool) bool {
	if ok() {
		return true
	}
	deadline := time.Now().Add(timeout)
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for range tick.C {
		if ok() {
			return true
		}
		if !time.Now().Before(deadline) {
			return ok()
		}
	}
	return false
}
