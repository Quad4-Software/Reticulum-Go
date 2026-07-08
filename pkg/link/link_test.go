// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/resource"
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
	serverIdent, err := identity.New()
	if err != nil {
		t.Fatalf("Failed to create server identity: %v", err)
	}

	clientIdent, err := identity.New()
	if err != nil {
		t.Fatalf("Failed to create client identity: %v", err)
	}

	mockTrans := &mockTransport{
		sentPackets: make([]*packet.Packet, 0),
	}

	serverDest, err := destination.New(serverIdent, destination.In, destination.Single, "testapp", mockTrans, "server")
	if err != nil {
		t.Fatalf("Failed to create server destination: %v", err)
	}

	expectedResponse := []byte("response data")
	testPath := "/test/path"

	err = serverDest.RegisterRequestHandler(testPath, func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) []byte {
		if path != testPath {
			t.Errorf("Expected path %s, got %s", testPath, path)
		}
		return expectedResponse
	}, destination.AllowAll, nil)
	if err != nil {
		t.Fatalf("Failed to register request handler: %v", err)
	}

	// Test the handler is registered correctly
	pathHash := identity.TruncatedHash([]byte(testPath))
	handler := serverDest.GetRequestHandler(pathHash)
	if handler == nil {
		t.Fatal("Handler not found after registration")
	}

	// Call the handler
	testLinkID := make([]byte, 16)
	result := handler(pathHash, []byte("test data"), []byte("request-id"), testLinkID, clientIdent, time.Now())

	if result == nil {
		t.Fatal("Handler returned nil")
	}

	responseBytes, ok := result.([]byte)
	if !ok {
		t.Fatalf("Handler returned unexpected type: %T", result)
	}

	if !bytes.Equal(responseBytes, expectedResponse) {
		t.Errorf("Expected response %q, got %q", expectedResponse, responseBytes)
	}
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

func TestLinkResponseHandling(t *testing.T) {
	// This test verifies the basic structure for response handling
	// Full integration testing would require a proper transport setup

	requestID := []byte("test-request-id-")
	responseData := []byte("response payload")

	receipt := &RequestReceipt{
		requestID: requestID,
		status:    StatusPending,
	}

	// Verify initial state
	if receipt.status != StatusPending {
		t.Errorf("Expected initial status PENDING, got %d", receipt.status)
	}

	// Simulate setting response
	receipt.response = responseData
	receipt.status = StatusActive

	if !bytes.Equal(receipt.response, responseData) {
		t.Errorf("Expected response %q, got %q", responseData, receipt.response)
	}

	if receipt.status != StatusActive {
		t.Errorf("Expected status ACTIVE after response, got %d", receipt.status)
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
}
