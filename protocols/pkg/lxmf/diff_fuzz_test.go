// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

type unpackBatchResult struct {
	Accept     bool   `json:"accept"`
	SourceHash string `json:"source_hash"`
	DestHash   string `json:"dest_hash"`
	Timestamp  any    `json:"timestamp"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Error      string `json:"error"`
}

// Differential fuzz: feed identical valid, mutated-valid, and arbitrary LXMF
// wire bytes to the Go unpacker and the real Python LXMF unpacker. Accept/
// reject decisions must agree. Skipped when python is absent.
func TestDifferentialUnpackVsPython(t *testing.T) {
	if testing.Short() {
		t.Skip("differential fuzz skipped in -short mode")
	}
	requireInterop(t)

	src, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dst, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)

	// seed corpus: valid packed messages with varied fields
	var seeds [][]byte
	for i := 0; i < 6; i++ {
		msg, err := NewMessage(dst.Hash(), src.Hash(), []byte(fmt.Sprintf("t%d", i)), []byte(fmt.Sprintf("body-%d", i)), nil)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := msg.Pack(src)
		if err != nil {
			t.Fatal(err)
		}
		seeds = append(seeds, raw)
	}

	rng := rand.New(rand.NewSource(1337))
	var cases [][]byte
	cases = append(cases, seeds...)
	// single bit flips across the whole wire format
	for _, seed := range seeds {
		for pos := 0; pos < len(seed) && len(cases) < 2400; pos += 3 {
			mut := append([]byte(nil), seed...)
			mut[pos] ^= 1 << uint(rng.Intn(8))
			cases = append(cases, mut)
		}
	}
	// truncations and extensions
	for _, seed := range seeds {
		for _, cut := range []int{1, 16, 32, len(seed) / 2, len(seed) - 1} {
			if cut > 0 && cut < len(seed) {
				cases = append(cases, seed[:cut])
			}
		}
		ext := append(append([]byte(nil), seed...), 0x00, 0xff)
		cases = append(cases, ext)
	}
	// arbitrary bytes
	for i := 0; i < 300; i++ {
		n := rng.Intn(200)
		b := make([]byte, n)
		_, _ = rng.Read(b)
		cases = append(cases, b)
	}

	t.Logf("differential corpus: %d cases", len(cases))

	// batch through python
	blobs := make([]string, len(cases))
	for i, c := range cases {
		blobs[i] = hex.EncodeToString(c)
	}
	start := time.Now()
	resp, err := runInterop(map[string]any{"cmd": "unpack_batch", "blobs": blobs})
	if err != nil {
		t.Fatalf("python batch unpack: %v", err)
	}
	batch := struct{ Results []unpackBatchResult }{}
	for _, raw := range resp.Results {
		var r unpackBatchResult
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		batch.Results = append(batch.Results, r)
	}
	t.Logf("python batch: %d results in %v", len(batch.Results), time.Since(start))
	if len(batch.Results) != len(cases) {
		t.Fatalf("python returned %d results for %d cases", len(batch.Results), len(cases))
	}

	divergences := 0
	pyAccepts, goAccepts := 0, 0
	for i, c := range cases {
		py := batch.Results[i]
		_, goErr := Unpack(c, RecallSource)
		goAccept := goErr == nil
		if py.Accept {
			pyAccepts++
		}
		if goAccept {
			goAccepts++
		}
		if py.Accept != goAccept {
			// Documented python quirk: LXMessage.unpack_from_bytes parses
			// structure only and never verifies the signature (validation
			// happens later at delivery). Go verifies inline at unpack and
			// is therefore strictly stronger: a py-accept/go-reject pair is
			// allowed only when the Go error is the signature check.
			if py.Accept && !goAccept && goErr != nil {
				// Allowed stricter-than-python classes, both documented in
				// golxmd.md: python unpack_from_bytes never verifies the
				// Ed25519 signature (validation happens later at delivery),
				// and python's msgpack accepts trailing bytes after the
				// packed payload. Real python senders produce neither, so
				// rejecting both is wire-compatible garbage rejection.
				if errors.Is(goErr, ErrSignatureInvalid) || strings.Contains(goErr.Error(), "trailing payload bytes") {
					continue
				}
			}
			divergences++
			if divergences <= 10 {
				t.Logf("divergence case %d len=%d py=%v go=%v (goErr=%v pyErr=%s) hex=%s",
					i, len(c), py.Accept, goAccept, goErr, py.Error, hex.EncodeToString(c[:min(len(c), 48)]))
			}
		}
	}
	t.Logf("accepts: python=%d go=%d divergences=%d/%d", pyAccepts, goAccepts, divergences, len(cases))
	if divergences > 0 {
		t.Errorf("%d accept/reject divergences between Go and python unpack", divergences)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
