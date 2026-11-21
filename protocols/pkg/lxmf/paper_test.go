// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPaperURI_RoundTrip(t *testing.T) {
	cases := [][]byte{
		{0x00},
		bytes.Repeat([]byte{0xAB}, DestinationLength),
		bytes.Repeat([]byte{0x42}, 200),
	}
	for i, payload := range cases {
		uri, err := PaperURI(payload)
		if err != nil {
			t.Fatalf("[%d] PaperURI: %v", i, err)
		}
		if !strings.HasPrefix(uri, "lxm://") {
			t.Errorf("[%d] missing lxm:// prefix: %q", i, uri)
		}
		if strings.Contains(uri, "=") {
			t.Errorf("[%d] padding leaked into URI: %q", i, uri)
		}
		decoded, err := DecodePaperURI(uri)
		if err != nil {
			t.Fatalf("[%d] DecodePaperURI: %v", i, err)
		}
		if !bytes.Equal(decoded, payload) {
			t.Errorf("[%d] round-trip mismatch: %x vs %x", i, decoded, payload)
		}
	}
}

func TestPaperURI_RejectsEmpty(t *testing.T) {
	if _, err := PaperURI(nil); err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestPaperURI_RejectsOversized(t *testing.T) {
	too := make([]byte, PaperMDU+1)
	if _, err := PaperURI(too); err == nil {
		t.Fatalf("expected error for payload exceeding PaperMDU (%d)", PaperMDU)
	}
}

func TestDecodePaperURI_RejectsBadPrefix(t *testing.T) {
	_, err := DecodePaperURI("https://example.com/abc")
	if !errors.Is(err, ErrInvalidURI) {
		t.Fatalf("expected ErrInvalidURI, got %v", err)
	}
}

func TestDecodePaperURI_RejectsEmptyPayload(t *testing.T) {
	_, err := DecodePaperURI("lxm://")
	if !errors.Is(err, ErrInvalidURI) {
		t.Fatalf("expected ErrInvalidURI, got %v", err)
	}
}

func TestDecodePaperURI_RejectsInvalidBase64(t *testing.T) {
	_, err := DecodePaperURI("lxm://not_valid_base64!!!")
	if !errors.Is(err, ErrInvalidURI) {
		t.Fatalf("expected ErrInvalidURI, got %v", err)
	}
}

func TestDecodePaperURI_AcceptsPadded(t *testing.T) {
	original := []byte("padding-test")
	uri, err := PaperURI(original)
	if err != nil {
		t.Fatalf("PaperURI: %v", err)
	}
	for pad := range 4 {
		padded := uri + strings.Repeat("=", pad)
		decoded, err := DecodePaperURI(padded)
		if err != nil {
			t.Fatalf("padded[%d]: %v", pad, err)
		}
		if !bytes.Equal(decoded, original) {
			t.Errorf("padded[%d] mismatch", pad)
		}
	}
}
