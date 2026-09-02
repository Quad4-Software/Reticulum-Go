// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"time"

	"quad4/reticulum-go/pkg/selfcheck"
)

// RunSelfCheck runs the host OS preflight checklist.
func RunSelfCheck(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("self-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit JSON report")
	quick := fs.Bool("quick", false, "core and platform only")
	full := fs.Bool("full", false, "include optional interface probes")
	interop := fs.Bool("interop", false, "include optional external tool checks")
	strict := fs.Bool("strict", false, "treat warnings as failures")
	binary := fs.String("binary", "", "path to reticulum-go for daemon and CLI checks")
	timeout := fs.Duration("timeout", 45*time.Second, "overall check timeout")
	bindFlagUsage(fs, "reticulum-go self-check - host OS preflight",
		"Runs platform checks for dependencies, permissions, and optional interop tools.",
		[]helpLine{
			{Cmd: "reticulum-go self-check [flags]"},
			{Cmd: "rgoselfcheck [flags]"},
		},
		"reticulum-go self-check",
		"reticulum-go self-check -json",
		"reticulum-go self-check -full -strict",
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	binPath := *binary
	if binPath == "" {
		if exe, err := os.Executable(); err == nil {
			binPath = exe
		}
	}
	if binPath != "" {
		if abs, err := filepath.Abs(binPath); err == nil {
			binPath = abs
		}
	}

	opts := selfcheck.Options{
		Quick:      *quick,
		Full:       *full,
		Interop:    *interop || os.Getenv("RETICULUM_SELF_CHECK_INTEROP") == "1",
		Strict:     *strict,
		BinaryPath: binPath,
		Timeout:    *timeout,
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	rep := selfcheck.Run(ctx, opts)
	if *jsonOut {
		if err := rep.FormatJSON(stdout); err != nil {
			diagErr(stderr, "json", err)
			return 1
		}
	} else {
		if err := rep.FormatText(stdout); err != nil {
			diagErr(stderr, "report", err)
			return 1
		}
	}
	return rep.ExitCode(*strict)
}
