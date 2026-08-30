// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build (linux || darwin || windows) && !lxstamp_nogpu

package lxstamper

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

var gpuEngine *gpuEngineState

type gpuEngineState struct {
	api     *openclAPI
	device  clDeviceID
	ctx     clContext
	queue   clCommandQueue
	program clProgram
	kernel  clKernel
	vendor  string
	name    string
	mu      sync.Mutex
}

type clCandidate struct {
	platform clPlatformID
	device   clDeviceID
	vendor   string
	name     string
	cus      uint32
	score    int
}

func openGPUEngine() (*gpuEngineState, error) {
	api, err := loadOpenCL()
	if err != nil {
		return nil, err
	}
	dev, err := pickGPUDevice(api)
	if err != nil {
		return nil, err
	}
	var st int32
	device := dev.device
	ctx := api.createContext(nil, 1, &device, 0, 0, &st)
	if st != clSuccess || ctx == 0 {
		return nil, fmt.Errorf("opencl: create context status %d", st)
	}
	queue := api.createQueue(ctx, device, 0, &st)
	if st != clSuccess || queue == 0 {
		api.releaseContext(ctx)
		return nil, fmt.Errorf("opencl: create queue status %d", st)
	}
	srcBytes := append([]byte(lxstampOpenCLKernel), 0)
	srcPtr := &srcBytes[0]
	srcPtrs := []*byte{srcPtr}
	length := uintptr(len(lxstampOpenCLKernel))
	program := api.createProgram(ctx, 1, &srcPtrs[0], &length, &st)
	if st != clSuccess || program == 0 {
		api.releaseQueue(queue)
		api.releaseContext(ctx)
		return nil, fmt.Errorf("opencl: create program status %d", st)
	}
	if st := api.buildProgram(program, 1, &device, cString("-cl-std=CL1.2"), 0, 0); st != clSuccess {
		log := programBuildLog(api, program, device)
		api.releaseProgram(program)
		api.releaseQueue(queue)
		api.releaseContext(ctx)
		return nil, fmt.Errorf("opencl: build status %d: %s", st, log)
	}
	kernel := api.createKernel(program, cString("lxstamp_search"), &st)
	if st != clSuccess || kernel == 0 {
		api.releaseProgram(program)
		api.releaseQueue(queue)
		api.releaseContext(ctx)
		return nil, fmt.Errorf("opencl: create kernel status %d", st)
	}
	return &gpuEngineState{
		api:     api,
		device:  device,
		ctx:     ctx,
		queue:   queue,
		program: program,
		kernel:  kernel,
		vendor:  dev.vendor,
		name:    dev.name,
	}, nil
}

func programBuildLog(api *openclAPI, program clProgram, device clDeviceID) string {
	var n uintptr
	if st := api.getBuildInfo(program, device, clProgramBuildLog, 0, nil, &n); st != clSuccess || n == 0 {
		return ""
	}
	buf := make([]byte, n)
	if st := api.getBuildInfo(program, device, clProgramBuildLog, n, unsafe.Pointer(&buf[0]), nil); st != clSuccess {
		return ""
	}
	return strings.TrimRight(string(buf), "\x00")
}

func pickGPUDevice(api *openclAPI) (*clCandidate, error) {
	var nPlat uint32
	if st := api.getPlatformIDs(0, nil, &nPlat); st != clSuccess || nPlat == 0 {
		return nil, fmt.Errorf("opencl: no platforms (status %d)", st)
	}
	plats := make([]clPlatformID, nPlat)
	if st := api.getPlatformIDs(nPlat, &plats[0], nil); st != clSuccess {
		return nil, fmt.Errorf("opencl: list platforms status %d", st)
	}
	var best *clCandidate
	for _, p := range plats {
		for _, dtype := range []uint64{clDeviceTypeGPU, clDeviceTypeAccelerator} {
			var nDev uint32
			if st := api.getDeviceIDs(p, dtype, 0, nil, &nDev); st != clSuccess || nDev == 0 {
				continue
			}
			devs := make([]clDeviceID, nDev)
			if st := api.getDeviceIDs(p, dtype, nDev, &devs[0], nil); st != clSuccess {
				continue
			}
			for _, d := range devs {
				c, err := describeDevice(api, p, d)
				if err != nil {
					continue
				}
				if best == nil || c.score > best.score || (c.score == best.score && c.cus > best.cus) {
					cp := c
					best = &cp
				}
			}
		}
	}
	if best == nil {
		return nil, fmt.Errorf("opencl: no GPU or accelerator devices (NVIDIA/AMD/Intel ICD)")
	}
	return best, nil
}

func describeDevice(api *openclAPI, platform clPlatformID, device clDeviceID) (clCandidate, error) {
	name, err := api.infoString(func(param uint32, size uintptr, ptr unsafe.Pointer, sz *uintptr) int32 {
		return api.getDeviceInfo(device, param, size, ptr, sz)
	}, clDeviceName)
	if err != nil {
		return clCandidate{}, err
	}
	vendor, err := api.infoString(func(param uint32, size uintptr, ptr unsafe.Pointer, sz *uintptr) int32 {
		return api.getDeviceInfo(device, param, size, ptr, sz)
	}, clDeviceVendor)
	if err != nil {
		vendor = ""
	}
	var cus uint32
	api.getDeviceInfo(device, clDeviceMaxCU, unsafe.Sizeof(cus), unsafe.Pointer(&cus), nil)
	return clCandidate{
		platform: platform,
		device:   device,
		vendor:   vendor,
		name:     name,
		cus:      cus,
		score:    vendorScore(vendor, name),
	}, nil
}

func vendorScore(vendor, name string) int {
	s := strings.ToLower(vendor + " " + name)
	switch {
	case strings.Contains(s, "nvidia"), strings.Contains(s, "geforce"), strings.Contains(s, "quadro"), strings.Contains(s, "tesla"), strings.Contains(s, "rtx"):
		return 300
	case strings.Contains(s, "amd"), strings.Contains(s, "advanced micro"), strings.Contains(s, "radeon"), strings.Contains(s, "instinct"):
		return 300
	case strings.Contains(s, "intel"), strings.Contains(s, "arc"), strings.Contains(s, "xe "):
		return 280
	default:
		return 100
	}
}

func (e *gpuEngineState) generate(ctx context.Context, workblock []byte, stampCost int) ([]byte, int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ms, err := midstateOfPrefix(workblock)
	if err != nil {
		return nil, 0, err
	}
	target := stampTarget(stampCost)
	totalBits := uint64(len(workblock)+StampSize) * 8

	midWords := make([]uint32, 8)
	copy(midWords, ms.H[:])
	rem := make([]byte, 64)
	copy(rem, ms.Rem)
	remLen := uint32(len(ms.Rem))

	var seedBytes [8]byte
	if _, err := rand.Read(seedBytes[:]); err != nil {
		return nil, 0, err
	}
	seed := binary.BigEndian.Uint64(seedBytes[:])

	const batch = uint64(1 << 20)
	var base uint64

	api := e.api
	var st int32

	midBuf := api.createBuffer(e.ctx, clMemReadOnly|clMemCopyHostPtr, unsafe.Sizeof(midWords[0])*8, unsafe.Pointer(&midWords[0]), &st)
	if st != clSuccess {
		return nil, 0, fmt.Errorf("opencl: midstate buffer %d", st)
	}
	defer api.releaseMem(midBuf)

	remBuf := api.createBuffer(e.ctx, clMemReadOnly|clMemCopyHostPtr, 64, unsafe.Pointer(&rem[0]), &st)
	if st != clSuccess {
		return nil, 0, fmt.Errorf("opencl: rem buffer %d", st)
	}
	defer api.releaseMem(remBuf)

	tgtBuf := api.createBuffer(e.ctx, clMemReadOnly|clMemCopyHostPtr, 32, unsafe.Pointer(&target[0]), &st)
	if st != clSuccess {
		return nil, 0, fmt.Errorf("opencl: target buffer %d", st)
	}
	defer api.releaseMem(tgtBuf)

	foundBuf := api.createBuffer(e.ctx, clMemReadWrite, 4, nil, &st)
	if st != clSuccess {
		return nil, 0, fmt.Errorf("opencl: found buffer %d", st)
	}
	defer api.releaseMem(foundBuf)

	outBuf := api.createBuffer(e.ctx, clMemWriteOnly, StampSize, nil, &st)
	if st != clSuccess {
		return nil, 0, fmt.Errorf("opencl: out buffer %d", st)
	}
	defer api.releaseMem(outBuf)

	zero := uint32(0)
	if st := api.enqueueWrite(e.queue, foundBuf, 1, 0, 4, unsafe.Pointer(&zero), 0, 0, 0); st != clSuccess {
		return nil, 0, fmt.Errorf("opencl: clear found %d", st)
	}

	setU32 := func(idx uint32, v uint32) error {
		vv := v
		if st := api.setKernelArg(e.kernel, idx, unsafe.Sizeof(vv), unsafe.Pointer(&vv)); st != clSuccess {
			return fmt.Errorf("set arg %d: %d", idx, st)
		}
		return nil
	}
	setU64 := func(idx uint32, v uint64) error {
		vv := v
		if st := api.setKernelArg(e.kernel, idx, unsafe.Sizeof(vv), unsafe.Pointer(&vv)); st != clSuccess {
			return fmt.Errorf("set arg %d: %d", idx, st)
		}
		return nil
	}
	setMem := func(idx uint32, m clMem) error {
		mm := m
		if st := api.setKernelArg(e.kernel, idx, unsafe.Sizeof(mm), unsafe.Pointer(&mm)); st != clSuccess {
			return fmt.Errorf("set mem arg %d: %d", idx, st)
		}
		return nil
	}

	if err := setMem(0, midBuf); err != nil {
		return nil, 0, err
	}
	if err := setMem(1, remBuf); err != nil {
		return nil, 0, err
	}
	if err := setU32(2, remLen); err != nil {
		return nil, 0, err
	}
	if err := setU64(3, totalBits); err != nil {
		return nil, 0, err
	}
	if err := setMem(4, tgtBuf); err != nil {
		return nil, 0, err
	}
	if err := setU64(6, seed); err != nil {
		return nil, 0, err
	}
	if err := setMem(7, foundBuf); err != nil {
		return nil, 0, err
	}
	if err := setMem(8, outBuf); err != nil {
		return nil, 0, err
	}

	for {
		select {
		case <-ctx.Done():
			return nil, 0, ErrStampNotFound
		default:
		}
		if err := setU64(5, base); err != nil {
			return nil, 0, err
		}
		global := uintptr(batch)
		if st := api.enqueueNDRange(e.queue, e.kernel, 1, nil, &global, nil, 0, 0, 0); st != clSuccess {
			return nil, 0, fmt.Errorf("opencl: enqueue %d", st)
		}
		if st := api.finish(e.queue); st != clSuccess {
			return nil, 0, fmt.Errorf("opencl: finish %d", st)
		}
		var found uint32
		if st := api.enqueueRead(e.queue, foundBuf, 1, 0, 4, unsafe.Pointer(&found), 0, 0, 0); st != clSuccess {
			return nil, 0, fmt.Errorf("opencl: read found %d", st)
		}
		if found != 0 {
			stamp := make([]byte, StampSize)
			if st := api.enqueueRead(e.queue, outBuf, 1, 0, StampSize, unsafe.Pointer(&stamp[0]), 0, 0, 0); st != clSuccess {
				return nil, 0, fmt.Errorf("opencl: read stamp %d", st)
			}
			sum := hashWorkblockStamp(workblock, stamp)
			tgt := stampTarget(stampCost)
			if bytes.Compare(sum[:], tgt[:]) > 0 {
				return nil, 0, fmt.Errorf("opencl: gpu stamp failed cpu verify")
			}
			if !StampValid(stamp, stampCost, workblock) {
				return nil, 0, fmt.Errorf("opencl: gpu stamp failed StampValid")
			}
			return stamp, StampValue(workblock, stamp), nil
		}
		base += batch
		if st := api.enqueueWrite(e.queue, foundBuf, 1, 0, 4, unsafe.Pointer(&zero), 0, 0, 0); st != clSuccess {
			return nil, 0, fmt.Errorf("opencl: reset found %d", st)
		}
	}
}
