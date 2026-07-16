// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/resource"
)

func waitForResource(t *testing.T, ch <-chan []byte, label string, timeout time.Duration) []byte {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(timeout):
		t.Fatalf("%s: timeout waiting for resource", label)
		return nil
	}
}

func sendResourceAndWait(t *testing.T, initLink, respLink *Link, payload []byte, autoCompress bool, label string) {
	t.Helper()

	if err := respLink.SetResourceStrategy(AcceptAll); err != nil {
		t.Fatalf("%s: SetResourceStrategy: %v", label, err)
	}

	got := make(chan []byte, 1)
	respLink.SetResourceConcludedCallback(func(p any) {
		if b, ok := p.([]byte); ok {
			got <- append([]byte(nil), b...)
		}
	})

	res, err := resource.New(payload, autoCompress)
	if err != nil {
		t.Fatalf("%s: resource.New: %v", label, err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- initLink.SendResource(res) }()

	deadline := 30 * time.Second
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("%s: SendResource error: %v", label, err)
		}
	case <-time.After(deadline):
		t.Fatalf("%s: SendResource timeout", label)
	}

	received := waitForResource(t, got, label, deadline)

	wantSum := sha256.Sum256(payload)
	gotSum := sha256.Sum256(received)
	if wantSum != gotSum {
		t.Fatalf("%s: payload hash mismatch (want %d bytes, got %d bytes)",
			label, len(payload), len(received))
	}
}

func TestResourceInterop_PlaintextSmall(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()
	sendResourceAndWait(t, initLink, respLink, []byte("hello reticulum world"), false, "plaintext_small")
}

func TestResourceInterop_PlaintextOneSegment(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()
	payload := bytes.Repeat([]byte{0x55}, initLink.mdu/2)
	sendResourceAndWait(t, initLink, respLink, payload, false, "plaintext_one_segment")
}

func TestResourceInterop_PlaintextMultiSegment(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLinkAsync(t)
	defer cleanup()
	payload := bytes.Repeat([]byte{0xA1}, 800)
	sendResourceAndWait(t, initLink, respLink, payload, false, "plaintext_multi_segment")
}

// TestResourceInterop_PlaintextMultiHMUSegment sends a plaintext payload
// large enough that the resource's hashmap spans several hashmap-update
// (HMU) segments beyond the one included in the initial advertisement. This
// specifically exercises chooseHashmapUpdateSegment/HashmapSegment on the
// sending side and applyHashmapSegment on the receiving side agreeing on
// identical segment boundaries: the very first segment's offset is always
// zero regardless of any per-side SDU/MDU mismatch, so only a transfer with
// at least one HMU round trip beyond it can catch a desync there.
func TestResourceInterop_PlaintextMultiHMUSegment(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()
	payload := bytes.Repeat([]byte{0x5A, 0xC3, 0x91, 0x7E}, 100000)
	sendResourceAndWait(t, initLink, respLink, payload, false, "plaintext_multi_hmu_segment")
}

func TestResourceInterop_CompressedHighlyRedundant(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()
	payload := bytes.Repeat([]byte{0xAA}, 64*1024)
	sendResourceAndWait(t, initLink, respLink, payload, true, "compressed_highly_redundant")
}

func TestResourceInterop_CompressedTextLike(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()
	payload := bytes.Repeat([]byte("Reticulum-Go interop test payload, line "), 2048)
	sendResourceAndWait(t, initLink, respLink, payload, true, "compressed_textlike")
}

func TestResourceInterop_CompressedSmallBelowThreshold(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()
	payload := []byte("tiny payload that may or may not actually compress")
	sendResourceAndWait(t, initLink, respLink, payload, true, "compressed_small")
}

func TestResourceInterop_PayloadNearAutoCompressMax(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()
	n := max(resource.AutoCompressMaxSize/2, 1024)
	payload := bytes.Repeat([]byte{0x33}, n)
	if int64(len(payload)) >= int64(resource.AutoCompressMaxSize) {
		t.Fatalf("test payload %d exceeds AutoCompressMaxSize=%d", len(payload), resource.AutoCompressMaxSize)
	}
	sendResourceAndWait(t, initLink, respLink, payload, true, "near_auto_compress_max")
}

func TestResourceInterop_SplitLargePayload(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()
	payload := bytes.Repeat([]byte{0xAB}, resource.MaxEfficientSize+4096)
	sendResourceAndWait(t, initLink, respLink, payload, false, "split_large")
}

func TestResourceInterop_BackToBackTransfers(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLinkAsync(t)
	defer cleanup()

	if err := respLink.SetResourceStrategy(AcceptAll); err != nil {
		t.Fatalf("SetResourceStrategy: %v", err)
	}

	gotCh := make(chan []byte, 16)
	respLink.SetResourceConcludedCallback(func(p any) {
		if b, ok := p.([]byte); ok {
			gotCh <- append([]byte(nil), b...)
		}
	})

	payloads := [][]byte{
		bytes.Repeat([]byte{0x01}, 200),
		bytes.Repeat([]byte("abc"), 64),
		bytes.Repeat([]byte{0xFF}, 64*1024),
		[]byte("final transfer"),
	}

	for i, p := range payloads {
		res, err := resource.New(p, i != 1)
		if err != nil {
			t.Fatalf("payload %d resource.New: %v", i, err)
		}

		errCh := make(chan error, 1)
		go func() { errCh <- initLink.SendResource(res) }()

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("payload %d SendResource: %v", i, err)
			}
		case <-time.After(30 * time.Second):
			t.Fatalf("payload %d SendResource timeout", i)
		}

		var got []byte
		select {
		case got = <-gotCh:
		case <-time.After(30 * time.Second):
			t.Fatalf("payload %d delivery timeout", i)
		}

		wantSum := sha256.Sum256(p)
		gotSum := sha256.Sum256(got)
		if wantSum != gotSum {
			t.Fatalf("payload %d hash mismatch (want %d bytes, got %d bytes)", i, len(p), len(got))
		}
	}
}

func TestResourceInterop_RejectStrategyHonored(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	if err := respLink.SetResourceStrategy(AcceptNone); err != nil {
		t.Fatalf("SetResourceStrategy AcceptNone: %v", err)
	}

	delivered := make(chan struct{}, 1)
	respLink.SetResourceConcludedCallback(func(any) {
		select {
		case delivered <- struct{}{}:
		default:
		}
	})

	res, err := resource.New([]byte("should not arrive"), false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- initLink.SendResource(res) }()

	select {
	case <-delivered:
		t.Fatal("AcceptNone responder should not deliver resource payload")
	case <-time.After(2 * time.Second):
	}

	select {
	case <-errCh:
	default:
	}
}

func TestResourceInterop_AppStrategyConsultsCallback(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	if err := respLink.SetResourceStrategy(AcceptApp); err != nil {
		t.Fatalf("SetResourceStrategy AcceptApp: %v", err)
	}

	var (
		consultedMu sync.Mutex
		consulted   int
	)
	respLink.SetResourceCallback(func(any) bool {
		consultedMu.Lock()
		consulted++
		consultedMu.Unlock()
		return false
	})

	res, err := resource.New([]byte("app-strategy probe"), false)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}

	go func() { _ = initLink.SendResource(res) }()

	deadline := time.After(3 * time.Second)
	for {
		consultedMu.Lock()
		c := consulted
		consultedMu.Unlock()
		if c > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("resource callback was never consulted under AcceptApp")
		case <-time.After(50 * time.Millisecond):
		}
	}
}
