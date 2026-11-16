// SPDX-License-Identifier: LicenseRef-Reticulum
package session

import (
	"fmt"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv/proto"
)

// Endpoint listens for and dials RNV sessions on rnv.media.
type Endpoint struct {
	mu   sync.Mutex
	tr   *transport.Transport
	id   *identity.Identity
	dest *destination.Destination
	cfg  Config
}

// Bind creates an inbound rnv.media destination. Does not announce.
func Bind(tr *transport.Transport, id *identity.Identity, cfg Config) (*Endpoint, error) {
	if tr == nil || id == nil {
		return nil, rnv.ErrNilArgument
	}
	cfg = cfg.withDefaults()
	dest, err := destination.New(id, destination.In, destination.Single, proto.AppName, tr, proto.MediaAspect)
	if err != nil {
		return nil, err
	}
	dest.AcceptsLinks(true)
	ep := &Endpoint{tr: tr, id: id, dest: dest, cfg: cfg}
	dest.SetLinkEstablishedCallback(func(v any) {
		lnk, ok := v.(*link.Link)
		if !ok || lnk == nil {
			return
		}
		conn, err := acceptConn(ep, lnk)
		if err != nil {
			lnk.Teardown()
			return
		}
		if ep.cfg.Handlers.OnConn != nil {
			ep.cfg.Handlers.OnConn(conn)
		}
	})
	return ep, nil
}

// Destination returns the inbound destination.
func (ep *Endpoint) Destination() *destination.Destination {
	if ep == nil {
		return nil
	}
	return ep.dest
}

// Hash returns the local destination hash.
func (ep *Endpoint) Hash() []byte {
	if ep == nil || ep.dest == nil {
		return nil
	}
	return cloneHash(ep.dest.GetHash())
}

// Announce publishes the destination. Never called automatically by Bind.
func (ep *Endpoint) Announce() error {
	if ep == nil || ep.dest == nil {
		return rnv.ErrNilArgument
	}
	app, err := proto.EncodeAnnounceAppData(proto.AnnounceAppData{
		Version: proto.ProtocolVersion,
		Caps:    proto.CapsBitmap(ep.cfg.Caps),
		Profile: ep.cfg.Caps.Preferred,
	})
	if err != nil {
		return err
	}
	return ep.dest.Announce(false, app, nil)
}

// Dial establishes a link to peerHash, exchanges HELLO, and returns a Conn.
func (ep *Endpoint) Dial(peerHash []byte) (*Conn, error) {
	if ep == nil {
		return nil, rnv.ErrNilArgument
	}
	if len(peerHash) != proto.IdentityHashLen {
		return nil, fmt.Errorf("%w: peer hash", rnv.ErrBadFieldLength)
	}
	peerHash = cloneHash(peerHash)

	if !ep.tr.HasPath(peerHash) {
		_ = ep.tr.RequestPath(peerHash, "", nil, true)
	}
	if !waitUntil(ep.cfg.DialTimeout, 50*time.Millisecond, func() bool { return ep.tr.HasPath(peerHash) }) {
		return nil, fmt.Errorf("%w: no path", rnv.ErrDialTimeout)
	}
	remote, err := identity.Recall(peerHash)
	if err != nil || remote == nil {
		return nil, fmt.Errorf("%w: recall", rnv.ErrDialTimeout)
	}
	destOut, err := destination.FromHash(peerHash, remote, destination.Single, ep.tr)
	if err != nil {
		return nil, err
	}

	closedCh := make(chan struct{})
	var closeOnce sync.Once
	lnk := link.NewLink(destOut, ep.tr, nil, nil, func(*link.Link) {
		closeOnce.Do(func() { close(closedCh) })
	})
	if err := lnk.Establish(); err != nil {
		return nil, err
	}
	lnk.Start()
	if !waitUntil(ep.cfg.DialTimeout, 25*time.Millisecond, lnk.IsActive) {
		lnk.Teardown()
		return nil, fmt.Errorf("%w: link", rnv.ErrDialTimeout)
	}
	if err := lnk.Identify(ep.id); err != nil {
		lnk.Teardown()
		return nil, err
	}

	conn := newConn(ep, lnk, peerHash, true)
	conn.attachLink()
	// Brief settle so the peer can attach inbound callbacks before HELLO.
	time.Sleep(20 * time.Millisecond)
	if err := conn.sendHello(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.waitHello(ep.cfg.HelloTimeout); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func acceptConn(ep *Endpoint, lnk *link.Link) (*Conn, error) {
	var peer []byte
	if rid := lnk.GetRemoteIdentity(); rid != nil {
		peer = cloneHash(rid.Hash())
	}
	conn := newConn(ep, lnk, peer, false)
	conn.attachLink()
	if err := conn.sendHello(); err != nil {
		return nil, err
	}
	go func() {
		_ = conn.waitHello(ep.cfg.HelloTimeout)
	}()
	return conn, nil
}

func waitUntil(timeout, step time.Duration, ready func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ready() {
			return true
		}
		time.Sleep(step)
	}
	return ready()
}
