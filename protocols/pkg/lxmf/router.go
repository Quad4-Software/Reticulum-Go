// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// RouterOptions configures an lxmd-style LXMF router instance.
type RouterOptions struct {
	Config      Config
	StoragePath string
	MessagesDir string
	OnInbound   string
}

// Router is the lxmd server core for delivery and optional propagation.
type Router struct {
	mu sync.RWMutex

	identity  *identity.Identity
	transport *transport.Transport
	cfg       Config

	storagePath string
	messagesDir string
	onInbound   string

	deliveryDest    *destination.Destination
	propagationDest *destination.Destination
	controlDest     *destination.Destination

	propagationEnabled bool
	propagationStart   time.Time

	store *MessageStore

	peersMu      sync.RWMutex
	peers        map[string]*Peer
	staticPeers  map[string]struct{}
	controlAllow map[string]struct{}

	inboundStampCost *int
	enforceStamps    bool

	ignoredList map[string]struct{}
	allowedList map[string]struct{}

	deliveredMu      sync.Mutex
	locallyDelivered map[string]float64
	processedMu      sync.Mutex
	locallyProcessed map[string]float64

	clientPropagationReceived   int64
	clientPropagationServed     int64
	unpeeredPropagationIncoming int64
	unpeeredPropagationRxBytes  int64

	propMu             sync.Mutex
	validatedPeerLinks map[string]bool
	acceptedOfferLinks map[string]byte
	validatingFrom     map[string]float64
	throttledPeers     map[string]float64
	peerDistributionQ  []peerDistEntry

	linksMu      sync.Mutex
	directLinks  map[string]*link.Link
	propLinks    map[string]*link.Link
	controlLinks map[string]*link.Link

	deliveryHandler *deliveryAnnounceHandler
	propHandler     *propagationAnnounceHandler

	// tickets and stampCosts mirror upstream available_tickets and
	// outbound_stamp_costs, persisted under storagePath.
	tickets    *ticketBook
	stampCosts *stampCostBook

	// pathRequests debounces path requests like upstream
	// LXMRouter.request_path.
	prMu         sync.Mutex
	pathRequests *pathRequestTracker

	stop chan struct{}
	wg   sync.WaitGroup
}

type peerDistEntry struct {
	transientID []byte
	fromPeer    *Peer
}

// NewRouter creates a router bound to identity and transport. Call RegisterDelivery and optionally EnablePropagation before Start.
func NewRouter(id *identity.Identity, tr *transport.Transport, opts RouterOptions) (*Router, error) {
	if id == nil {
		return nil, errors.New("lxmf: nil identity")
	}
	if tr == nil {
		return nil, errors.New("lxmf: nil transport")
	}
	if opts.StoragePath == "" {
		return nil, errors.New("lxmf: storage path required")
	}
	cfg := opts.Config
	if cfg.LXMF.DisplayName == "" && cfg.Propagation.NodeName == "" {
		cfg = DefaultConfig()
		if opts.Config.LXMF.DisplayName != "" {
			cfg.LXMF.DisplayName = opts.Config.LXMF.DisplayName
		}
		if opts.Config.Propagation.NodeName != "" {
			cfg.Propagation.NodeName = opts.Config.Propagation.NodeName
		}
	}

	storagePath := filepath.Clean(opts.StoragePath)
	if err := os.MkdirAll(storagePath, 0o700); err != nil {
		return nil, fmt.Errorf("lxmf: storage path: %w", err)
	}

	onInbound := opts.OnInbound
	if onInbound == "" {
		onInbound = cfg.LXMF.OnInbound
	}

	r := &Router{
		identity:           id,
		transport:          tr,
		cfg:                cfg,
		storagePath:        storagePath,
		messagesDir:        opts.MessagesDir,
		onInbound:          onInbound,
		peers:              make(map[string]*Peer),
		staticPeers:        make(map[string]struct{}),
		controlAllow:       make(map[string]struct{}),
		ignoredList:        make(map[string]struct{}),
		allowedList:        make(map[string]struct{}),
		locallyDelivered:   make(map[string]float64),
		locallyProcessed:   make(map[string]float64),
		validatedPeerLinks: make(map[string]bool),
		acceptedOfferLinks: make(map[string]byte),
		tickets:            newTicketBook(storagePath),
		stampCosts:         newStampCostBook(storagePath),
		directLinks:        make(map[string]*link.Link),
		propLinks:          make(map[string]*link.Link),
		controlLinks:       make(map[string]*link.Link),
		validatingFrom:     make(map[string]float64),
		throttledPeers:     make(map[string]float64),
		pathRequests:       newPathRequestTracker(tr),
		stop:               make(chan struct{}),
	}

	for _, h := range cfg.Propagation.ControlAllowed {
		decoded, err := decodeHashHex(h)
		if err != nil {
			return nil, err
		}
		r.controlAllow[peerKey(decoded)] = struct{}{}
	}
	r.controlAllow[peerKey(id.Hash())] = struct{}{}

	staticHashes, err := cfg.Propagation.StaticPeerHashes()
	if err != nil {
		return nil, err
	}
	for _, h := range staticHashes {
		r.staticPeers[peerKey(h)] = struct{}{}
	}

	if cfg.Propagation.AuthRequired {
		for _, h := range cfg.Propagation.ControlAllowed {
			decoded, err := decodeHashHex(h)
			if err == nil {
				r.allowedList[peerKey(decoded)] = struct{}{}
			}
		}
	}

	r.deliveryHandler = &deliveryAnnounceHandler{router: r}
	r.propHandler = &propagationAnnounceHandler{router: r}
	tr.RegisterAnnounceHandler(r.deliveryHandler)
	tr.RegisterAnnounceHandler(r.propHandler)

	r.loadPersistedState()
	return r, nil
}

// RegisterDelivery registers the inbound lxmf.delivery destination and enables ratchets.
func (r *Router) RegisterDelivery(displayName string, stampCost *int) (*destination.Destination, error) {
	dest, err := NewDeliveryDestination(r.identity, r.transport)
	if err != nil {
		return nil, err
	}

	ratchetDir := filepath.Join(r.storagePath, "ratchets")
	if err := os.MkdirAll(ratchetDir, 0o700); err != nil {
		return nil, err
	}
	ratchetPath := filepath.Join(ratchetDir, peerKey(dest.GetHash())+".ratchets")
	dest.EnableRatchets(ratchetPath)

	dest.SetLinkEstablishedCallback(func(v any) {
		r.deliveryLinkEstablished(v)
	})

	if displayName == "" {
		displayName = r.cfg.LXMF.DisplayName
	}
	var nameElem, costElem any
	if displayName != "" {
		nameElem = []byte(displayName)
	}
	if stampCost != nil && *stampCost > 0 && *stampCost < 255 {
		costElem = int64(*stampCost)
	}
	appData, err := marshalAnnounceAppData([]any{nameElem, costElem, []any{int64(SFCompression)}})
	if err == nil {
		dest.SetDefaultAppData(appData)
	}

	r.mu.Lock()
	r.deliveryDest = dest
	r.inboundStampCost = stampCost
	r.mu.Unlock()

	r.transport.RegisterDestination(dest.GetHash(), r)
	return dest, nil
}

// EnablePropagation activates the propagation node, message store, and peer sync handlers.
func (r *Router) EnablePropagation() error {
	// Upstream clamps the per-sync limit to at least the per-transfer
	// limit so a single max size message always fits in one sync.
	if r.cfg.Propagation.PropagationSyncMaxAcceptedKB <= 0 ||
		r.cfg.Propagation.PropagationSyncMaxAcceptedKB < r.cfg.Propagation.PropagationTransferMaxAcceptedKB {
		r.cfg.Propagation.PropagationSyncMaxAcceptedKB = r.cfg.Propagation.PropagationTransferMaxAcceptedKB
	}

	propDest, err := destination.New(r.identity, destination.In, destination.Single, AppName, r.transport, "propagation")
	if err != nil {
		return err
	}

	prioritised, err := r.cfg.Propagation.PrioritisedHashes()
	if err != nil {
		return err
	}
	storeDir := filepath.Join(r.storagePath, "messagestore")
	store, err := NewMessageStore(storeDir, r.cfg.Propagation.MessageStorageLimitMB, prioritised)
	if err != nil {
		return err
	}

	appData, err := r.propagationAnnounceAppData()
	if err != nil {
		return err
	}
	propDest.SetDefaultAppData(appData)
	propDest.SetLinkEstablishedCallback(func(v any) {
		r.propagationLinkEstablished(v)
	})

	if err := propDest.RegisterRequestHandlerAny(PathOffer, r.offerRequestHandler, destination.AllowAll, nil); err != nil {
		return err
	}
	if err := propDest.RegisterRequestHandlerAny(PathMessageGet, r.messageGetRequestHandler, destination.AllowAll, nil); err != nil {
		return err
	}

	controlDest, err := destination.New(r.identity, destination.In, destination.Single, AppName, r.transport, "propagation", "control")
	if err != nil {
		return err
	}
	allowed := make([][]byte, 0, len(r.controlAllow))
	for k := range r.controlAllow {
		h, err := hexDecodeKey(k)
		if err == nil {
			allowed = append(allowed, h)
		}
	}
	if err := controlDest.RegisterRequestHandlerAny(PathStatsGet, r.statsGetRequestHandler, destination.AllowList, allowed); err != nil {
		return err
	}
	if err := controlDest.RegisterRequestHandlerAny(PathSyncRequest, r.peerSyncRequestHandler, destination.AllowList, allowed); err != nil {
		return err
	}
	if err := controlDest.RegisterRequestHandlerAny(PathUnpeerRequest, r.peerUnpeerRequestHandler, destination.AllowList, allowed); err != nil {
		return err
	}

	controlDest.SetLinkEstablishedCallback(func(v any) {
		lnk, ok := v.(*link.Link)
		if !ok || lnk == nil {
			return
		}
		key := hex.EncodeToString(lnk.GetLinkID())
		r.linksMu.Lock()
		r.controlLinks[key] = lnk
		r.linksMu.Unlock()
		lnk.SetLinkClosedCallback(func(closed *link.Link) {
			r.linksMu.Lock()
			delete(r.controlLinks, hex.EncodeToString(closed.GetLinkID()))
			r.linksMu.Unlock()
		})
	})

	r.mu.Lock()
	r.propagationDest = propDest
	r.controlDest = controlDest
	r.store = store
	r.propagationEnabled = true
	r.propagationStart = time.Now()
	r.cfg.Propagation.EnableNode = true
	r.mu.Unlock()

	// Peers persist per propagation node and require the message store
	// for handled and unhandled message bookkeeping, so they load here.
	r.loadPeers()

	r.transport.RegisterDestination(controlDest.GetHash(), controlDest)

	for key := range r.staticPeers {
		h, err := hexDecodeKey(key)
		if err != nil {
			continue
		}
		r.peerFromStatic(h)
	}

	if r.cfg.Propagation.AnnounceAtStart {
		go r.announcePropagationNode()
	}
	return nil
}

// Start launches background maintenance jobs.
func (r *Router) Start() {
	r.wg.Add(1)
	go r.jobLoop()
}

// Stop shuts down background jobs and persists state.
func (r *Router) Stop() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
	r.wg.Wait()
	r.saveLocallyDelivered()
	r.saveLocallyProcessed()
	r.saveNodeStats()
	r.savePeers()
}

// DeliveryDestination returns the registered delivery destination.
func (r *Router) DeliveryDestination() *destination.Destination {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.deliveryDest
}

// PropagationDestination returns the propagation destination when enabled.
func (r *Router) PropagationDestination() *destination.Destination {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.propagationDest
}

func (r *Router) isOwnPropagationHash(h []byte) bool {
	if len(h) != DestinationLength {
		return false
	}
	pd := r.PropagationDestination()
	if pd == nil {
		return false
	}
	return bytes.Equal(h, pd.GetHash())
}

// IgnoreDestination adds a source hash to the inbound ignore list.
func (r *Router) IgnoreDestination(hash []byte) {
	r.mu.Lock()
	r.ignoredList[peerKey(hash)] = struct{}{}
	r.mu.Unlock()
}

// AllowDestination adds an identity hash allowed when auth_required is set.
func (r *Router) AllowDestination(hash []byte) {
	r.mu.Lock()
	r.allowedList[peerKey(hash)] = struct{}{}
	r.mu.Unlock()
}

// SetInboundStampCost sets the required inbound delivery stamp cost,
// matching upstream set_inbound_stamp_cost: nil or a cost below 1
// clears the requirement, costs must be below 255.
func (r *Router) SetInboundStampCost(cost *int) {
	r.mu.Lock()
	if cost != nil && (*cost < 1 || *cost >= 255) {
		cost = nil
	}
	r.inboundStampCost = cost
	r.mu.Unlock()
}

// EnforceStamps enables inbound stamp enforcement.
func (r *Router) EnforceStamps(enabled bool) {
	r.mu.Lock()
	r.enforceStamps = enabled
	r.mu.Unlock()
}

func (r *Router) propagationAnnounceAppData() ([]byte, error) {
	nodeName := r.cfg.Propagation.NodeName
	if nodeName == "" {
		nodeName = "Anonymous Propagation Node"
	}
	transfer := int(r.cfg.Propagation.PropagationTransferMaxAcceptedKB)
	syncLimit := int(r.cfg.Propagation.PropagationSyncMaxAcceptedKB)
	// Upstream announces node_state = propagation_node and not
	// from_static_only. This only runs when the node is enabled.
	nodeState := !r.cfg.Propagation.FromStaticOnly
	return EncodePNAnnounceAppData(
		time.Now().Unix(),
		nodeState,
		transfer,
		syncLimit,
		r.cfg.Propagation.PropagationStampCostTarget,
		r.cfg.Propagation.PropagationStampCostFlexibility,
		r.cfg.Propagation.PeeringCost,
		nodeName,
	)
}

// requestPath emits a debounced path request for destHash, matching
// upstream LXMRouter.request_path. The tracker is created lazily so
// Router values built without NewRouter still work.
func (r *Router) requestPath(destHash []byte, recursive bool) bool {
	r.prMu.Lock()
	if r.pathRequests == nil {
		r.pathRequests = newPathRequestTracker(r.transport)
	}
	t := r.pathRequests
	r.prMu.Unlock()
	return t.requestPath(destHash, recursive)
}

func (r *Router) isStaticPeer(hash []byte) bool {
	_, ok := r.staticPeers[peerKey(hash)]
	return ok
}

func (r *Router) hasMessage(hash []byte) bool {
	key := peerKey(hash)
	r.deliveredMu.Lock()
	_, ok := r.locallyDelivered[key]
	r.deliveredMu.Unlock()
	return ok
}

func hexDecodeKey(key string) ([]byte, error) {
	return decodeHashHex(key)
}
