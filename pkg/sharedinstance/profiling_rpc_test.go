// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sharedinstance

import (
	"strings"
	"sync"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/profiler"
	"quad4/reticulum-go/pkg/transport"
)

func TestProfilingResultsMsgpackRoundTrip(t *testing.T) {
	profiler.Reset()
	defer profiler.Reset()
	profiler.Do("wire.roundtrip", func() {})

	cfg := &common.ReticulumConfig{EnableTransport: false, InMemoryStorage: true}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	h := &RPCHandler{Transport: tr}
	raw := h.Handle(map[string]any{"get": "profiling_results"})
	packed, err := msgpack.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := msgpack.Unmarshal(packed, &decoded); err != nil {
		t.Fatal(err)
	}
	m, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("decoded type %T", decoded)
	}
	entry, ok := m["wire.roundtrip"]
	if !ok {
		t.Fatalf("missing tag in %#v", m)
	}
	em, ok := entry.(map[string]any)
	if !ok {
		t.Fatalf("entry type %T", entry)
	}
	name, _ := em["name"].(string)
	if name != "wire.roundtrip" {
		t.Fatalf("name=%q", name)
	}
	text := profiler.FormatResults(profiler.Results())
	if !strings.Contains(text, "wire.roundtrip") {
		t.Fatalf("format:\n%s", text)
	}
}

func TestRPCHandlerProfilingRace(t *testing.T) {
	profiler.Reset()
	defer profiler.Reset()
	cfg := &common.ReticulumConfig{EnableTransport: false, InMemoryStorage: true}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	h := &RPCHandler{Transport: tr}

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 100 {
				profiler.Do("race.tag", func() {})
			}
		}()
		go func() {
			defer wg.Done()
			for range 100 {
				_ = h.Handle(map[string]any{"get": "profiling_results"})
				_ = h.Handle(map[string]any{"get": "interface_stats"})
				_ = h.Handle(map[string]any{"get": "link_count"})
			}
		}()
	}
	wg.Wait()
}
