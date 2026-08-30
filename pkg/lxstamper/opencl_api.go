// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build (linux || darwin || windows) && !lxstamp_nogpu

package lxstamper

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

type clPlatformID = uintptr
type clDeviceID = uintptr
type clContext = uintptr
type clCommandQueue = uintptr
type clProgram = uintptr
type clKernel = uintptr
type clMem = uintptr

const (
	clSuccess             = 0
	clDeviceTypeGPU       = 1 << 2
	clDeviceTypeAccelerator = 1 << 3
	clMemReadOnly         = 1 << 2
	clMemWriteOnly        = 1 << 1
	clMemReadWrite        = 1 << 0
	clMemCopyHostPtr      = 1 << 5

	clPlatformName   = 0x0902
	clDeviceName     = 0x102B
	clDeviceVendor   = 0x102C
	clDeviceType     = 0x1000
	clDeviceMaxCU    = 0x1021
	clProgramBuildLog = 0x1183
)

type openclAPI struct {
	lib uintptr

	getPlatformIDs   func(uint32, *clPlatformID, *uint32) int32
	getPlatformInfo  func(clPlatformID, uint32, uintptr, unsafe.Pointer, *uintptr) int32
	getDeviceIDs     func(clPlatformID, uint64, uint32, *clDeviceID, *uint32) int32
	getDeviceInfo    func(clDeviceID, uint32, uintptr, unsafe.Pointer, *uintptr) int32
	createContext    func(*int32, uint32, *clDeviceID, uintptr, uintptr, *int32) clContext
	createQueue      func(clContext, clDeviceID, uint64, *int32) clCommandQueue
	createProgram    func(clContext, uint32, **byte, *uintptr, *int32) clProgram
	buildProgram     func(clProgram, uint32, *clDeviceID, *byte, uintptr, uintptr) int32
	getBuildInfo     func(clProgram, clDeviceID, uint32, uintptr, unsafe.Pointer, *uintptr) int32
	createKernel     func(clProgram, *byte, *int32) clKernel
	createBuffer     func(clContext, uint64, uintptr, unsafe.Pointer, *int32) clMem
	setKernelArg     func(clKernel, uint32, uintptr, unsafe.Pointer) int32
	enqueueNDRange   func(clCommandQueue, clKernel, uint32, *uintptr, *uintptr, *uintptr, uint32, uintptr, uintptr) int32
	enqueueRead      func(clCommandQueue, clMem, uint32, uintptr, uintptr, unsafe.Pointer, uint32, uintptr, uintptr) int32
	enqueueWrite     func(clCommandQueue, clMem, uint32, uintptr, uintptr, unsafe.Pointer, uint32, uintptr, uintptr) int32
	finish           func(clCommandQueue) int32
	releaseMem       func(clMem) int32
	releaseKernel    func(clKernel) int32
	releaseProgram   func(clProgram) int32
	releaseQueue     func(clCommandQueue) int32
	releaseContext   func(clContext) int32
}

func openCLLibNames() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"OpenCL.dll"}
	case "darwin":
		return []string{"/System/Library/Frameworks/OpenCL.framework/OpenCL", "libOpenCL.dylib"}
	default:
		return []string{"libOpenCL.so.1", "libOpenCL.so"}
	}
}

func loadOpenCL() (*openclAPI, error) {
	var lib uintptr
	var err error
	for _, name := range openCLLibNames() {
		lib, err = purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			break
		}
	}
	if lib == 0 {
		return nil, fmt.Errorf("opencl: load library: %w", err)
	}
	api := &openclAPI{lib: lib}
	purego.RegisterLibFunc(&api.getPlatformIDs, lib, "clGetPlatformIDs")
	purego.RegisterLibFunc(&api.getPlatformInfo, lib, "clGetPlatformInfo")
	purego.RegisterLibFunc(&api.getDeviceIDs, lib, "clGetDeviceIDs")
	purego.RegisterLibFunc(&api.getDeviceInfo, lib, "clGetDeviceInfo")
	purego.RegisterLibFunc(&api.createContext, lib, "clCreateContext")
	purego.RegisterLibFunc(&api.createQueue, lib, "clCreateCommandQueue")
	purego.RegisterLibFunc(&api.createProgram, lib, "clCreateProgramWithSource")
	purego.RegisterLibFunc(&api.buildProgram, lib, "clBuildProgram")
	purego.RegisterLibFunc(&api.getBuildInfo, lib, "clGetProgramBuildInfo")
	purego.RegisterLibFunc(&api.createKernel, lib, "clCreateKernel")
	purego.RegisterLibFunc(&api.createBuffer, lib, "clCreateBuffer")
	purego.RegisterLibFunc(&api.setKernelArg, lib, "clSetKernelArg")
	purego.RegisterLibFunc(&api.enqueueNDRange, lib, "clEnqueueNDRangeKernel")
	purego.RegisterLibFunc(&api.enqueueRead, lib, "clEnqueueReadBuffer")
	purego.RegisterLibFunc(&api.enqueueWrite, lib, "clEnqueueWriteBuffer")
	purego.RegisterLibFunc(&api.finish, lib, "clFinish")
	purego.RegisterLibFunc(&api.releaseMem, lib, "clReleaseMemObject")
	purego.RegisterLibFunc(&api.releaseKernel, lib, "clReleaseKernel")
	purego.RegisterLibFunc(&api.releaseProgram, lib, "clReleaseProgram")
	purego.RegisterLibFunc(&api.releaseQueue, lib, "clReleaseCommandQueue")
	purego.RegisterLibFunc(&api.releaseContext, lib, "clReleaseContext")
	return api, nil
}

func (a *openclAPI) infoString(get func(uint32, uintptr, unsafe.Pointer, *uintptr) int32, param uint32) (string, error) {
	var n uintptr
	if st := get(param, 0, nil, &n); st != clSuccess {
		return "", fmt.Errorf("opencl info size status %d", st)
	}
	buf := make([]byte, n)
	if st := get(param, n, unsafe.Pointer(&buf[0]), nil); st != clSuccess {
		return "", fmt.Errorf("opencl info status %d", st)
	}
	for len(buf) > 0 && buf[len(buf)-1] == 0 {
		buf = buf[:len(buf)-1]
	}
	return string(buf), nil
}

func cString(s string) *byte {
	b := append([]byte(s), 0)
	return &b[0]
}
