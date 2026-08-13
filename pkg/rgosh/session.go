// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/channel"
)

// Message is the channel message interface used by sessions.
type Message = channel.MessageBase

// Sender delivers outbound protocol messages.
type Sender interface {
	Send(Message) error
}

// Config holds per-session settings. Call Copy before attaching to a session.
type Config struct {
	Compat          bool
	AllowAll        bool
	Allowed         [][]byte
	DefaultCmd      []string
	ForcedCommand   bool
	RemoteCmdAsArgs bool
	SoftwareVersion string
	Capabilities    uint16
	LineMode        bool
	Listener        bool
}

// Copy returns an isolated deep copy (prevents argv / allowlist pollution).
func (c Config) Copy() Config {
	out := c
	if c.Allowed != nil {
		out.Allowed = make([][]byte, len(c.Allowed))
		for i, h := range c.Allowed {
			out.Allowed[i] = append([]byte(nil), h...)
		}
	}
	if c.DefaultCmd != nil {
		out.DefaultCmd = append([]string(nil), c.DefaultCmd...)
	}
	return out
}

// Session is one rgosh connection FSM.
type Session struct {
	mu sync.Mutex

	cfg    Config
	state  State
	sender Sender

	remoteHash []byte
	authed     bool

	StartProcess func(exec ExecRequest) (ProcessHandle, error)
	OnExit       func(code int)
	OnStdout     func(data []byte)
	OnStderr     func(data []byte)
	OnAuthDenied func(reason string)
	OnError      func(msg string, fatal bool)
	OnTeardown   func()

	outboundMu sync.Mutex

	proc    ProcessHandle
	spawned bool
	closed  bool

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once

	// Early stdin can arrive before StartProcess finishes. Buffer until proc is set.
	stdinPending []byte
	stdinEOF     bool

	// Client: Exit can reorder ahead of Stream on UDP. Hold OnExit until stream
	// EOFs arrive or a short grace elapses.
	clientExitCode  *int
	clientStdoutEOF bool
	clientStderrEOF bool
	exitNotifyOnce  sync.Once
}

// ExecRequest is the resolved remote command.
type ExecRequest struct {
	Cmdline        []string
	PipeStdin      bool
	PipeStdout     bool
	PipeStderr     bool
	Term           string
	Rows           int
	Cols           int
	HPix           int
	VPix           int
	RemoteIdentity []byte
}

// ProcessHandle is a running remote process.
type ProcessHandle interface {
	Stdin() WriteCloser
	Stdout() Reader
	Stderr() Reader
	SetWinSize(rows, cols, hpix, vpix int) error
	Wait() (int, error)
	Kill() error
}

// WriteCloser is stdin to a process.
type WriteCloser interface {
	Write([]byte) (int, error)
	Close() error
}

// Reader is stdout/stderr from a process.
type Reader interface {
	Read([]byte) (int, error)
}

// NewSession builds a session from an isolated config copy.
func NewSession(cfg Config, sender Sender) *Session {
	c := cfg.Copy()
	if c.SoftwareVersion == "" {
		c.SoftwareVersion = DefaultSoftware
	}
	if c.Capabilities == 0 {
		c.Capabilities = CapLineMode | CapCoalesce
	}
	ctx, cancel := context.WithCancel(context.Background())
	if cs, ok := sender.(ChannelSender); ok {
		cs.Ctx = ctx
		sender = cs
	}
	s := &Session{
		cfg:    c,
		sender: sender,
		ctx:    ctx,
		cancel: cancel,
	}
	if c.Listener {
		if c.AllowAll {
			s.state = StateWaitVers
			s.authed = true
		} else {
			s.state = StateWaitIdent
		}
	} else {
		s.state = StateWaitVers
	}
	return s
}

// State returns the current FSM state.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// ConfigSnapshot returns a copy of the session config.
func (s *Session) ConfigSnapshot() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Copy()
}

// SetRemoteIdentity records link identify result. Listener only.
func (s *Session) SetRemoteIdentity(hash []byte) bool {
	s.mu.Lock()
	if !s.cfg.Listener {
		s.mu.Unlock()
		return true
	}
	if s.state != StateWaitIdent && s.state != StateWaitVers {
		ok := s.authed
		s.mu.Unlock()
		return ok
	}
	if s.cfg.AllowAll {
		s.authed = true
		s.remoteHash = append([]byte(nil), hash...)
		if s.state == StateWaitIdent {
			s.state = StateWaitVers
		}
		s.mu.Unlock()
		return true
	}
	if !allowedContains(s.cfg.Allowed, hash) {
		s.state = StateTeardown
		s.authed = false
		reason := "identity not allowed"
		_ = s.sendLocked(&AuthDeniedMessage{Reason: reason})
		if s.cfg.Compat {
			_ = s.sendLocked(&ErrorMessage{Compat: true, Msg: reason, Fatal: true})
		}
		cb := s.OnAuthDenied
		td := s.OnTeardown
		s.mu.Unlock()
		if cb != nil {
			cb(reason)
		}
		if td != nil {
			td()
		}
		return false
	}
	s.authed = true
	s.remoteHash = append([]byte(nil), hash...)
	if s.state == StateWaitIdent {
		s.state = StateWaitVers
	}
	if !s.cfg.Compat {
		_ = s.sendLocked(&AuthOKMessage{})
	}
	s.mu.Unlock()
	return true
}

// Close cancels pumps and kills the remote process. Safe to call more than once.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Lock()
		s.closed = true
		proc := s.proc
		if s.state != StateTeardown && s.state != StateError {
			s.state = StateTeardown
		}
		s.mu.Unlock()
		if proc != nil {
			_ = proc.Kill()
		}
	})
}

// HandleMessage processes one inbound protocol message.
func (s *Session) HandleMessage(msg Message) error {
	s.mu.Lock()
	if s.state == StateTeardown || s.state == StateError {
		// UDP can deliver Stream after Exit. Still accept client stream data.
		if _, ok := msg.(*StreamMessage); !ok || s.cfg.Listener {
			s.mu.Unlock()
			return nil
		}
	}
	var err error
	var after func()
	switch m := msg.(type) {
	case *NoopMessage:
		if s.cfg.Listener {
			err = s.sendLocked(&NoopMessage{Compat: s.cfg.Compat})
		}
	case *VersionMessage:
		err = s.handleVersionLocked(m)
	case *AuthOKMessage:
		if !s.cfg.Listener && !s.cfg.Compat {
			s.authed = true
		}
	case *AuthDeniedMessage:
		s.state = StateTeardown
		cb := s.OnAuthDenied
		td := s.OnTeardown
		reason := m.Reason
		after = func() {
			if cb != nil {
				cb(reason)
			}
			if td != nil {
				td()
			}
		}
	case *ExecMessage:
		after, err = s.handleExecLocked(m)
	case *StreamMessage:
		err = s.handleStreamLocked(m)
	case *WinSizeMessage:
		err = s.handleWinSizeLocked(m)
	case *ExitMessage:
		after, err = s.handleExitLocked(m)
	case *ErrorMessage:
		cb := s.OnError
		msg, fatal := m.Msg, m.Fatal
		if fatal {
			s.state = StateError
		}
		td := s.OnTeardown
		after = func() {
			if cb != nil {
				cb(msg, fatal)
				return
			}
			if fatal && td != nil {
				td()
			}
		}
	}
	s.mu.Unlock()
	if after != nil {
		after()
	}
	return err
}

func (s *Session) handleVersionLocked(m *VersionMessage) error {
	if s.cfg.Listener {
		if s.state == StateWaitCmd || s.state == StateRunning {
			// Idempotent: late or retried version after handshake.
			reply := &VersionMessage{
				Compat:          s.cfg.Compat,
				SoftwareVersion: s.cfg.SoftwareVersion,
				ProtocolVersion: ProtocolVersion,
				Capabilities:    s.cfg.Capabilities,
			}
			if s.cfg.Compat {
				reply.ProtocolVersion = CompatProtocolVersion
			}
			return s.sendLocked(reply)
		}
		if s.state != StateWaitVers {
			return s.denyProtocolLocked("version in wrong state")
		}
		if !s.authed && !s.cfg.AllowAll {
			return s.denyProtocolLocked("not authenticated")
		}
		reply := &VersionMessage{
			Compat:          s.cfg.Compat,
			SoftwareVersion: s.cfg.SoftwareVersion,
			ProtocolVersion: ProtocolVersion,
			Capabilities:    s.cfg.Capabilities,
		}
		if s.cfg.Compat {
			reply.ProtocolVersion = CompatProtocolVersion
		}
		if err := s.sendLocked(reply); err != nil {
			return err
		}
		s.state = StateWaitCmd
		return nil
	}
	if s.state == StateWaitVers {
		s.state = StateWaitCmd
	}
	_ = m
	return nil
}

func (s *Session) handleExecLocked(m *ExecMessage) (func(), error) {
	if !s.cfg.Listener {
		return nil, nil
	}
	if s.state != StateWaitCmd {
		return nil, s.denyProtocolLocked("exec before ready")
	}
	if !s.authed && !s.cfg.AllowAll {
		return nil, s.denyProtocolLocked("exec without auth")
	}
	cmdline := append([]string(nil), m.Cmdline...)
	if s.cfg.ForcedCommand {
		if len(cmdline) > 0 {
			return nil, s.denyProtocolLocked("Remote command line not allowed by listener")
		}
		cmdline = append([]string(nil), s.cfg.DefaultCmd...)
	} else if s.cfg.RemoteCmdAsArgs && len(cmdline) > 0 {
		cmdline = append(append([]string(nil), s.cfg.DefaultCmd...), cmdline...)
	} else if len(cmdline) == 0 {
		cmdline = append([]string(nil), s.cfg.DefaultCmd...)
	}
	if len(cmdline) == 0 {
		return nil, s.denyProtocolLocked("empty command")
	}
	req := ExecRequest{
		Cmdline:        cmdline,
		PipeStdin:      m.PipeStdin,
		PipeStdout:     m.PipeStdout,
		PipeStderr:     m.PipeStderr,
		Term:           m.Term,
		Rows:           m.Rows,
		Cols:           m.Cols,
		HPix:           m.HPix,
		VPix:           m.VPix,
		RemoteIdentity: append([]byte(nil), s.remoteHash...),
	}
	start := s.StartProcess
	if start == nil {
		return nil, s.denyProtocolLocked("no process starter")
	}
	s.spawned = true
	s.state = StateRunning
	return func() {
		proc, err := start(req)
		s.mu.Lock()
		if err != nil {
			s.spawned = false
			s.state = StateError
			_ = s.sendLocked(&ErrorMessage{Compat: s.cfg.Compat, Msg: err.Error(), Fatal: true})
			s.mu.Unlock()
			return
		}
		s.proc = proc
		if s.closed {
			s.mu.Unlock()
			_ = proc.Kill()
			return
		}
		pending := s.stdinPending
		s.stdinPending = nil
		eof := s.stdinEOF
		s.stdinEOF = false
		s.mu.Unlock()
		if len(pending) > 0 {
			_, _ = proc.Stdin().Write(pending)
		}
		if eof {
			_ = proc.Stdin().Close()
		}
		go s.pumpProcess(proc)
	}, nil
}

func (s *Session) handleStreamLocked(m *StreamMessage) error {
	if s.cfg.Listener {
		if s.state != StateRunning {
			return nil
		}
		if m.StreamID != StreamStdin {
			return nil
		}
		if s.proc == nil {
			if len(m.Data) > 0 {
				room := MaxStdinPending - len(s.stdinPending)
				if room > 0 {
					chunk := m.Data
					if len(chunk) > room {
						chunk = chunk[:room]
					}
					s.stdinPending = append(s.stdinPending, chunk...)
				}
			}
			if m.EOF {
				s.stdinEOF = true
			}
			return nil
		}
		if len(m.Data) > 0 {
			_, _ = s.proc.Stdin().Write(m.Data)
		}
		if m.EOF {
			_ = s.proc.Stdin().Close()
		}
		return nil
	}
	if len(m.Data) > 0 {
		switch m.StreamID {
		case StreamStdout:
			if s.OnStdout != nil {
				s.OnStdout(m.Data)
			}
		case StreamStderr:
			if s.OnStderr != nil {
				s.OnStderr(m.Data)
			}
		}
	}
	if m.EOF {
		switch m.StreamID {
		case StreamStdout:
			s.clientStdoutEOF = true
		case StreamStderr:
			s.clientStderrEOF = true
		}
		s.maybeFinishClientLocked()
	}
	return nil
}

func (s *Session) handleWinSizeLocked(m *WinSizeMessage) error {
	if s.state != StateRunning || s.proc == nil {
		return nil
	}
	return s.proc.SetWinSize(m.Rows, m.Cols, m.HPix, m.VPix)
}

func (s *Session) handleExitLocked(m *ExitMessage) (func(), error) {
	if s.cfg.Listener {
		return nil, nil
	}
	code := m.ReturnCode
	s.clientExitCode = &code
	s.state = StateTeardown
	delay := 100 * time.Millisecond
	if !s.clientStdoutEOF || !s.clientStderrEOF {
		delay = 750 * time.Millisecond
	}
	// Always defer OnExit briefly so reordered Stream packets can land.
	return func() {
		time.AfterFunc(delay, func() {
			s.mu.Lock()
			after := s.finishClientCallbacksLocked()
			s.mu.Unlock()
			if after != nil {
				after()
			}
		})
	}, nil
}

// maybeFinishClientLocked accelerates completion once Exit and both EOFs are in.
// Caller must hold s.mu.
func (s *Session) maybeFinishClientLocked() {
	if s.clientExitCode == nil || !s.clientStdoutEOF || !s.clientStderrEOF {
		return
	}
	after := s.finishClientCallbacksLocked()
	if after != nil {
		go after()
	}
}

// finishClientCallbacksLocked returns OnExit/OnTeardown once. Caller holds s.mu.
func (s *Session) finishClientCallbacksLocked() func() {
	if s.clientExitCode == nil {
		return nil
	}
	code := *s.clientExitCode
	cb := s.OnExit
	td := s.OnTeardown
	fired := false
	s.exitNotifyOnce.Do(func() { fired = true })
	if !fired {
		return nil
	}
	return func() {
		if cb != nil {
			cb(code)
		}
		if td != nil {
			td()
		}
	}
}

func (s *Session) pumpProcess(proc ProcessHandle) {
	s.mu.Lock()
	sender := s.sender
	compat := s.cfg.Compat
	ctx := s.ctx
	s.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}

	stdoutEOF := make(chan struct{})
	stderrEOF := make(chan struct{})
	go s.copyProcessStream(ctx, sender, compat, StreamStdout, proc.Stdout(), stdoutEOF)
	go s.copyProcessStream(ctx, sender, compat, StreamStderr, proc.Stderr(), stderrEOF)

	waitCh := make(chan int, 1)
	go func() {
		code, _ := proc.Wait()
		waitCh <- code
	}()

	code := 1
	select {
	case code = <-waitCh:
	case <-ctx.Done():
		_ = proc.Kill()
		select {
		case code = <-waitCh:
		case <-time.After(2 * time.Second):
		}
	}

	drain := time.NewTimer(DrainAfterExit)
	defer drain.Stop()
	stdoutDone, stderrDone := false, false
	for !stdoutDone || !stderrDone {
		select {
		case <-stdoutEOF:
			stdoutDone = true
			stdoutEOF = nil
		case <-stderrEOF:
			stderrDone = true
			stderrEOF = nil
		case <-drain.C:
			stdoutDone = true
			stderrDone = true
		}
	}

	if sender != nil {
		_ = sender.Send(&ExitMessage{Compat: compat, ReturnCode: code})
	}
	if d, ok := sender.(interface{ WaitTxIdle(time.Duration) bool }); ok {
		_ = d.WaitTxIdle(5 * time.Second)
	}
	s.mu.Lock()
	s.state = StateTeardown
	cb := s.OnExit
	s.mu.Unlock()
	if cb != nil {
		cb(code)
	}
}

func (s *Session) copyProcessStream(ctx context.Context, sender Sender, compat bool, streamID int, r Reader, done chan struct{}) {
	defer close(done)
	buf := make([]byte, 4096)
	for {
		if ctx.Err() != nil {
			_ = s.sendStreamChunks(ctx, sender, compat, streamID, nil, true)
			return
		}
		n, err := r.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			_ = s.sendStreamChunks(ctx, sender, compat, streamID, data, false)
			s.mu.Lock()
			onOut := s.OnStdout
			onErr := s.OnStderr
			s.mu.Unlock()
			if streamID == StreamStdout && onOut != nil {
				onOut(data)
			}
			if streamID == StreamStderr && onErr != nil {
				onErr(data)
			}
		}
		if err != nil {
			_ = s.sendStreamChunks(ctx, sender, compat, streamID, nil, true)
			return
		}
	}
}

func (s *Session) sendStreamChunks(ctx context.Context, sender Sender, compat bool, streamID int, data []byte, eof bool) error {
	if sender == nil {
		return nil
	}
	mdu := MaxStreamChunk
	if rs, ok := sender.(interface{ MDU() int }); ok {
		if n := rs.MDU(); n > 0 {
			mdu = n
		}
	}
	for {
		if rs, ok := sender.(interface{ WaitReady(context.Context) error }); ok {
			waitCtx := ctx
			if waitCtx == nil {
				waitCtx = context.Background()
			}
			if err := rs.WaitReady(waitCtx); err != nil {
				return err
			}
		}
		if len(data) == 0 {
			if eof {
				return sender.Send(&StreamMessage{Compat: compat, StreamID: streamID, EOF: true})
			}
			return nil
		}
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		chunk, n, comp := compressAdaptive(data, mdu)
		if n < 1 {
			n = min(len(data), mdu)
			chunk = data[:n]
			comp = false
		}
		data = data[n:]
		sendEOF := eof && len(data) == 0
		if err := sender.Send(&StreamMessage{
			Compat:     compat,
			StreamID:   streamID,
			Data:       chunk,
			Compressed: comp,
			EOF:        sendEOF,
		}); err != nil {
			return err
		}
		if sendEOF {
			return nil
		}
	}
}

// SendVersion initiates version handshake.
func (s *Session) SendVersion() error {
	s.outboundMu.Lock()
	defer s.outboundMu.Unlock()

	s.mu.Lock()
	// Initiators must not send Version after leaving WaitVers: a late Version
	// while the peer is in WAIT_CMD is a Python rnsh protocol error.
	if !s.cfg.Listener && s.state != StateWaitVers {
		s.mu.Unlock()
		return nil
	}
	msg := &VersionMessage{
		Compat:          s.cfg.Compat,
		SoftwareVersion: s.cfg.SoftwareVersion,
		ProtocolVersion: ProtocolVersion,
		Capabilities:    s.cfg.Capabilities,
	}
	if s.cfg.Compat {
		msg.ProtocolVersion = CompatProtocolVersion
	}
	err := s.sendLocked(msg)
	s.mu.Unlock()
	return err
}

// SendExec sends an execute request (initiator).
func (s *Session) SendExec(req ExecRequest) error {
	s.outboundMu.Lock()
	defer s.outboundMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateWaitCmd && s.state != StateWaitVers {
		return fmt.Errorf("rgosh: cannot exec in state %s", s.state)
	}
	msg := &ExecMessage{
		Compat:     s.cfg.Compat,
		Cmdline:    append([]string(nil), req.Cmdline...),
		PipeStdin:  req.PipeStdin,
		PipeStdout: req.PipeStdout,
		PipeStderr: req.PipeStderr,
		Term:       req.Term,
		Rows:       req.Rows,
		Cols:       req.Cols,
		HPix:       req.HPix,
		VPix:       req.VPix,
	}
	if err := s.sendLocked(msg); err != nil {
		return err
	}
	s.state = StateRunning
	return nil
}

// SendStream sends stream data.
func (s *Session) SendStream(streamID int, data []byte, eof bool) error {
	s.mu.Lock()
	sender := s.sender
	compat := s.cfg.Compat
	ctx := s.ctx
	s.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sendStreamChunks(ctx, sender, compat, streamID, data, eof)
}

// SendWinSize sends a window size update.
func (s *Session) SendWinSize(rows, cols, hpix, vpix int) error {
	return s.Send(&WinSizeMessage{
		Compat: s.cfg.Compat,
		Rows:   rows,
		Cols:   cols,
		HPix:   hpix,
		VPix:   vpix,
	})
}

// Send delivers a message through the sender without holding the session lock
// across channel IO (avoids deadlocks with pumpProcess).
func (s *Session) Send(msg Message) error {
	s.mu.Lock()
	sender := s.sender
	s.mu.Unlock()
	if sender == nil {
		return fmt.Errorf("rgosh: nil sender")
	}
	return sender.Send(msg)
}

// Authed reports whether the session passed identity checks.
func (s *Session) Authed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authed
}

// Spawned reports whether a process was started (listener).
func (s *Session) Spawned() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.spawned
}

// MutateDefaultCmdAppend mutates this session's copy only (isolation tests).
func (s *Session) MutateDefaultCmdAppend(arg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.DefaultCmd = append(s.cfg.DefaultCmd, arg)
}

// sendLocked sends while temporarily releasing s.mu so channel IO cannot
// deadlock against pumpProcess. Caller must hold s.mu. On return s.mu is held.
func (s *Session) sendLocked(msg Message) error {
	if s.sender == nil {
		return fmt.Errorf("rgosh: nil sender")
	}
	sender := s.sender
	s.mu.Unlock()
	err := sender.Send(msg)
	s.mu.Lock()
	return err
}

func (s *Session) denyProtocolLocked(reason string) error {
	s.state = StateTeardown
	s.authed = false
	if !s.cfg.Compat {
		_ = s.sendLocked(&AuthDeniedMessage{Reason: reason})
	}
	_ = s.sendLocked(&ErrorMessage{Compat: s.cfg.Compat, Msg: reason, Fatal: true})
	return fmt.Errorf("rgosh: %s", reason)
}

func allowedContains(list [][]byte, hash []byte) bool {
	for _, h := range list {
		if bytes.Equal(h, hash) {
			return true
		}
	}
	return false
}

// AllowedHex formats identity hashes for diagnostics.
func AllowedHex(list [][]byte) []string {
	out := make([]string, 0, len(list))
	for _, h := range list {
		out = append(out, hex.EncodeToString(h))
	}
	return out
}
