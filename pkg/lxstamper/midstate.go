// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"crypto/sha256"
	"encoding"
	"encoding/binary"
	"errors"
)

// sha256Midstate is SHA-256 state after every complete 64-byte block of a prefix.
type sha256Midstate struct {
	H            [8]uint32
	Rem          []byte
	PrefixLen    int
	Marshaled    []byte
}

// midstateOfPrefix compresses all complete SHA-256 blocks of prefix.
func midstateOfPrefix(prefix []byte) (sha256Midstate, error) {
	n := len(prefix) &^ 63
	d := sha256.New()
	if n > 0 {
		if _, err := d.Write(prefix[:n]); err != nil {
			return sha256Midstate{}, err
		}
	}
	mar, ok := d.(encoding.BinaryMarshaler)
	if !ok {
		return sha256Midstate{}, errors.New("lxstamper: sha256 midstate unsupported")
	}
	raw, err := mar.MarshalBinary()
	if err != nil {
		return sha256Midstate{}, err
	}
	if len(raw) < 4+32 {
		return sha256Midstate{}, errors.New("lxstamper: sha256 midstate truncated")
	}
	var ms sha256Midstate
	for i := range ms.H {
		ms.H[i] = binary.BigEndian.Uint32(raw[4+4*i : 4+4*i+4])
	}
	ms.Rem = append([]byte(nil), prefix[n:]...)
	ms.PrefixLen = len(prefix)
	// Marshal after writing only full blocks leaves nx=0. Rebuild marshal with rem for Sum path.
	ms.Marshaled = make([]byte, 0, 4+32+64+8)
	ms.Marshaled = append(ms.Marshaled, 's', 'h', 'a', 0x03)
	for _, w := range ms.H {
		ms.Marshaled = binary.BigEndian.AppendUint32(ms.Marshaled, w)
	}
	x := make([]byte, 64)
	copy(x, ms.Rem)
	ms.Marshaled = append(ms.Marshaled, x...)
	ms.Marshaled = binary.BigEndian.AppendUint64(ms.Marshaled, uint64(len(prefix)))
	return ms, nil
}

// hashFromMidstateStamp returns SHA256(prefix||stamp) using a prepared midstate.
func hashFromMidstateStamp(ms sha256Midstate, stamp []byte) ([32]byte, error) {
	d := sha256.New()
	un, ok := d.(encoding.BinaryUnmarshaler)
	if !ok {
		return [32]byte{}, errors.New("lxstamper: sha256 midstate unsupported")
	}
	if err := un.UnmarshalBinary(ms.Marshaled); err != nil {
		return [32]byte{}, err
	}
	if _, err := d.Write(stamp); err != nil {
		return [32]byte{}, err
	}
	var out [32]byte
	d.Sum(out[:0])
	return out, nil
}
