// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/pbt/pkg/pbt"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/resource"
)

// resourceAdvOutcome classifies processResourceAdvertisement results
// under AcceptAll for request gating and size checks.
type resourceAdvOutcome int

const (
	advAccept resourceAdvOutcome = iota
	advIgnore
	advReject
)

func classifyResourceAdv(adv *resource.ResourceAdvertisement, hasHandlers bool, sdu int) resourceAdvOutcome {
	if adv.IsRequest && adv.RequestID != nil {
		if !hasHandlers {
			return advIgnore
		}
		if !incomingResourceAllowed(adv, sdu) {
			return advReject
		}
		return advAccept
	}
	if adv.IsResponse && adv.RequestID != nil {
		// Unknown responses are ignored (no pending request in these exploratory checks).
		return advIgnore
	}
	if !incomingResourceAllowed(adv, sdu) {
		return advReject
	}
	return advAccept
}

func incomingResourceAllowed(adv *resource.ResourceAdvertisement, sdu int) bool {
	if sdu <= 0 || adv.Parts <= 0 {
		return false
	}
	maxSegmentBytes := int64(resource.MaxEfficientSize) + 4096
	maxParts := min(max(int(maxSegmentBytes/int64(sdu))+8, 1), int(resource.MaxSegments))
	if adv.Parts > maxParts {
		return false
	}
	if adv.TransferSize < 0 || adv.TransferSize > maxSegmentBytes {
		return false
	}
	if adv.TransferSize > int64(adv.Parts)*int64(sdu) {
		return false
	}
	if len(adv.Hashmap) == 0 || len(adv.Hashmap)%resource.MapHashLen != 0 {
		return false
	}
	return true
}

func TestExploratoryResourceAdvRequestGate(t *testing.T) {
	const sdu = 384
	honest := &resource.ResourceAdvertisement{
		Parts:        4,
		TransferSize: 4 * sdu,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x51}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x52}, 32),
		Hashmap:      makeFakeHashmap(1),
		RequestID:    bytes.Repeat([]byte{0x53}, 16),
		IsRequest:    true,
		Flags:        resource.AdvFlagIsRequest,
	}

	if got := classifyResourceAdv(honest, false, sdu); got != advIgnore {
		t.Fatalf("no handlers: got %d want ignore", got)
	}
	if got := classifyResourceAdv(honest, true, sdu); got != advAccept {
		t.Fatalf("with handlers: got %d want accept", got)
	}

	bomb := *honest
	bomb.TransferSize = int64(resource.MaxEfficientSize) + 4097
	if got := classifyResourceAdv(&bomb, true, sdu); got != advReject {
		t.Fatalf("oversized request: got %d want reject", got)
	}
}

func TestExploratoryProcessResourceAdvertisementMatchesClassifier(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	dest, err := destination.New(id, destination.Out, destination.Single, "orcladv", nil, "svc")
	if err != nil {
		t.Fatalf("destination.New: %v", err)
	}
	if err := dest.RegisterRequestHandler("echo", func(string, []byte, []byte, []byte, *identity.Identity, int64) []byte {
		return []byte("ok")
	}, destination.AllowAll, nil); err != nil {
		t.Fatalf("RegisterRequestHandler: %v", err)
	}

	cases := []struct {
		name        string
		handlers    bool
		adv         *resource.ResourceAdvertisement
		wantOutcome resourceAdvOutcome
	}{
		{
			name:     "request_no_handlers",
			handlers: false,
			adv: &resource.ResourceAdvertisement{
				Parts: 4, TransferSize: 4 * 384, DataSize: 1024,
				RandomHash: bytes.Repeat([]byte{0x61}, resource.RandomHashSize),
				Hash:       bytes.Repeat([]byte{0x62}, 32),
				Hashmap:    makeFakeHashmap(1),
				RequestID:  bytes.Repeat([]byte{0x63}, 16),
				IsRequest:  true, Flags: resource.AdvFlagIsRequest,
			},
			wantOutcome: advIgnore,
		},
		{
			name:     "request_with_handlers",
			handlers: true,
			adv: &resource.ResourceAdvertisement{
				Parts: 4, TransferSize: 4 * 384, DataSize: 1024,
				RandomHash: bytes.Repeat([]byte{0x64}, resource.RandomHashSize),
				Hash:       bytes.Repeat([]byte{0x65}, 32),
				Hashmap:    makeFakeHashmap(1),
				RequestID:  bytes.Repeat([]byte{0x66}, 16),
				IsRequest:  true, Flags: resource.AdvFlagIsRequest,
			},
			wantOutcome: advAccept,
		},
		{
			name:     "oversized_plain",
			handlers: false,
			adv: &resource.ResourceAdvertisement{
				Parts: 4, TransferSize: int64(resource.MaxEfficientSize) + 4097, DataSize: 1024,
				RandomHash: bytes.Repeat([]byte{0x67}, resource.RandomHashSize),
				Hash:       bytes.Repeat([]byte{0x68}, 32),
				Hashmap:    makeFakeHashmap(1),
			},
			wantOutcome: advReject,
		},
		{
			name:     "response_unknown",
			handlers: false,
			adv: &resource.ResourceAdvertisement{
				Parts: 4, TransferSize: 4 * 384, DataSize: 1024,
				RandomHash: bytes.Repeat([]byte{0x69}, resource.RandomHashSize),
				Hash:       bytes.Repeat([]byte{0x6A}, 32),
				Hashmap:    makeFakeHashmap(1),
				RequestID:  bytes.Repeat([]byte{0x6B}, 16),
				IsResponse: true, Flags: resource.AdvFlagIsResponse,
			},
			wantOutcome: advIgnore,
		},
		{
			name:     "honest_plain",
			handlers: false,
			adv: &resource.ResourceAdvertisement{
				Parts: 4, TransferSize: 4 * 384, DataSize: 1024,
				RandomHash: bytes.Repeat([]byte{0x6C}, resource.RandomHashSize),
				Hash:       bytes.Repeat([]byte{0x6D}, 32),
				Hashmap:    makeFakeHashmap(1),
			},
			wantOutcome: advAccept,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyResourceAdv(tc.adv, tc.handlers, 384); got != tc.wantOutcome {
				t.Fatalf("classifier: got %d want %d", got, tc.wantOutcome)
			}

			l := &Link{mdu: 384}
			l.status.Store(int32(StatusActive))
			l.resourceStrategy = AcceptAll
			if tc.handlers {
				l.destination = dest
			}
			defer l.resetIncomingResource()

			err := l.processResourceAdvertisement(packTestAdvertisement(t, tc.adv))
			l.incomingMu.Lock()
			rx := l.incomingRx
			l.incomingMu.Unlock()

			switch tc.wantOutcome {
			case advIgnore:
				if err != nil {
					t.Fatalf("ignore returned error: %v", err)
				}
				if rx != nil {
					t.Fatal("ignore started incoming resource")
				}
				if l.GetStatus() != StatusActive {
					t.Fatalf("status=%d want Active", l.GetStatus())
				}
			case advAccept:
				if err != nil {
					t.Fatalf("accept returned error: %v", err)
				}
				if rx == nil {
					t.Fatal("accept did not start incoming resource")
				}
			case advReject:
				if err == nil {
					t.Fatal("reject expected error")
				}
				if rx != nil {
					t.Fatal("reject started incoming resource")
				}
				_ = l.abortInvalidResourceAdvertisement(err)
				if l.GetStatus() != StatusClosed {
					t.Fatalf("status=%d want Closed after abort", l.GetStatus())
				}
			}
		})
	}
}

func TestPBTResourceAdvTransferSizeBound(t *testing.T) {
	const sdu = 384
	sizes := pbt.IntRange(0, int(resource.MaxEfficientSize)+8192)
	prop := pbt.ForAll(
		"transfer size bound vs beginIncomingResource",
		sizes,
		func(tsz int) bool {
			parts := 4
			adv := &resource.ResourceAdvertisement{
				Parts:        parts,
				TransferSize: int64(tsz),
				DataSize:     int64(tsz),
				RandomHash:   bytes.Repeat([]byte{0x71}, resource.RandomHashSize),
				Hash:         bytes.Repeat([]byte{0x72}, 32),
				Hashmap:      makeFakeHashmap(1),
			}
			wantOK := incomingResourceAllowed(adv, sdu)

			l := &Link{mdu: sdu}
			l.status.Store(int32(StatusActive))
			l.resourceStrategy = AcceptAll
			err := l.processResourceAdvertisement(mustPackAdv(adv))
			l.resetIncomingResource()
			gotOK := err == nil
			return gotOK == wantOK
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(91))
}

func TestPBTResourceAdvRequestGate(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	dest, err := destination.New(id, destination.Out, destination.Single, "pbtadv", nil, "svc")
	if err != nil {
		t.Fatalf("destination.New: %v", err)
	}
	if err := dest.RegisterRequestHandler("p", func(string, []byte, []byte, []byte, *identity.Identity, int64) []byte {
		return nil
	}, destination.AllowAll, nil); err != nil {
		t.Fatalf("RegisterRequestHandler: %v", err)
	}

	gen := pbt.Tuple2("handlers_x_parts", pbt.Bool(), pbt.IntRange(1, 16))
	prop := pbt.ForAll(
		"request ads need registered handlers",
		gen,
		func(v pbt.Tuple2Value[bool, int]) bool {
			hasHandlers := v.First
			parts := v.Second
			adv := &resource.ResourceAdvertisement{
				Parts:        parts,
				TransferSize: int64(parts) * 384,
				DataSize:     int64(parts) * 100,
				RandomHash:   bytes.Repeat([]byte{0x81}, resource.RandomHashSize),
				Hash:         bytes.Repeat([]byte{0x82}, 32),
				Hashmap:      makeFakeHashmap(1),
				RequestID:    bytes.Repeat([]byte{0x83}, 16),
				IsRequest:    true,
				Flags:        resource.AdvFlagIsRequest,
			}
			want := classifyResourceAdv(adv, hasHandlers, 384)

			l := &Link{mdu: 384}
			l.status.Store(int32(StatusActive))
			l.resourceStrategy = AcceptAll
			if hasHandlers {
				l.destination = dest
			}
			err := l.processResourceAdvertisement(mustPackAdv(adv))
			l.incomingMu.Lock()
			rx := l.incomingRx
			l.incomingMu.Unlock()
			l.resetIncomingResource()

			switch want {
			case advIgnore:
				return err == nil && rx == nil
			case advAccept:
				return err == nil && rx != nil
			case advReject:
				return err != nil && rx == nil
			default:
				return false
			}
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(64), pbt.WithSeed(92))
}

func mustPackAdv(adv *resource.ResourceAdvertisement) []byte {
	b, err := adv.Pack(0, 384)
	if err != nil {
		panic(err)
	}
	return b
}

// FuzzProcessResourceAdvertisementExploratory feeds adversarial RESOURCE_ADV blobs
// through processResourceAdvertisement and checks status laws on success or abort.
func FuzzProcessResourceAdvertisementExploratory(f *testing.F) {
	id, err := identity.New()
	if err != nil {
		f.Fatal(err)
	}
	destWith, err := destination.New(id, destination.Out, destination.Single, "fuzzadv", nil, "with")
	if err != nil {
		f.Fatal(err)
	}
	if err := destWith.RegisterRequestHandler("echo", func(string, []byte, []byte, []byte, *identity.Identity, int64) []byte {
		return []byte("ok")
	}, destination.AllowAll, nil); err != nil {
		f.Fatal(err)
	}

	seeds := []*resource.ResourceAdvertisement{
		{
			Parts: 4, TransferSize: 4 * 384, DataSize: 1024,
			RandomHash: bytes.Repeat([]byte{0x91}, resource.RandomHashSize),
			Hash:       bytes.Repeat([]byte{0x92}, 32),
			Hashmap:    makeFakeHashmap(1),
		},
		{
			Parts: 4, TransferSize: 4 * 384, DataSize: 1024,
			RandomHash: bytes.Repeat([]byte{0x93}, resource.RandomHashSize),
			Hash:       bytes.Repeat([]byte{0x94}, 32),
			Hashmap:    makeFakeHashmap(1),
			RequestID:  bytes.Repeat([]byte{0x95}, 16),
			IsRequest:  true, Flags: resource.AdvFlagIsRequest,
		},
		{
			Parts: 4, TransferSize: int64(resource.MaxEfficientSize) + 4097, DataSize: 1024,
			RandomHash: bytes.Repeat([]byte{0x96}, resource.RandomHashSize),
			Hash:       bytes.Repeat([]byte{0x97}, 32),
			Hashmap:    makeFakeHashmap(1),
		},
		{
			Parts: 4, TransferSize: 4 * 384, DataSize: 1024,
			RandomHash: bytes.Repeat([]byte{0x98}, resource.RandomHashSize),
			Hash:       bytes.Repeat([]byte{0x99}, 32),
			Hashmap:    makeFakeHashmap(1),
			RequestID:  bytes.Repeat([]byte{0x9A}, 16),
			IsResponse: true, Flags: resource.AdvFlagIsResponse,
		},
	}
	for _, adv := range seeds {
		f.Add(mustPackAdv(adv), byte(AcceptAll), true)
		f.Add(mustPackAdv(adv), byte(AcceptNone), false)
	}
	f.Add([]byte{}, byte(AcceptAll), false)
	f.Add([]byte{0xff, 0x00}, byte(AcceptAll), true)
	neg, _ := msgpack.Marshal(map[string]any{
		"t": int64(-1), "d": int64(1), "n": 1,
		"h": make([]byte, 32), "r": []byte{1, 2, 3, 4}, "m": make([]byte, resource.MapHashLen),
	})
	f.Add(neg, byte(AcceptAll), false)

	f.Fuzz(func(t *testing.T, data []byte, strategyByte byte, withHandlers bool) {
		if len(data) > 1<<14 {
			t.Skip()
		}
		l := &Link{mdu: 384}
		l.status.Store(int32(StatusActive))
		l.resourceStrategy = strategyByte % 3
		if withHandlers {
			l.destination = destWith
		}

		err := l.processResourceAdvertisement(data)
		l.incomingMu.Lock()
		rx := l.incomingRx
		l.incomingMu.Unlock()

		if err != nil {
			if rx != nil {
				t.Fatal("error path left incoming resource active")
			}
			_ = l.abortInvalidResourceAdvertisement(err)
			if l.GetStatus() != StatusClosed {
				t.Fatalf("status=%d want Closed after abort", l.GetStatus())
			}
			return
		}
		if l.GetStatus() != StatusActive {
			t.Fatalf("success path status=%d want Active", l.GetStatus())
		}
		// Request ads without handlers must never start a transfer.
		if adv, uerr := resource.UnpackResourceAdvertisement(data); uerr == nil {
			if adv.IsRequest && adv.RequestID != nil && !withHandlers && rx != nil {
				t.Fatal("request advertisement accepted without handlers")
			}
		}
		l.resetIncomingResource()
	})
}

// FuzzAbortInvalidResourceAdvertisement ensures abort is idempotent on
// Closed links and always closes Active ones.
func FuzzAbortInvalidResourceAdvertisement(f *testing.F) {
	f.Add(byte(StatusActive), "boom")
	f.Add(byte(StatusClosed), "already closed")
	f.Add(byte(StatusPending), "pending")

	f.Fuzz(func(t *testing.T, statusByte byte, msg string) {
		if len(msg) > 256 {
			t.Skip()
		}
		l := &Link{mdu: 384}
		st := statusByte % 5
		l.status.Store(int32(st))
		err := l.abortInvalidResourceAdvertisement(errString(msg))
		if err == nil {
			t.Fatal("abort must return the error")
		}
		after := l.GetStatus()
		if st == StatusActive && after != StatusClosed {
			t.Fatalf("Active abort status=%d want Closed", after)
		}
		if st == StatusClosed && after != StatusClosed {
			t.Fatalf("Closed abort changed status to %d", after)
		}
	})
}

type errString string

func (e errString) Error() string { return string(e) }
