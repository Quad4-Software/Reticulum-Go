// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build windows

package sandbox

import (
	"unsafe"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"golang.org/x/sys/windows"
)

func applyPlatform(cfg *common.ReticulumConfig) error {
	if err := applyJobLimits(); err != nil {
		debug.Log(debug.DebugError, "Job object limits failed", "error", err)
	}

	if err := disableMiniDumps(); err != nil {
		debug.Log(debug.DebugError, "Disable mini-dumps failed", "error", err)
	}

	debug.Log(debug.DebugInfo, "Sandbox applied", "platform", "windows")
	return nil
}

func applyJobLimits() error {
	hJob, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(hJob)

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS |
				windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY |
				windows.JOB_OBJECT_LIMIT_JOB_MEMORY,
			ActiveProcessLimit: 256,
		},
		ProcessMemoryLimit: 2 << 30, // 2 GiB per process
		JobMemoryLimit:     3 << 30, // 3 GiB total for the job
	}

	_, err = windows.SetInformationJobObject(
		hJob,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		return err
	}

	hProcess, err := windows.GetCurrentProcess()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(hProcess)

	return windows.AssignProcessToJobObject(hJob, hProcess)
}

func disableMiniDumps() error {
	windows.SetErrorMode(windows.SEM_FAILCRITICALERRORS | windows.SEM_NOGPFAULTERRORBOX)
	return nil
}
