// SPDX-License-Identifier: LicenseRef-Reticulum

//go:build !((linux || darwin) && (amd64 || arm64)) || lxstamp_nogpu

package lxmf

import "context"

var gpuEngine *gpuEngineState

type gpuEngineState struct {
	vendor string
	name   string
}

func openGPUEngine() (*gpuEngineState, error) {
	return nil, ErrGPUUnavailable
}

func (e *gpuEngineState) generate(ctx context.Context, workblock []byte, stampCost int) ([]byte, int, error) {
	return nil, 0, ErrGPUUnavailable
}

func (e *gpuEngineState) workblock(material []byte, rounds int) ([]byte, error) {
	return nil, ErrGPUUnavailable
}

func (e *gpuEngineState) batchValidate(cands []StampCandidate, targetCost, expandRounds int) ([]bool, error) {
	return nil, ErrGPUUnavailable
}
