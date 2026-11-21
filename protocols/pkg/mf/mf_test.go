package mf

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

const (
	testMessageText = "Hello, Reticulum!"
	errFmtHex       = "expected hex hash %s, got %s"
)

func TestMessage_PackUnpack(t *testing.T) {
	senderHash, _ := hex.DecodeString(testHashHex)
	text := testMessageText

	msg := &Message{
		SenderHash: senderHash,
		Text:       text,
	}

	packed, err := msg.Pack()
	if err != nil {
		t.Fatalf("Pack failed: %v", err)
	}

	if len(packed) != SenderHashLength+len(text) {
		t.Errorf("expected length %d, got %d", SenderHashLength+len(text), len(packed))
	}

	unpacked, err := Unpack(packed)
	if err != nil {
		t.Fatalf("Unpack failed: %v", err)
	}

	if !bytes.Equal(unpacked.SenderHash, senderHash) {
		t.Errorf("expected sender hash %x, got %x", senderHash, unpacked.SenderHash)
	}

	if unpacked.Text != text {
		t.Errorf("expected text %q, got %q", text, unpacked.Text)
	}
}

func TestUnpack_TooShort(t *testing.T) {
	data := make([]byte, SenderHashLength-1)
	_, err := Unpack(data)
	if err == nil {
		t.Error("expected error for too short data, got nil")
	}
}

func TestMessage_GroupPackUnpack(t *testing.T) {
	senderHash, _ := hex.DecodeString(testHashHex)
	groupHash, _ := hex.DecodeString("fedcba9876543210fedcba9876543210")
	text := testMessageText

	msg, err := NewMessageWithGroup(senderHash, groupHash, text)
	if err != nil {
		t.Fatalf("NewMessageWithGroup failed: %v", err)
	}
	packed, err := msg.Pack()
	if err != nil {
		t.Fatalf("Pack failed: %v", err)
	}
	unpacked, err := Unpack(packed)
	if err != nil {
		t.Fatalf("Unpack failed: %v", err)
	}
	if !bytes.Equal(unpacked.GroupHash, groupHash) {
		t.Fatalf("group hash mismatch")
	}
	if unpacked.Text != text {
		t.Fatalf("text mismatch")
	}
}

func TestMessage_QoL(t *testing.T) {
	senderHash, _ := hex.DecodeString(testHashHex)
	text := testMessageText

	msg, err := NewMessage(senderHash, text)
	if err != nil {
		t.Fatalf("NewMessage failed: %v", err)
	}

	if msg.Len() != SenderHashLength+len(text) {
		t.Errorf("expected length %d, got %d", SenderHashLength+len(text), msg.Len())
	}

	if msg.FormatSenderHash() != testHashHex {
		t.Errorf(errFmtHex, testHashHex, msg.FormatSenderHash())
	}

	expectedStr := "Message{Sender: " + testHashHex + ", Text: \"Hello, Reticulum!\"}"
	if msg.String() != expectedStr {
		t.Errorf("expected string %q, got %q", expectedStr, msg.String())
	}

	msg2, _ := NewMessage(senderHash, text)
	if !msg.Equal(msg2) {
		t.Error("expected messages to be equal")
	}

	msg3, _ := NewMessage(senderHash, "Different text")
	if msg.Equal(msg3) {
		t.Error("expected messages to be different")
	}

	groupA := bytes.Repeat([]byte{0xaa}, SenderHashLength)
	groupB := bytes.Repeat([]byte{0xbb}, SenderHashLength)
	withA, err := NewMessageWithGroup(senderHash, groupA, text)
	if err != nil {
		t.Fatalf("NewMessageWithGroup: %v", err)
	}
	withB, err := NewMessageWithGroup(senderHash, groupB, text)
	if err != nil {
		t.Fatalf("NewMessageWithGroup: %v", err)
	}
	if withA.Equal(withB) {
		t.Fatal("Equal ignored group hash")
	}
	withA2, _ := NewMessageWithGroup(senderHash, groupA, text)
	if !withA.Equal(withA2) {
		t.Fatal("same group should compare equal")
	}

	mut := append([]byte(nil), senderHash...)
	owned, err := NewMessage(mut, text)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	mut[0] ^= 0xff
	if !bytes.Equal(owned.SenderHash, senderHash) {
		t.Fatal("NewMessage aliased caller hash slice")
	}
}

func TestNewMessageFromHex(t *testing.T) {
	text := testMessageText
	msg, err := NewMessageFromHex(testHashHex, text)
	if err != nil {
		t.Fatalf("NewMessageFromHex failed: %v", err)
	}

	if msg.FormatSenderHash() != testHashHex {
		t.Errorf(errFmtHex, testHashHex, msg.FormatSenderHash())
	}
}

func TestValidation(t *testing.T) {
	// Invalid hash length
	_, err := NewMessage([]byte{0x01}, "test")
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("invalid hash length")) {
		t.Errorf("expected invalid hash length error, got %v", err)
	}

	// Message too long
	longText := make([]byte, MaxMessageSize+1)
	for i := range longText {
		longText[i] = 'a'
	}
	_, err = NewMessage(make([]byte, SenderHashLength), string(longText))
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("message text too long")) {
		t.Errorf("expected message text too long error, got %v", err)
	}
}

func TestPeer_QoL(t *testing.T) {
	hash, _ := hex.DecodeString(testHashHex)
	peer := &Peer{
		Hash:    hash,
		AppData: "test user",
	}

	if err := peer.Validate(); err != nil {
		t.Errorf("Validate failed: %v", err)
	}

	expectedStr := "Peer{Hash: " + testHashHex + ", AppData: \"test user\"}"
	if peer.String() != expectedStr {
		t.Errorf("expected string %q, got %q", expectedStr, peer.String())
	}
}

func TestUtilityFunctions(t *testing.T) {
	hash, err := SenderHashFromHex(testHashHex)
	if err != nil {
		t.Fatalf("SenderHashFromHex failed: %v", err)
	}
	if hex.EncodeToString(hash) != testHashHex {
		t.Errorf(errFmtHex, testHashHex, hex.EncodeToString(hash))
	}

	if err := ValidateSenderHash(hash); err != nil {
		t.Errorf("ValidateSenderHash failed: %v", err)
	}

	if err := ValidateSenderHash([]byte{0x01}); err == nil {
		t.Error("expected error for invalid hash length")
	}
}

func TestMessenger_SendMessage(t *testing.T) {
	// Setup a minimal Reticulum environment
	cfg := testconfig.New()
	tr := transport.NewTransport(cfg)

	id, _ := identity.NewIdentity()
	dest, _ := destination.New(id, destination.In, destination.Single, "test", tr)

	m := NewMessenger(tr, dest)

	// Create a mock remote peer with a destination
	peerId, _ := identity.NewIdentity()
	peerDest, _ := destination.New(peerId, destination.In, destination.Single, "test", tr)
	peerHash := peerDest.GetHash()

	// Manually remember the identity so SendMessage can find it
	identity.Remember(nil, peerHash, peerId.GetPublicKey(), nil)

	// Try to send a message (it will fail to find a path, but that proves the logic works up to that point)
	err := m.SendMessage(peerHash, "Hello!")
	if err != nil && !errors.Is(err, common.ErrNoPathToDestination) {
		t.Errorf("SendMessage failed with unexpected error: %v", err)
	}
}

func TestParseHashGrouped(t *testing.T) {
	raw, err := hex.DecodeString(testHashHex)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseHash(" <" + FormatHash(raw) + "> ")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("got %x", got)
	}
	if _, err := ParseHash(" <> "); !errors.Is(err, ErrInvalidHash) {
		t.Fatalf("empty err %v", err)
	}
}

func TestNewMessageFromHexGrouped(t *testing.T) {
	raw, err := hex.DecodeString(testHashHex)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := NewMessageFromHex(FormatHash(raw), testMessageText)
	if err != nil {
		t.Fatal(err)
	}
	if msg.FormatSenderHash() != testHashHex {
		t.Fatalf("got %s", msg.FormatSenderHash())
	}
}

func TestSendHashRecallTimeout(t *testing.T) {
	cfg := testconfig.New()
	cfg.ShareInstance = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	cfg.ConfigPath = t.TempDir() + "/config"
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(id, destination.In, destination.Single, "mf", tr)
	if err != nil {
		t.Fatal(err)
	}
	m := NewMessenger(tr, dest)
	err = m.sendHash(FormatHash(bytes.Repeat([]byte{0xff}, SenderHashLength)), "hi", 50*time.Millisecond)
	if !errors.Is(err, ErrRecall) {
		t.Fatalf("err = %v", err)
	}
}
