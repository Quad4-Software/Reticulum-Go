// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/resource"
)

func TestResourceInterop_WithMetadata(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	if err := respLink.SetResourceStrategy(AcceptAll); err != nil {
		t.Fatal(err)
	}
	got := make(chan IncomingResource, 1)
	respLink.SetResourceConcludedCallback(func(p any) {
		switch v := p.(type) {
		case IncomingResource:
			got <- v
		case []byte:
			got <- IncomingResource{Data: v}
		}
	})

	payload := []byte("metadata body payload")
	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := res.SetMetadata(map[string]any{"name": []byte("meta.bin")}); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- initLink.SendResource(res) }()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("SendResource: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("SendResource timeout")
	}

	var recv IncomingResource
	select {
	case recv = <-got:
	case <-time.After(30 * time.Second):
		t.Fatal("receive timeout")
	}
	if !bytes.Equal(recv.Data, payload) {
		t.Fatalf("data mismatch %q vs %q", recv.Data, payload)
	}
	name, _ := recv.Metadata["name"].([]byte)
	if !bytes.Equal(name, []byte("meta.bin")) {
		t.Fatalf("metadata name %q", name)
	}
}

func TestResourceInterop_MetadataGoToGoConcurrent(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()
	if err := respLink.SetResourceStrategy(AcceptAll); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	respLink.SetResourceConcludedCallback(func(p any) {
		defer wg.Done()
		ir, ok := p.(IncomingResource)
		if !ok {
			t.Errorf("expected IncomingResource, got %T", p)
			return
		}
		if string(ir.Data) != "x" {
			t.Errorf("data %q", ir.Data)
		}
	})
	res, err := resource.New([]byte("x"), false)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.SetMetadata(map[string]any{"name": []byte("x")})
	if err := initLink.SendResource(res); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("timeout")
	}
}
