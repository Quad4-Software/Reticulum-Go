// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package selfcheck runs host OS preflight checks for reticulum-go.
package selfcheck

import (
	"context"
	"os"
	"runtime"
	"runtime/debug"
)

const (
	envChild     = "RETICULUM_SELFCHECK_CHILD"
	envChildDir  = "RETICULUM_SELFCHECK_DIR"
	childSandbox = "sandbox"
)

func goos() string   { return runtime.GOOS }
func goarch() string { return runtime.GOARCH }

func goVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.GoVersion != "" {
		return bi.GoVersion
	}
	return runtime.Version()
}

// ChildExitCode reports whether this process is a sandbox self-check child.
// When handled is true, callers in main or TestMain should os.Exit(code).
func ChildExitCode() (code int, handled bool) {
	if os.Getenv(envChild) != childSandbox {
		return 0, false
	}
	return runSandboxChild(), true
}

// Run executes the self-check catalog and returns a report.
func Run(ctx context.Context, opts Options) Report {
	if ctx == nil {
		ctx = context.Background()
	}
	rep := Report{
		GOOS:      goos(),
		GOARCH:    goarch(),
		GoVersion: goVersion(),
	}

	rep.Results = append(rep.Results, checkRuntime()...)
	rep.Results = append(rep.Results, checkCrypto()...)
	rep.Results = append(rep.Results, checkIdentityFile(opts)...)
	rep.Results = append(rep.Results, checkPacket())
	rep.Results = append(rep.Results, checkBinaryCLI(opts))

	rep.Results = append(rep.Results, checkSandbox(ctx, opts))
	rep.Results = append(rep.Results, checkSeccomp())
	rep.Results = append(rep.Results, checkSecuremem())
	rep.Results = append(rep.Results, checkKeyring())
	rep.Results = append(rep.Results, checkPoller())

	if !opts.Quick {
		rep.Results = append(rep.Results, checkUDP())
		rep.Results = append(rep.Results, checkTCP())
		rep.Results = append(rep.Results, checkLocalInterface())
		if opts.Full {
			rep.Results = append(rep.Results, checkQUIC())
			rep.Results = append(rep.Results, checkHTTPS())
			rep.Results = append(rep.Results, checkVSOCK())
			rep.Results = append(rep.Results, checkPipe())
			rep.Results = append(rep.Results, checkSerial())
		}
		if !opts.SkipDaemon {
			rep.Results = append(rep.Results, checkDaemon(ctx, opts))
		}
	}

	if opts.Interop || os.Getenv("RETICULUM_SELF_CHECK_INTEROP") == "1" {
		rep.Results = append(rep.Results, checkCrossref())
		rep.Results = append(rep.Results, checkPythonRNS())
		rep.Results = append(rep.Results, checkBindings())
	}

	return rep
}
