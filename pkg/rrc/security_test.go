// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
)

func TestSecurity_MarshalRejectsWrongVersion(t *testing.T) {
	sender := bytes.Repeat([]byte{0x01}, IdentityLength)
	env := mustEnvelope(t, TypePing, sender)
	env.Version = 99
	if _, err := env.Marshal(); !errors.Is(err, ErrWrongVersion) {
		t.Fatalf("err=%v", err)
	}
}

func TestSecurity_MarshalRejectsBadDestinationLength(t *testing.T) {
	sender := bytes.Repeat([]byte{0x02}, IdentityLength)
	env := mustEnvelope(t, TypeNotice, sender)
	env.Destination = []byte{1, 2, 3}
	env.HasDestination = true
	if _, err := env.Marshal(); !errors.Is(err, ErrBadFieldLength) {
		t.Fatalf("err=%v", err)
	}
}

func TestSecurity_UnmarshalRejectsWrongVersion(t *testing.T) {
	sender := bytes.Repeat([]byte{0x03}, IdentityLength)
	m := map[uint64]any{
		KeyVersion:   uint64(99),
		KeyType:      TypePing,
		KeyMsgID:     bytes.Repeat([]byte{1}, MessageIDLength),
		KeyTimestamp: uint64(1),
		KeySender:    sender,
	}
	raw, err := mustMarshalMap(m)
	if err != nil {
		t.Fatal(err)
	}
	_, err = UnmarshalEnvelope(raw)
	if !errors.Is(err, ErrWrongVersion) {
		t.Fatalf("err=%v", err)
	}
}

func TestSecurity_UnmarshalRejectsNonStringRoom(t *testing.T) {
	sender := bytes.Repeat([]byte{0x04}, IdentityLength)
	m := map[uint64]any{
		KeyVersion:   ProtocolVersion,
		KeyType:      TypeMsg,
		KeyMsgID:     bytes.Repeat([]byte{1}, MessageIDLength),
		KeyTimestamp: uint64(1),
		KeySender:    sender,
		KeyRoom:      uint64(1),
	}
	raw, err := mustMarshalMap(m)
	if err != nil {
		t.Fatal(err)
	}
	_, err = UnmarshalEnvelope(raw)
	if err == nil || !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("err=%v", err)
	}
}

func TestSecurity_NewEnvelopeRejectsShortSender(t *testing.T) {
	_, err := NewEnvelope(TypePing, []byte{1, 2, 3})
	if !errors.Is(err, ErrBadFieldLength) {
		t.Fatalf("err=%v", err)
	}
}

func TestSecurity_EnvelopeFromClonesSender(t *testing.T) {
	sender := bytes.Repeat([]byte{0x05}, IdentityLength)
	msgID := bytes.Repeat([]byte{0x06}, MessageIDLength)
	env, err := envelopeFrom(TypeMsg, sender, msgID, 1)
	if err != nil {
		t.Fatal(err)
	}
	sender[0] = 0xff
	if env.Sender[0] == 0xff {
		t.Fatal("sender alias")
	}
}

func TestSecurity_RateLimitTokenBucket(t *testing.T) {
	h := &Hub{cfg: HubConfig{Limits: HubLimits{RateLimitMsgsPerMinute: 4}}}
	h.cfg.applyDefaults()
	p := &hubPeer{tokens: 4, lastRefill: time.Now()}
	for i := range 4 {
		if !h.takeToken(p) {
			t.Fatalf("token %d", i)
		}
	}
	if h.takeToken(p) {
		t.Fatal("fifth message should be rate limited")
	}
	p.lastRefill = time.Now().Add(-61 * time.Second)
	if !h.takeToken(p) {
		t.Fatal("tokens should refill after one minute")
	}
}

func TestTakeToken_NeverExceedsCap(t *testing.T) {
	f := func(limit uint8, sleeps uint8) bool {
		cap := float64((int(limit) % 120) + 1)
		h := &Hub{cfg: HubConfig{Limits: HubLimits{RateLimitMsgsPerMinute: uint64(cap)}}}
		h.cfg.applyDefaults()
		p := &hubPeer{tokens: 0, lastRefill: time.Now()}
		for i := 0; i < int(sleeps%20)+1; i++ {
			p.lastRefill = p.lastRefill.Add(-2 * time.Second)
			_ = h.takeToken(p)
			if p.tokens > cap {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 80}); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeNick_StripsControls(t *testing.T) {
	cases := []string{"\x00", "\n\r", "\talice", "bob\x7f"}
	for _, in := range cases {
		out := SanitizeNick(in)
		if strings.ContainsAny(out, "\x00\n\r\x7f") {
			t.Fatalf("sanitize left controls: in=%q out=%q", in, out)
		}
	}
}

func TestValidateResourceEnvelopeBody_RejectsSHA256Type(t *testing.T) {
	body := map[uint64]any{
		ResourceKeyID:     []byte{1, 2, 3, 4},
		ResourceKeyKind:   ResourceKindBlob,
		ResourceKeySize:   uint64(1),
		ResourceKeySHA256: "not-bytes",
	}
	_, reason := ValidateResourceEnvelopeBody(body)
	if reason == "" || !strings.Contains(reason, "sha256") {
		t.Fatalf("reason=%q", reason)
	}
}

type denyPolicy struct {
	joinErr    error
	contentErr error
}

func (d *denyPolicy) OnLink(*link.Link)                          {}
func (d *denyPolicy) OnIdentified([]byte) error                  { return nil }
func (d *denyPolicy) AfterWelcome([]byte)                        {}
func (d *denyPolicy) AllowJoin([]byte, string, any) error        { return d.joinErr }
func (d *denyPolicy) AfterJoin([]byte, string)                   {}
func (d *denyPolicy) AfterPart([]byte, string)                   {}
func (d *denyPolicy) AllowContent([]byte, *Envelope) error       { return d.contentErr }
func (d *denyPolicy) Intercept([]byte, *Envelope) bool           { return false }
func (d *denyPolicy) OnPong([]byte)                              {}
func (d *denyPolicy) OnResourceEnvelope([]byte, *Envelope) error { return nil }
