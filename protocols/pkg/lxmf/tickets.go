// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// Ticket lifetimes mirror LXMessage ticket constants upstream.
const (
	TicketExpirySecs   = 21 * 24 * 60 * 60
	TicketGraceSecs    = 5 * 24 * 60 * 60
	TicketRenewSecs    = 14 * 24 * 60 * 60
	TicketIntervalSecs = 1 * 24 * 60 * 60

	stampCostExpirySecs = 45 * 24 * 60 * 60
)

type outboundTicketEntry struct {
	expires float64
	ticket  []byte
}

// ticketBook tracks issued and received LXMF tickets, matching upstream
// available_tickets with optional msgpack persistence.
type ticketBook struct {
	mu             sync.Mutex
	outbound       map[string]outboundTicketEntry
	inbound        map[string]map[string]float64
	lastDeliveries map[string]float64
	storagePath    string
}

func newTicketBook(storagePath string) *ticketBook {
	b := &ticketBook{
		outbound:       make(map[string]outboundTicketEntry),
		inbound:        make(map[string]map[string]float64),
		lastDeliveries: make(map[string]float64),
		storagePath:    storagePath,
	}
	b.load()
	return b
}

// generateTicket issues a ticket for destinationHash, matching upstream
// generate_ticket: returns [expires, ticket] or nil when a ticket was
// delivered recently enough that another is not needed yet.
func (b *ticketBook) generateTicket(destHash []byte, expiry float64) []any {
	if b == nil {
		return nil
	}
	now := float64(time.Now().Unix())
	key := peerKey(destHash)

	b.mu.Lock()
	defer b.mu.Unlock()

	if last, ok := b.lastDeliveries[key]; ok && now-last < TicketIntervalSecs {
		return nil
	}
	if tickets, ok := b.inbound[key]; ok {
		for ticketHex, expires := range tickets {
			if expires-now > TicketRenewSecs {
				ticket, err := hexDecodeKey(ticketHex)
				if err == nil {
					return []any{expires, ticket}
				}
			}
		}
	} else {
		b.inbound[key] = make(map[string]float64)
	}

	expires := now + expiry
	ticket := make([]byte, TicketLength)
	if _, err := rand.Read(ticket); err != nil {
		return nil
	}
	b.inbound[key][peerKey(ticket)] = expires
	b.saveLocked()
	return []any{expires, ticket}
}

// rememberTicket stores a ticket received in FIELD_TICKET, matching
// upstream remember_ticket.
func (b *ticketBook) rememberTicket(destHash []byte, expires float64, ticket []byte) {
	if b == nil || len(ticket) != TicketLength {
		return
	}
	b.mu.Lock()
	b.outbound[peerKey(destHash)] = outboundTicketEntry{
		expires: expires,
		ticket:  append([]byte(nil), ticket...),
	}
	b.mu.Unlock()
	b.save()
}

// outboundTicket returns a remembered ticket for destinationHash when it
// is still valid, matching upstream get_outbound_ticket.
func (b *ticketBook) outboundTicket(destHash []byte) []byte {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if entry, ok := b.outbound[peerKey(destHash)]; ok && entry.expires > float64(time.Now().Unix()) {
		return append([]byte(nil), entry.ticket...)
	}
	return nil
}

// outboundTicketExpiry returns the remembered ticket expiry, matching
// upstream get_outbound_ticket_expiry.
func (b *ticketBook) outboundTicketExpiry(destHash []byte) (float64, bool) {
	if b == nil {
		return 0, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if entry, ok := b.outbound[peerKey(destHash)]; ok && entry.expires > float64(time.Now().Unix()) {
		return entry.expires, true
	}
	return 0, false
}

// inboundTickets lists unexpired tickets issued to destinationHash,
// matching upstream get_inbound_tickets.
func (b *ticketBook) inboundTickets(destHash []byte) [][]byte {
	if b == nil {
		return nil
	}
	now := float64(time.Now().Unix())
	b.mu.Lock()
	defer b.mu.Unlock()
	tickets := b.inbound[peerKey(destHash)]
	out := make([][]byte, 0, len(tickets))
	for ticketHex, expires := range tickets {
		if now < expires {
			if ticket, err := hexDecodeKey(ticketHex); err == nil {
				out = append(out, ticket)
			}
		}
	}
	return out
}

// cleanAvailableTickets expires tickets, matching upstream
// clean_available_tickets: outbound tickets expire at expiry, inbound
// tickets at expiry plus the grace period.
func (b *ticketBook) clean() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cleanLocked()
}

func (b *ticketBook) cleanLocked() {
	now := float64(time.Now().Unix())
	for key, entry := range b.outbound {
		if now > entry.expires {
			delete(b.outbound, key)
		}
	}
	for key, tickets := range b.inbound {
		for ticketHex, expires := range tickets {
			if now > expires+TicketGraceSecs {
				delete(tickets, ticketHex)
			}
		}
		if len(tickets) == 0 {
			delete(b.inbound, key)
		}
	}
}

func (b *ticketBook) save() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.saveLocked()
}

func (b *ticketBook) saveLocked() {
	if b.storagePath == "" {
		return
	}
	if err := os.MkdirAll(b.storagePath, 0o700); err != nil {
		return
	}

	// Encode with raw bytes keys to match the upstream msgpack file
	// layout for available_tickets.
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	_ = enc.EncodeMapLen(3)

	_ = enc.EncodeString("outbound")
	_ = enc.EncodeMapLen(len(b.outbound))
	for key, entry := range b.outbound {
		dh, err := hexDecodeKey(key)
		if err != nil {
			continue
		}
		_ = enc.EncodeBytes(dh)
		_ = enc.Encode([]any{entry.expires, entry.ticket})
	}

	_ = enc.EncodeString("inbound")
	_ = enc.EncodeMapLen(len(b.inbound))
	for key, tickets := range b.inbound {
		dh, err := hexDecodeKey(key)
		if err != nil {
			continue
		}
		_ = enc.EncodeBytes(dh)
		_ = enc.EncodeMapLen(len(tickets))
		for ticketHex, expires := range tickets {
			ticket, terr := hexDecodeKey(ticketHex)
			if terr != nil {
				continue
			}
			_ = enc.EncodeBytes(ticket)
			_ = enc.Encode([]any{expires})
		}
	}

	_ = enc.EncodeString("last_deliveries")
	_ = enc.EncodeMapLen(len(b.lastDeliveries))
	for key, at := range b.lastDeliveries {
		if dh, err := hexDecodeKey(key); err == nil {
			_ = enc.EncodeBytes(dh)
			_ = enc.Encode(at)
		}
	}

	writePath := filepath.Join(b.storagePath, "available_tickets")
	tempPath := writePath + ".tmp"
	if err := os.WriteFile(tempPath, buf.Bytes(), 0o600); err == nil {
		_ = os.Rename(tempPath, writePath)
	}
}

func (b *ticketBook) load() {
	if b == nil || b.storagePath == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(b.storagePath, "available_tickets"))
	if err != nil {
		return
	}

	// Upstream keys these maps by raw destination hash bytes, so decode
	// manually to accept bin keys.
	dec := msgpack.NewDecoder(bytes.NewReader(data))
	sections, err := dec.DecodeMapLen()
	if err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for i := 0; i < sections; i++ {
		key, err := dec.DecodeString()
		if err != nil {
			return
		}
		switch key {
		case "outbound":
			b.loadOutboundTickets(dec)
		case "inbound":
			b.loadInboundTickets(dec)
		case "last_deliveries":
			b.loadLastDeliveries(dec)
		default:
			if _, err := dec.DecodeInterfaceLoose(); err != nil {
				return
			}
		}
	}
	b.cleanLocked()
}

func (b *ticketBook) loadOutboundTickets(dec *msgpack.Decoder) {
	n, err := dec.DecodeMapLen()
	if err != nil {
		return
	}
	for i := 0; i < n; i++ {
		dh, err := dec.DecodeBytes()
		if err != nil {
			return
		}
		entry, err := dec.DecodeInterface()
		if err != nil {
			return
		}
		list, ok := entry.([]any)
		if !ok || len(list) < 2 {
			continue
		}
		expires, _ := numericField(list[0])
		ticket, _ := list[1].([]byte)
		if len(ticket) == TicketLength {
			b.outbound[peerKey(dh)] = outboundTicketEntry{expires: expires, ticket: ticket}
		}
	}
}

func (b *ticketBook) loadInboundTickets(dec *msgpack.Decoder) {
	n, err := dec.DecodeMapLen()
	if err != nil {
		return
	}
	for i := 0; i < n; i++ {
		dh, err := dec.DecodeBytes()
		if err != nil {
			return
		}
		tn, err := dec.DecodeMapLen()
		if err != nil {
			return
		}
		for j := 0; j < tn; j++ {
			ticket, err := dec.DecodeBytes()
			if err != nil {
				return
			}
			entry, err := dec.DecodeInterface()
			if err != nil {
				return
			}
			list, ok := entry.([]any)
			if !ok || len(list) < 1 || len(ticket) != TicketLength {
				continue
			}
			expires, _ := numericField(list[0])
			if b.inbound[peerKey(dh)] == nil {
				b.inbound[peerKey(dh)] = make(map[string]float64)
			}
			b.inbound[peerKey(dh)][peerKey(ticket)] = expires
		}
	}
}

func (b *ticketBook) loadLastDeliveries(dec *msgpack.Decoder) {
	n, err := dec.DecodeMapLen()
	if err != nil {
		return
	}
	for i := 0; i < n; i++ {
		dh, err := dec.DecodeBytes()
		if err != nil {
			return
		}
		v, err := dec.DecodeInterface()
		if err != nil {
			return
		}
		if at, ok := numericField(v); ok {
			b.lastDeliveries[peerKey(dh)] = at
		}
	}
}

type stampCostEntry struct {
	at   float64
	cost int64
}

// stampCostBook tracks outbound stamp costs learned from announces,
// matching upstream outbound_stamp_costs with msgpack persistence.
type stampCostBook struct {
	mu          sync.Mutex
	costs       map[string]stampCostEntry
	storagePath string
}

func newStampCostBook(storagePath string) *stampCostBook {
	b := &stampCostBook{
		costs:       make(map[string]stampCostEntry),
		storagePath: storagePath,
	}
	b.load()
	return b
}

// update records a stamp cost announced by destinationHash, matching
// upstream update_stamp_cost.
func (b *stampCostBook) update(destHash []byte, cost int64, ok bool) {
	if b == nil || len(destHash) != DestinationLength || !ok {
		return
	}
	b.mu.Lock()
	b.costs[peerKey(destHash)] = stampCostEntry{at: float64(time.Now().Unix()), cost: cost}
	b.mu.Unlock()
	b.save()
}

// get returns the last announced stamp cost for destinationHash,
// matching upstream get_outbound_stamp_cost.
func (b *stampCostBook) get(destHash []byte) (int64, bool) {
	if b == nil {
		return 0, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	entry, ok := b.costs[peerKey(destHash)]
	return entry.cost, ok
}

// clean expires entries older than STAMP_COST_EXPIRY, matching upstream
// clean_outbound_stamp_costs.
func (b *stampCostBook) clean() {
	if b == nil {
		return
	}
	now := float64(time.Now().Unix())
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, entry := range b.costs {
		if now > entry.at+stampCostExpirySecs {
			delete(b.costs, key)
		}
	}
}

func (b *stampCostBook) save() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.saveLocked()
}

func (b *stampCostBook) saveLocked() {
	if b.storagePath == "" {
		return
	}
	if err := os.MkdirAll(b.storagePath, 0o700); err != nil {
		return
	}
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	_ = enc.EncodeMapLen(len(b.costs))
	for key, entry := range b.costs {
		if dh, err := hexDecodeKey(key); err == nil {
			_ = enc.EncodeBytes(dh)
			_ = enc.Encode([]any{entry.at, entry.cost})
		}
	}
	writePath := filepath.Join(b.storagePath, "outbound_stamp_costs")
	tempPath := writePath + ".tmp"
	if err := os.WriteFile(tempPath, buf.Bytes(), 0o600); err == nil {
		_ = os.Rename(tempPath, writePath)
	}
}

func (b *stampCostBook) load() {
	if b == nil || b.storagePath == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(b.storagePath, "outbound_stamp_costs"))
	if err != nil {
		return
	}

	dec := msgpack.NewDecoder(bytes.NewReader(data))
	n, err := dec.DecodeMapLen()
	if err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for i := 0; i < n; i++ {
		dh, err := dec.DecodeBytes()
		if err != nil {
			return
		}
		entry, err := dec.DecodeInterface()
		if err != nil {
			return
		}
		list, ok := entry.([]any)
		if !ok || len(list) < 2 || len(dh) != DestinationLength {
			continue
		}
		at, _ := numericField(list[0])
		cost, hasCost := numericField(list[1])
		if !hasCost {
			continue
		}
		b.costs[peerKey(dh)] = stampCostEntry{at: at, cost: int64(cost)}
	}
	b.cleanLocked()
}

func (b *stampCostBook) cleanLocked() {
	now := float64(time.Now().Unix())
	for key, entry := range b.costs {
		if now > entry.at+stampCostExpirySecs {
			delete(b.costs, key)
		}
	}
}

// rememberTicketEntry extracts and stores a FIELD_TICKET entry from an
// inbound message, matching upstream lxmf_delivery ticket bookkeeping.
func rememberTicketField(b *ticketBook, msg *LXMessage) {
	if b == nil || msg == nil || !msg.SignatureValidated {
		return
	}
	entry, ok := msg.Fields[FieldTicket]
	if !ok {
		return
	}
	list, ok := entry.([]any)
	if !ok || len(list) < 2 {
		return
	}
	expires, _ := numericField(list[0])
	ticket, _ := list[1].([]byte)
	if expires <= float64(time.Now().Unix()) || len(ticket) != TicketLength {
		return
	}
	b.rememberTicket(msg.SourceHash, expires, ticket)
}
