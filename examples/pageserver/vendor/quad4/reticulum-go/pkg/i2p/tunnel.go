// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package i2p

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"quad4/reticulum-go/pkg/debug"
)

const defaultSetupTimeout = 120 * time.Second

// TunnelStatus tracks setup outcome for observability.
type TunnelStatus struct {
	SetupRan    bool
	SetupFailed bool
	Err         error
}

// ClientTunnel proxies a local TCP port to a remote I2P destination via SAM.
type ClientTunnel struct {
	client     *Client
	session    *Session
	localAddr  string
	remoteDest string
	status     atomic.Value // TunnelStatus
	listener   net.Listener
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewClientTunnel(client *Client, remoteDest string, localPort int) (*ClientTunnel, error) {
	if client == nil {
		client = NewClient("")
	}
	if localPort == 0 {
		p, err := FreePort()
		if err != nil {
			return nil, err
		}
		localPort = p
	}
	return &ClientTunnel{
		client:     client,
		localAddr:  net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort)),
		remoteDest: remoteDest,
	}, nil
}

func (t *ClientTunnel) LocalAddr() string { return t.localAddr }

func (t *ClientTunnel) Status() TunnelStatus {
	if v := t.status.Load(); v != nil {
		return v.(TunnelStatus)
	}
	return TunnelStatus{}
}

func (t *ClientTunnel) Run(ctx context.Context) error {
	ctx, t.cancel = context.WithCancel(ctx)
	setupCtx, cancelSetup := context.WithTimeout(ctx, defaultSetupTimeout)
	defer cancelSetup()

	destB64, err := ResolveDestination(t.remoteDest, func(name string) (string, error) {
		return t.client.NamingLookup(setupCtx, name)
	})
	if err != nil {
		t.setStatus(true, true, err)
		return err
	}
	sess, err := t.client.OpenSession(setupCtx, GenerateSessionID(), transientDest)
	if err != nil {
		t.setStatus(true, true, err)
		return err
	}
	t.session = sess
	ln, err := net.Listen("tcp", t.localAddr)
	if err != nil {
		t.setStatus(true, true, err)
		return err
	}
	t.listener = ln
	t.setStatus(true, false, nil)
	debug.Log(debug.DebugInfo, "I2P client tunnel listening", "addr", t.localAddr, "dest", t.remoteDest)

	t.wg.Add(1)
	go t.acceptLoop(ctx, destB64)
	return nil
}

func (t *ClientTunnel) acceptLoop(ctx context.Context, destB64 string) {
	defer t.wg.Done()
	for {
		if ctx.Err() != nil {
			return
		}
		if t.listener == nil {
			return
		}
		conn, err := t.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		go t.handleClient(ctx, conn, destB64)
	}
}

func (t *ClientTunnel) handleClient(ctx context.Context, local net.Conn, destB64 string) {
	defer local.Close()
	streamCtx, cancel := context.WithTimeout(ctx, defaultSetupTimeout)
	defer cancel()
	remote, err := t.client.StreamConnect(streamCtx, t.session.ID, destB64)
	if err != nil {
		debug.Log(debug.DebugError, "I2P stream connect failed", "error", err)
		return
	}
	defer remote.Close()
	go ioCopyAndClose(remote, local)
	ioCopyAndClose(local, remote)
}

func (t *ClientTunnel) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
	if t.listener != nil {
		_ = t.listener.Close()
	}
	if t.session != nil {
		_ = t.session.Close()
		t.session = nil
	}
	t.wg.Wait()
}

func (t *ClientTunnel) setStatus(ran, failed bool, err error) {
	t.status.Store(TunnelStatus{SetupRan: ran, SetupFailed: failed, Err: err})
}

// ServerTunnel accepts inbound I2P connections and forwards to a local service.
type ServerTunnel struct {
	client      *Client
	session     *Session
	localAddr   string
	destination *Destination
	status      atomic.Value
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewServerTunnel(client *Client, destination *Destination, localPort int) (*ServerTunnel, error) {
	if client == nil {
		client = NewClient("")
	}
	if localPort == 0 {
		p, err := FreePort()
		if err != nil {
			return nil, err
		}
		localPort = p
	}
	return &ServerTunnel{
		client:      client,
		localAddr:   net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort)),
		destination: destination,
	}, nil
}

func (t *ServerTunnel) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
	if t.session != nil {
		_ = t.session.Close()
		t.session = nil
	}
	t.wg.Wait()
}

func (t *ServerTunnel) setStatus(ran, failed bool, err error) {
	t.status.Store(TunnelStatus{SetupRan: ran, SetupFailed: failed, Err: err})
}

func ioCopyAndClose(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	if c, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = c.CloseWrite()
	}
}

func (t *ServerTunnel) LocalAddr() string { return t.localAddr }

func (t *ServerTunnel) Destination() *Destination { return t.destination }

func (t *ServerTunnel) Status() TunnelStatus {
	if v := t.status.Load(); v != nil {
		return v.(TunnelStatus)
	}
	return TunnelStatus{}
}

func (t *ServerTunnel) Run(ctx context.Context) error {
	ctx, t.cancel = context.WithCancel(ctx)
	setupCtx, cancelSetup := context.WithTimeout(ctx, defaultSetupTimeout)
	defer cancelSetup()

	dest := transientDest
	if t.destination != nil && t.destination.PrivateKeyB64() != "" {
		dest = t.destination.PrivateKeyB64()
	}
	sess, err := t.client.OpenSession(setupCtx, GenerateSessionID(), dest)
	if err != nil {
		t.setStatus(true, true, err)
		return err
	}
	t.session = sess
	if t.destination == nil {
		t.setStatus(true, true, ErrTunnelSetup)
		return ErrTunnelSetup
	}
	t.setStatus(true, false, nil)
	debug.Log(debug.DebugInfo, "I2P server tunnel ready", "b32", t.destination.Base32())

	t.wg.Add(1)
	go t.acceptLoop(ctx)
	return nil
}

func (t *ServerTunnel) acceptLoop(ctx context.Context) {
	defer t.wg.Done()
	for {
		if ctx.Err() != nil {
			return
		}
		acceptCtx, cancel := context.WithTimeout(ctx, defaultSetupTimeout)
		incoming, err := t.client.StreamAccept(acceptCtx, t.session.ID)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			debug.Log(debug.DebugError, "I2P stream accept failed", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		go t.handleIncoming(ctx, incoming)
	}
}

func (t *ServerTunnel) handleIncoming(ctx context.Context, incoming net.Conn) {
	defer incoming.Close()
	br := make([]byte, 4096)
	n, err := incoming.Read(br)
	if err != nil {
		return
	}
	buf := br[:n]
	lineEnd := -1
	for i, b := range buf {
		if b == '\n' {
			lineEnd = i
			break
		}
	}
	if lineEnd < 0 {
		return
	}
	destLine := string(buf[:lineEnd])
	rest := buf[lineEnd+1:]
	if _, err := NewDestinationFromB64(destLine); err != nil {
		debug.Log(debug.DebugTrace, "I2P inbound peer destination", "error", err)
	}

	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	local, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", t.localAddr)
	if err != nil {
		return
	}
	defer local.Close()
	if len(rest) > 0 {
		_, _ = local.Write(rest)
	}
	go ioCopyAndClose(incoming, local)
	ioCopyAndClose(local, incoming)
}
