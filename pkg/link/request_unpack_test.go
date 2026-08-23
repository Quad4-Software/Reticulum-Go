// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"os"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
)

func TestUnpackLinkRequestBytePayload(t *testing.T) {
	pathHash := make([]byte, 16)
	b, err := msgpack.Marshal([]any{int64(1), pathHash, []byte("hello")})
	if err != nil {
		t.Fatal(err)
	}
	_, _, payload, err := unpackLinkRequest(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "hello" {
		t.Fatalf("payload=%q", payload)
	}
}

func TestUnpackLinkRequestPythonRngitList(t *testing.T) {
	b, err := os.ReadFile("/tmp/py-req.msgpack")
	if err != nil {
		t.Skip("python sample not generated")
	}
	at, pathHash, payload, err := unpackLinkRequest(b)
	if err != nil {
		t.Fatal(err)
	}
	if at.IsZero() {
		t.Fatal("zero requested_at")
	}
	if len(pathHash) != 16 {
		t.Fatalf("path_hash len=%d", len(pathHash))
	}
	if len(payload) == 0 {
		t.Fatal("empty payload")
	}
}
