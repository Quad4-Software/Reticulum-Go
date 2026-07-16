// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package cli implements reticulum-go subcommands (daemon tools and pageserver).
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"quad4/reticulum-go/pkg/term"
)

// Command names accepted as subcommands or argv0 aliases.
const (
	CmdDaemon     = "daemon"
	CmdStatus     = "status"
	CmdID         = "id"
	CmdProbe      = "probe"
	CmdPath       = "path"
	CmdCP         = "cp"
	CmdX          = "x"
	CmdPageserver = "pageserver"
	CmdDebug      = "debug"
	CmdSlow       = "slow"
	CmdSelfCheck  = "self-check"
)

// DaemonFunc starts the network daemon. Injected by cmd/reticulum-go to avoid
// an import cycle with the daemon package main.
type DaemonFunc func(args []string) int

// Options configures the root dispatcher.
type Options struct {
	Stdout io.Writer
	Stderr io.Writer
	// Argv0 is os.Args[0] (used for symlink / rename aliases).
	Argv0 string
	// VersionLine is printed for -v / --version / version.
	VersionLine string
	// RunDaemon starts the daemon when selected.
	RunDaemon DaemonFunc
}

// Main dispatches based on argv0 aliases or the first argument.
// Returns a process exit code.
func Main(args []string, opt Options) int {
	if opt.Stdout == nil {
		opt.Stdout = os.Stdout
	}
	if opt.Stderr == nil {
		opt.Stderr = os.Stderr
	}

	cmd, rest, ok := resolveCommand(opt.Argv0, args)
	if !ok {
		if len(args) > 0 {
			switch args[0] {
			case "-h", "--help", "help":
				printRootHelp(opt.Stdout)
				return 0
			case "-v", "--version", "version":
				fmt.Fprintln(opt.Stdout, opt.VersionLine)
				return 0
			}
		}
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			// Bare binary, or daemon flags without an explicit "daemon" subcommand.
			if opt.RunDaemon == nil {
				fmt.Fprintln(opt.Stderr, "daemon entry not configured")
				return 1
			}
			return opt.RunDaemon(args)
		}
		fmt.Fprintf(opt.Stderr, "%s\n\n", errMsg(opt.Stderr, fmt.Sprintf("unknown command %q", args[0])))
		printRootHelp(opt.Stderr)
		return 2
	}

	switch cmd {
	case CmdDaemon:
		if opt.RunDaemon == nil {
			fmt.Fprintln(opt.Stderr, "daemon entry not configured")
			return 1
		}
		return opt.RunDaemon(rest)
	case CmdStatus:
		return RunStatus(rest, opt)
	case CmdID:
		return RunID(rest, opt)
	case CmdProbe:
		return RunProbe(rest, opt)
	case CmdPath:
		return RunPath(rest, opt)
	case CmdCP:
		return RunCP(rest, opt)
	case CmdX:
		return RunX(rest, opt)
	case CmdPageserver:
		return RunPageserver(rest, opt)
	case CmdDebug:
		return RunDebug(rest, opt)
	case CmdSlow:
		return RunSlow(rest, opt)
	case CmdSelfCheck:
		return RunSelfCheck(rest, opt)
	default:
		fmt.Fprintf(opt.Stderr, "unknown command %q\n\n", cmd)
		printRootHelp(opt.Stderr)
		return 2
	}
}

func resolveCommand(argv0 string, args []string) (cmd string, rest []string, ok bool) {
	base := strings.ToLower(filepath.Base(argv0))
	base = strings.TrimSuffix(base, ".exe")

	if alias := aliasFromArgv0(base); alias != "" {
		return alias, args, true
	}

	if len(args) == 0 {
		return "", nil, false
	}

	switch args[0] {
	case CmdDaemon, CmdStatus, CmdID, CmdProbe, CmdPath, CmdCP, CmdX, CmdPageserver, CmdDebug, CmdSlow, CmdSelfCheck:
		return args[0], args[1:], true
	case "selfcheck", "rgoselfcheck":
		return CmdSelfCheck, args[1:], true
	case "rgostatus":
		return CmdStatus, args[1:], true
	case "rgoid":
		return CmdID, args[1:], true
	case "rgoprobe":
		return CmdProbe, args[1:], true
	case "rgopath":
		return CmdPath, args[1:], true
	case "rgocp":
		return CmdCP, args[1:], true
	case "rgox", "rnx":
		return CmdX, args[1:], true
	case "rgoslow":
		return CmdSlow, args[1:], true
	default:
		return "", nil, false
	}
}

func aliasFromArgv0(base string) string {
	switch base {
	case "rgostatus", "reticulum-go-status":
		return CmdStatus
	case "rgoid", "reticulum-go-id":
		return CmdID
	case "rgoprobe", "reticulum-go-probe":
		return CmdProbe
	case "rgopath", "reticulum-go-path":
		return CmdPath
	case "rgocp", "reticulum-go-cp":
		return CmdCP
	case "rgox", "rnx", "reticulum-go-x":
		return CmdX
	case "rgopageserver", "reticulum-go-pageserver":
		return CmdPageserver
	case "rgoslow", "reticulum-go-slow":
		return CmdSlow
	case "rgoselfcheck", "reticulum-go-self-check":
		return CmdSelfCheck
	default:
		return ""
	}
}

func cliIO(opt []Options) (stdout, stderr io.Writer) {
	stdout, stderr = os.Stdout, os.Stderr
	if len(opt) == 0 {
		return stdout, stderr
	}
	o := opt[0]
	if o.Stdout != nil {
		stdout = o.Stdout
	}
	if o.Stderr != nil {
		stderr = o.Stderr
	}
	return stdout, stderr
}

func okMsg(w io.Writer, s string) string   { return term.GreenW(w, s) }
func errMsg(w io.Writer, s string) string  { return term.RedW(w, s) }
func warnMsg(w io.Writer, s string) string { return term.YellowW(w, s) }
func infoMsg(w io.Writer, s string) string { return term.CyanW(w, s) }

func printRootHelp(w io.Writer) {
	fmt.Fprintf(w, `reticulum-go - Reticulum network stack (Go)

Usage:
  reticulum-go                          run the network daemon
  reticulum-go daemon [flags]           run the network daemon
  reticulum-go status [flags]           interface and transport status (RPC)
  reticulum-go id [flags]               identities, hashes, sign and encrypt
  reticulum-go probe [flags] <name> <hash>
  reticulum-go path [flags]             path table, drop, blackhole
  reticulum-go slow [flags]             find slow interfaces, paths, transfers
  reticulum-go cp [flags]               file transfer over links
  reticulum-go x [flags]                remote command execution (rnx)
  reticulum-go pageserver [flags]       NomadNet-style page and file server
  reticulum-go debug [flags]            effective config, rate table, RPC dump
  reticulum-go self-check [flags]       host OS preflight checklist

Global:
  -h, --help       print this help
  -v, --version    print version

Legacy tool names (rgostatus, rgoid, rgoprobe, rgopath, rgocp, rgox, rnx, rgoslow) work as
subcommands or when the binary is installed under those names (symlinks).

Configuration defaults to ~/.reticulum-go/config.
`)
}
