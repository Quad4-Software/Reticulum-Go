// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package i2p

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
)

const (
	ed25519SigType    = 7
	transientDest     = "TRANSIENT"
	defaultSAMMinVer  = "3.1"
	defaultSAMMaxVer  = "3.1"
	defaultSAMAddress = "127.0.0.1:7656"
	defaultSAMTimeout = 30
	// DefaultSessionOptions enables encrypted LeaseSets for modern peers.
	DefaultSessionOptions = "i2cp.leaseSetEncType=6,4"
)

var (
	i2pEncoding = base64.NewEncoding("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-~")
	validB32    = regexp.MustCompile(`^([a-zA-Z0-9]{52})\.b32\.i2p$`)
	validB32Raw = regexp.MustCompile(`^[a-zA-Z0-9]{52}$`)
	validB64    = regexp.MustCompile(`^([a-zA-Z0-9\-~=]{516,528})$`)
)

func i2pB64Encode(b []byte) string {
	return i2pEncoding.EncodeToString(b)
}

func i2pB64Decode(s string) ([]byte, error) {
	return i2pEncoding.DecodeString(s)
}

// Destination is an I2P destination with optional private key material.
type Destination struct {
	data       []byte
	base64     string
	privateKey []byte
}

func NewDestinationFromB64(b64 string) (*Destination, error) {
	raw, err := i2pB64Decode(b64)
	if err != nil {
		return nil, err
	}
	return &Destination{
		data:   append([]byte(nil), raw...),
		base64: b64,
	}, nil
}

func NewDestinationFromPrivateB64(b64 string) (*Destination, error) {
	raw, err := i2pB64Decode(b64)
	if err != nil {
		return nil, err
	}
	if len(raw) < 387 {
		return nil, fmt.Errorf("i2p: private key too short")
	}
	certLen := int(binary.BigEndian.Uint16(raw[385:387]))
	if len(raw) < 387+certLen {
		return nil, fmt.Errorf("i2p: private key too short")
	}
	pub := raw[:387+certLen]
	return &Destination{
		data:       append([]byte(nil), pub...),
		base64:     i2pB64Encode(pub),
		privateKey: append([]byte(nil), raw...),
	}, nil
}

func (d *Destination) Base32() string {
	sum := sha256.Sum256(d.data)
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return strings.ToLower(enc.EncodeToString(sum[:]))[:52]
}

func (d *Destination) Base64() string { return d.base64 }

func (d *Destination) PrivateKeyB64() string {
	if len(d.privateKey) == 0 {
		return ""
	}
	return i2pB64Encode(d.privateKey)
}

func ResolveDestination(name string, lookup func(string) (string, error)) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("i2p: empty destination")
	}
	if validB64.MatchString(name) {
		return name, nil
	}
	// SAM STREAM CONNECT accepts .b32.i2p directly. Skip NAMING LOOKUP for
	// these because i2pd often returns INVALID_KEY when no session tunnels
	// exist yet to fetch the LeaseSet.
	if validB32.MatchString(strings.ToLower(name)) {
		return strings.ToLower(name), nil
	}
	if validB32Raw.MatchString(name) {
		return strings.ToLower(name) + ".b32.i2p", nil
	}
	if strings.HasSuffix(name, ".i2p") && lookup != nil {
		val, err := lookup(name)
		if err != nil {
			return "", err
		}
		return val, nil
	}
	return name, nil
}
