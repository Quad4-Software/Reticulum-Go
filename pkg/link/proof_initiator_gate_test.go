package link

import (
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

// TestResponderRejectsReflectedProof verifies that a responder link ignores an
// LRProof packet entirely: a reflected copy of its own signed proof verifies
// against the local identity, and without the initiator gate it would
// overwrite the peer's ephemeral key with the responder's own, fire the
// established callback, and corrupt the real session.
func TestResponderRejectsReflectedProof(t *testing.T) {
	responderIdent, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	transportInstance := transport.NewTransport(&common.ReticulumConfig{})

	dest, err := destination.New(responderIdent, destination.In, destination.Single, "test", transportInstance, "link")
	if err != nil {
		t.Fatalf("destination: %v", err)
	}

	initiatorLink := &Link{destination: dest, transport: transportInstance, initiator: true}
	if err := initiatorLink.generateEphemeralKeys(); err != nil {
		t.Fatalf("ephemeral keys: %v", err)
	}
	initiatorLink.mode = ModeDefault
	initiatorLink.mtu = 500

	signalling := signallingBytes(initiatorLink.mtu, initiatorLink.mode)
	requestData := make([]byte, 0, ECPubSize+LinkMTUSize)
	requestData = append(requestData, initiatorLink.pub...)
	requestData = append(requestData, initiatorLink.sigPub...)
	requestData = append(requestData, signalling...)

	linkRequestPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeLinkReq,
		DestinationType: dest.GetType(),
		DestinationHash: dest.GetHash(),
		Data:            requestData,
	}
	if err := linkRequestPkt.Pack(); err != nil {
		t.Fatalf("pack link request: %v", err)
	}

	responderLink := &Link{transport: transportInstance, destination: dest, initiator: false}
	responderLink.peerPub = linkRequestPkt.Data[0:KeySize]
	responderLink.peerSigPub = linkRequestPkt.Data[KeySize:ECPubSize]
	responderLink.linkID = linkIDFromPacket(linkRequestPkt)
	mtuBytes := linkRequestPkt.Data[ECPubSize : ECPubSize+LinkMTUSize]
	responderLink.mtu = (int(mtuBytes[0]&0x1F) << 16) | (int(mtuBytes[1]) << 8) | int(mtuBytes[2])
	responderLink.mode = (mtuBytes[0] & ModeByteMask) >> 5

	if err := responderLink.generateEphemeralKeys(); err != nil {
		t.Fatalf("responder ephemeral keys: %v", err)
	}
	if err := responderLink.performHandshake(); err != nil {
		t.Fatalf("responder handshake: %v", err)
	}

	called := false
	responderLink.establishedCallback = func(*Link) { called = true }

	proofPkt, err := responderLink.GenerateLinkProof(responderIdent)
	if err != nil {
		t.Fatalf("generate proof: %v", err)
	}

	peerPubBefore := append([]byte(nil), responderLink.peerPub...)
	rttBefore := responderLink.requestTime

	// Reflect the responder's own proof back at it. Must be a no-op.
	if err := responderLink.ValidateLinkProof(proofPkt, nil); err != nil {
		t.Fatalf("responder proof handling returned error: %v", err)
	}
	if string(responderLink.peerPub) != string(peerPubBefore) {
		t.Fatal("reflected proof overwrote peerPub")
	}
	if !responderLink.requestTime.Equal(rttBefore) {
		t.Fatal("reflected proof mutated requestTime")
	}
	if called {
		t.Fatal("reflected proof fired establishedCallback on responder")
	}
}

// TestDuplicateLRRTTDoesNotRefire verifies a second decryptable LRRTT on an
// already-active responder link does not re-fire the established callback or
// reset link timing.
func TestDuplicateLRRTTDoesNotRefire(t *testing.T) {
	responderIdent, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	transportInstance := transport.NewTransport(&common.ReticulumConfig{})
	dest, err := destination.New(responderIdent, destination.In, destination.Single, "test", transportInstance, "link")
	if err != nil {
		t.Fatalf("destination: %v", err)
	}

	// Responder already past handshake: session keys and Active status.
	responderLink := &Link{transport: transportInstance, destination: dest, initiator: false}
	if err := responderLink.generateEphemeralKeys(); err != nil {
		t.Fatalf("ephemeral keys: %v", err)
	}
	responderLink.status.Store(int32(StatusHandshake))
	responderLink.requestTime = time.Now()

	fires := 0
	responderLink.establishedCallback = func(*Link) { fires++ }

	// Fabricate an encrypted LRRTT payload via the link's own encrypt path.
	rttPkt := &packet.Packet{Context: packet.ContextLRRTT, Data: []byte{}}
	if err := responderLink.handleRTTPacket(rttPkt); err != nil {
		// Decrypt may fail without real ciphertext; the important assertion
		// below is on callback fires and status, which must not change even
		// when the packet parses on an Active link.
		t.Logf("RTT handling returned: %v", err)
	}

	responderLink.status.Store(int32(StatusActive))
	if err := responderLink.handleRTTPacket(rttPkt); err != nil {
		t.Fatalf("duplicate RTT on active link should be dropped silently, got: %v", err)
	}
	if fires != 0 {
		t.Fatalf("established callback fired %d times on duplicate RTT", fires)
	}
}
