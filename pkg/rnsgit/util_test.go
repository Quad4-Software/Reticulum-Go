// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import "testing"

func TestParseRNSURL(t *testing.T) {
	dest, group, repo, err := ParseRNSURL("rns://0123456789abcdef0123456789abcdef/public/demo")
	if err != nil {
		t.Fatal(err)
	}
	if dest != "0123456789abcdef0123456789abcdef" || group != "public" || repo != "demo" {
		t.Fatalf("got %s %s %s", dest, group, repo)
	}
}

func TestSanRef(t *testing.T) {
	if SanRef("refs/heads/main") == "" {
		t.Fatal("expected valid ref")
	}
	if SanRef("../bad") != "" {
		t.Fatal("expected invalid ref")
	}
}

func TestEncodeMixedRequestForPush(t *testing.T) {
	b, err := EncodeMixedRequest(map[any]any{
		IdxRepository: "public/demo",
		"bundle":      []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := DecodeRequest(b)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := RepoFromRequest(req); !ok || got != "public/demo" {
		t.Fatalf("repo %q ok=%v", got, ok)
	}
}

func TestEncodeFetchRefsRoundTrip(t *testing.T) {
	b, err := EncodeMixedRequest(map[any]any{
		IdxRepository: "public/demo",
		"refs": []map[string]string{
			{"sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "ref": "refs/heads/main"},
		},
		"have": []string{"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := DecodeRequest(b)
	if err != nil {
		t.Fatal(err)
	}
	refsAny, ok := req["refs"]
	if !ok {
		t.Fatal("missing refs")
	}
	refsList, ok := refsAny.([]any)
	if !ok {
		t.Fatalf("refs type %T", refsAny)
	}
	if len(refsList) != 1 {
		t.Fatalf("refs len %d", len(refsList))
	}
}

func TestOKMetadataPacked(t *testing.T) {
	packed := OKMetadataPacked()
	code, ok := MetadataResultCodeRaw(packed)
	if !ok || code != ResOK {
		t.Fatalf("code=%d ok=%v", code, ok)
	}
}
