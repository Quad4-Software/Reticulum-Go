// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Package stamps implements the RNS 1.6.0 stamp primitives (Stamps.py):
// fixed-point cost quantization, target computation, detached hash stamps,
// stamped X25519 keys, and destination stamp persistence.
package stamps

import (
	"crypto/sha256"
	"math"
	"math/big"
	"sort"

	"github.com/Quad4-Software/Reticulum-Go/pkg/cryptography"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

const (
	StampLength = 14 // RNS.Identity.STAMPLENGTH, bytes
	StampSize   = 32 // RNS.Stamps.STAMP_SIZE, default detached stamp size

	StampCostCap = 100 // RNS.Transport.STAMP_COST_CAP
	LinkCostCap  = 40  // RNS.Transport.LINK_COST_CAP

	WorkblockExpandChunk  = 256
	WorkblockExpandRounds = 3000

	mantissaBits = 33
	costFPBits   = 8
	costFP       = 1 << costFPBits
)

// StampWorkblock returns the HKDF-expanded workblock for material. An
// expandRounds of 0 (or negative) returns material itself, matching
// Stamps.stamp_workblock(expand_rounds=0).
func StampWorkblock(material []byte, expandRounds int) ([]byte, error) {
	if expandRounds <= 0 {
		return material, nil
	}
	out := make([]byte, 0, WorkblockExpandChunk*expandRounds)
	nBuf := make([]byte, 0, 16)
	for n := 0; n < expandRounds; n++ {
		var err error
		nBuf, err = msgpack.AppendMarshal(nBuf[:0], n)
		if err != nil {
			return nil, err
		}
		salt := sha256.Sum256(append(append([]byte{}, material...), nBuf...))
		chunk, err := cryptography.DeriveKey(material, salt[:], nil, WorkblockExpandChunk)
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
	}
	return out, nil
}

// CostToFP converts a float cost to the 8-bit-fraction fixed-point form,
// clamped to [0, StampCostCap*256]. Matches Stamps.cost_to_fp.
func CostToFP(cost float64) int {
	v := int(math.Floor(cost*costFP + 0.5))
	return max(0, min(v, StampCostCap*costFP))
}

func FPToBytes(fp int) [2]byte {
	return [2]byte{byte((fp >> costFPBits) & 0xFF), byte(fp & 0xFF)}
}

func CostToBytes(cost float64) [2]byte {
	return FPToBytes(CostToFP(cost))
}

func FPFromBytes(b []byte) int {
	return int(b[0])<<costFPBits | int(b[1])
}

func CostFromBytes(b []byte) float64 {
	return CostFromFP(FPFromBytes(b))
}

func CostFromFP(fp int) float64 {
	return float64(fp) / costFP
}

func QuantizeCost(cost float64) float64 {
	return CostFromFP(CostToFP(cost))
}

// TargetFromFP computes the 256-bit stamp acceptance target for a fixed-point
// cost. Matches Stamps.target_from_fp.
func TargetFromFP(fp int) *big.Int {
	v := 256*costFP - fp
	i, f := v/costFP, v%costFP
	t := new(big.Int).SetUint64(tab[f])
	t.Lsh(t, uint(i))
	return t.Rsh(t, mantissaBits)
}

func TargetFromCost(cost float64) *big.Int {
	return TargetFromFP(CostToFP(cost))
}

// StampValue returns the fixed-point value of a stamp as a float
// (value_fp/256), matching Stamps.stamp_value.
func StampValue(workblock, stamp []byte) float64 {
	hn := new(big.Int).SetBytes(fullHash(workblock, stamp))
	if hn.Sign() == 0 {
		return 256
	}
	nb := mantissaBits + 1
	bl := hn.BitLen()
	lz := 256 - bl

	var mn uint64
	if bl > nb {
		mn = new(big.Int).Rsh(hn, uint(bl-nb)).Uint64()
	} else {
		mn = new(big.Int).Lsh(hn, uint(nb-bl)).Uint64()
	}

	climb := sort.Search(costFP, func(i int) bool { return tab[i] > mn })
	valueFP := ((lz + 1) << costFPBits) - climb
	return CostFromFP(valueFP)
}

// StampValid reports whether sha256(workblock+stamp) <= target(cost),
// matching Stamps.stamp_valid.
func StampValid(stamp []byte, cost float64, workblock []byte) bool {
	target := TargetFromCost(cost)
	return hashLE(target, fullHash(workblock, stamp))
}

// StampValueLegacy returns the integer leading-zero value, matching
// Stamps.stamp_value_legacy.
func StampValueLegacy(workblock, stamp []byte) int {
	hn := new(big.Int).SetBytes(fullHash(workblock, stamp))
	return 256 - hn.BitLen()
}

func fullHash(workblock, stamp []byte) []byte {
	h := sha256.New()
	h.Write(workblock)
	h.Write(stamp)
	return h.Sum(nil)
}

// hashLE compares a big-endian 32-byte hash to target without per-round
// big.Int allocation.
func hashLE(target *big.Int, hash []byte) bool {
	if target.BitLen() > 256 {
		return true
	}
	var tb [32]byte
	target.FillBytes(tb[:])
	for i := range tb {
		if hash[i] != tb[i] {
			return hash[i] < tb[i]
		}
	}
	return true
}
