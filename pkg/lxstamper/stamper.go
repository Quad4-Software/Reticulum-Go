// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package lxstamper implements LXStamper-compatible proof-of-work stamps.
// Discovery uses DiscoveryRounds. Delivery, propagation, and peering round
// counts match Python LXMF for callers outside this module.
package lxstamper

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/cryptography"
)

// Workblock expansion rounds aligned with LXStamper and RNS Discovery.
const (
	DiscoveryRounds   = 20
	DeliveryRounds    = 3000
	PropagationRounds = 1000
	PeeringRounds     = 25

	StampSize = 32
)

// ErrStampNotFound means GenerateStamp ended before finding a stamp.
var ErrStampNotFound = errors.New("lxstamper: stamp generation cancelled")

// StampWorkblock returns the HKDF-expanded workblock (256 * expandRounds bytes).
func StampWorkblock(material []byte, expandRounds int) ([]byte, error) {
	if expandRounds <= 0 {
		return nil, errors.New("lxstamper: expandRounds must be positive")
	}
	if len(material) == 0 {
		return nil, errors.New("lxstamper: workblock material required")
	}

	out := make([]byte, 0, 256*expandRounds)
	saltSrc := make([]byte, 0, len(material)+16)
	nBuf := make([]byte, 0, 16)
	for n := range expandRounds {
		var err error
		nBuf, err = msgpack.AppendMarshal(nBuf[:0], n)
		if err != nil {
			return nil, fmt.Errorf("lxstamper: workblock msgpack: %w", err)
		}
		saltSrc = append(saltSrc[:0], material...)
		saltSrc = append(saltSrc, nBuf...)
		saltSum := sha256.Sum256(saltSrc)
		block, err := cryptography.DeriveKey(material, saltSum[:], nil, 256)
		if err != nil {
			return nil, fmt.Errorf("lxstamper: workblock hkdf: %w", err)
		}
		out = append(out, block...)
	}
	return out, nil
}

func hashWorkblockStamp(workblock, stamp []byte) [32]byte {
	h := sha256.New()
	h.Write(workblock)
	h.Write(stamp)
	var sum [32]byte
	h.Sum(sum[:0])
	return sum
}

// StampValue returns the leading-zero-bit score of SHA256(workblock||stamp).
func StampValue(workblock, stamp []byte) int {
	if len(stamp) == 0 {
		return 0
	}
	sum := hashWorkblockStamp(workblock, stamp)

	value := 0
	for _, b := range sum {
		if b == 0 {
			value += 8
			continue
		}
		for bit := 7; bit >= 0; bit-- {
			if b&(1<<bit) != 0 {
				return value
			}
			value++
		}
		return value
	}
	return value
}

// StampValid reports whether the stamp satisfies targetCost against workblock
// using the LXStamper numeric threshold (not bit-count alone).
func StampValid(stamp []byte, targetCost int, workblock []byte) bool {
	if targetCost <= 0 {
		return true
	}
	if len(stamp) != StampSize {
		return false
	}
	if targetCost > 256 {
		return false
	}
	sum := hashWorkblockStamp(workblock, stamp)
	target := stampTarget(targetCost)
	return bytes.Compare(sum[:], target[:]) <= 0
}

// MeetsCost reports whether stamp passes both StampValid and StampValue >= cost.
// RNS Discovery accepts an announce only when both checks succeed.
func MeetsCost(stamp []byte, targetCost int, workblock []byte) bool {
	if !StampValid(stamp, targetCost, workblock) {
		return false
	}
	if targetCost <= 0 {
		return len(stamp) == StampSize
	}
	return StampValue(workblock, stamp) >= targetCost
}

// PNStampEntry is one validated propagation stamp batch entry.
type PNStampEntry struct {
	TransientID []byte
	LxmData     []byte
	Value       int
	Stamp       []byte
}

// ValidatePNStamps validates each transient message and returns entries meeting targetCost.
func ValidatePNStamps(messages [][]byte, targetCost int) []PNStampEntry {
	if len(messages) == 0 {
		return nil
	}
	out := make([]PNStampEntry, 0, len(messages))
	for _, transientData := range messages {
		tid, lxm, value, stamp := ValidatePNStamp(transientData, targetCost)
		if tid == nil {
			continue
		}
		out = append(out, PNStampEntry{
			TransientID: append([]byte(nil), tid...),
			LxmData:     lxm,
			Value:       value,
			Stamp:       stamp,
		})
	}
	return out
}

// ValidatePNStamp checks PN transient data (payload bytes + 32-byte stamp).
func ValidatePNStamp(transientData []byte, targetCost int) (transientID, lxmData []byte, value int, stamp []byte) {
	if len(transientData) <= StampSize {
		return nil, nil, 0, nil
	}
	cut := len(transientData) - StampSize
	lxm := transientData[:cut]
	st := transientData[cut:]
	tidSum := sha256.Sum256(lxm)
	wb, err := StampWorkblock(tidSum[:], PropagationRounds)
	if err != nil {
		return nil, nil, 0, nil
	}
	if !StampValid(st, targetCost, wb) {
		return nil, nil, 0, nil
	}
	return tidSum[:], append([]byte(nil), lxm...), StampValue(wb, st), append([]byte(nil), st...)
}

// ValidatePeeringKey checks peeringKey against targetCost using the peering workblock.
func ValidatePeeringKey(peeringID, peeringKey []byte, targetCost int) bool {
	wb, err := StampWorkblock(peeringID, PeeringRounds)
	if err != nil {
		return false
	}
	return StampValid(peeringKey, targetCost, wb)
}

// GenerateStamp searches for a stamp meeting stampCost. Parallel workers respect ctx.
func GenerateStamp(ctx context.Context, messageID []byte, stampCost, expandRounds int) ([]byte, int, error) {
	if stampCost <= 0 {
		return nil, 0, errors.New("lxstamper: stampCost must be positive")
	}
	if expandRounds <= 0 {
		expandRounds = DeliveryRounds
	}
	wb, err := StampWorkblock(messageID, expandRounds)
	if err != nil {
		return nil, 0, err
	}
	target := stampTarget(stampCost)

	workers := max(runtime.NumCPU(), 1)

	type result struct {
		stamp []byte
	}

	resCh := make(chan result, 1)
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	base := sha256.New()
	base.Write(wb)
	mar, ok := base.(encoding.BinaryMarshaler)
	var midState []byte
	if ok {
		midState, err = mar.MarshalBinary()
		if err != nil {
			midState = nil
		}
	}

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			candidate := make([]byte, StampSize)
			h := sha256.New()
			un, unOK := h.(encoding.BinaryUnmarshaler)
			useMid := unOK && len(midState) > 0
			var concat []byte
			if !useMid {
				concat = make([]byte, 0, len(wb)+StampSize)
			}
			for {
				select {
				case <-subCtx.Done():
					return
				default:
				}
				if _, err := rand.Read(candidate); err != nil {
					return
				}
				var sum [32]byte
				if useMid {
					if err := un.UnmarshalBinary(midState); err != nil {
						return
					}
					h.Write(candidate)
					h.Sum(sum[:0])
				} else {
					concat = append(concat[:0], wb...)
					concat = append(concat, candidate...)
					sum = sha256.Sum256(concat)
				}
				if bytes.Compare(sum[:], target[:]) <= 0 {
					stamp := append([]byte(nil), candidate...)
					select {
					case resCh <- result{stamp: stamp}:
						cancel()
					case <-subCtx.Done():
					}
					return
				}
			}
		})
	}

	go func() {
		wg.Wait()
		close(resCh)
	}()

	res, ok := <-resCh
	if !ok {
		return nil, 0, ErrStampNotFound
	}
	return res.stamp, StampValue(wb, res.stamp), nil
}

// GenerateStampWithDeadline wraps GenerateStamp with a deadline.
func GenerateStampWithDeadline(parent context.Context, messageID []byte, stampCost, expandRounds int, deadline time.Time) ([]byte, int, error) {
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	return GenerateStamp(ctx, messageID, stampCost, expandRounds)
}

func stampTarget(cost int) (target [32]byte) {
	if cost >= 256 {
		return target
	}
	pos := 256 - cost
	byteIdx := 31 - (pos / 8)
	bitIdx := pos % 8
	if byteIdx < 0 {
		return target
	}
	target[byteIdx] = 1 << bitIdx
	return target
}
