// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sandbox

import (
	"io"
	"os/exec"
	"sync/atomic"
)

var execRlimits atomic.Bool

// SetExecRlimits enables conservative child rlimits for later StartLimited
// and OutputLimited calls. Safe to call before interface construction.
func SetExecRlimits(enabled bool) {
	execRlimits.Store(enabled)
}

// StartLimited starts cmd and applies child rlimits when enabled.
func StartLimited(cmd *exec.Cmd) error {
	if cmd == nil {
		return nil
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if execRlimits.Load() && cmd.Process != nil {
		applyChildRlimits(cmd.Process.Pid)
	}
	return nil
}

// OutputLimited runs cmd and returns stdout, applying child rlimits when enabled.
func OutputLimited(cmd *exec.Cmd) ([]byte, error) {
	if cmd == nil {
		return nil, nil
	}
	if !execRlimits.Load() {
		return cmd.Output()
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := StartLimited(cmd); err != nil {
		return nil, err
	}
	out, readErr := io.ReadAll(stdout)
	waitErr := cmd.Wait()
	if waitErr != nil {
		return out, waitErr
	}
	return out, readErr
}
