// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"testing"
	"time"
)

func TestTicketBookGenerateAndRedeem(t *testing.T) {
	b := newTicketBook("")
	dest := bytes.Repeat([]byte{0xaa}, DestinationLength)

	entry := b.generateTicket(dest, TicketExpirySecs)
	if entry == nil || len(entry) != 2 {
		t.Fatalf("generateTicket returned %#v", entry)
	}
	expires, _ := numericField(entry[0])
	ticket, _ := entry[1].([]byte)
	if len(ticket) != TicketLength {
		t.Fatalf("ticket length %d, want %d", len(ticket), TicketLength)
	}
	if expires <= float64(time.Now().Unix()) {
		t.Fatalf("ticket expiry %f is not in the future", expires)
	}

	// A second call reuses the ticket while enough validity remains.
	again := b.generateTicket(dest, TicketExpirySecs)
	if again == nil {
		t.Fatal("generateTicket did not reuse the issued ticket")
	}
	reused, _ := again[1].([]byte)
	if !bytes.Equal(reused, ticket) {
		t.Fatal("generateTicket issued a new ticket instead of reusing")
	}

	// inboundTickets lists the unexpired ticket for validation.
	tickets := b.inboundTickets(dest)
	if len(tickets) != 1 || !bytes.Equal(tickets[0], ticket) {
		t.Fatalf("inboundTickets = %d entries, want the issued ticket", len(tickets))
	}
}

func TestTicketBookRememberAndRedeemOutbound(t *testing.T) {
	b := newTicketBook("")
	dest := bytes.Repeat([]byte{0xbb}, DestinationLength)
	ticket := bytes.Repeat([]byte{0xcc}, TicketLength)

	b.rememberTicket(dest, float64(time.Now().Unix())+3600, ticket)
	got := b.outboundTicket(dest)
	if !bytes.Equal(got, ticket) {
		t.Fatalf("outboundTicket = %x, want %x", got, ticket)
	}
	exp, ok := b.outboundTicketExpiry(dest)
	if !ok || exp <= float64(time.Now().Unix()) {
		t.Fatalf("outboundTicketExpiry = %f, %v", exp, ok)
	}

	// Expired tickets are not redeemed.
	b.rememberTicket(dest, float64(time.Now().Unix())-10, ticket)
	if b.outboundTicket(dest) != nil {
		t.Fatal("outboundTicket returned an expired ticket")
	}
}

func TestTicketBookDeliveryPacing(t *testing.T) {
	b := newTicketBook("")
	dest := bytes.Repeat([]byte{0xdd}, DestinationLength)

	b.mu.Lock()
	b.lastDeliveries[peerKey(dest)] = float64(time.Now().Unix())
	b.mu.Unlock()
	if entry := b.generateTicket(dest, TicketExpirySecs); entry != nil {
		t.Fatal("generateTicket issued a ticket inside the delivery interval")
	}
}

func TestTicketBookPersistence(t *testing.T) {
	dir := t.TempDir()
	dest := bytes.Repeat([]byte{0xee}, DestinationLength)

	b := newTicketBook(dir)
	b.rememberTicket(dest, float64(time.Now().Unix())+3600, bytes.Repeat([]byte{0x11}, TicketLength))
	entry := b.generateTicket(dest, TicketExpirySecs)
	if entry == nil {
		t.Fatal("generateTicket returned nil")
	}

	reloaded := newTicketBook(dir)
	got := reloaded.outboundTicket(dest)
	if !bytes.Equal(got, bytes.Repeat([]byte{0x11}, TicketLength)) {
		t.Fatalf("reloaded outboundTicket = %x", got)
	}
	tickets := reloaded.inboundTickets(dest)
	if len(tickets) != 1 {
		t.Fatalf("reloaded inboundTickets = %d entries", len(tickets))
	}
}

func TestStampCostBookUpdateGetPersist(t *testing.T) {
	dir := t.TempDir()
	dest := bytes.Repeat([]byte{0x22}, DestinationLength)

	b := newStampCostBook(dir)
	if _, ok := b.get(dest); ok {
		t.Fatal("unexpected stamp cost before any announce")
	}
	b.update(dest, 8, true)
	if cost, ok := b.get(dest); !ok || cost != 8 {
		t.Fatalf("stamp cost = %d, %v, want 8", cost, ok)
	}

	reloaded := newStampCostBook(dir)
	if cost, ok := reloaded.get(dest); !ok || cost != 8 {
		t.Fatalf("reloaded stamp cost = %d, %v, want 8", cost, ok)
	}
}

func TestRememberTicketField(t *testing.T) {
	b := newTicketBook("")
	src := bytes.Repeat([]byte{0x33}, DestinationLength)
	ticket := bytes.Repeat([]byte{0x44}, TicketLength)
	expires := float64(time.Now().Unix()) + 3600

	msg := &LXMessage{
		SourceHash:         src,
		SignatureValidated: true,
		Fields:             map[byte]any{FieldTicket: []any{expires, ticket}},
	}
	rememberTicketField(b, msg)
	if !bytes.Equal(b.outboundTicket(src), ticket) {
		t.Fatal("ticket field was not remembered")
	}

	// Unvalidated signatures must not register tickets.
	b2 := newTicketBook("")
	msg.SignatureValidated = false
	rememberTicketField(b2, msg)
	if b2.outboundTicket(src) != nil {
		t.Fatal("unvalidated message registered a ticket")
	}

	// Expired ticket entries are ignored.
	b3 := newTicketBook("")
	msg.SignatureValidated = true
	msg.Fields[FieldTicket] = []any{float64(time.Now().Unix()) - 5, ticket}
	rememberTicketField(b3, msg)
	if b3.outboundTicket(src) != nil {
		t.Fatal("expired ticket entry was remembered")
	}
}

func TestMessengerOutboundTicketStamp(t *testing.T) {
	m := &Messenger{tickets: newTicketBook(""), stampCosts: newStampCostBook("")}
	dest := bytes.Repeat([]byte{0x55}, DestinationLength)
	ticket := bytes.Repeat([]byte{0x66}, TicketLength)
	cost := 16

	m.tickets.rememberTicket(dest, float64(time.Now().Unix())+3600, ticket)
	msg := &LXMessage{
		DestinationHash: dest,
		Hash:            bytes.Repeat([]byte{0x77}, 32),
		StampCost:       &cost,
	}

	m.prepareOutbound(msg)
	if !bytes.Equal(msg.OutboundTicket, ticket) {
		t.Fatal("outbound ticket was not attached")
	}
	stamped, err := m.stampOutbound(t.Context(), msg)
	if err != nil || !stamped {
		t.Fatalf("stampOutbound = %v, %v", stamped, err)
	}
	want := truncatedHash(ticket, msg.Hash)
	if !bytes.Equal(msg.Stamp, want) {
		t.Fatal("ticket stamp does not match truncatedHash(ticket+messageID)")
	}
	if msg.StampValue != StampValueTicket {
		t.Fatalf("stamp value = %d, want %d", msg.StampValue, StampValueTicket)
	}
}
