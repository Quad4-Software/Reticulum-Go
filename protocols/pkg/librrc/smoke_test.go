// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/librrc"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func TestSmokeVersion(t *testing.T) {
	if librrc.Version() != "1.0" {
		t.Fatalf("version %q", librrc.Version())
	}
}

func TestSmokeEnvelopeRoundTrip(t *testing.T) {
	sender := make([]byte, rrc.IdentityLength)
	for i := range sender {
		sender[i] = byte(i)
	}
	h, code := librrc.EnvelopeCreate(rrc.TypeMsg, sender)
	if code != librrc.OK {
		t.Fatalf("create: %d", code)
	}
	defer librrc.EnvelopeDestroy(h)

	if code := librrc.EnvelopeSetRoom(h, "lobby"); code != librrc.OK {
		t.Fatalf("room: %d", code)
	}
	if code := librrc.EnvelopeSetBodyText(h, "hello"); code != librrc.OK {
		t.Fatalf("body: %d", code)
	}

	data, code := librrc.EnvelopeMarshal(h)
	if code != librrc.OK || len(data) == 0 {
		t.Fatalf("marshal: %d len=%d", code, len(data))
	}

	h2, code := librrc.EnvelopeUnmarshal(data)
	if code != librrc.OK {
		t.Fatalf("unmarshal: %d", code)
	}
	defer librrc.EnvelopeDestroy(h2)

	text, code := librrc.EnvelopeGetBodyText(h2)
	if code != librrc.OK || text != "hello" {
		t.Fatalf("body text %q code=%d", text, code)
	}
}

func TestSmokeNodeLifecycle(t *testing.T) {
	node, code := librrc.NodeCreate("")
	if code != librrc.OK {
		t.Fatalf("node create: %d", code)
	}
	defer librrc.NodeDestroy(node)

	if code := librrc.NodeStart(node); code != librrc.OK {
		t.Fatalf("start: %d", code)
	}
	if code := librrc.NodeStop(node); code != librrc.OK {
		t.Fatalf("stop: %d", code)
	}
}

func TestSmokeIdentity(t *testing.T) {
	id, code := librrc.IdentityGenerate()
	if code != librrc.OK {
		t.Fatalf("generate: %d", code)
	}
	defer librrc.IdentityDestroy(id)

	hash, code := librrc.IdentityHash(id)
	if code != librrc.OK || len(hash) != rrc.IdentityLength {
		t.Fatalf("hash len=%d code=%d", len(hash), code)
	}
}

func TestSmokeNormalizeSanitize(t *testing.T) {
	room, code := librrc.NormalizeRoom("  #Lobby ")
	if code != librrc.OK || room != "#lobby" {
		t.Fatalf("room %q code=%d", room, code)
	}
	nick, code := librrc.SanitizeNick(" alice ")
	if code != librrc.OK || nick != "alice" {
		t.Fatalf("nick %q code=%d", nick, code)
	}
}
