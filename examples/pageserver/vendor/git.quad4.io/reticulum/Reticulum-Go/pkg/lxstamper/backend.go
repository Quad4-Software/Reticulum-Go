// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"context"
	"errors"
)

// GenerateStamp searches for a stamp meeting stampCost using the parallel
// CPU path. The former OpenCL backend was removed. Stamps are byte-identical
// either way, so this is a performance simplification, not a wire change.
func GenerateStamp(ctx context.Context, messageID []byte, stampCost, expandRounds int) ([]byte, int, error) {
	return GenerateStampCPU(ctx, messageID, stampCost, expandRounds)
}

// GenerateStampCPU searches for a stamp meeting stampCost on the CPU.
func GenerateStampCPU(ctx context.Context, messageID []byte, stampCost, expandRounds int) ([]byte, int, error) {
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
	return cpuMineStamp(ctx, wb, stampCost)
}
