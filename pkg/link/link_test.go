// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"

	"github.com/Quad4-Software/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

type mockTransport struct {
	sentPackets []*packet.Packet
}

func (m *mockTransport) SendPacket(pkt *packet.Packet) error {
	m.sentPackets = append(m.sentPackets, pkt)
	return nil
}

func (m *mockTransport) RegisterLink(linkID []byte, link any) {
}

func (m *mockTransport) GetConfig() *common.ReticulumConfig {
	return &common.ReticulumConfig{}
}

func (m *mockTransport) GetInterfaces() map[string]common.NetworkInterface {
	return make(map[string]common.NetworkInterface)
}

func (m *mockTransport) RegisterDestination(hash []byte, dest any) {
}

func TestLinkRequestResponse(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	expectedResponse := []byte("response data")
	testPath := "test/path"

	var gotPath string
	var gotPayload, gotLinkID, gotRequestID []byte
	pathHash := identity.TruncatedHash([]byte(testPath))
	if err := respLink.destination.RegisterRequestHandler(testPath, func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) []byte {
		gotPath = path
		gotPayload = append([]byte(nil), data...)
		gotLinkID = append([]byte(nil), linkID...)
		gotRequestID = append([]byte(nil), requestID...)
		return expectedResponse
	}, destination.AllowAll, nil); err != nil {
		t.Fatalf("RegisterRequestHandler: %v", err)
	}

	handler := respLink.destination.GetRequestHandler(pathHash)
	if handler == nil {
		t.Fatal("handler not found after registration")
	}

	payload := []byte("test data")
	plaintext := packRequest(t, time.Now().Unix(), pathHash, payload)
	pkt := &packet.Packet{Data: plaintext}
	if err := pkt.Pack(); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if err := respLink.handleRequest(plaintext, pkt.TruncatedHash()); err != nil {
		t.Fatalf("handleRequest: %v", err)
	}
	if gotPath != testPath {
		t.Fatalf("handler path %q want %q", gotPath, testPath)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Fatalf("handler payload %q want %q", gotPayload, payload)
	}
	if !bytes.Equal(gotLinkID, respLink.GetLinkID()) {
		t.Fatal("handler linkID does not match the responder link")
	}
	if !bytes.Equal(gotRequestID, pkt.TruncatedHash()) {
		t.Fatal("handler requestID is not the request packet hash")
	}
	_ = initLink
}

func TestLinkRequestHandlerNotFound(t *testing.T) {
	serverIdent, _ := identity.New()
	mockTrans := &mockTransport{sentPackets: make([]*packet.Packet, 0)}

	serverDest, _ := destination.New(serverIdent, destination.In, destination.Single, "testapp", mockTrans, "server")

	nonExistentPath := "/does/not/exist"
	pathHash := identity.TruncatedHash([]byte(nonExistentPath))

	handler := serverDest.GetRequestHandler(pathHash)
	if handler != nil {
		t.Error("Expected no handler for non-existent path, but found one")
	}
}

// TestLinkResponseHandling drives the real response path: a receipt is
// registered on the initiating link and an inbound msgpack response
// completes it, fires the callback, and removes it from pending.
func TestLinkResponseHandling(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, _, cleanup := establishInteropLink(t)
	defer cleanup()

	requestID := bytes.Repeat([]byte{0x42}, 16)
	receipt := &RequestReceipt{
		link:      initLink,
		requestID: requestID,
		pathHash:  bytes.Repeat([]byte{0x11}, 16),
		status:    StatusPending,
		done:      make(chan struct{}),
	}
	done := make(chan *RequestReceipt, 1)
	receipt.responseCb = func(r *RequestReceipt) { done <- r }

	if err := initLink.registerPendingRequest(receipt); err != nil {
		t.Fatalf("registerPendingRequest: %v", err)
	}

	responseData := []byte("response payload")
	packed, err := msgpack.Marshal([]any{requestID, responseData})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := initLink.handleResponse(packed); err != nil {
		t.Fatalf("handleResponse: %v", err)
	}

	select {
	case got := <-done:
		if got != receipt {
			t.Fatal("callback received different receipt")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("response callback never fired")
	}
	if receipt.status != StatusActive {
		t.Fatalf("receipt status=%d want Active", receipt.status)
	}
	if !bytes.Equal(receipt.response, responseData) {
		t.Fatalf("response %q want %q", receipt.response, responseData)
	}
	if n := len(initLink.pendingRequests); n != 0 {
		t.Fatalf("pendingRequests still has %d entries", n)
	}
}

func TestSelectRequestedPartIndexes_HandlesDuplicateMapHashes(t *testing.T) {
	const sdu = 32
	payload := bytes.Repeat([]byte{0x4D}, 320)

	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}
	identityEncrypt := func(plain []byte) ([]byte, error) {
		return bytes.Repeat([]byte{0x7C}, len(plain)), nil
	}
	if err := res.PrepareOutboundForLink(identityEncrypt, sdu); err != nil {
		t.Fatalf("PrepareOutboundForLink: %v", err)
	}

	firstPart := res.OutboundCiphertextSlice(0, sdu)
	hashSum := sha256.Sum256(append(append([]byte{}, firstPart...), res.GetRandomHash()...))
	mapHash := hashSum[:resource.MapHashLen]
	candidates := res.PartIndicesForMapHash(mapHash)
	if len(candidates) < 2 {
		t.Fatalf("expected duplicate map-hash candidates, got %d", len(candidates))
	}

	var reqHashes []byte
	for range candidates {
		reqHashes = append(reqHashes, mapHash...)
	}

	indexes := selectRequestedPartIndexes(res, reqHashes, 0)
	if len(indexes) != len(candidates) {
		t.Fatalf("expected %d selected indexes, got %d", len(candidates), len(indexes))
	}

	seen := make(map[int]struct{}, len(indexes))
	for _, idx := range indexes {
		seen[idx] = struct{}{}
	}
	if len(seen) != len(candidates) {
		t.Fatalf("expected %d unique indexes, got %d", len(candidates), len(seen))
	}
}

func TestSelectRequestedPartIndexes_PrefersUnsentAcrossBatches(t *testing.T) {
	const sdu = 32
	payload := bytes.Repeat([]byte{0x7F}, 320)

	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}
	identityEncrypt := func(plain []byte) ([]byte, error) {
		return bytes.Repeat([]byte{0x22}, len(plain)), nil
	}
	if err := res.PrepareOutboundForLink(identityEncrypt, sdu); err != nil {
		t.Fatalf("PrepareOutboundForLink: %v", err)
	}

	firstPart := res.OutboundCiphertextSlice(0, sdu)
	hashSum := sha256.Sum256(append(append([]byte{}, firstPart...), res.GetRandomHash()...))
	mapHash := hashSum[:resource.MapHashLen]
	candidates := res.PartIndicesForMapHash(mapHash)
	if len(candidates) < 8 {
		t.Fatalf("expected at least 8 duplicate candidates, got %d", len(candidates))
	}

	reqHashes := make([]byte, 0, resource.Window*resource.MapHashLen)
	for range resource.Window {
		reqHashes = append(reqHashes, mapHash...)
	}

	firstBatch := selectRequestedPartIndexes(res, reqHashes, 0)
	if len(firstBatch) != resource.Window {
		t.Fatalf("expected first batch size %d, got %d", resource.Window, len(firstBatch))
	}
	for _, idx := range firstBatch {
		_ = res.MarkOutboundPartSent(idx)
	}

	secondBatch := selectRequestedPartIndexes(res, reqHashes, 0)
	if len(secondBatch) != resource.Window {
		t.Fatalf("expected second batch size %d, got %d", resource.Window, len(secondBatch))
	}

	for _, idx := range secondBatch {
		for _, already := range firstBatch {
			if idx == already {
				t.Fatalf("expected second batch to avoid already-sent index %d", idx)
			}
		}
	}
}

func TestChooseHashmapUpdateSegment_SelectsNextSegmentBoundary(t *testing.T) {
	const sdu = 384
	payload := bytes.Repeat([]byte{0x52}, 40000)

	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}
	identityEncrypt := func(plain []byte) ([]byte, error) {
		return append([]byte(nil), plain...), nil
	}
	if err := res.PrepareOutboundForLink(identityEncrypt, sdu); err != nil {
		t.Fatalf("PrepareOutboundForLink: %v", err)
	}

	entries := resource.HashmapEntriesPerSegment(sdu)
	if entries <= 0 {
		t.Fatalf("expected positive hashmap entries per segment, got %d", entries)
	}
	totalParts := int(res.GetSegments())
	if totalParts <= entries {
		t.Fatalf("expected total parts > entries, got parts=%d entries=%d", totalParts, entries)
	}

	boundaryIndex := entries - 1
	boundarySlice := res.OutboundCiphertextSlice(boundaryIndex, sdu)
	if len(boundarySlice) == 0 {
		t.Fatal("boundary slice empty")
	}
	sum := sha256.Sum256(append(append([]byte{}, boundarySlice...), res.GetRandomHash()...))
	anchor := sum[:resource.MapHashLen]

	segment, _, ok := chooseHashmapUpdateSegment(res, sdu, anchor, 0)
	if !ok {
		t.Fatal("expected chooseHashmapUpdateSegment to succeed")
	}
	if segment != 1 {
		t.Fatalf("expected next segment index 1, got %d", segment)
	}

	// After the sender advances receiverMinPart past the anchor (as it does
	// once an HMU is sent), a lost HMU must still be findable via lookback.
	segment2, nextMin, ok := chooseHashmapUpdateSegment(res, sdu, anchor, entries)
	if !ok {
		t.Fatal("expected HMU lookback to find anchor after receiverMinPart advanced")
	}
	if segment2 != 1 {
		t.Fatalf("expected lookback segment 1, got %d", segment2)
	}
	if nextMin != entries {
		t.Fatalf("expected nextMin %d, got %d", entries, nextMin)
	}
}
