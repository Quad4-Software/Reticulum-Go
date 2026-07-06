// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/resource"
	"quad4/reticulum-go/pkg/transport"
)

func packNomadnetMetadata(t *testing.T, metadata map[string]any, data []byte) []byte {
	t.Helper()
	packedMeta, err := msgpack.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if len(packedMeta) > 0xFFFFFF {
		t.Fatalf("metadata too large for test")
	}
	prefix := []byte{
		byte(len(packedMeta) >> 16),
		byte(len(packedMeta) >> 8),
		byte(len(packedMeta)),
	}
	out := append(append([]byte(nil), prefix...), packedMeta...)
	return append(out, data...)
}

func TestSplitResourceMetadata_ExtractsFileNameAndPayload(t *testing.T) {
	fileBytes := []byte("the quick brown fox jumps over the lazy dog")
	payload := packNomadnetMetadata(t, map[string]any{"name": []byte("guide.txt")}, fileBytes)

	adv := &resource.ResourceAdvertisement{HasMetadata: true}
	data, meta := splitResourceMetadata(payload, adv)

	if !bytes.Equal(data, fileBytes) {
		t.Fatalf("payload mismatch: got %q want %q", data, fileBytes)
	}
	if meta == nil {
		t.Fatal("expected metadata to be extracted")
	}
	name, ok := meta["name"].([]byte)
	if !ok {
		t.Fatalf("expected metadata name to be []byte, got %T", meta["name"])
	}
	if string(name) != "guide.txt" {
		t.Fatalf("filename mismatch: got %q", name)
	}
}

func TestSplitResourceMetadata_NoMetadataFlagReturnsPayloadUnchanged(t *testing.T) {
	payload := []byte("plain page bytes, no metadata attached")
	adv := &resource.ResourceAdvertisement{HasMetadata: false}

	data, meta := splitResourceMetadata(payload, adv)
	if !bytes.Equal(data, payload) {
		t.Fatalf("payload should be unchanged when HasMetadata is false")
	}
	if meta != nil {
		t.Fatalf("expected nil metadata when HasMetadata is false")
	}
}

func TestSplitResourceMetadata_TruncatedPrefixIsSafe(t *testing.T) {
	adv := &resource.ResourceAdvertisement{HasMetadata: true}

	data, meta := splitResourceMetadata([]byte{0x00}, adv)
	if !bytes.Equal(data, []byte{0x00}) || meta != nil {
		t.Fatalf("expected passthrough for undersized payload, got data=%v meta=%v", data, meta)
	}
}

func TestSplitResourceMetadata_OversizedLengthPrefixIsSafe(t *testing.T) {
	adv := &resource.ResourceAdvertisement{HasMetadata: true}
	payload := []byte{0xFF, 0xFF, 0xFF, 0x01, 0x02}

	data, meta := splitResourceMetadata(payload, adv)
	if !bytes.Equal(data, payload) || meta != nil {
		t.Fatalf("expected passthrough for oversized length prefix, got data=%v meta=%v", data, meta)
	}
}

func TestCompleteRequestWithResourcePayload_FileMetadataBypassesEnvelopeUnwrap(t *testing.T) {
	l := &Link{}
	fileBytes := []byte("binary file contents that must not be mangled")
	metadata := map[string]any{"name": []byte("archive.zip")}

	req := &RequestReceipt{requestID: []byte("req-1"), status: StatusPending}
	l.pendingRequests = append(l.pendingRequests, req)

	done := make(chan struct{})
	req.SetResponseCallback(func(*RequestReceipt) { close(done) })

	l.completeRequestWithResourcePayload(req, fileBytes, metadata)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("response callback was not invoked")
	}

	if got := req.GetResponse(); !bytes.Equal(got, fileBytes) {
		t.Fatalf("file payload was mangled: got %q want %q", got, fileBytes)
	}
	name, _ := req.GetMetadata()["name"].([]byte)
	if string(name) != "archive.zip" {
		t.Fatalf("metadata not attached to receipt: %v", req.GetMetadata())
	}
	if len(l.pendingRequests) != 0 {
		t.Fatalf("expected request to be removed from pendingRequests")
	}
}

func TestCompleteRequestWithResourcePayload_NoMetadataStillUnwrapsEnvelope(t *testing.T) {
	l := &Link{}
	req := &RequestReceipt{requestID: []byte("req-2"), status: StatusPending}
	l.pendingRequests = append(l.pendingRequests, req)

	packed, err := msgpack.Marshal([]any{req.requestID, []byte("page body")})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	l.completeRequestWithResourcePayload(req, packed, nil)

	if got := req.GetResponse(); !bytes.Equal(got, []byte("page body")) {
		t.Fatalf("expected unwrapped response, got %q", got)
	}
	if req.GetMetadata() != nil {
		t.Fatalf("expected nil metadata for non-file response")
	}
}

func TestLinkRequestPassesMapDataAsDict(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)

	cfgA := &common.ReticulumConfig{}
	trA := transport.NewTransport(cfgA)
	defer trA.Close()
	idA, _ := identity.New()

	cfgB := &common.ReticulumConfig{}
	trB := transport.NewTransport(cfgB)
	defer trB.Close()

	pipeA := NewPipeInterface("pipeA")
	pipeB := NewPipeInterface("pipeB")
	pipeA.peer = pipeB
	pipeB.peer = pipeA
	pipeA.tr = trA
	pipeB.tr = trB
	_ = trA.RegisterInterface("pipeA", pipeA)
	_ = trB.RegisterInterface("pipeB", pipeB)

	destA, _ := destination.New(idA, destination.In, destination.Single, "nomadnetwork", trA, "node")
	destA.AcceptsLinks(true)

	var receivedMap map[string]any
	var receivedRaw any
	destA.RegisterRequestHandler("/page/index.mu", func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) []byte {
		var decoded any
		if err := msgpack.Unmarshal(data, &decoded); err == nil {
			receivedRaw = decoded
			if m, ok := decoded.(map[string]any); ok {
				receivedMap = m
			}
		}
		return []byte("ok")
	}, destination.AllowAll, nil)

	_ = destA.Announce(false, nil, nil)
	time.Sleep(100 * time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(1)
	linkB := NewLink(destA, trB, pipeB, func(l *Link) { wg.Done() }, nil)
	_ = linkB.Establish()

	select {
	case <-time.After(2 * time.Second):
		t.Fatal("link establishment timeout")
	case <-func() chan struct{} {
		ch := make(chan struct{})
		go func() { wg.Wait(); close(ch) }()
		return ch
	}():
	}

	requestData := map[string]string{"var_name": "alice", "field_message": "hi"}
	receipt, err := linkB.Request("/page/index.mu", requestData, 2*time.Second)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	done := make(chan struct{})
	receipt.SetResponseCallback(func(r *RequestReceipt) { close(done) })

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("response timeout")
	}

	if receivedMap == nil {
		t.Fatalf("handler did not receive a dict; got %#v (raw type %T)", receivedRaw, receivedRaw)
	}
	if receivedMap["var_name"] != "alice" {
		t.Errorf("var_name mismatch: %#v", receivedMap["var_name"])
	}
	if receivedMap["field_message"] != "hi" {
		t.Errorf("field_message mismatch: %#v", receivedMap["field_message"])
	}
}
