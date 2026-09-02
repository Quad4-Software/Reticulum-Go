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
	CmdSH         = "sh"
	CmdPageserver = "pageserver"
	CmdDebug      = "debug"
	CmdSlow       = "slow"
	CmdSelfCheck  = "self-check"
	CmdSpeedtest  = "speedtest"
	CmdDump       = "dump"
	CmdSnapshot   = "snapshot"
	CmdZen        = "zen"
	CmdGit        = "git"
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

	base := strings.ToLower(filepath.Base(opt.Argv0))
	base = strings.TrimSuffix(base, ".exe")
	if base == "git-remote-rns" {
		return RunGitRemoteRNS(args, opt)
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
	case CmdSH:
		return RunSH(rest, opt)
	case CmdPageserver:
		return RunPageserver(rest, opt)
	case CmdDebug:
		return RunDebug(rest, opt)
	case CmdSlow:
		return RunSlow(rest, opt)
	case CmdSelfCheck:
		return RunSelfCheck(rest, opt)
	case CmdSpeedtest:
		return RunSpeedtest(rest, opt)
	case CmdDump:
		return RunDump(rest, opt)
	case CmdSnapshot:
		return RunSnapshot(rest, opt)
	case CmdZen:
		return RunZen(rest, opt)
	case CmdGit:
		if len(rest) > 0 && rest[0] == "remote-rns" {
			return RunGitRemoteRNS(rest[1:], opt)
		}
		return RunGit(rest, opt)
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
	case CmdDaemon, CmdStatus, CmdID, CmdProbe, CmdPath, CmdCP, CmdX, CmdSH, CmdPageserver, CmdDebug, CmdSlow, CmdSelfCheck, CmdSpeedtest, CmdDump, CmdSnapshot, CmdZen, CmdGit:
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
	case "rgosh":
		return CmdSH, args[1:], true
	case "rgoslow":
		return CmdSlow, args[1:], true
	case "rgospeed":
		return CmdSpeedtest, args[1:], true
	case "rgodump":
		return CmdDump, args[1:], true
	case "rgosnap":
		return CmdSnapshot, args[1:], true
	case "rgozen", "reticulum-go-zen":
		return CmdZen, args[1:], true
	case "rgogit", "reticulum-go-git", "git-remote-rns":
		return CmdGit, args[1:], true
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
	case "rgosh", "reticulum-go-sh":
		return CmdSH
	case "rgopageserver", "reticulum-go-pageserver":
		return CmdPageserver
	case "rgoslow", "reticulum-go-slow":
		return CmdSlow
	case "rgoselfcheck", "reticulum-go-self-check":
		return CmdSelfCheck
	case "rgospeed", "reticulum-go-speedtest":
		return CmdSpeedtest
	case "rgodump", "reticulum-go-dump":
		return CmdDump
	case "rgosnap", "reticulum-go-snapshot":
		return CmdSnapshot
	case "rgozen", "reticulum-go-zen":
		return CmdZen
	case "rgogit", "reticulum-go-git", "git-remote-rns":
		return CmdGit
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
func dimMsg(w io.Writer, s string) string  { return term.DimW(w, s) }
func boldMsg(w io.Writer, s string) string { return term.BoldW(w, s) }

func diagErr(w io.Writer, label string, err error) {
	fmt.Fprintf(w, "%s: %v\n", errMsg(w, label), err)
}

func printRootHelp(w io.Writer) {
	helpTitle(w, "reticulum-go - Reticulum network stack (Go)")
	helpUsageHeader(w)
	helpUsageLines(w,
		helpLine{"reticulum-go", "run the network daemon"},
		helpLine{"reticulum-go daemon [flags]", "run the network daemon"},
		helpLine{"reticulum-go status [flags]", "interface and transport status (RPC)"},
		helpLine{"reticulum-go id [flags]", "identities, hashes, sign and encrypt"},
		helpLine{"reticulum-go probe [flags] <name> <hash>", ""},
		helpLine{"reticulum-go path [flags]", "path table, drop, blackhole"},
		helpLine{"reticulum-go slow [flags]", "find slow interfaces, paths, transfers"},
		helpLine{"reticulum-go speedtest [flags]", "loopback link throughput smoke"},
		helpLine{"reticulum-go dump [flags]", "decode RNS packets from hex or pcap"},
		helpLine{"reticulum-go snapshot [flags]", "path table, links, and health JSON (RPC)"},
		helpLine{"reticulum-go cp [flags]", "file transfer over links"},
		helpLine{"reticulum-go x [flags]", "remote command execution (rnx)"},
		helpLine{"reticulum-go sh [flags]", "interactive remote shell (rgosh)"},
		helpLine{"reticulum-go git [flags]", "Git-over-Reticulum node (rngit)"},
		helpLine{"reticulum-go pageserver [flags]", "NomadNet-style page and file server"},
		helpLine{"reticulum-go debug [flags]", "effective config, rate table, RPC dump"},
		helpLine{"reticulum-go self-check [flags]", "host OS preflight checklist"},
		helpLine{"reticulum-go zen [flags] [packages]", "scan for path/link footguns (go fix style)"},
	)
	fmt.Fprintln(w)
	helpSection(w, "Global:")
	fmt.Fprintf(w, "  %s       print this help\n", dimMsg(w, "-h, --help"))
	fmt.Fprintf(w, "  %s    print version\n", dimMsg(w, "-v, --version"))
	fmt.Fprintln(w)
	helpNote(w, "Legacy tool names (rgostatus, rgoid, rgoprobe, rgopath, rgocp, rgox, rnx, rgosh, rgoslow, rgospeed, rgodump, rgosnap, rgozen) work as subcommands or when the binary is installed under those names (symlinks).")
	helpNote(w, "Configuration defaults to ~/.reticulum-go/config.")
}
