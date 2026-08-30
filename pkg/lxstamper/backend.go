// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
)

// StampBackend names the GenerateStamp implementation in use.
type StampBackend string

const (
	StampBackendCPU StampBackend = "cpu"
	StampBackendGPU StampBackend = "gpu"
)

// ErrGPUUnavailable means no usable OpenCL GPU was found or init failed.
var ErrGPUUnavailable = errors.New("lxstamper: gpu unavailable")

var (
	backendOnce   sync.Once
	cachedBackend StampBackend
	gpuInitErr    error
)

// PreferredStampBackend returns the configured preference: auto, cpu, or gpu.
// Set RNS_LXSTAMP_BACKEND to auto (default), cpu, or gpu.
func PreferredStampBackend() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RNS_LXSTAMP_BACKEND")))
	switch v {
	case "cpu", "gpu", "auto":
		return v
	default:
		return "auto"
	}
}

// ActiveStampBackend reports which backend the process will try first after init.
func ActiveStampBackend() StampBackend {
	ensureBackend()
	return cachedBackend
}

// GPUAvailable reports whether an OpenCL GPU backend initialized successfully.
func GPUAvailable() bool {
	ensureBackend()
	return cachedBackend == StampBackendGPU && gpuEngine != nil
}

// GPUDeviceInfo returns vendor and device name when a GPU backend is active.
func GPUDeviceInfo() (vendor, name string, ok bool) {
	ensureBackend()
	if gpuEngine == nil {
		return "", "", false
	}
	return gpuEngine.vendor, gpuEngine.name, true
}

func ensureBackend() {
	backendOnce.Do(func() {
		pref := PreferredStampBackend()
		if pref == "cpu" {
			cachedBackend = StampBackendCPU
			return
		}
		eng, err := openGPUEngine()
		if err != nil {
			gpuInitErr = errors.Join(ErrGPUUnavailable, err)
			cachedBackend = StampBackendCPU
			return
		}
		gpuEngine = eng
		cachedBackend = StampBackendGPU
	})
}

// GenerateStamp searches for a stamp meeting stampCost.
// With RNS_LXSTAMP_BACKEND=auto (default) it uses an OpenCL GPU when one is
// detected (NVIDIA, AMD, or Intel ICD) and falls back to CPU on any failure.
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

	pref := PreferredStampBackend()
	ensureBackend()

	if pref != "cpu" && gpuEngine != nil {
		stamp, value, err := gpuEngine.generate(ctx, wb, stampCost)
		if err == nil {
			return stamp, value, nil
		}
		if pref == "gpu" {
			return nil, 0, err
		}
	}
	if pref == "gpu" && gpuEngine == nil {
		if gpuInitErr != nil {
			return nil, 0, gpuInitErr
		}
		return nil, 0, ErrGPUUnavailable
	}
	return cpuMineStamp(ctx, wb, stampCost)
}

// GenerateStampCPU forces the CPU path (for benchmarks and tests).
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

// GenerateStampGPU forces the GPU path. Returns ErrGPUUnavailable when no GPU.
func GenerateStampGPU(ctx context.Context, messageID []byte, stampCost, expandRounds int) ([]byte, int, error) {
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
	ensureBackend()
	if gpuEngine == nil {
		if gpuInitErr != nil {
			return nil, 0, gpuInitErr
		}
		return nil, 0, ErrGPUUnavailable
	}
	return gpuEngine.generate(ctx, wb, stampCost)
}
