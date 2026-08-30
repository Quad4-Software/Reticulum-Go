// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"runtime"
	"sync"
)

// StampCandidate is one material+stamp pair for batch validation.
type StampCandidate struct {
	Material []byte
	Stamp    []byte
}

// ValidateStampBatch reports which candidates meet targetCost for the given
// expandRounds. Uses GPU streaming validation when available and the batch is
// large enough, otherwise a parallel CPU path. Results are LXStamper-compatible.
func ValidateStampBatch(cands []StampCandidate, targetCost, expandRounds int) []bool {
	out := make([]bool, len(cands))
	if len(cands) == 0 {
		return out
	}
	if expandRounds <= 0 {
		expandRounds = DeliveryRounds
	}

	ensureBackend()
	// GPU kernel caps material at 64 bytes and mishandles cost<=0 thresholding.
	// Stay on CPU for those cases so results match MeetsCost.
	useGPU := PreferredStampBackend() != "cpu" &&
		gpuEngine != nil &&
		len(cands) >= 4 &&
		expandRounds >= gpuWorkblockMinRounds &&
		targetCost > 0 &&
		!batchHasLongMaterial(cands)
	if useGPU {
		ok, err := gpuEngine.batchValidate(cands, targetCost, expandRounds)
		if err == nil {
			return ok
		}
		if PreferredStampBackend() == "gpu" {
			// Forced GPU failed: mark all false rather than silently changing semantics.
			return out
		}
	}
	return validateStampBatchCPU(cands, targetCost, expandRounds)
}

func batchHasLongMaterial(cands []StampCandidate) bool {
	for _, c := range cands {
		if len(c.Material) > 64 {
			return true
		}
	}
	return false
}

func validateStampBatchCPU(cands []StampCandidate, targetCost, expandRounds int) []bool {
	out := make([]bool, len(cands))
	workers := max(min(runtime.GOMAXPROCS(0), len(cands)), 1)
	var wg sync.WaitGroup
	chunk := (len(cands) + workers - 1) / workers
	for w := range workers {
		start := w * chunk
		if start >= len(cands) {
			break
		}
		end := min(start+chunk, len(cands))
		wg.Go(func() {
			for i := start; i < end; i++ {
				c := cands[i]
				if len(c.Stamp) != StampSize || len(c.Material) == 0 {
					continue
				}
				wb, err := stampWorkblockCPU(c.Material, expandRounds)
				if err != nil {
					continue
				}
				out[i] = MeetsCost(c.Stamp, targetCost, wb)
			}
		})
	}
	wg.Wait()
	return out
}

// ValidatePNStamps validates each transient message and returns entries meeting targetCost.
// Validation is parallel across messages.
func ValidatePNStamps(messages [][]byte, targetCost int) []PNStampEntry {
	if len(messages) == 0 {
		return nil
	}

	type slot struct {
		ok  bool
		ent PNStampEntry
	}
	slots := make([]slot, len(messages))
	workers := max(min(runtime.GOMAXPROCS(0), len(messages)), 1)
	var wg sync.WaitGroup
	chunk := (len(messages) + workers - 1) / workers
	for w := range workers {
		start := w * chunk
		if start >= len(messages) {
			break
		}
		end := min(start+chunk, len(messages))
		wg.Go(func() {
			for i := start; i < end; i++ {
				tid, lxm, value, stamp := ValidatePNStamp(messages[i], targetCost)
				if tid == nil {
					continue
				}
				slots[i] = slot{ok: true, ent: PNStampEntry{
					TransientID: append([]byte(nil), tid...),
					LxmData:     lxm,
					Value:       value,
					Stamp:       stamp,
				}}
			}
		})
	}
	wg.Wait()

	out := make([]PNStampEntry, 0, len(messages))
	for _, s := range slots {
		if s.ok {
			out = append(out, s.ent)
		}
	}
	return out
}
