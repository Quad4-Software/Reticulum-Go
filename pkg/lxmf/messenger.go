// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/link"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

// MessageHandler receives one unpacked inbound LXMessage per packet (signature state is on msg).
type MessageHandler func(msg *LXMessage, iface common.NetworkInterface)

// Messenger sends and receives LXMF over a Transport. The destination must be inbound Single.
type Messenger struct {
	transport *transport.Transport
	dest      *destination.Destination

	mu        sync.RWMutex
	handler   MessageHandler
	resolver  SourceResolver
	onRecvErr func(error)

	propLinkMu   sync.Mutex
	propLink     *link.Link
	propLinkNode []byte

	deliveryLinkMu   sync.Mutex
	deliveryLink     *link.Link
	deliveryLinkPeer []byte

	// backchannelLinks maps remote delivery destination hashes to links
	// those remotes established inbound, matching upstream
	// backchannel_links for outbound delivery reuse.
	backchannelLinks map[string]*link.Link

	// deliveredTIDs tracks transient ids of locally delivered messages,
	// matching upstream locally_delivered_transient_ids.
	deliveredMu   sync.Mutex
	deliveredTIDs map[string]float64

	// tickets mirrors upstream available_tickets and stampCosts mirrors
	// outbound_stamp_costs learned from announces.
	tickets    *ticketBook
	stampCosts *stampCostBook

	inboundStampCost *int
	enforceStamps    bool

	// retainSyncedOnNode mirrors upstream retain_synced_on_node. When
	// set, already-held messages are not reported back to the node, so
	// the node keeps them for other clients.
	retainSyncedOnNode bool

	// pathRequests debounces path requests like upstream
	// LXMRouter.request_path.
	prMu         sync.Mutex
	pathRequests *pathRequestTracker
}

// NewMessenger registers d's packet callback for inbound LXMF. Use NewDeliveryDestination for lxmf.delivery naming.
func NewMessenger(t *transport.Transport, d *destination.Destination) *Messenger {
	m := &Messenger{
		transport:        t,
		dest:             d,
		resolver:         RecallSource,
		backchannelLinks: make(map[string]*link.Link),
		deliveredTIDs:    make(map[string]float64),
		tickets:          newTicketBook(""),
		stampCosts:       newStampCostBook(""),
		pathRequests:     newPathRequestTracker(t),
	}
	d.SetPacketCallback(m.onPacket)
	d.SetLinkEstablishedCallback(m.onLinkEstablished)
	t.RegisterDestination(d.GetHash(), m)
	t.RegisterAnnounceHandler(&messengerAnnounceHandler{m: m})
	return m
}

// requestPath emits a debounced path request for destHash, matching
// upstream LXMRouter.request_path. The tracker is created lazily so
// Messenger values built without NewMessenger still work.
func (m *Messenger) requestPath(destHash []byte, recursive bool) bool {
	m.prMu.Lock()
	if m.pathRequests == nil {
		m.pathRequests = newPathRequestTracker(m.transport)
	}
	t := m.pathRequests
	m.prMu.Unlock()
	return t.requestPath(destHash, recursive)
}

// pathRequestWait returns the upstream path_request_wait bound used to
// size path waits on slow mediums.
func (m *Messenger) pathRequestWait() time.Duration {
	m.prMu.Lock()
	t := m.pathRequests
	m.prMu.Unlock()
	if t == nil {
		return PathRequestWait
	}
	return t.pathRequestWait()
}

// messengerAnnounceHandler records announced stamp costs for outbound
// delivery, matching upstream LXMFDeliveryAnnounceHandler.
type messengerAnnounceHandler struct {
	m *Messenger
}

func (h *messengerAnnounceHandler) AspectFilter() []string {
	return []string{AppName + ".delivery"}
}

func (h *messengerAnnounceHandler) ReceivePathResponses() bool { return true }

func (h *messengerAnnounceHandler) ReceivedAnnounce(destHash []byte, identAny any, appData []byte, hops uint8) error {
	_ = identAny
	_ = hops
	if h.m == nil {
		return nil
	}
	cost, ok, err := StampCostFromAppData(appData)
	if err != nil {
		return nil
	}
	h.m.stampCosts.update(destHash, cost, ok)
	return nil
}

// SetStoragePath enables persistence of available tickets and outbound
// stamp costs under path, matching upstream storage files.
func (m *Messenger) SetStoragePath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickets = newTicketBook(path)
	m.stampCosts = newStampCostBook(path)
}

// SetRetainSyncedOnNode mirrors upstream set_retain_node_lxms. When
// enabled, FetchPropagated does not report already-held messages back to
// the propagation node, so they stay stored there.
func (m *Messenger) SetRetainSyncedOnNode(enabled bool) {
	m.mu.Lock()
	m.retainSyncedOnNode = enabled
	m.mu.Unlock()
}

// SetInboundStampCost requires inbound messages to carry a stamp of the
// given cost, matching upstream set_inbound_stamp_cost. Nil disables.
func (m *Messenger) SetInboundStampCost(cost *int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cost != nil && (*cost < 1 || *cost >= 255) {
		cost = nil
	}
	m.inboundStampCost = cost
}

// EnforceStamps drops inbound messages whose stamp does not meet the
// configured cost, matching upstream enforce_stamps.
func (m *Messenger) EnforceStamps(enabled bool) {
	m.mu.Lock()
	m.enforceStamps = enabled
	m.mu.Unlock()
}

// onLinkEstablished wires inbound direct-delivery links: link packets carry
// the packed LXMF payload and resources are accepted up to a sane bound.
func (m *Messenger) onLinkEstablished(v any) {
	lnk, ok := v.(*link.Link)
	if !ok || lnk == nil {
		return
	}
	m.wireDeliveryLink(lnk)
}

// wireDeliveryLink installs the delivery callbacks on a link and registers
// remote identifications as backchannels, matching upstream
// delivery_link_established and delivery_remote_identified.
func (m *Messenger) wireDeliveryLink(lnk *link.Link) {
	_ = lnk.SetResourceStrategy(link.AcceptApp)
	lnk.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		m.noteDelivered(data)
		m.onPacket(m.directInner(data), nil)
	})
	lnk.SetResourceCallback(func(adv any) bool {
		// Upstream accepts delivery resources up to the per-transfer
		// limit, delivery_per_transfer_limit * 1000 bytes.
		return resourceAdvertisedSize(adv) <= int64(DeliveryLimitKBDefault)*1000
	})
	lnk.SetResourceConcludedCallback(func(res any) {
		if data := extractResourceData(res); len(data) > 0 {
			m.noteDelivered(data)
			m.onPacket(m.directInner(data), nil)
		}
	})
	lnk.SetRemoteIdentifiedCallback(func(l *link.Link, id *identity.Identity) {
		if id == nil {
			return
		}
		dh, err := deliveryDestinationHash(id)
		if err != nil {
			return
		}
		m.deliveryLinkMu.Lock()
		m.backchannelLinks[hex.EncodeToString(dh)] = l
		m.deliveryLinkMu.Unlock()
	})
	lnk.SetLinkClosedCallback(func(closed *link.Link) {
		m.deliveryLinkMu.Lock()
		for key, l := range m.backchannelLinks {
			if l == closed {
				delete(m.backchannelLinks, key)
			}
		}
		if m.deliveryLink == closed {
			m.deliveryLink = nil
			m.deliveryLinkPeer = nil
		}
		m.deliveryLinkMu.Unlock()
	})
}

// noteDelivered records the transient id of a delivered wire payload,
// which is the destination hash followed by the encrypted inner data.
func (m *Messenger) noteDelivered(lxmfData []byte) {
	if len(lxmfData) < DestinationLength {
		return
	}
	sum := sha256.Sum256(lxmfData)
	now := float64(time.Now().Unix())
	m.deliveredMu.Lock()
	m.deliveredTIDs[hex.EncodeToString(sum[:])] = now
	// Transient ids are kept for MESSAGE_EXPIRY*6 upstream.
	for k, ts := range m.deliveredTIDs {
		if now-ts > messageExpirySeconds*6 {
			delete(m.deliveredTIDs, k)
		}
	}
	m.deliveredMu.Unlock()
}

// hasDelivered reports whether the transient id was already delivered,
// matching upstream has_message.
func (m *Messenger) hasDelivered(transientID []byte) bool {
	m.deliveredMu.Lock()
	defer m.deliveredMu.Unlock()
	_, ok := m.deliveredTIDs[hex.EncodeToString(transientID)]
	return ok
}

// directInner strips the destination hash prefix from link-delivered LXMF
// payloads. Direct deliveries carry the full packed message on the wire,
// while onPacket expects the inner source, signature, and payload.
func (m *Messenger) directInner(data []byte) []byte {
	if len(data) <= DestinationLength {
		return nil
	}
	if !bytes.Equal(data[:DestinationLength], m.DestinationHash()) {
		return nil
	}
	return data[DestinationLength:]
}

// NewDeliveryDestination returns the inbound lxmf.delivery destination for id.
// ProveAll matches upstream delivery_packet, which proves every inbound link
// packet so senders can mark direct deliveries delivered.
func NewDeliveryDestination(id *identity.Identity, t *transport.Transport) (*destination.Destination, error) {
	dest, err := destination.New(id, destination.In, destination.Single, AppName, t, "delivery")
	if err != nil {
		return nil, err
	}
	dest.SetProofStrategy(destination.ProveAll)
	return dest, nil
}

// NewDeliveryMessenger is NewDeliveryDestination plus NewMessenger.
func NewDeliveryMessenger(id *identity.Identity, t *transport.Transport) (*Messenger, error) {
	dest, err := NewDeliveryDestination(id, t)
	if err != nil {
		return nil, err
	}
	return NewMessenger(t, dest), nil
}

// Destination returns the local RNS destination.
func (m *Messenger) Destination() *destination.Destination {
	return m.dest
}

// DestinationHash returns the local destination hash.
func (m *Messenger) DestinationHash() []byte {
	return m.dest.GetHash()
}

// SetMessageHandler sets the inbound callback; nil disables delivery.
func (m *Messenger) SetMessageHandler(h MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handler = h
}

// SetSourceResolver sets signature verification lookup; default is RecallSource.
func (m *Messenger) SetSourceResolver(r SourceResolver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r == nil {
		m.resolver = RecallSource
		return
	}
	m.resolver = r
}

func (m *Messenger) SetReceiveError(fn func(error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRecvErr = fn
}

// Compose builds an outbound message from this destination as source.
func (m *Messenger) Compose(destinationHash []byte, title, content string, fields map[byte]any) (*LXMessage, error) {
	return NewMessage(destinationHash, m.DestinationHash(), []byte(title), []byte(content), fields)
}

// prepareOutbound applies upstream handle_outbound pre-pack steps: auto
// stamp cost from announces, outbound ticket redemption, and ticket
// inclusion for the destination.
func (m *Messenger) prepareOutbound(msg *LXMessage) {
	if msg.StampCost == nil {
		if cost, ok := m.stampCosts.get(msg.DestinationHash); ok && cost > 0 {
			c := int(cost)
			msg.StampCost = &c
		}
	}
	if len(msg.OutboundTicket) == 0 {
		msg.OutboundTicket = m.tickets.outboundTicket(msg.DestinationHash)
	}
	if msg.IncludeTicket {
		if ticket := m.tickets.generateTicket(msg.DestinationHash, TicketExpirySecs); ticket != nil {
			msg.SetField(FieldTicket, ticket)
		}
	}
}

// stampOutbound applies the upstream get_stamp order: an outbound ticket
// short-circuits PoW, an existing stamp is reused, and a required stamp
// cost triggers PoW generation. Returns true when a stamp was set.
func (m *Messenger) stampOutbound(ctx context.Context, msg *LXMessage) (bool, error) {
	if len(msg.OutboundTicket) == TicketLength {
		msg.Stamp = truncatedHash(msg.OutboundTicket, msg.Hash)
		msg.StampValue = StampValueTicket
		msg.StampValid = true
		return true, nil
	}
	if len(msg.Stamp) > 0 || msg.StampCost == nil || *msg.StampCost <= 0 {
		return false, nil
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
	}
	Info("generating lxmf delivery stamp", "cost", *msg.StampCost)
	stamp, value, err := GenerateStamp(ctx, msg.Hash, *msg.StampCost, WorkblockExpandRounds)
	if err != nil {
		return false, fmt.Errorf("stamp generation: %w", err)
	}
	Verbose("lxmf delivery stamp ready", "value", value)
	msg.Stamp = stamp
	msg.StampValue = value
	msg.StampValid = true
	return true, nil
}

// Send packs, signs, stamps when required, and sends one opportunistic
// encrypted packet. The peer must be in identity.Recall.
func (m *Messenger) Send(msg *LXMessage) error {
	return m.send(context.Background(), msg)
}

func (m *Messenger) send(ctx context.Context, msg *LXMessage) error {
	if msg == nil {
		return errors.New("lxmf: nil message")
	}
	if len(msg.DestinationHash) != DestinationLength {
		return fmt.Errorf("destination: %w", ErrInvalidHashLength)
	}

	remoteIdentity, err := identity.Recall(msg.DestinationHash)
	if err != nil {
		return fmt.Errorf("destination identity not found: %w", err)
	}
	if remoteIdentity == nil {
		return ErrDestinationUnknown
	}

	target, err := destination.FromHash(msg.DestinationHash, remoteIdentity, destination.Single, m.transport)
	if err != nil {
		return fmt.Errorf("create target destination: %w", err)
	}

	signer := m.dest.GetIdentity()
	if signer == nil {
		return errors.New("lxmf: local destination has no identity")
	}

	m.prepareOutbound(msg)

	if _, err := msg.Pack(signer); err != nil {
		return err
	}
	stamped, err := m.stampOutbound(ctx, msg)
	if err != nil {
		return err
	}
	if stamped {
		if _, err := msg.Pack(signer); err != nil {
			return err
		}
	}

	innerPayload, err := msg.EncryptedPayload()
	if err != nil {
		return err
	}

	encrypted, err := target.Encrypt(innerPayload)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	pkt := packet.NewPacket(
		packet.DestinationSingle,
		encrypted,
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		true,
		packet.FlagUnset,
	)
	pkt.DestinationHash = append([]byte(nil), msg.DestinationHash...)

	// Upstream handle_outbound pre-emptively requests the path for an
	// opportunistic message when it is unknown, then waits
	// path_request_wait before delivery attempts. SendPacket needs a
	// resolved path, so wait for the response here.
	if !m.transport.HasPath(msg.DestinationHash) {
		m.requestPath(msg.DestinationHash, true)
		deadline := time.Now().Add(m.pathRequestWait())
		for !m.transport.HasPath(msg.DestinationHash) && time.Now().Before(deadline) {
			time.Sleep(pathPollInterval)
		}
	}

	if err := pkt.Pack(); err != nil {
		return fmt.Errorf("packet packing failed: %w", err)
	}

	if err := m.transport.SendPacket(pkt); err != nil {
		return fmt.Errorf("packet sending failed: %w", err)
	}

	msg.Method = MethodOpportunistic
	msg.Representation = RepresentationPacket
	msg.State = StateSent
	return nil
}

// SendText composes and sends a text-only message.
func (m *Messenger) SendText(destinationHash []byte, title, content string) (*LXMessage, error) {
	msg, err := m.Compose(destinationHash, title, content, nil)
	if err != nil {
		return nil, err
	}
	if err := m.Send(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// SendStamped packs the message, generates a PoW stamp for stampCost, and sends opportunistically.
func (m *Messenger) SendStamped(msg *LXMessage, stampCost int) error {
	return m.SendStampedContext(context.Background(), msg, stampCost)
}

func (m *Messenger) SendStampedContext(ctx context.Context, msg *LXMessage, stampCost int) error {
	if msg == nil {
		return errors.New("lxmf: nil message")
	}
	if stampCost > 0 {
		msg.StampCost = &stampCost
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return m.send(ctx, msg)
}

// SendPropagated uploads a packed message to propNodeHash via an RNS link.
func (m *Messenger) SendPropagated(msg *LXMessage, propNodeHash []byte, pnStampCost int) error {
	if msg == nil {
		return errors.New("lxmf: nil message")
	}
	signer := m.dest.GetIdentity()
	if signer == nil {
		return errors.New("lxmf: local destination has no identity")
	}
	m.prepareOutbound(msg)
	if _, err := msg.Pack(signer); err != nil {
		return err
	}
	stamped, err := m.stampOutbound(context.Background(), msg)
	if err != nil {
		return err
	}
	if stamped {
		if _, err := msg.Pack(signer); err != nil {
			return fmt.Errorf("re-pack with stamp: %w", err)
		}
	}
	if err := m.packForPropagation(msg, pnStampCost); err != nil {
		return err
	}

	lnk, err := m.ensurePropagationLink(propNodeHash)
	if err != nil {
		return err
	}
	if err := m.sendPropagationPayload(lnk, msg.PropagationPacked); err != nil {
		return err
	}

	msg.Method = MethodPropagated
	msg.Representation = RepresentationPacket
	if len(msg.PropagationPacked) > LinkPacketMaxContent {
		msg.Representation = RepresentationResource
	}
	msg.State = StateSent
	return nil
}

// SendStampedPropagated is SendPropagated with a required delivery stamp cost.
func (m *Messenger) SendStampedPropagated(msg *LXMessage, propNodeHash []byte, stampCost, pnStampCost int) error {
	if msg == nil {
		return errors.New("lxmf: nil message")
	}
	if stampCost > 0 {
		msg.StampCost = &stampCost
	}
	return m.SendPropagated(msg, propNodeHash, pnStampCost)
}

// Receive implements inbound delivery for the lxmf.delivery destination, decrypts,
// sends a delivery proof to the sender, and dispatches the LXMF payload.
func (m *Messenger) Receive(pkt *packet.Packet, iface common.NetworkInterface) bool {
	if pkt == nil {
		return false
	}
	if pkt.PacketType == packet.PacketTypeLinkReq {
		return m.dest.Receive(pkt, iface)
	}

	plaintext, err := m.decryptInbound(pkt.Data)
	if err != nil {
		Warning("inbound lxmf decrypt failed", "error", err, "packet_len", len(pkt.Data))
		m.receiveError(fmt.Errorf("decrypt: %w", err))
		return false
	}

	if err := sendDeliveryProof(m.dest, pkt, iface); err != nil {
		Warning("inbound lxmf proof failed", "error", err)
	}

	// The wire form is the destination hash followed by the ciphertext.
	lxmfData := append(append([]byte(nil), m.DestinationHash()...), pkt.Data...)
	m.noteDelivered(lxmfData)

	m.onPacket(plaintext, iface)
	return true
}

// HandleIncomingLinkRequest delegates link requests to the wrapped
// destination. The messenger registers itself as the transport destination,
// so without this inbound link requests for direct delivery are dropped.
func (m *Messenger) HandleIncomingLinkRequest(pkt any, tr any, iface common.NetworkInterface) error {
	return m.dest.HandleIncomingLinkRequest(pkt, tr, iface)
}

// EnableRatchets enables destination ratchet keys for inbound decryption and persistence.
func (m *Messenger) EnableRatchets(path string) bool {
	return m.dest.EnableRatchets(path)
}

func (m *Messenger) decryptInbound(ciphertext []byte) ([]byte, error) {
	id := m.dest.GetIdentity()
	if id == nil {
		return nil, errors.New("no identity available for decryption")
	}

	ratchets := m.dest.GetRatchets()
	if idRatchets := id.GetRatchets(); len(idRatchets) > 0 {
		ratchets = append(ratchets, idRatchets...)
	}

	receiver := &common.RatchetIDReceiver{}
	return id.Decrypt(ciphertext, ratchets, false, receiver)
}

func (m *Messenger) onPacket(plaintext []byte, iface common.NetworkInterface) {
	m.mu.RLock()
	handler := m.handler
	resolver := m.resolver
	m.mu.RUnlock()

	if len(plaintext) < DestinationLength+SignatureLength {
		m.receiveError(fmt.Errorf("inbound: %w", ErrMessageTooShort))
		return
	}

	msg, err := UnpackFromBytes(m.DestinationHash(), plaintext, resolver)
	if err != nil && msg == nil {
		Warning("inbound lxmf unpack failed", "error", err, "plaintext_len", len(plaintext))
		m.receiveError(fmt.Errorf("unpack: %w", err))
		return
	}
	if err != nil {
		Debug("inbound lxmf unpack completed with error", "error", err,
			"signature_validated", msg.SignatureValidated, "unverified_reason", msg.UnverifiedReason)
	}

	// Upstream lxmf_delivery remembers inbound FIELD_TICKET entries and
	// enforces a required stamp cost when one is configured.
	rememberTicketField(m.tickets, msg)

	m.mu.RLock()
	stampCost := m.inboundStampCost
	enforce := m.enforceStamps
	m.mu.RUnlock()
	if stampCost != nil && *stampCost > 0 {
		ok, verr := msg.ValidateStamp(*stampCost, m.tickets.inboundTickets(msg.SourceHash))
		if verr == nil {
			msg.StampValid = ok
		}
		if !msg.StampValid && enforce {
			Warning("dropping message with invalid stamp", "hash", hex.EncodeToString(msg.Hash))
			return
		}
	}

	// Upstream drops already delivered messages by inner hash and
	// records it in locally_delivered_transient_ids on delivery.
	if len(msg.Hash) > 0 {
		hashKey := hex.EncodeToString(msg.Hash)
		m.deliveredMu.Lock()
		_, dup := m.deliveredTIDs[hashKey]
		if !dup {
			m.deliveredTIDs[hashKey] = float64(time.Now().Unix())
		}
		m.deliveredMu.Unlock()
		if dup {
			return
		}
	}

	if handler != nil {
		handler(msg, iface)
	}
}

func (m *Messenger) receiveError(err error) {
	if err == nil {
		return
	}
	m.mu.RLock()
	fn := m.onRecvErr
	m.mu.RUnlock()
	if fn != nil {
		fn(err)
	}
}
