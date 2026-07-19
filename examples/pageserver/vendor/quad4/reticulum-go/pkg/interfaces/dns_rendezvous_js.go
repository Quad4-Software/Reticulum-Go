// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build js

package interfaces

import (
	"errors"
	"time"
)

// DNSRendezvousInterface is unavailable on js/wasm.
type DNSRendezvousInterface struct {
	BaseInterface
}

// DNSRendezvousOptions is a js stub.
type DNSRendezvousOptions struct {
	Domain          string
	ListenAddr      string
	ResolveInterval time.Duration
	LookupTXT       DNSLookupTXT
}

// DNSLookupTXT is a js stub type.
type DNSLookupTXT func(name string) ([]string, error)

// NewDNSRendezvousInterface returns an error on js/wasm.
func NewDNSRendezvousInterface(name string, enabled bool, opts DNSRendezvousOptions) (*DNSRendezvousInterface, error) {
	return nil, errors.New("DNSRendezvousInterface is not available on js/wasm")
}

func ParseRNSTXT(txt string) (DNSRendezvousEndpoint, bool) {
	return DNSRendezvousEndpoint{}, false
}

// DNSRendezvousEndpoint is a js stub.
type DNSRendezvousEndpoint struct {
	Proto string
	Host  string
	Port  int
}
