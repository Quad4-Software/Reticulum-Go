// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"net"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	defaultBlockFastFlapping    = true
	defaultFastFlapThresholdSec = 20.0
	defaultFastFlapGrace        = 5
	defaultFastFlapExpirySec    = 12 * 60 * 60
)

// fastFlapEntry tracks short-lived reconnects from one remote IP.
// Fields: started, lastFlap, flaps.
type fastFlapEntry struct {
	started  time.Time
	lastFlap time.Time
	flaps    int
}

// BackboneInterface is a high-throughput TCP server using the process-wide I/O hub.
// Each accepted client becomes a spawned BackboneClientInterface, matching reference
// Reticulum's epoll backbone model.
type BackboneInterface struct {
	BaseInterface
	listener   net.Listener
	bindAddr   string
	bindPort   int
	preferIPv6 bool
	hub        *backbone.Hub
	spawned    []*BackboneClientInterface
	spawnMu    sync.Mutex
	spawnHook  func(*BackboneClientInterface)
	callback   common.PacketCallback
	done       chan struct{}
	stopOnce   sync.Once

	blockFastFlapping bool
	fastFlapThreshold time.Duration
	fastFlapGrace     int
	fastFlapExpiry    time.Duration
	fastFlapMu        sync.Mutex
	fastFlapping      map[string]*fastFlapEntry
}

// NewBackboneInterface binds a local TCP listener using cfg.Address/cfg.Port or cfg.Interface.
func NewBackboneInterface(name string, cfg *common.InterfaceConfig, hub *backbone.Hub, spawn func(*BackboneClientInterface)) (*BackboneInterface, error) {
	if hub == nil {
		hub = backbone.Get()
	}
	if hub == nil {
		return nil, fmt.Errorf("backbone I/O hub not initialised")
	}

	bindAddr := cfg.Address
	bindPort := cfg.Port

	if cfg.Interface != "" {
		iface, err := net.InterfaceByName(cfg.Interface)
		if err != nil {
			return nil, fmt.Errorf("find interface %q: %w", cfg.Interface, err)
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("list addresses for %q: %w", cfg.Interface, err)
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP
			if cfg.PreferIPv6 {
				if ip.To4() == nil {
					bindAddr = ip.String()
					break
				}
			} else {
				if ip.To4() != nil {
					bindAddr = ip.String()
					break
				}
			}
		}
		if bindAddr == "" && len(addrs) > 0 {
			if ipnet, ok := addrs[0].(*net.IPNet); ok {
				bindAddr = ipnet.IP.String()
			}
		}
	}

	if bindPort <= 0 {
		return nil, fmt.Errorf("no port for BackboneInterface %q", name)
	}

	blockFlap := defaultBlockFastFlapping
	if cfg != nil && cfg.BlockFastFlappingSet {
		blockFlap = cfg.BlockFastFlapping
	}
	thresholdSec := defaultFastFlapThresholdSec
	if cfg != nil && cfg.FastFlappingThreshold > 0 {
		thresholdSec = cfg.FastFlappingThreshold
	}
	grace := defaultFastFlapGrace
	if cfg != nil && cfg.FastFlappingGrace > 0 {
		grace = cfg.FastFlappingGrace
	}
	expirySec := float64(defaultFastFlapExpirySec)
	if cfg != nil && cfg.FastFlappingBlockTimeMin > 0 {
		expirySec = cfg.FastFlappingBlockTimeMin * 60
	}

	bi := &BackboneInterface{
		BaseInterface:     NewBaseInterface(name, common.IFTypeBackbone, cfg.Enabled),
		bindAddr:          bindAddr,
		bindPort:          bindPort,
		preferIPv6:        cfg.PreferIPv6,
		hub:               hub,
		spawnHook:         spawn,
		done:              make(chan struct{}),
		blockFastFlapping: blockFlap,
		fastFlapThreshold: time.Duration(thresholdSec * float64(time.Second)),
		fastFlapGrace:     grace,
		fastFlapExpiry:    time.Duration(expirySec * float64(time.Second)),
		fastFlapping:      make(map[string]*fastFlapEntry),
	}
	bi.MTU = backboneHWMTU
	bi.Bitrate = backboneServerBitrateGuess
	bi.In = true
	bi.Out = false
	return bi, nil
}

func (bi *BackboneInterface) String() string {
	addr := bi.bindAddr
	if addr == "" {
		if bi.preferIPv6 {
			addr = "[::0]"
		} else {
			addr = "0.0.0.0"
		}
	}
	return fmt.Sprintf("BackboneInterface[%s/%s:%d]", bi.Name, addr, bi.bindPort)
}

func (bi *BackboneInterface) Start() error {
	bi.Mutex.Lock()
	if bi.listener != nil {
		bi.Mutex.Unlock()
		return nil
	}
	select {
	case <-bi.done:
		bi.done = make(chan struct{})
		bi.stopOnce = sync.Once{}
	default:
	}
	bi.Mutex.Unlock()

	addr := net.JoinHostPort(bi.bindAddr, fmt.Sprintf("%d", bi.bindPort))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("backbone listen: %w", common.WrapListenError(err))
	}

	bi.Mutex.Lock()
	bi.listener = ln
	bi.Online = true
	bi.Mutex.Unlock()

	if err := bi.hub.RegisterListener(ln, bi.acceptConn); err != nil {
		_ = ln.Close()
		return err
	}
	return nil
}

func (bi *BackboneInterface) Stop() error {
	bi.Mutex.Lock()
	bi.Online = false
	if bi.listener != nil {
		_ = bi.listener.Close()
		bi.listener = nil
	}
	bi.Mutex.Unlock()

	bi.spawnMu.Lock()
	spawned := append([]*BackboneClientInterface(nil), bi.spawned...)
	bi.spawned = nil
	bi.spawnMu.Unlock()

	for _, c := range spawned {
		_ = c.Stop()
	}

	bi.stopOnce.Do(func() {
		close(bi.done)
	})
	return nil
}

func (bi *BackboneInterface) acceptConn(conn net.Conn) {
	remoteIP := peerIP(conn)
	if bi.isFastFlappingBlocked(remoteIP) {
		debug.Log(debug.DebugVerbose, "Ignoring incoming connection from fast-flapping IP", "ip", remoteIP)
		_ = conn.Close()
		return
	}

	client := newSpawnedBackboneClient(bi, conn)
	bi.spawnMu.Lock()
	bi.spawned = append(bi.spawned, client)
	cb := bi.callback
	hook := bi.spawnHook
	bi.spawnMu.Unlock()

	if cb != nil {
		client.SetPacketCallback(cb)
	}
	if hook != nil {
		hook(client)
	}
	if err := client.attachStream(); err != nil {
		debug.Log(debug.DebugError, "backbone spawn attach failed", "error", err)
		_ = client.Stop()
	}
}

func peerIP(conn net.Conn) string {
	if conn == nil || conn.RemoteAddr() == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return conn.RemoteAddr().String()
	}
	return host
}

func (bi *BackboneInterface) isFastFlappingBlocked(remoteIP string) bool {
	if !bi.blockFastFlapping || remoteIP == "" {
		return false
	}
	now := time.Now()
	bi.fastFlapMu.Lock()
	defer bi.fastFlapMu.Unlock()
	bi.expireFastFlapsLocked(now)
	ffe, ok := bi.fastFlapping[remoteIP]
	if !ok {
		return false
	}
	return ffe.flaps > bi.fastFlapGrace
}

// BlockedIPCount returns how many remote IPs are currently blocked for flapping.
func (bi *BackboneInterface) BlockedIPCount() int {
	if !bi.blockFastFlapping {
		return 0
	}
	now := time.Now()
	bi.fastFlapMu.Lock()
	defer bi.fastFlapMu.Unlock()
	bi.expireFastFlapsLocked(now)
	count := 0
	for _, ffe := range bi.fastFlapping {
		if ffe.flaps > bi.fastFlapGrace {
			count++
		}
	}
	return count
}

func (bi *BackboneInterface) expireFastFlapsLocked(now time.Time) {
	for ip, ffe := range bi.fastFlapping {
		if now.Sub(ffe.lastFlap) > bi.fastFlapExpiry {
			delete(bi.fastFlapping, ip)
			debug.Log(debug.DebugVerbose, "Fast-flapping block expired", "ip", ip)
		}
	}
}

func (bi *BackboneInterface) recordFastFlap(remoteIP string, connectedFor time.Duration) {
	if !bi.blockFastFlapping || remoteIP == "" {
		return
	}
	if connectedFor >= bi.fastFlapThreshold {
		return
	}
	now := time.Now()
	bi.fastFlapMu.Lock()
	defer bi.fastFlapMu.Unlock()
	ffe := bi.fastFlapping[remoteIP]
	if ffe == nil {
		ffe = &fastFlapEntry{started: now}
		bi.fastFlapping[remoteIP] = ffe
	}
	ffe.lastFlap = now
	ffe.flaps++
	if ffe.flaps > bi.fastFlapGrace {
		debug.Log(debug.DebugError, "Ignoring further connections due to fast-flapping",
			"ip", remoteIP, "flaps", ffe.flaps, "connected_for", connectedFor)
	} else {
		debug.Log(debug.DebugVerbose, "Backbone client fast flapping",
			"ip", remoteIP, "flaps", ffe.flaps, "connected_for", connectedFor)
	}
}

func (bi *BackboneInterface) removeSpawned(client *BackboneClientInterface) {
	bi.spawnMu.Lock()
	defer bi.spawnMu.Unlock()
	for i, c := range bi.spawned {
		if c == client {
			bi.spawned = append(bi.spawned[:i], bi.spawned[i+1:]...)
			return
		}
	}
}

func (bi *BackboneInterface) SetPacketCallback(cb common.PacketCallback) {
	bi.Mutex.Lock()
	bi.callback = cb
	bi.packetCallback = cb
	bi.Mutex.Unlock()
	bi.spawnMu.Lock()
	for _, c := range bi.spawned {
		c.SetPacketCallback(cb)
	}
	bi.spawnMu.Unlock()
}

func (bi *BackboneInterface) ProcessOutgoing([]byte) error {
	return nil
}

func (bi *BackboneInterface) Send(data []byte, address string) error {
	if err := common.RejectReceiveOnly(bi); err != nil {
		return err
	}
	bi.spawnMu.Lock()
	spawned := append([]*BackboneClientInterface(nil), bi.spawned...)
	bi.spawnMu.Unlock()
	for _, c := range spawned {
		if err := c.Send(data, address); err != nil {
			debug.Log(debug.DebugVerbose, "backbone fan-out send error", "client", c.Name, "error", err)
		}
	}
	return nil
}

func (bi *BackboneInterface) GetConn() net.Conn { return nil }

func (bi *BackboneInterface) Clients() int {
	bi.spawnMu.Lock()
	defer bi.spawnMu.Unlock()
	return len(bi.spawned)
}
