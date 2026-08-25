// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"strings"

	"quad4/reticulum-go/pkg/rnsgit"
	"quad4/reticulum-go/pkg/rnsutil"
	"quad4/reticulum-go/pkg/term"
)

// RunGit implements reticulum-go git (rgogit).
func RunGit(args []string, opt ...Options) int {
	stdout, _ := cliIO(opt)
	if len(args) > 0 && args[0] == "remote-rns" {
		return rnsgit.RunGitRemoteRNS(args[1:], os.Getenv("RNS_CONFIG"))
	}
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		printGitHelp(stdout)
		return 0
	}
	if len(args) == 0 {
		return runGitServer(args, opt...)
	}
	switch args[0] {
	case "create", "fork", "mirror", "sync", "perms", "release", "work":
		return runGitMgmt(args, opt...)
	case "-p", "--print-identity":
		return runGitPrintIdentity(opt...)
	case "-s", "--service":
		return runGitServer(append([]string{"-s"}, args[1:]...), opt...)
	default:
		return runGitServer(args, opt...)
	}
}

// RunGitRemoteRNS is the git-remote-rns entry point.
func RunGitRemoteRNS(args []string, opt ...Options) int {
	fs := flag.NewFlagSet("git-remote-rns", flag.ContinueOnError)
	_, stderr := cliIO(opt)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit structured progress events")
	configDir := fs.String("config", "", "rngit config directory")
	rnsDir := fs.String("rnsconfig", "", "Reticulum config directory")
	rnsConfig := os.Getenv("RNS_CONFIG")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *rnsDir != "" {
		rnsConfig = *rnsDir
	}
	if len(fs.Args()) < 2 {
		usageErr(stderr, "git-remote-rns <remote-name> <url>")
		return 1
	}
	url := fs.Args()[1]
	if !strings.HasPrefix(strings.ToLower(url), rnsgit.ProtoRNS) {
		fmt.Fprintln(stderr, "Invalid URL scheme. Must be rns://")
		return 1
	}
	destHex, group, repo, err := rnsgit.ParseRNSURL(url)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	cfgDir := *configDir
	if cfgDir == "" {
		cfgDir = os.Getenv("RNGIT_CONFIG")
	}
	client, err := rnsgit.NewClient(rnsgit.ClientOptions{
		ConfigDir:    cfgDir,
		RNSConfigDir: rnsConfig,
		DestHex:      destHex,
		Group:        group,
		Repo:         repo,
		JSONProgress: *jsonOut,
	})
	if err != nil {
		fmt.Fprintf(stderr, "git-remote-rns failed: %v\n", err)
		return 255
	}
	defer client.Close()
	ctx, cancel := rnsutil.CLIWaitContext(0)
	defer cancel()
	fmt.Fprint(os.Stderr, "Requesting path...")
	if err := client.RunGitHelper(ctx, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(stderr, "git-remote-rns failed: %v\n", err)
		return 255
	}
	return 0
}

func runGitServer(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("git", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		configDir   string
		rnsConfig   string
		printID     bool
		serviceMode bool
		identity    string
	)
	fs.StringVar(&configDir, "config", "", "rngit config directory")
	fs.StringVar(&rnsConfig, "rnsconfig", "", "Reticulum config directory")
	fs.StringVar(&identity, "identity", "", "server identity path")
	fs.BoolVar(&printID, "p", false, "print identity and destination hashes")
	fs.BoolVar(&printID, "print-identity", false, "same as -p")
	fs.BoolVar(&serviceMode, "s", false, "service mode")
	bindFlagUsage(fs, "reticulum-go git - Git over Reticulum (rngit-compatible)",
		"Run a repository node or print identity hashes. See reticulum-go git -h for all subcommands.",
		[]helpLine{
			{Cmd: "reticulum-go git [-config dir] [-rnsconfig dir] [-s]"},
			{Cmd: "reticulum-go git -p"},
		},
		"reticulum-go git -p",
		"reticulum-go git -config ~/.rngit",
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if configDir == "" {
		configDir = os.Getenv("RNGIT_CONFIG")
	}
	if err := rnsgit.EnsureServerConfig(configDir); err != nil {
		diagErr(stderr, "config", err)
		return 1
	}
	cfg, err := rnsgit.LoadServerConfig(configDir)
	if err != nil {
		diagErr(stderr, "config", err)
		return 1
	}
	idPath := identity
	if idPath == "" {
		idPath = cfg.IdentityPath
	}
	id, err := rnsgit.PrepareGitIdentity(idPath)
	if err != nil {
		diagErr(stderr, "identity", err)
		return 1
	}
	node, err := rnsgit.NewNode(cfg, id)
	if err != nil {
		diagErr(stderr, "node", err)
		return 1
	}
	if printID {
		fmt.Fprintf(stdout, "Git Peer Identity         : <%s>\n", term.CyanW(stdout, node.IdentityHash()))
		fmt.Fprintf(stdout, "Repositories Destination  : <%s>\n", term.CyanW(stdout, node.ReposDestHash()))
		return 0
	}
	if serviceMode {
		_ = serviceMode
	}
	if rnsConfig == "" {
		rnsConfig = cfg.RNSConfigDir
	}
	if rnsConfig == "" {
		rnsCfg, err := rnsutil.LoadConfigDir("")
		if err == nil && rnsCfg != nil {
			rnsConfig = rnsCfg.ConfigPath
		}
	}
	fmt.Fprintf(stderr, "%s Reticulum Git Node listening on <%s>\n", infoMsg(stderr, "Notice"), node.ReposDestHash())
	if err := node.RunService(rnsConfig); err != nil {
		fmt.Fprintf(stderr, "git node: %v\n", err)
		return 1
	}
	return 0
}

func runGitPrintIdentity(opt ...Options) int {
	return runGitServer([]string{"--print-identity"}, opt...)
}

func runGitMgmt(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	if len(args) < 2 {
		usageErr(stderr, fmt.Sprintf("reticulum-go git %s <rns://...> ...", args[0]))
		return 2
	}
	sub := args[0]
	remote := args[1]
	cfgDir := os.Getenv("RNGIT_CONFIG")
	client, err := rnsgit.NewMgmtClient(cfgDir, "")
	if err != nil {
		diagErr(stderr, "client", err)
		return 1
	}
	defer client.Close()
	ctx, cancel := rnsutil.CLIWaitContext(0)
	defer cancel()
	if err := client.Connect(ctx, remote); err != nil {
		diagErr(stderr, "connect", err)
		return 1
	}
	switch sub {
	case "create":
		if err := client.CreateRepo(ctx, remote); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Repository created\n")
	case "sync":
		if err := client.SyncRepo(ctx, remote); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Repository synced\n")
	case "fork", "mirror":
		if len(args) < 3 {
			usageErr(stderr, fmt.Sprintf("reticulum-go git %s <source> <target>", sub))
			return 2
		}
		if err := client.CloneRemote(ctx, args[1], args[2], sub); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Repository %sd\n", sub)
	default:
		fmt.Fprintf(stderr, "subcommand %q not implemented via CLI yet\n", sub)
		return 2
	}
	return 0
}

func printGitHelp(w io.Writer) {
	helpTitle(w, "reticulum-go git - Git over Reticulum (rngit-compatible)")
	helpUsageHeader(w)
	helpUsageLines(w,
		helpLine{"reticulum-go git [-config dir] [-rnsconfig dir] [-s]", "run repository node"},
		helpLine{"reticulum-go git -p", "print destination hashes"},
		helpLine{"reticulum-go git create <rns://node/group/repo>", "create repository"},
		helpLine{"reticulum-go git fork <source> <target>", "fork repository"},
		helpLine{"reticulum-go git mirror <source> <target>", "mirror repository"},
		helpLine{"reticulum-go git sync <rns://node/group/repo>", "sync mirror/fork"},
		helpLine{"reticulum-go git remote-rns <name> <rns://...>", "git remote helper"},
	)
	fmt.Fprintln(w)
	helpNote(w, "Install git-remote-rns symlink to reticulum-go for transparent git clone rns:// URLs.")
}

// EmitGitJSON writes a JSON event line when -json is set.
func EmitGitJSON(w io.Writer, event string, fields map[string]any) {
	if w == nil {
		return
	}
	m := map[string]any{"event": event}
	maps.Copy(m, fields)
	b, _ := json.Marshal(m)
	fmt.Fprintln(w, string(b))
}

// GitBundleVerify wraps git bundle verify for tests.
func GitBundleVerify(path string) error {
	return exec.Command("git", "bundle", "verify", "-q", path).Run() // #nosec G204 -- fixed git binary
}
