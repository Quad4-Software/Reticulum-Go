// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/blackhole"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/pathfinder"
	"quad4/reticulum-go/pkg/rate"
)

var (
	transportInstance *Transport
	transportMutex    sync.Mutex
)

type PathInfo struct {
	NextHop     []byte
	Interface   string
	Hops        uint8
	LastUpdated time.Time
}

type hash16 struct {
	bytes [packet.TruncatedHashLength]byte
	n     int
}

type destinationPacketReceiver interface {
	Receive(pkt *packet.Packet, iface common.NetworkInterface)
}

type destinationLinkRequestHandler interface {
	HandleIncomingLinkRequest(pkt any, transport any, networkIface common.NetworkInterface) error
}

type registeredDestination struct {
	raw                any
	packetReceiver     destinationPacketReceiver
	linkRequestHandler destinationLinkRequestHandler
}

func hash16FromSlice(b []byte) hash16 {
	var k hash16
	if len(b) > len(k.bytes) {
		b = b[:len(k.bytes)]
	}
	copy(k.bytes[:], b)
	k.n = len(b)
	return k
}

type pendingDiscoveryPR struct {
	destHash []byte
	exclude  common.NetworkInterface
}

type Transport struct {
	mutex                 sync.RWMutex
	config                *common.ReticulumConfig
	interfaces            map[string]common.NetworkInterface
	links                 map[hash16]LinkInterface
	destinations          map[hash16]registeredDestination
	announceRate          *rate.Limiter
	seenAnnounces         map[string]time.Time
	packetHandleSem       chan struct{}
	pathfinder            *pathfinder.PathFinder
	announceHandlers      []announce.Handler
	paths                 map[[PathMapKeySize]byte]*common.Path
	receipts              []*packet.PacketReceipt
	receiptsMutex         sync.RWMutex
	pathRequests          map[string]time.Time
	pathStates            map[[PathMapKeySize]byte]byte
	discoveryPathRequests map[string]*DiscoveryPathRequest
	discoveryPRTags       map[string]bool
	announceTable         map[string]*PathAnnounceEntry
	heldAnnounces         map[string]*PathAnnounceEntry
	transportIdentity     *identity.Identity
	pathRequestDest       any
	blackholeTable        *blackhole.Table
	linkTable             *linkRelayTable
	lastPathRequest       map[[PathMapKeySize]byte]time.Time
	ifaceStates           *ifaceStateTable
	pendingDiscoveryPRs   []pendingDiscoveryPR
	pendingDiscoveryPRMu  sync.Mutex
	discoveryDraining     atomic.Bool
	done                  chan struct{}
	stopOnce              sync.Once
}

// SetBlackholeTable sets the blackhole table. HandleAnnounce drops blackholed
// identities; path lookups consult the same table.
func (t *Transport) SetBlackholeTable(tab *blackhole.Table) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.blackholeTable = tab
}

// BlackholeTable returns the active table or nil. The table is internally
// synchronized; the returned pointer is safe to use.
func (t *Transport) BlackholeTable() *blackhole.Table {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.blackholeTable
}

type DiscoveryPathRequest struct {
	DestinationHash []byte
	Timeout         time.Time
	RequestingIface common.NetworkInterface
}

type PathAnnounceEntry struct {
	CreatedAt         time.Time
	RetransmitTimeout time.Time
	Retries           int
	ReceivedFrom      common.NetworkInterface
	AnnounceHops      byte
	Packet            *packet.Packet
	LocalRebroadcasts int
	BlockRebroadcasts bool
	AttachedInterface common.NetworkInterface
}

type Path struct {
	NextHop   []byte
	Interface common.NetworkInterface
	HopCount  byte
}

func NewTransport(cfg *common.ReticulumConfig) *Transport {
	t := &Transport{
		interfaces:            make(map[string]common.NetworkInterface),
		paths:                 make(map[[PathMapKeySize]byte]*common.Path),
		seenAnnounces:         make(map[string]time.Time),
		packetHandleSem:       make(chan struct{}, MaxConcurrentPacketHandlers),
		announceRate:          rate.NewLimiter(rate.DefaultBurstFreq, AnnounceRateKbps),
		mutex:                 sync.RWMutex{},
		config:                cfg,
		links:                 make(map[hash16]LinkInterface),
		destinations:          make(map[hash16]registeredDestination),
		pathfinder:            pathfinder.NewPathFinder(),
		receipts:              make([]*packet.PacketReceipt, 0),
		receiptsMutex:         sync.RWMutex{},
		pathRequests:          make(map[string]time.Time),
		pathStates:            make(map[[PathMapKeySize]byte]byte),
		discoveryPathRequests: make(map[string]*DiscoveryPathRequest),
		discoveryPRTags:       make(map[string]bool),
		announceTable:         make(map[string]*PathAnnounceEntry),
		heldAnnounces:         make(map[string]*PathAnnounceEntry),
		linkTable:             newLinkRelayTable(),
		lastPathRequest:       make(map[[PathMapKeySize]byte]time.Time),
		ifaceStates:           newIfaceStateTable(),
		pendingDiscoveryPRs:   make([]pendingDiscoveryPR, 0, maxQueuedDiscoveryPRs),
		done:                  make(chan struct{}),
	}

	transportIdent, err := identity.LoadOrCreateTransportIdentity(cfg.ConfigPath)
	if err == nil {
		t.transportIdentity = transportIdent
	}

	go t.startMaintenanceJobs()

	return t
}

func (t *Transport) startMaintenanceJobs() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.cleanupExpiredPaths()
			t.cleanupExpiredDiscoveryRequests()
			t.cleanupExpiredAnnounces()
			t.cleanupExpiredReceipts()
			t.cleanupSeenAnnounces()
			if tab := t.BlackholeTable(); tab != nil {
				tab.SweepExpired()
			}
			if t.linkTable != nil {
				t.linkTable.sweep(time.Duration(StaleTime) * time.Second)
			}
			t.cleanupExpiredPathRequestThrottle()
			t.releaseHeldAnnounces()
		case <-t.done:
			return
		}
	}
}

func (t *Transport) cleanupSeenAnnounces() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	cutoff := time.Now().Add(-SeenAnnounceTTL)
	for k, v := range t.seenAnnounces {
		if v.Before(cutoff) {
			delete(t.seenAnnounces, k)
		}
	}
}

func (t *Transport) cleanupExpiredPaths() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	now := time.Now()
	pathExpiry := 7 * 24 * time.Hour

	for destHash, path := range t.paths {
		if now.Sub(path.LastUpdated) > pathExpiry {
			delete(t.paths, destHash)
			delete(t.pathStates, destHash)
			debug.Log(debug.DebugVerbose, "Expired path", "dest_hash", fmt.Sprintf("%x", destHash[:8]))
		}
	}
}

func (t *Transport) cleanupExpiredDiscoveryRequests() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	now := time.Now()
	for destHash, req := range t.discoveryPathRequests {
		if now.After(req.Timeout) {
			delete(t.discoveryPathRequests, destHash)
			debug.Log(debug.DebugVerbose, "Expired discovery path request", "dest_hash", fmt.Sprintf("%x", destHash[:8]))
		}
	}
}

func (t *Transport) cleanupExpiredAnnounces() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	announceExpiry := 24 * time.Hour

	for destHash, entry := range t.announceTable {
		if entry != nil && time.Since(entry.CreatedAt) > announceExpiry {
			delete(t.announceTable, destHash)
			debug.Log(debug.DebugVerbose, "Expired announce entry", "dest_hash", fmt.Sprintf("%x", destHash[:8]))
		}
	}

	for destHash, entry := range t.heldAnnounces {
		if entry != nil && time.Since(entry.CreatedAt) > announceExpiry {
			delete(t.heldAnnounces, destHash)
		}
	}
}

// releaseHeldAnnounces replays announces held by per-interface ingress control
// through handleAnnouncePacket after burst-clear timing allows.
func (t *Transport) releaseHeldAnnounces() {
	if t.ifaceStates == nil {
		return
	}
	for _, entry := range t.ifaceStates.snapshot() {
		st := entry.state
		if st == nil || st.ingress == nil {
			continue
		}
		t.mutex.RLock()
		iface, ok := t.interfaces[entry.name]
		t.mutex.RUnlock()
		if !ok || iface == nil {
			continue
		}
		for {
			_, data, ok := st.ingress.ReleaseHeldAnnounce()
			if !ok {
				break
			}
			if err := t.handleAnnouncePacket(data, iface); err != nil {
				debug.Log(debug.DebugVerbose,
					"Released held announce failed reprocessing",
					"iface", entry.name, "error", err)
			}
		}
	}
}

// cleanupExpiredPathRequestThrottle drops last-path-request entries older than
// the throttle window so the map cannot grow without bound.
func (t *Transport) cleanupExpiredPathRequestThrottle() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	cutoff := time.Now().Add(-2 * PathRequestMI)
	for k, ts := range t.lastPathRequest {
		if ts.Before(cutoff) {
			delete(t.lastPathRequest, k)
		}
	}
}

func (t *Transport) cleanupExpiredReceipts() {
	t.receiptsMutex.Lock()
	defer t.receiptsMutex.Unlock()

	oldLen := len(t.receipts)
	write := 0
	for read := range oldLen {
		receipt := t.receipts[read]
		if receipt != nil && !receipt.IsTimedOut() {
			status := receipt.GetStatus()
			if status == packet.ReceiptSent || status == packet.ReceiptDelivered {
				t.receipts[write] = receipt
				write++
			}
		}
	}
	if write < oldLen {
		for i := write; i < oldLen; i++ {
			t.receipts[i] = nil
		}
		t.receipts = t.receipts[:write]
		debug.Log(debug.DebugVerbose, "Cleaned up expired receipts", "remaining", write)
	}
}

func (t *Transport) MarkPathUnresponsive(destHash []byte) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.pathStates[pathMapKey(destHash)] = StateUnresponsive
}

func (t *Transport) MarkPathResponsive(destHash []byte) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.pathStates[pathMapKey(destHash)] = StateResponsive
}

func (t *Transport) PathIsUnresponsive(destHash []byte) bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	state, exists := t.pathStates[pathMapKey(destHash)]
	return exists && state == StateUnresponsive
}

// RegisterDestination registers a destination to receive incoming link requests.
func (t *Transport) RegisterDestination(hash []byte, dest any) {
	key := hash16FromSlice(hash)
	registered := registeredDestination{raw: dest}
	if recv, ok := dest.(destinationPacketReceiver); ok {
		registered.packetReceiver = recv
	}
	if handler, ok := dest.(destinationLinkRequestHandler); ok {
		registered.linkRequestHandler = handler
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.destinations[key] = registered
	debug.Log(debug.DebugTrace, "Registered destination with transport", "hash", fmt.Sprintf("%x", hash))
}

// CreateIncomingLink builds a link for an incoming request without importing
// the link package (stub; returns nil until implemented).
func (t *Transport) CreateIncomingLink(dest any, networkIface common.NetworkInterface) any {
	debug.Log(debug.DebugTrace, "CreateIncomingLink called (not yet fully implemented)")
	return nil
}

func GetTransportInstance() *Transport {
	transportMutex.Lock()
	defer transportMutex.Unlock()
	return transportInstance
}

func SetTransportInstance(t *Transport) {
	transportMutex.Lock()
	defer transportMutex.Unlock()
	transportInstance = t
}

// abstractBaseInterfaceTypes names pointer types that must not be registered
// alone; concrete interfaces must embed a base and override Send and related methods.
var abstractBaseInterfaceTypes = map[string]struct{}{
	"*common.BaseInterface":     {},
	"*interfaces.BaseInterface": {},
}

// assertConcreteInterface rejects abstract base interface pointer types listed
// in abstractBaseInterfaceTypes; wrappers that embed a base type are still allowed.
func assertConcreteInterface(iface common.NetworkInterface) error {
	if iface == nil {
		return errors.New("nil network interface")
	}
	rt := reflect.TypeOf(iface)
	if rt.Kind() != reflect.Pointer {
		return fmt.Errorf("network interface must be a pointer, got %s", rt.Kind())
	}
	name := "*" + rt.Elem().PkgPath() + "." + rt.Elem().Name()
	short := "*" + rt.Elem().String()
	if _, bad := abstractBaseInterfaceTypes[short]; bad {
		return fmt.Errorf("refusing to register abstract base interface type %s; embed it in a concrete interface that overrides Send/ProcessOutgoing", name)
	}
	return nil
}

func (t *Transport) RegisterInterface(name string, iface common.NetworkInterface) error {
	if err := assertConcreteInterface(iface); err != nil {
		return err
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	if _, exists := t.interfaces[name]; exists {
		return errors.New("interface already registered")
	}

	t.registerInterfaceLocked(name, iface)
	return nil
}

// registerInterfaceLocked registers iface under name. Transport mutex must be held.
func (t *Transport) registerInterfaceLocked(name string, iface common.NetworkInterface) {
	iface.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		t.HandlePacket(data, iface)
	})
	t.interfaces[name] = iface
	cfg := t.interfaceConfig(name)
	t.ifaceStates.put(name, buildIfaceState(cfg))
	applyIfacePRConfig(iface, cfg)
}

func (t *Transport) invalidateInterfaceReferencesLocked(iface common.NetworkInterface) {
	if iface == nil {
		return
	}
	for k, p := range t.paths {
		if p != nil && p.Interface == iface {
			delete(t.paths, k)
			delete(t.pathStates, k)
		}
	}
	for k, req := range t.discoveryPathRequests {
		if req != nil && req.RequestingIface == iface {
			delete(t.discoveryPathRequests, k)
		}
	}
	for k, e := range t.announceTable {
		if e != nil && (e.ReceivedFrom == iface || e.AttachedInterface == iface) {
			delete(t.announceTable, k)
		}
	}
	for k, e := range t.heldAnnounces {
		if e != nil && (e.ReceivedFrom == iface || e.AttachedInterface == iface) {
			delete(t.heldAnnounces, k)
		}
	}
	if t.linkTable != nil {
		t.linkTable.removeEntriesReferencing(iface)
	}
	for k, linkObj := range t.links {
		if linkObj == nil {
			continue
		}
		if ni := linkObj.LinkedNetworkInterface(); ni != nil && ni == iface {
			delete(t.links, k)
		}
	}
}

// UnregisterInterface removes a logical interface and drops paths, link relay
// rows, discovery path requests, and announce cache entries tied to it.
func (t *Transport) UnregisterInterface(name string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	iface, ok := t.interfaces[name]
	if !ok {
		return
	}
	t.invalidateInterfaceReferencesLocked(iface)
	iface.SetPacketCallback(nil)
	delete(t.interfaces, name)
	t.ifaceStates.delete(name)
}

// ReplaceInterface swaps the registered implementation for name, scrubbing
// transport state that referenced the previous instance. If name was not
// registered, behaves like [Transport.RegisterInterface].
func (t *Transport) ReplaceInterface(name string, iface common.NetworkInterface) error {
	if err := assertConcreteInterface(iface); err != nil {
		return err
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if old, ok := t.interfaces[name]; ok && old != nil {
		t.invalidateInterfaceReferencesLocked(old)
		old.SetPacketCallback(nil)
		delete(t.interfaces, name)
		t.ifaceStates.delete(name)
	}
	t.registerInterfaceLocked(name, iface)
	return nil
}

// SetReticulumConfig replaces the config pointer used for per-interface limits
// (e.g. after hot reload). Call after reloading disk config.
func (t *Transport) SetReticulumConfig(cfg *common.ReticulumConfig) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.config = cfg
}

// interfaceConfig returns config for name by map key or by InterfaceConfig.Name, or nil.
func (t *Transport) interfaceConfig(name string) *common.InterfaceConfig {
	if t.config == nil || t.config.Interfaces == nil {
		return nil
	}
	if cfg, ok := t.config.Interfaces[name]; ok {
		return cfg
	}
	for _, cfg := range t.config.Interfaces {
		if cfg != nil && cfg.Name == name {
			return cfg
		}
	}
	return nil
}

func (t *Transport) GetInterface(name string) (common.NetworkInterface, error) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	iface, exists := t.interfaces[name]
	if !exists {
		return nil, errors.New("interface not found")
	}

	return iface, nil
}

// registeredIface pairs a logical interface name with its implementation
// for snapshots taken under the transport mutex.
type registeredIface struct {
	name  string
	iface common.NetworkInterface
}

// snapshotRegisteredInterfaces returns a shallow copy of current interfaces.
// Callers may call iface methods without holding the transport mutex.
func (t *Transport) snapshotRegisteredInterfaces() []registeredIface {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	out := make([]registeredIface, 0, len(t.interfaces))
	for name, iface := range t.interfaces {
		if iface != nil {
			out = append(out, registeredIface{name: name, iface: iface})
		}
	}
	return out
}

func (t *Transport) Close() error {
	t.stopOnce.Do(func() {
		close(t.done)
	})

	t.mutex.Lock()
	defer t.mutex.Unlock()

	for _, iface := range t.interfaces {
		iface.Detach()
	}

	return nil
}

type Link struct {
	mutex               sync.RWMutex
	destination         []byte
	establishedAt       time.Time
	lastInbound         time.Time
	lastOutbound        time.Time
	lastData            time.Time
	rtt                 time.Duration
	establishedCb       func()
	closedCb            func()
	packetCb            func([]byte, *packet.Packet)
	resourceCb          func(any) bool
	resourceStrategy    int
	resourceStartedCb   func(any)
	resourceConcludedCb func(any)
	remoteIdentifiedCb  func(*Link, []byte)
	connectedCb         func()
	disconnectedCb      func()
	remoteIdentity      []byte
	physicalStats       bool
	staleTime           time.Duration
	staleGrace          time.Duration
	status              int
}

type Destination struct {
	Identity  any
	Direction int
	Type      int
	AppName   string
	Aspects   []string
}

func NewLink(dest []byte, establishedCallback func(), closedCallback func()) *Link {
	return &Link{
		destination:   dest,
		establishedAt: time.Now(),
		lastInbound:   time.Now(),
		lastOutbound:  time.Now(),
		lastData:      time.Now(),
		establishedCb: establishedCallback,
		closedCb:      closedCallback,
		staleTime:     time.Duration(StaleTime) * time.Second,
		staleGrace:    time.Duration(StaleGrace) * time.Second,
	}
}

func (l *Link) GetAge() time.Duration {
	return time.Since(l.establishedAt)
}

func (l *Link) NoInboundFor() time.Duration {
	return time.Since(l.lastInbound)
}

func (l *Link) NoOutboundFor() time.Duration {
	return time.Since(l.lastOutbound)
}

func (l *Link) NoDataFor() time.Duration {
	return time.Since(l.lastData)
}

func (l *Link) InactiveFor() time.Duration {
	inbound := l.NoInboundFor()
	outbound := l.NoOutboundFor()
	if inbound < outbound {
		return inbound
	}
	return outbound
}

func (l *Link) SetPacketCallback(cb func([]byte, *packet.Packet)) {
	l.packetCb = cb
}

func (l *Link) SetResourceCallback(cb func(any) bool) {
	l.resourceCb = cb
}

func (l *Link) Teardown() {
	if l.disconnectedCb != nil {
		l.disconnectedCb()
	}
	if l.closedCb != nil {
		l.closedCb()
	}
}

func (l *Link) Send(data []byte) any {
	l.mutex.Lock()
	l.lastOutbound = time.Now()
	l.lastData = time.Now()
	l.mutex.Unlock()

	packet := &LinkPacket{
		Destination: l.destination,
		Data:        data,
		Timestamp:   time.Now(),
	}

	if l.rtt == 0.0 {
		l.rtt = l.InactiveFor()
	}

	err := packet.send()
	if err != nil {
		return nil
	}

	return packet
}

func (t *Transport) RegisterAnnounceHandler(handler announce.Handler) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.announceHandlers = append(t.announceHandlers, handler)
}

func (t *Transport) UnregisterAnnounceHandler(handler announce.Handler) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	for i, h := range t.announceHandlers {
		if h == handler {
			t.announceHandlers = append(t.announceHandlers[:i], t.announceHandlers[i+1:]...)
			break
		}
	}
}

func (t *Transport) notifyAnnounceHandlers(destHash []byte, identity any, appData []byte, hops uint8) {
	t.mutex.RLock()
	handlers := make([]announce.Handler, len(t.announceHandlers))
	copy(handlers, t.announceHandlers)
	t.mutex.RUnlock()

	for _, handler := range handlers {
		if err := handler.ReceivedAnnounce(destHash, identity, appData, hops); err != nil {
			debug.Log(debug.DebugError, "Error in announce handler", "error", err)
		}
	}
}

func (t *Transport) HasPath(destinationHash []byte) bool {
	key := pathMapKey(destinationHash)
	ttl := time.Duration(PathRequestTTL) * time.Second

	t.mutex.RLock()
	path, exists := t.paths[key]
	t.mutex.RUnlock()
	if !exists {
		return false
	}
	if time.Since(path.LastUpdated) <= ttl {
		return true
	}

	t.mutex.Lock()
	if cur, ok := t.paths[key]; ok && time.Since(cur.LastUpdated) > ttl {
		delete(t.paths, key)
	}
	t.mutex.Unlock()
	return false
}

func (t *Transport) HopsTo(destinationHash []byte) uint8 {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	path, exists := t.paths[pathMapKey(destinationHash)]
	if !exists {
		return PathfinderM
	}

	return path.HopCount
}

func (t *Transport) NextHop(destinationHash []byte) []byte {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	path, exists := t.paths[pathMapKey(destinationHash)]
	if !exists {
		return nil
	}

	return path.NextHop
}

func (t *Transport) NextHopInterface(destinationHash []byte) string {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	path, exists := t.paths[pathMapKey(destinationHash)]
	if !exists || path == nil || path.Interface == nil {
		return ""
	}

	return path.Interface.GetName()
}

func (t *Transport) RequestPath(destinationHash []byte, onInterface string, tag []byte, recursive bool) error {
	if tag == nil {
		t.mutex.Lock()
		key := pathMapKey(destinationHash)
		if last, ok := t.lastPathRequest[key]; ok && time.Since(last) < PathRequestMI {
			t.mutex.Unlock()
			debug.Log(debug.DebugVerbose, "Throttling path request",
				"dest_hash", fmt.Sprintf("%x", destinationHash),
				"since_last", time.Since(last))
			return nil
		}
		t.lastPathRequest[key] = time.Now()
		t.mutex.Unlock()
		tag = make([]byte, 16)
		if _, err := rand.Read(tag); err != nil {
			return fmt.Errorf("failed to generate random tag: %w", err)
		}
	}

	var pathRequestData []byte
	if t.transportIdentity != nil {
		tid := t.transportIdentity.Hash()
		pathRequestData = make([]byte, 0, len(destinationHash)+len(tid)+len(tag))
		pathRequestData = append(pathRequestData, destinationHash...)
		pathRequestData = append(pathRequestData, tid...)
		pathRequestData = append(pathRequestData, tag...)
	} else {
		pathRequestData = make([]byte, 0, len(destinationHash)+len(tag))
		pathRequestData = append(pathRequestData, destinationHash...)
		pathRequestData = append(pathRequestData, tag...)
	}

	pathRequestName := "rnstransport.path.request"
	nameHashFull := sha256.Sum256([]byte(pathRequestName))
	nameHash10 := nameHashFull[:10]
	finalHashFull := sha256.Sum256(nameHash10)
	pathRequestDestHash := finalHashFull[:16]

	pkt := packet.NewPacket(
		packet.DestinationPlain,
		pathRequestData,
		0x00,
		0x00,
		packet.PropagationBroadcast,
		0x00,
		nil,
		false,
		0x00,
	)
	pkt.DestinationHash = pathRequestDestHash

	if err := pkt.Pack(); err != nil {
		return fmt.Errorf("failed to pack path request: %w", err)
	}

	debug.Log(debug.DebugInfo, "Sending path request", "dest_hash", fmt.Sprintf("%x", destinationHash), "data_len", len(pathRequestData), "packet_len", len(pkt.Raw))

	if onInterface != "" {
		t.mutex.RLock()
		iface, ok := t.interfaces[onInterface]
		t.mutex.RUnlock()
		if !ok || iface == nil {
			return fmt.Errorf("interface not found: %s", onInterface)
		}
		if !iface.IsEnabled() {
			return fmt.Errorf("interface offline or disabled: %s", onInterface)
		}
		if err := iface.Send(pkt.Raw, ""); err != nil {
			return err
		}
		iface.SentPathRequest()
		return nil
	}

	for _, e := range t.snapshotRegisteredInterfaces() {
		if !e.iface.IsEnabled() {
			continue
		}
		if err := e.iface.Send(pkt.Raw, ""); err != nil {
			debug.Log(debug.DebugError, "Failed to send path request on interface", "interface", e.iface.GetName(), "error", err)
		} else {
			e.iface.SentPathRequest()
		}
	}

	return nil
}

func (t *Transport) updatePathUnlocked(destinationHash []byte, nextHop []byte, interfaceName string, hops uint8) {
	iface, exists := t.interfaces[interfaceName]
	if !exists {
		debug.Log(debug.DebugInfo, "Interface not found", "name", interfaceName)
		return
	}

	key := pathMapKey(destinationHash)
	t.paths[key] = &common.Path{
		NextHop:     nextHop,
		Interface:   iface,
		Hops:        hops,
		HopCount:    hops,
		LastUpdated: time.Now(),
	}
	t.pathStates[key] = StateUnknown
}

func (t *Transport) UpdatePath(destinationHash []byte, nextHop []byte, interfaceName string, hops uint8) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.updatePathUnlocked(destinationHash, nextHop, interfaceName, hops)
}

func (t *Transport) HandleAnnounce(data []byte, sourceIface common.NetworkInterface) error {
	if len(data) < MinAnnouncePacketSize {
		return fmt.Errorf("announce packet too small: %d bytes", len(data))
	}

	debug.Log(debug.DebugAll, "Transport handling announce", "bytes", len(data), "source", sourceIface.GetName())

	destHash := data[1 : 32+1]
	identity := data[32+1 : 16+32+1]
	appData := data[16+32+1:]

	if tab := t.BlackholeTable(); tab != nil {
		identityHash := sha256.Sum256(identity)
		if tab.Has(identityHash[:blackhole.HashLen]) {
			debug.Log(debug.DebugAll, "Dropping announce: identity blackholed",
				"dest_hash", fmt.Sprintf("%x", destHash[:8]))
			return nil
		}
	}

	hashData := data[2:]
	announceHash := sha256.Sum256(hashData)
	hashStr := string(announceHash[:])

	t.mutex.Lock()
	if last, ok := t.seenAnnounces[hashStr]; ok {
		if time.Since(last) < SeenAnnounceTTL {
			t.mutex.Unlock()
			debug.Log(debug.DebugAll, "Ignoring duplicate announce", "hash", fmt.Sprintf("%x", announceHash[:8]))
			return nil
		}
	}
	t.seenAnnounces[hashStr] = time.Now()
	t.mutex.Unlock()

	if data[0] >= MaxHops {
		debug.Log(debug.DebugAll, "Announce exceeded max hops", "hops", data[0])
		return nil
	}

	if !t.transportEnabled() {
		debug.Log(debug.DebugVerbose, "Not relaying announce (HandleAnnounce): transport disabled")
		return nil
	}

	var delay time.Duration
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		debug.Log(debug.DebugAll, "Failed to generate random delay", "error", err)
		delay = 0
	} else {
		windowMs := max(int64(PathfinderRW*1000.0), 1)
		delay = time.Duration(int64(binary.BigEndian.Uint64(b)%uint64(windowMs))) * time.Millisecond // #nosec G115
	}
	time.Sleep(delay)

	if !t.announceRate.Allow() {
		debug.Log(debug.DebugAll, "Announce rate limit exceeded, queuing")
		return nil
	}

	data[0]++

	var lastErr error
	for _, e := range t.snapshotRegisteredInterfaces() {
		iface := e.iface
		name := e.name
		if iface == sourceIface || !iface.IsEnabled() {
			continue
		}

		if !iface.GetBandwidthAvailable() {
			debug.Log(debug.DebugVerbose, "Skipping announce forwarding on interface due to bandwidth cap", "name", name)
			continue
		}

		debug.Log(debug.DebugAll, "Forwarding announce on interface", "name", name)
		if err := iface.Send(data, ""); err != nil {
			debug.Log(debug.DebugAll, "Failed to forward announce", "name", name, "error", err)
			lastErr = err
		}
	}

	t.notifyAnnounceHandlers(destHash, identity, appData, data[0])

	return lastErr
}

func (t *Transport) NewDestination(identity any, direction int, destType int, appName string, aspects ...string) *Destination {
	return &Destination{
		Identity:  identity,
		Direction: direction,
		Type:      destType,
		AppName:   appName,
		Aspects:   aspects,
	}
}

func (t *Transport) NewLink(dest []byte, establishedCallback func(), closedCallback func()) *Link {
	return NewLink(dest, establishedCallback, closedCallback)
}

type PathRequest struct {
	DestinationHash []byte
	Tag             []byte
	TTL             int
	Recursive       bool
}

type LinkPacket struct {
	Destination []byte
	Data        []byte
	Timestamp   time.Time
}

func (p *LinkPacket) send() error {
	t := GetTransportInstance()
	if t == nil {
		return errors.New("transport not initialized")
	}

	header := make([]byte, 0, 64)
	header = append(header, PacketTypeLink)
	header = append(header, p.Destination...)

	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(p.Timestamp.Unix())) // #nosec G115
	header = append(header, ts...)

	packet := append(header, p.Data...)

	nextHop := t.NextHop(p.Destination)
	if nextHop == nil {
		return errors.New("no path to destination")
	}

	ifaceName := t.NextHopInterface(p.Destination)
	t.mutex.RLock()
	iface, ok := t.interfaces[ifaceName]
	t.mutex.RUnlock()
	if !ok || iface == nil {
		return errors.New("interface not found")
	}

	return iface.Send(packet, "")
}

func (t *Transport) sendPathRequest(req *PathRequest, interfaceName string) error {
	if req.TTL < 0 || req.TTL > PathRequestTTLMax {
		return fmt.Errorf("path request TTL out of range: %d", req.TTL)
	}
	packet := &PathRequestPacket{
		Type:            PacketTypeAnnounce,
		DestinationHash: req.DestinationHash,
		Tag:             req.Tag,
		TTL:             byte(req.TTL),
		Recursive:       req.Recursive,
	}

	buf := make([]byte, 0, 128)
	buf = append(buf, packet.Type)
	buf = append(buf, packet.DestinationHash...)
	buf = append(buf, packet.Tag...)
	buf = append(buf, packet.TTL)
	if packet.Recursive {
		buf = append(buf, wireFlagTrue)
	} else {
		buf = append(buf, wireFlagFalse)
	}

	t.mutex.RLock()
	iface, ok := t.interfaces[interfaceName]
	t.mutex.RUnlock()
	if !ok || iface == nil {
		return errors.New("interface not found")
	}

	return iface.Send(buf, "")
}

type PathRequestPacket struct {
	Type            byte
	DestinationHash []byte
	Tag             []byte
	TTL             byte
	Recursive       bool
}

type NetworkInterface struct {
	Name    string
	Addr    *net.UDPAddr
	Conn    *net.UDPConn
	MTU     int
	Enabled bool
}

func SendAnnounce(packet []byte) error {
	t := GetTransportInstance()
	if t == nil {
		return errors.New("transport not initialized")
	}

	var lastErr error
	for _, e := range t.snapshotRegisteredInterfaces() {
		if !e.iface.IsEnabled() {
			continue
		}
		if err := e.iface.Send(packet, ""); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

func (t *Transport) HandlePacket(data []byte, iface common.NetworkInterface) {
	if len(data) < 2 {
		debug.Log(debug.DebugVerbose, "Dropping packet: insufficient length", "bytes", len(data))
		return
	}

	headerByte := data[0]
	packetType := headerByte & HeaderPacketTypeMask
	headerType := (headerByte & HeaderTypeMask) >> HeaderTypeShift
	contextFlag := (headerByte & HeaderContextFlagMask) >> HeaderContextFlagShift
	propType := (headerByte & HeaderPropTypeMask) >> HeaderPropTypeShift
	destType := (headerByte & HeaderDestTypeMask) >> HeaderDestTypeShift

	debug.Log(debug.DebugVerbose, "TRANSPORT: Packet received", "type", fmt.Sprintf("0x%02x", packetType), "header", headerType, "context", contextFlag, "propType", propType, "destType", destType, "size", len(data))
	debug.Log(debug.DebugTrace, "Interface and raw header", "name", iface.GetName(), "header", fmt.Sprintf("0x%02x", headerByte))

	if len(data) == SuspiciousLinkPacketSize {
		debug.Log(debug.DebugError, "67-byte packet detected", "header", fmt.Sprintf("0x%02x", headerByte), "packet_type_bits", fmt.Sprintf("0x%02x", packetType), "first_32_bytes", fmt.Sprintf("%x", data[:32]))
	}

	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	dispatch := func() {
		switch packetType {
		case PacketTypeAnnounce:
			debug.Log(debug.DebugVerbose, "Processing announce packet")
			if err := t.handleAnnouncePacket(dataCopy, iface); err != nil {
				debug.Log(debug.DebugInfo, "Announce handling failed", "error", err)
			}
		case PacketTypeLink:
			debug.Log(debug.DebugVerbose, "Processing link packet (type=0x02)", "packet_size", len(dataCopy))
			t.handleLinkPacket(dataCopy, iface, PacketTypeLink)
		case packet.PacketTypeProof:
			debug.Log(debug.DebugVerbose, "Processing proof packet")
			pkt := &packet.Packet{Raw: dataCopy}
			if err := pkt.Unpack(); err != nil {
				debug.Log(debug.DebugInfo, "Failed to unpack proof packet", "error", err)
				return
			}
			t.handleProofPacket(pkt, iface)
		case 0:
			if destType == DestTypeLink {
				debug.Log(debug.DebugVerbose, "Processing link data packet (dest_type=3)", "packet_size", len(dataCopy))
				t.handleLinkPacket(dataCopy, iface, 0)
			} else {
				debug.Log(debug.DebugVerbose, "Processing data packet (type 0x00)", "packet_size", len(dataCopy), "dest_type", destType, "header_type", headerType)
				t.handleTransportPacket(dataCopy, iface)
			}
		default:
			debug.Log(debug.DebugInfo, "Unknown packet type", "type", fmt.Sprintf("0x%02x", packetType), "source", iface.GetName())
		}
	}

	select {
	case t.packetHandleSem <- struct{}{}:
		go func() {
			defer func() { <-t.packetHandleSem }()
			dispatch()
		}()
	default:
		dispatch()
	}
}

func (t *Transport) handleAnnouncePacket(data []byte, iface common.NetworkInterface) error {
	debug.Log(debug.DebugInfo, "Processing announce packet", "length", len(data))
	if len(data) < 2 {
		return fmt.Errorf("packet too small for header")
	}

	headerByte1 := data[0]
	hopCount := data[1]

	ifacFlag := (headerByte1 & HeaderIFACMask) >> HeaderIFACShift
	headerType := (headerByte1 & HeaderTypeMask) >> HeaderTypeShift
	contextFlag := (headerByte1 & HeaderContextFlagMask) >> HeaderContextFlagShift
	propType := (headerByte1 & HeaderPropTypeMask) >> HeaderPropTypeShift
	destType := (headerByte1 & HeaderDestTypeMask) >> HeaderDestTypeShift
	packetType := headerByte1 & HeaderPacketTypeMask

	debug.Log(debug.DebugTrace, "Announce header", "ifac", ifacFlag, "headerType", headerType, "context", contextFlag, "propType", propType, "destType", destType, "packetType", packetType)

	startIdx := HeaderSize
	if ifacFlag == 1 {
		startIdx++
	}

	addrSize := AddrHashSize
	if headerType == 1 {
		addrSize = DoubleAddrSize
	}

	minSize := startIdx + addrSize + ContextByteLen
	if len(data) < minSize {
		return fmt.Errorf("packet too small: %d bytes", len(data))
	}

	var destinationHash []byte
	var context byte
	var payload []byte
	var receivedFrom []byte

	if headerType == 0 {
		destinationHash = data[startIdx : startIdx+AddrHashSize]
		context = data[startIdx+AddrHashSize]
		payload = data[startIdx+AddrHashSize+ContextByteLen:]
		receivedFrom = destinationHash
	} else {
		receivedFrom = make([]byte, AddrHashSize)
		copy(receivedFrom, data[startIdx:startIdx+AddrHashSize])
		destinationHash = data[startIdx+AddrHashSize : startIdx+DoubleAddrSize]
		context = data[startIdx+DoubleAddrSize]
		payload = data[startIdx+DoubleAddrSize+ContextByteLen:]
	}

	debug.Log(debug.DebugInfo, "Destination hash", "hash", fmt.Sprintf("%x", destinationHash))
	debug.Log(debug.DebugInfo, "Context and payload", "context", fmt.Sprintf("%02x", context), "payload_len", len(payload))
	debug.Log(debug.DebugInfo, "Packet total length", "length", len(data))

	var id *identity.Identity
	var appData []byte
	var pubKey []byte

	minAnnounceSize := 64 + 10 + 10 + 64
	if len(payload) < minAnnounceSize {
		debug.Log(debug.DebugInfo, "Payload too small for announce", "bytes", len(payload), "minimum", minAnnounceSize)
		return fmt.Errorf("payload too small for announce")
	}

	pos := 0
	pubKey = payload[pos : pos+64]
	pos += 64
	nameHash := payload[pos : pos+10]
	pos += 10
	randomHash := payload[pos : pos+10]
	pos += 10

	var ratchetData []byte
	if contextFlag == 1 {
		if len(payload) < pos+32+64 {
			debug.Log(debug.DebugInfo, "Payload too small for announce with ratchet")
			return fmt.Errorf("payload too small for announce with ratchet")
		}
		ratchetData = payload[pos : pos+32]
		pos += 32
	}

	signature := payload[pos : pos+64]
	pos += 64
	appData = payload[pos:]

	ratchetHex := ""
	if len(ratchetData) > 0 {
		ratchetHex = fmt.Sprintf("%x", ratchetData[:8])
	} else {
		ratchetHex = "(none)"
	}
	debug.Log(debug.DebugInfo, "Parsed announce", "pubKey", fmt.Sprintf("%x", pubKey[:8]), "nameHash", fmt.Sprintf("%x", nameHash), "randomHash", fmt.Sprintf("%x", randomHash), "ratchet", ratchetHex, "appData_len", len(appData))

	id = identity.FromPublicKey(pubKey)
	if id == nil {
		debug.Log(debug.DebugInfo, "Failed to create identity from public key")
		return fmt.Errorf("invalid identity")
	}
	debug.Log(debug.DebugInfo, "Successfully created identity")

	signData := make([]byte, 0)
	signData = append(signData, destinationHash...)
	signData = append(signData, pubKey...)
	signData = append(signData, nameHash...)
	signData = append(signData, randomHash...)
	if len(ratchetData) > 0 {
		signData = append(signData, ratchetData...)
	}
	signData = append(signData, appData...)

	debug.Log(debug.DebugInfo, "Verifying signature", "data_len", len(signData))

	if !id.Verify(signData, signature) {
		debug.Log(debug.DebugInfo, "Signature verification failed - announce rejected")
		return fmt.Errorf("invalid announce signature")
	}
	debug.Log(debug.DebugInfo, "Signature verification successful")

	hashMaterial := make([]byte, 0)
	hashMaterial = append(hashMaterial, nameHash...)
	hashMaterial = append(hashMaterial, id.Hash()...)
	expectedHashFull := sha256.Sum256(hashMaterial)
	expectedHash := expectedHashFull[:16]

	debug.Log(debug.DebugInfo, "Destination hash validation", "received", fmt.Sprintf("%x", destinationHash), "expected", fmt.Sprintf("%x", expectedHash))

	if !bytes.Equal(destinationHash, expectedHash) {
		debug.Log(debug.DebugInfo, "Destination hash mismatch - announce rejected")
		return fmt.Errorf("destination hash mismatch")
	}
	debug.Log(debug.DebugInfo, "Destination hash validation successful")

	if len(appData) > 0 {
		debug.Log(debug.DebugInfo, "Accepted announce with app_data", "data", fmt.Sprintf("%x", appData), "string", string(appData))
	}

	identity.Remember(data, destinationHash, pubKey, appData)

	hashData := data[2:]
	announceHash := sha256.Sum256(hashData)
	hashStr := string(announceHash[:])

	debug.Log(debug.DebugInfo, "Announce hash", "hash", fmt.Sprintf("%x", announceHash[:8]))

	t.mutex.Lock()
	if last, ok := t.seenAnnounces[hashStr]; ok {
		if time.Since(last) < SeenAnnounceTTL {
			t.mutex.Unlock()
			debug.Log(debug.DebugInfo, "Ignoring duplicate announce", "hash", fmt.Sprintf("%x", announceHash[:8]))
			return nil
		}
	}
	t.mutex.Unlock()

	isNewDest := iface != nil && !t.HasPath(destinationHash)

	debug.Log(debug.DebugInfo, "Processing new announce")

	if iface != nil {
		nextHop := receivedFrom
		if len(nextHop) == 0 {
			nextHop = destinationHash
		}
		t.mutex.Lock()
		t.updatePathUnlocked(destinationHash, nextHop, iface.GetName(), hopCount+1)
		t.mutex.Unlock()
		debug.Log(debug.DebugInfo, "Registered path", "hash", fmt.Sprintf("%x", destinationHash), "interface", iface.GetName(), "hops", hopCount+1, "nextHop", fmt.Sprintf("%x", nextHop))
	}

	debug.Log(debug.DebugInfo, "Notifying announce handlers", "destHash", fmt.Sprintf("%x", destinationHash), "appDataLen", len(appData))
	t.notifyAnnounceHandlers(destinationHash, id, appData, hopCount+1)
	debug.Log(debug.DebugInfo, "Announce handlers notified")

	t.mutex.Lock()
	t.seenAnnounces[hashStr] = time.Now()
	t.mutex.Unlock()

	if iface != nil {
		if st := t.ifaceStates.get(iface.GetName()); st != nil && st.ingress != nil {
			if !st.ingress.ProcessAnnounce(string(announceHash[:]), data, isNewDest) {
				debug.Log(debug.DebugVerbose,
					"Announce held by ingress control",
					"iface", iface.GetName(),
					"dest_hash", fmt.Sprintf("%x", destinationHash),
					"queue_depth", st.ingress.HeldCount())
				return nil
			}
		}
	}

	if hopCount >= MaxHops {
		debug.Log(debug.DebugInfo, "Announce exceeded max hops", "hops", hopCount)
		return nil
	}
	debug.Log(debug.DebugInfo, "Hop count OK", "hops", hopCount)

	if !t.transportEnabled() {
		debug.Log(debug.DebugVerbose, "Not forwarding announce: transport disabled",
			"dest_hash", fmt.Sprintf("%x", destinationHash))
		return nil
	}

	if !t.announceRate.Allow() {
		debug.Log(debug.DebugInfo, "Announce rate limit exceeded, not forwarding")
		return nil
	}
	debug.Log(debug.DebugInfo, "Bandwidth check passed")

	var delay time.Duration
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		debug.Log(debug.DebugAll, "Failed to generate random delay", "error", err)
		delay = 0
	} else {
		windowMs := max(int64(PathfinderRW*1000.0), 1)
		delay = time.Duration(int64(binary.BigEndian.Uint64(b)%uint64(windowMs))) * time.Millisecond // #nosec G115
	}
	time.Sleep(delay)

	data[1]++

	destKey := string(destinationHash)
	var lastErr error
	for _, e := range t.snapshotRegisteredInterfaces() {
		name := e.name
		outIface := e.iface
		if outIface == iface || !outIface.IsEnabled() {
			continue
		}

		if !outIface.GetBandwidthAvailable() {
			debug.Log(debug.DebugVerbose, "Skipping announce forwarding on interface due to bandwidth cap", "name", name)
			continue
		}

		if st := t.ifaceStates.get(name); st != nil && st.egress != nil {
			if !st.egress.AllowAnnounce(destKey) {
				debug.Log(debug.DebugVerbose,
					"Skipping announce forwarding due to per-destination rate target",
					"iface", name,
					"dest_hash", fmt.Sprintf("%x", destinationHash))
				continue
			}
		}

		debug.Log(debug.DebugAll, "Forwarding announce on interface", "name", name)
		if err := outIface.Send(data, ""); err != nil {
			debug.Log(debug.DebugAll, "Failed to forward announce", "name", name, "error", err)
			lastErr = err
		}
	}

	return lastErr
}

func (t *Transport) handleLinkPacket(data []byte, iface common.NetworkInterface, packetType byte) {
	startTime := time.Now()
	debug.Log(debug.DebugVerbose, "Handling link packet", "bytes", len(data), "packet_type", fmt.Sprintf("0x%02x", packetType), "interface", iface.GetName())

	pkt := &packet.Packet{Raw: data}

	if packetType == PacketTypeLink {
		debug.Log(debug.DebugVerbose, "Processing LINKREQUEST (type=0x02)", "interface", iface.GetName())

		if err := pkt.Unpack(); err != nil {
			debug.Log(debug.DebugError, "Failed to unpack link request", "error", err, "elapsed", time.Since(startTime).Seconds())
			return
		}

		if t.forwardTransportPacket(pkt, data, iface) {
			return
		}

		destHash := pkt.DestinationHash
		if len(destHash) > 16 {
			destHash = destHash[:16]
		}

		debug.Log(debug.DebugVerbose, "Link request for destination", "hash", fmt.Sprintf("%x", destHash), "interface", iface.GetName())

		destKey := hash16FromSlice(destHash)

		t.mutex.RLock()
		destIface, exists := t.destinations[destKey]
		t.mutex.RUnlock()

		if !exists {
			debug.Log(debug.DebugError, "No destination registered for hash", "hash", fmt.Sprintf("%x", destHash), "elapsed", time.Since(startTime).Seconds())
			return
		}

		debug.Log(debug.DebugVerbose, "Found registered destination", "hash", fmt.Sprintf("%x", destHash), "elapsed", time.Since(startTime).Seconds())

		reqStartTime := time.Now()
		t.handleIncomingLinkRequest(pkt, destIface, iface)
		debug.Log(debug.DebugVerbose, "Link request handling completed", "elapsed", time.Since(reqStartTime).Seconds(), "total_elapsed", time.Since(startTime).Seconds())
		return
	}

	debug.Log(debug.DebugVerbose, "Processing link data packet", "interface", iface.GetName())

	if err := pkt.Unpack(); err != nil {
		debug.Log(debug.DebugError, "Failed to unpack link data packet", "error", err, "interface", iface.GetName())
		return
	}

	linkID := pkt.DestinationHash
	if len(linkID) > 16 {
		linkID = linkID[:16]
	}

	debug.Log(debug.DebugVerbose, "Link data for link ID", "link_id", fmt.Sprintf("%x", linkID), "context", fmt.Sprintf("0x%02x", pkt.Context), "packet_type", fmt.Sprintf("0x%02x", pkt.PacketType), "interface", iface.GetName())

	linkKey := hash16FromSlice(linkID)

	t.mutex.RLock()
	linkObj, exists := t.links[linkKey]
	t.mutex.RUnlock()

	if exists && linkObj != nil {
		debug.Log(debug.DebugVerbose, "Routing packet to established link")
		if err := linkObj.HandleInbound(pkt); err != nil {
			debug.Log(debug.DebugError, "Error handling inbound packet", "error", err)
		}
		return
	}

	if t.forwardLinkData(data, iface) {
		return
	}

	debug.Log(debug.DebugVerbose, "No established link found for link ID", "link_id", fmt.Sprintf("%x", linkID))
}

func (t *Transport) handleIncomingLinkRequest(pkt *packet.Packet, destIface registeredDestination, networkIface common.NetworkInterface) {
	startTime := time.Now()
	debug.Log(debug.DebugVerbose, "Handling incoming link request", "interface", networkIface.GetName())

	linkID := pkt.Data
	if len(linkID) == 0 {
		debug.Log(debug.DebugVerbose, "No link ID in link request packet", "elapsed", time.Since(startTime).Seconds())
		return
	}

	debug.Log(debug.DebugVerbose, "Link request with ID", "id", fmt.Sprintf("%x", linkID[:8]), "full_id", fmt.Sprintf("%x", linkID), "elapsed", time.Since(startTime).Seconds())

	if destIface.linkRequestHandler == nil {
		debug.Log(debug.DebugError, "Destination does not have HandleIncomingLinkRequest method", "elapsed", time.Since(startTime).Seconds())
		return
	}
	callStartTime := time.Now()
	if err := destIface.linkRequestHandler.HandleIncomingLinkRequest(pkt, t, networkIface); err != nil {
		debug.Log(debug.DebugError, "Failed to handle incoming link request", "error", err, "call_elapsed", time.Since(callStartTime).Seconds(), "total_elapsed", time.Since(startTime).Seconds())
		return
	}
	debug.Log(debug.DebugVerbose, "Link request handled successfully by destination", "call_elapsed", time.Since(callStartTime).Seconds(), "total_elapsed", time.Since(startTime).Seconds())
}

func (t *Transport) handlePathResponse(data []byte, iface common.NetworkInterface) {
	if len(data) < MinPathResponseSize {
		return
	}

	destHash := data[:DoubleAddrSize]
	hops := data[DoubleAddrSize]
	var nextHop []byte

	if len(data) > MinPathResponseSize {
		nextHop = data[MinPathResponseSize:]
	}

	if iface != nil {
		t.UpdatePath(destHash, nextHop, iface.GetName(), hops)
	}
}

func (t *Transport) handleTransportPacket(data []byte, iface common.NetworkInterface) {
	if len(data) < 2 {
		return
	}

	pkt := &packet.Packet{Raw: data}
	if err := pkt.Unpack(); err != nil {
		debug.Log(debug.DebugInfo, "Failed to unpack transport packet", "error", err)
		return
	}

	headerByte := data[0]
	packetType := headerByte & HeaderPacketTypeMask
	destType := (headerByte & HeaderDestTypeMask) >> HeaderDestTypeShift

	if packetType == packet.PacketTypeData {
		if destType == DestTypePlain {
			if len(data) < MinTransportPacketSize {
				return
			}

			context := data[MinTransportPacketSize-ContextByteLen]

			if context == packet.ContextPathResponse {
				t.handlePathResponse(data[MinTransportPacketSize:], iface)
				return
			}
		}

		if t.forwardTransportPacket(pkt, data, iface) {
			return
		}

		if destType == DestTypeLink && t.forwardLinkData(data, iface) {
			return
		}

		destHash := pkt.DestinationHash
		if len(destHash) > 16 {
			destHash = destHash[:16]
		}

		debug.Log(debug.DebugVerbose, "Looking up destination for data packet", "hash", fmt.Sprintf("%x", destHash))

		destKey := hash16FromSlice(destHash)

		t.mutex.RLock()
		destIface, exists := t.destinations[destKey]
		t.mutex.RUnlock()

		if exists {
			debug.Log(debug.DebugInfo, "Routing data packet to destination", "hash", fmt.Sprintf("%x", destHash))

			if destIface.packetReceiver != nil {
				destIface.packetReceiver.Receive(pkt, iface)
			} else {
				debug.Log(debug.DebugVerbose, "Destination does not have Receive method")
			}
		} else {
			debug.Log(debug.DebugVerbose, "No destination registered for hash", "hash", fmt.Sprintf("%x", destHash))
		}
	}
}

func (t *Transport) InitializePathRequestHandler() error {
	if t.transportIdentity == nil {
		return errors.New("transport identity not initialized")
	}

	pathRequestDest, err := destination.New(nil, destination.In, destination.Plain, "rnstransport", t, "path", "request")
	if err != nil {
		return fmt.Errorf("failed to create path request destination: %w", err)
	}

	pathRequestDest.SetPacketCallback(func(data []byte, iface common.NetworkInterface) {
		t.handlePathRequest(data, iface)
	})

	pathRequestDest.AcceptsLinks(true)
	t.pathRequestDest = pathRequestDest
	t.RegisterDestination(pathRequestDest.GetHash(), pathRequestDest)

	debug.Log(debug.DebugInfo, "Path request handler initialized")
	return nil
}

func (t *Transport) handlePathRequest(data []byte, iface common.NetworkInterface) {
	if len(data) < identity.TruncatedHashLength/8 {
		debug.Log(debug.DebugInfo, "Path request too short")
		return
	}

	destHash := data[:identity.TruncatedHashLength/8]
	var requestorTransportID []byte
	var tag []byte

	if len(data) > identity.TruncatedHashLength/8*2 {
		requestorTransportID = data[identity.TruncatedHashLength/8 : identity.TruncatedHashLength/8*2]
		tag = data[identity.TruncatedHashLength/8*2:]
		if len(tag) > identity.TruncatedHashLength/8 {
			tag = tag[:identity.TruncatedHashLength/8]
		}
	} else if len(data) > identity.TruncatedHashLength/8 {
		tag = data[identity.TruncatedHashLength/8:]
		if len(tag) > identity.TruncatedHashLength/8 {
			tag = tag[:identity.TruncatedHashLength/8]
		}
	}

	if tag == nil {
		debug.Log(debug.DebugInfo, "Ignoring tagless path request", "dest_hash", fmt.Sprintf("%x", destHash))
		return
	}

	uniqueTag := make([]byte, 0, len(destHash)+len(tag))
	uniqueTag = append(uniqueTag, destHash...)
	uniqueTag = append(uniqueTag, tag...)
	tagStr := string(uniqueTag)

	t.mutex.Lock()
	if t.discoveryPRTags[tagStr] {
		t.mutex.Unlock()
		debug.Log(debug.DebugInfo, "Ignoring duplicate path request", "dest_hash", fmt.Sprintf("%x", destHash), "tag", fmt.Sprintf("%x", tag))
		return
	}
	t.discoveryPRTags[tagStr] = true
	if len(t.discoveryPRTags) > DiscoveryPRTagsCap {
		t.discoveryPRTags = make(map[string]bool)
	}
	t.mutex.Unlock()

	if iface != nil {
		iface.ReceivedPathRequest()
	}

	t.processPathRequest(destHash, iface, requestorTransportID, tag)
}

func (t *Transport) processPathRequest(destHash []byte, attachedIface common.NetworkInterface, requestorTransportID []byte, tag []byte) {
	destHashStr := string(destHash)
	pathKey := pathMapKey(destHash)
	debug.Log(debug.DebugInfo, "Processing path request", "dest_hash", fmt.Sprintf("%x", destHash))

	destKey := hash16FromSlice(destHash)
	t.mutex.RLock()
	localDest, isLocal := t.destinations[destKey]
	path, hasPath := t.paths[pathKey]
	t.mutex.RUnlock()

	if hasPath && path != nil {
		ttl := time.Duration(PathRequestTTL) * time.Second
		if time.Since(path.LastUpdated) > ttl {
			t.mutex.Lock()
			if cur, ok := t.paths[pathKey]; ok && cur == path && time.Since(cur.LastUpdated) > ttl {
				delete(t.paths, pathKey)
				delete(t.pathStates, pathKey)
			}
			t.mutex.Unlock()
			hasPath = false
			path = nil
		}
	}

	if isLocal {
		if dest, ok := localDest.raw.(*destination.Destination); ok {
			debug.Log(debug.DebugInfo, "Answering path request for local destination", "dest_hash", fmt.Sprintf("%x", destHash))
			if err := dest.Announce(true, tag, attachedIface); err != nil {
				debug.Log(debug.DebugError, "Failed to announce local destination for path request", "error", err)
			}
		}
		return
	}

	if hasPath {
		if !t.transportEnabled() {
			debug.Log(debug.DebugVerbose, "Not answering remote path request: transport disabled",
				"dest_hash", fmt.Sprintf("%x", destHash))
			return
		}
		nextHop := path.NextHop
		if requestorTransportID != nil && bytes.Equal(nextHop, requestorTransportID) {
			debug.Log(debug.DebugInfo, "Not answering path request, next hop is requestor", "dest_hash", fmt.Sprintf("%x", destHash))
			return
		}

		debug.Log(debug.DebugInfo, "Answering path request with known path", "dest_hash", fmt.Sprintf("%x", destHash), "hops", path.HopCount)

		t.mutex.RLock()
		announceEntry, hasAnnounce := t.announceTable[destHashStr]
		t.mutex.RUnlock()

		if hasAnnounce && announceEntry != nil {
			now := time.Now()
			retries := 1
			localRebroadcasts := 0
			blockRebroadcasts := true
			announceHops := path.HopCount

			retransmitTimeout := now.Add(time.Duration(400) * time.Millisecond)

			entry := &PathAnnounceEntry{
				CreatedAt:         now,
				RetransmitTimeout: retransmitTimeout,
				Retries:           retries,
				ReceivedFrom:      path.Interface,
				AnnounceHops:      announceHops,
				Packet:            announceEntry.Packet,
				LocalRebroadcasts: localRebroadcasts,
				BlockRebroadcasts: blockRebroadcasts,
				AttachedInterface: attachedIface,
			}

			t.mutex.Lock()
			if _, held := t.announceTable[destHashStr]; held {
				t.heldAnnounces[destHashStr] = t.announceTable[destHashStr]
			}
			t.announceTable[destHashStr] = entry
			t.mutex.Unlock()
		}
		return
	}

	if attachedIface == nil {
		debug.Log(debug.DebugInfo, "Ignoring path request, no path known", "dest_hash", fmt.Sprintf("%x", destHash))
		return
	}
	if !t.transportEnabled() {
		debug.Log(debug.DebugVerbose, "Not rebroadcasting path request: transport disabled",
			"dest_hash", fmt.Sprintf("%x", destHash))
		return
	}
	if attachedIface.ShouldIngressLimitPR() {
		debug.Log(debug.DebugVerbose, "Not rebroadcasting path request: ingress limiting active",
			"dest_hash", fmt.Sprintf("%x", destHash), "iface", attachedIface.GetName())
		return
	}

	debug.Log(debug.DebugInfo, "Attempting to discover unknown path", "dest_hash", fmt.Sprintf("%x", destHash))

	t.mutex.Lock()
	if _, exists := t.discoveryPathRequests[destHashStr]; exists {
		t.mutex.Unlock()
		debug.Log(debug.DebugInfo, "Path request already pending", "dest_hash", fmt.Sprintf("%x", destHash))
		return
	}

	prEntry := &DiscoveryPathRequest{
		DestinationHash: destHash,
		Timeout:         time.Now().Add(15 * time.Second),
		RequestingIface: attachedIface,
	}
	t.discoveryPathRequests[destHashStr] = prEntry
	t.mutex.Unlock()

	t.queueDiscoveryPathRequest(destHash, attachedIface)
}

func (t *Transport) SendPacket(p *packet.Packet) error {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	debug.Log(debug.DebugVerbose, "Sending packet", "type", fmt.Sprintf("0x%02x", p.PacketType), "header", p.HeaderType)

	destHash := p.DestinationHash
	if len(destHash) > 16 {
		destHash = destHash[:16]
	}
	debug.Log(debug.DebugPackets, "Destination hash", "hash", fmt.Sprintf("%x", destHash))

	path, exists := t.paths[pathMapKey(destHash)]
	if !exists {
		debug.Log(debug.DebugInfo, "No path found for destination", "hash", fmt.Sprintf("%x", destHash))
		return errors.New("no path to destination")
	}

	if p.DestinationType != DestTypeLink && path.HopCount > 1 && len(path.NextHop) > 0 && !bytes.Equal(path.NextHop, destHash) {
		debug.Log(debug.DebugVerbose, "Rewrapping packet for transport", "destHash", fmt.Sprintf("%x", destHash), "nextHop", fmt.Sprintf("%x", path.NextHop), "hops", path.HopCount)
		p.HeaderType = packet.HeaderType2
		p.TransportType = packet.PropagationTransport
		p.TransportID = path.NextHop
		p.Packed = false
	}

	data, err := p.Serialize()
	if err != nil {
		debug.Log(debug.DebugInfo, "Packet serialization failed", "error", err)
		return fmt.Errorf("failed to serialize packet: %w", err)
	}
	debug.Log(debug.DebugTrace, "Serialized packet size", "bytes", len(data))

	debug.Log(debug.DebugTrace, "Using path", "interface", path.Interface.GetName(), "nextHop", fmt.Sprintf("%x", path.NextHop), "hops", path.HopCount)

	if err := path.Interface.Send(data, ""); err != nil {
		debug.Log(debug.DebugInfo, "Failed to send packet", "error", err)
		return fmt.Errorf("failed to send packet: %w", err)
	}

	p.Sent = true
	p.SentAt = time.Now()

	if p.CreateReceipt {
		receipt := packet.NewPacketReceipt(p)
		t.RegisterReceipt(receipt)
		debug.Log(debug.DebugPackets, "Created packet receipt")
	}

	debug.Log(debug.DebugAll, "Packet sent successfully")
	return nil
}

func (t *Transport) RegisterLink(linkID []byte, linkObj LinkInterface) {
	linkKey := hash16FromSlice(linkID)

	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.links[linkKey] = linkObj
	debug.Log(debug.DebugVerbose, "Registered link", "link_id", fmt.Sprintf("%x", linkID))
}

func (t *Transport) UnregisterLink(linkID []byte) {
	linkKey := hash16FromSlice(linkID)

	t.mutex.Lock()
	defer t.mutex.Unlock()

	delete(t.links, linkKey)
	debug.Log(debug.DebugVerbose, "Unregistered link", "link_id", fmt.Sprintf("%x", linkID))
}

func (l *Link) OnConnected(cb func()) {
	l.connectedCb = cb
	if !l.establishedAt.IsZero() && cb != nil {
		cb()
	}
}

func (l *Link) OnDisconnected(cb func()) {
	l.disconnectedCb = cb
}

func (l *Link) GetRemoteIdentity() []byte {
	return l.remoteIdentity
}

func (l *Link) TrackPhyStats(track bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.physicalStats = track
}

func (l *Link) GetRSSI() int {
	return 0
}

func (l *Link) GetSNR() float64 {
	return 0
}

func (l *Link) GetQ() float64 {
	return 0
}

func (l *Link) SetResourceStrategy(strategy int) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if strategy != AcceptNone && strategy != AcceptAll && strategy != AcceptApp {
		return errors.New("invalid resource strategy")
	}

	l.resourceStrategy = strategy
	return nil
}

func (l *Link) SetResourceStartedCallback(cb func(any)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.resourceStartedCb = cb
}

func (l *Link) SetResourceConcludedCallback(cb func(any)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.resourceConcludedCb = cb
}

func (l *Link) SetRemoteIdentifiedCallback(cb func(*Link, []byte)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.remoteIdentifiedCb = cb
}

func (l *Link) HandleResource(resource any) bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	switch l.resourceStrategy {
	case AcceptNone:
		return false
	case AcceptAll:
		return true
	case AcceptApp:
		if l.resourceCb != nil {
			return l.resourceCb(resource)
		}
		return false
	default:
		return false
	}
}

// SetIdentity sets the identity for the Transport.
func (t *Transport) SetIdentity(id *identity.Identity) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.transportIdentity = id
}

// Start initializes the Transport.
func (t *Transport) Start() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return nil
}

// LinkInterface is the channel-facing link API (status, RTT, send, proofs).
type LinkInterface interface {
	GetStatus() byte
	GetRTT() float64
	RTT() float64
	GetLinkID() []byte
	Send(data []byte) any
	Resend(packet any) error
	SetPacketTimeout(packet any, callback func(any), timeout time.Duration)
	SetPacketDelivered(packet any, callback func(any))
	HandleInbound(pkt *packet.Packet) error
	ValidateLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error
	// LinkedNetworkInterface returns the bound outbound iface, or nil if unknown.
	LinkedNetworkInterface() common.NetworkInterface
}

func (l *Link) GetRTT() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.rtt.Seconds()
}

func (l *Link) RTT() float64 {
	return l.GetRTT()
}

func (l *Link) Resend(p any) error {
	if pkt, ok := p.(*packet.Packet); ok {
		t := GetTransportInstance()
		if t == nil {
			return fmt.Errorf("transport not initialized")
		}
		return t.SendPacket(pkt)
	}
	return fmt.Errorf("invalid packet type")
}

func (l *Link) SetPacketTimeout(p any, callback func(any), timeout time.Duration) {
	if pkt, ok := p.(*packet.Packet); ok {
		time.AfterFunc(timeout, func() {
			callback(pkt)
		})
	}
}

func (l *Link) SetPacketDelivered(p any, callback func(any)) {
	if pkt, ok := p.(*packet.Packet); ok {
		l.mutex.Lock()
		l.rtt = time.Since(time.Now())
		l.mutex.Unlock()
		callback(pkt)
	}
}

func (l *Link) GetStatus() int {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.status
}

func CreateAnnouncePacket(destHash []byte, identity *identity.Identity, appData []byte, destName string, hops byte, config *common.ReticulumConfig) ([]byte, error) {
	debug.Log(debug.DebugInfo, "Creating announce packet", "destName", destName)
	debug.Log(debug.DebugInfo, "Input", "destHash", fmt.Sprintf("%x", destHash[:8]), "appData", string(appData), "hops", hops)

	headerByte := byte(
		(0 << 7) |
			(0 << 6) |
			(0 << 5) |
			(0 << 4) |
			(0 << 2) |
			PacketTypeAnnounce,
	)

	debug.Log(debug.DebugAll, "Created header byte", "header", fmt.Sprintf("0x%02x", headerByte), "hops", hops)
	packet := []byte{headerByte, hops}
	debug.Log(debug.DebugAll, "Initial packet size", "bytes", len(packet))

	if len(destHash) > 16 {
		destHash = destHash[:16]
	}
	debug.Log(debug.DebugAll, "Adding destination hash (16 bytes)", "hash", fmt.Sprintf("%x", destHash))
	packet = append(packet, destHash...)
	debug.Log(debug.DebugAll, "Packet size after adding destination hash", "bytes", len(packet))

	pubKey := identity.GetPublicKey()
	encKey := pubKey[:32]
	signKey := pubKey[32:]
	debug.Log(debug.DebugAll, "Full public key", "key", fmt.Sprintf("%x", pubKey))

	debug.Log(debug.DebugAll, "Adding encryption key (32 bytes)", "key", fmt.Sprintf("%x", encKey))
	packet = append(packet, encKey...)
	debug.Log(debug.DebugAll, "Packet size after adding encryption key", "bytes", len(packet))

	debug.Log(debug.DebugAll, "Adding signing key (32 bytes)", "key", fmt.Sprintf("%x", signKey))
	packet = append(packet, signKey...)
	debug.Log(debug.DebugAll, "Packet size after adding signing key", "bytes", len(packet))

	nameHash := sha256.Sum256([]byte(destName))
	debug.Log(debug.DebugAll, "Adding name hash", "destName", destName, "hash", fmt.Sprintf("%x", nameHash[:AnnounceNameHashSize]), "size", AnnounceNameHashSize)
	packet = append(packet, nameHash[:AnnounceNameHashSize]...)
	debug.Log(debug.DebugAll, "Packet size after adding name hash", "bytes", len(packet))

	randomBytes := make([]byte, AnnounceRandomBytesLen)
	_, err := rand.Read(randomBytes) // #nosec G104
	if err != nil {
		debug.Log(debug.DebugAll, "Failed to read random bytes", "error", err)
		return nil, err
	}
	timeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timeBytes, uint64(time.Now().Unix())) // #nosec G115
	tsSlice := timeBytes[8-AnnounceTimestampBytesLen:]
	debug.Log(debug.DebugAll, "Adding random hash", "random", fmt.Sprintf("%x", randomBytes), "time", fmt.Sprintf("%x", tsSlice), "size", AnnounceRandomHashSize)
	packet = append(packet, randomBytes...)
	packet = append(packet, tsSlice...)
	debug.Log(debug.DebugAll, "Packet size after adding random hash", "bytes", len(packet))

	nameBytes := []byte(destName)
	if len(nameBytes) > MsgpackBin8MaxLen || len(appData) > MsgpackBin8MaxLen {
		debug.Log(debug.DebugError, "announce name or app data exceeds msgpack bin8 limit", "nameLen", len(nameBytes), "appLen", len(appData))
		return nil, errors.New("announce name or app data exceeds msgpack bin8 limit")
	}
	appDataMsg := []byte{MsgpackArray2}

	appDataMsg = append(appDataMsg, MsgpackBin8, byte(len(nameBytes))) // #nosec G115 -- lengths checked against MsgpackBin8MaxLen
	appDataMsg = append(appDataMsg, nameBytes...)

	appDataMsg = append(appDataMsg, MsgpackBin8, byte(len(appData))) // #nosec G115 -- lengths checked against MsgpackBin8MaxLen
	appDataMsg = append(appDataMsg, appData...)

	signData := make([]byte, 0, len(destHash)+len(appDataMsg))
	signData = append(signData, destHash...)
	signData = append(signData, appDataMsg...)
	signature, err := identity.Sign(signData)
	if err != nil {
		return nil, fmt.Errorf("sign announce: %w", err)
	}
	debug.Log(debug.DebugAll, "Adding signature (64 bytes)", "signature", fmt.Sprintf("%x", signature))
	packet = append(packet, signature...)
	debug.Log(debug.DebugAll, "Packet size after adding signature", "bytes", len(packet))

	packet = append(packet, appDataMsg...)
	debug.Log(debug.DebugInfo, "Final packet size", "bytes", len(packet))
	debug.Log(debug.DebugInfo, "appDataMsg", "data", fmt.Sprintf("%x", appDataMsg), "len", len(appDataMsg))

	return packet, nil
}

func (t *Transport) GetInterfaces() map[string]common.NetworkInterface {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	interfaces := make(map[string]common.NetworkInterface, len(t.interfaces))
	maps.Copy(interfaces, t.interfaces)

	return interfaces
}

func (t *Transport) GetConfig() *common.ReticulumConfig {
	return t.config
}

func (t *Transport) RegisterReceipt(receipt *packet.PacketReceipt) {
	t.receiptsMutex.Lock()
	defer t.receiptsMutex.Unlock()
	t.receipts = append(t.receipts, receipt)
	debug.Log(debug.DebugPackets, "Registered packet receipt", "hash", fmt.Sprintf("%x", receipt.GetHash()[:8]))
}

func (t *Transport) UnregisterReceipt(receipt *packet.PacketReceipt) {
	t.receiptsMutex.Lock()
	defer t.receiptsMutex.Unlock()

	for i, r := range t.receipts {
		if r == receipt {
			t.receipts = append(t.receipts[:i], t.receipts[i+1:]...)
			debug.Log(debug.DebugPackets, "Unregistered packet receipt")
			return
		}
	}
}

func (t *Transport) handleProofPacket(pkt *packet.Packet, iface common.NetworkInterface) {
	debug.Log(debug.DebugPackets, "Processing proof packet", "size", len(pkt.Data), "context", fmt.Sprintf("0x%02x", pkt.Context))

	if pkt.Context == packet.ContextLRProof {
		linkID := pkt.DestinationHash
		if len(linkID) > 16 {
			linkID = linkID[:16]
		}

		debug.Log(debug.DebugInfo, "Received link proof packet", "link_id", fmt.Sprintf("%x", linkID), "data_len", len(pkt.Data))

		linkKey := hash16FromSlice(linkID)

		t.mutex.RLock()
		link, exists := t.links[linkKey]
		t.mutex.RUnlock()

		if exists && link != nil {
			debug.Log(debug.DebugInfo, "Found link for proof, validating", "link_id", fmt.Sprintf("%x", linkID), "interface", iface.GetName())
			startTime := time.Now()
			if err := link.ValidateLinkProof(pkt, iface); err != nil {
				debug.Log(debug.DebugError, "Link proof validation failed", "error", err, "link_id", fmt.Sprintf("%x", linkID), "elapsed", time.Since(startTime).Seconds())
			} else {
				debug.Log(debug.DebugInfo, "Link proof validated successfully", "link_id", fmt.Sprintf("%x", linkID), "elapsed", time.Since(startTime).Seconds())
			}
			return
		}
		debug.Log(debug.DebugInfo, "No link found for proof packet", "link_id", fmt.Sprintf("%x", linkID))
		return
	}

	if pkt.Context == packet.ContextResourcePRF {
		linkID := pkt.DestinationHash
		if len(linkID) > 16 {
			linkID = linkID[:16]
		}
		linkKey := hash16FromSlice(linkID)
		t.mutex.RLock()
		linkObj, exists := t.links[linkKey]
		t.mutex.RUnlock()
		if exists && linkObj != nil {
			if err := linkObj.HandleInbound(pkt); err != nil {
				debug.Log(debug.DebugError, "Resource proof handling failed", "error", err, "link_id", fmt.Sprintf("%x", linkID))
			}
			return
		}
		debug.Log(debug.DebugInfo, "No link found for resource proof packet", "link_id", fmt.Sprintf("%x", linkID))
		return
	}

	var proofHash []byte
	if len(pkt.Data) == packet.ExplicitLength {
		proofHash = pkt.Data[:identity.HashLength/8]
		debug.Log(debug.DebugPackets, "Explicit proof", "hash", fmt.Sprintf("%x", proofHash[:8]))
	} else {
		debug.Log(debug.DebugPackets, "Implicit proof")
	}

	t.receiptsMutex.RLock()
	receipts := make([]*packet.PacketReceipt, len(t.receipts))
	copy(receipts, t.receipts)
	t.receiptsMutex.RUnlock()

	for _, receipt := range receipts {
		receiptValidated := false

		if proofHash != nil {
			if receipt.MatchesHash(proofHash) {
				receiptValidated = receipt.ValidateProofPacket(pkt)
			}
		} else {
			receiptValidated = receipt.ValidateProofPacket(pkt)
		}

		if receiptValidated {
			debug.Log(debug.DebugPackets, "Proof validated for receipt")
			t.UnregisterReceipt(receipt)
			return
		}
	}

	debug.Log(debug.DebugPackets, "No matching receipt for proof")
}
