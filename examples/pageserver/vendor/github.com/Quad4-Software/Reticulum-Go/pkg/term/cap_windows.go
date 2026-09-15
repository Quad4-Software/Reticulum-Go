// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package term

import (
	"os"
	"sync"

	"golang.org/x/sys/windows"
	xterm "golang.org/x/term"
)

var (
	vtMu     sync.Mutex
	vtReady  = make(map[uintptr]bool)
	vtFailed = make(map[uintptr]bool)
)

func prepareColorFile(f *os.File) bool {
	if f == nil {
		return false
	}
	if !xterm.IsTerminal(int(f.Fd())) {
		return false
	}
	fd := f.Fd()
	vtMu.Lock()
	defer vtMu.Unlock()
	if vtReady[fd] {
		return true
	}
	if vtFailed[fd] {
		return false
	}
	if os.Getenv("WT_SESSION") != "" || os.Getenv("ANSICON") != "" {
		vtReady[fd] = true
		return true
	}
	handle := windows.Handle(fd)
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		vtFailed[fd] = true
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		vtReady[fd] = true
		return true
	}
	mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	if err := windows.SetConsoleMode(handle, mode); err != nil {
		vtFailed[fd] = true
		return false
	}
	vtReady[fd] = true
	return true
}
