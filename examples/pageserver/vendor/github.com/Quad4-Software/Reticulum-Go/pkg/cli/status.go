// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/discovery"
	"github.com/Quad4-Software/Reticulum-Go/pkg/node"
	"github.com/Quad4-Software/Reticulum-Go/pkg/rnsutil"
)

func RunStatus(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("rgostatus", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configDir := fs.String("config", "", "path to config directory")
	jsonOut := fs.Bool("json", false, "emit JSON")
	all := fs.Bool("a", false, "include all interfaces")
	nameFilter := fs.String("n", "", "filter interfaces by name substring")
	links := fs.Bool("l", false, "include link table stats")
	announceStats := fs.Bool("A", false, "show announce byte and count stats")
	prStats := fs.Bool("P", false, "show path-request byte and count stats")
	burstFilter := fs.Bool("B", false, "only show interfaces with active bursts")
	blockedIPs := fs.Bool("b", false, "list blocked IPs per interface")
	trafficTotals := fs.Bool("t", false, "show transport traffic totals")
	topology := fs.Bool("T", false, "show path topology grouped by interface")
	showPPS := fs.Bool("p", false, "display packets per second in totals")
	queues := fs.Bool("Q", false, "show inbound queue pressure (RNS 1.5.0, use -Q not -q)")
	profiling := fs.Bool("z", false, "display live profiling results when the instance provides them")
	monitor := fs.Bool("m", false, "continuously monitor status")
	monitorInterval := fs.Float64("I", 1, "refresh interval for monitor mode in seconds")
	discovered := fs.Bool("d", false, "list discovered interfaces")
	discoveredDetail := fs.Bool("D", false, "show details and config entries for discovered interfaces")
	showStale := fs.Bool("show-stale", false, "show stale discovery entries")
	showUnknown := fs.Bool("show-unknown", false, "show discovery entries without version info")
	attachName := fs.String("attach", "", "attach interface by name")
	detachName := fs.String("detach", "", "detach interface by name")
	reloadName := fs.String("reload", "", "reload interface by name")
	sortBy := fs.String("s", "", "sort by rate|rx|tx|rxs|txs|traffic|announce|arx|atx|prx|ptx|held|pvs|ivs|flt|arxc|atxc|prxc|ptxc|gravity")
	sortAsc := fs.Bool("r", false, "sort ascending (default descending)")
	timeout := fs.Duration("timeout", 10*time.Second, "RPC timeout")
	remoteHex := fs.String("R", "", "transport identity hash of remote instance")
	mgmtIdent := fs.String("i", "", "identity file for remote management")
	remoteTimeoutSec := fs.Float64("W", 0, "timeout for remote queries in seconds (0 = adaptive from interface bitrate)")
	fs.Float64Var(remoteTimeoutSec, "w", 0, "timeout for remote queries in seconds (Python rnstatus alias for -W)")
	quiet := fs.Bool("q", false, "quiet: suppress stderr hints")
	bindFlagUsage(fs, "rgostatus - interface and transport status",
		"Queries a running shared instance over RPC. Use -R for remote instance status.",
		[]helpLine{
			{Cmd: "rgostatus [flags] [filter]"},
			{Cmd: "reticulum-go status [flags] [filter]"},
		},
		"rgostatus",
		"rgostatus -json -l",
		"rgostatus -t -p",
		"rgostatus -m -I 2",
		"rgostatus -R <transport_hash> -i identity",
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		diagErr(stderr, "config", err)
		return 1
	}

	filter := *nameFilter
	if filter == "" && fs.NArg() > 0 {
		filter = fs.Arg(0)
	}

	statusOpts := rnsutil.StatusOptions{
		NameFilter:     filter,
		SortBy:         *sortBy,
		SortAsc:        *sortAsc,
		ShowAll:        *all,
		AnnounceStats:  *announceStats,
		PRStats:        *prStats,
		ShowBlockedIPs: *blockedIPs,
		QueueStats:     *queues,
		TrafficTotals:  *trafficTotals,
		BurstFilter:    *burstFilter,
		ShowPPS:        *showPPS,
	}

	runOnce := func(out io.Writer) int {
		if *attachName != "" || *detachName != "" || *reloadName != "" {
			return runInterfaceManage(cfg, *attachName, *detachName, *reloadName, *timeout, out, stderr)
		}
		if *topology {
			return runTopology(cfg, filter, *timeout, out, stderr)
		}
		if *discovered || *discoveredDetail {
			storageDir := ""
			if cfg.ConfigPath != "" {
				storageDir = filepath.Join(filepath.Dir(cfg.ConfigPath), "storage")
			}
			list, err := discovery.ListDiscoveredInterfaces(storageDir, discovery.ListOptions{
				NameFilter: filter,
			})
			if err != nil {
				fmt.Fprintf(stderr, "discovered interfaces: %v\n", err)
				return 1
			}
			list = rnsutil.FilterDiscovered(list, *showStale, *showUnknown)
			if *jsonOut {
				if err := rnsutil.WriteDiscoveredJSON(out, list); err != nil {
					diagErr(stderr, "json", err)
					return 1
				}
				return 0
			}
			if err := rnsutil.WriteDiscoveredHuman(out, list, rnsutil.DiscoveredOptions{
				Details:     *discoveredDetail,
				ShowStale:   *showStale,
				ShowUnknown: *showUnknown,
			}); err != nil {
				diagErr(stderr, "write", err)
				return 1
			}
			return 0
		}

		if *remoteHex != "" {
			return runStatusRemote(cfg, statusRemoteOpts{
				jsonOut:    *jsonOut,
				links:      *links,
				profiling:  *profiling,
				remoteHex:  *remoteHex,
				identPath:  *mgmtIdent,
				timeoutSec: *remoteTimeoutSec,
				statusOpts: statusOpts,
				stdout:     out,
				stderr:     stderr,
			})
		}

		client, err := rnsutil.DialRPC(cfg, nil)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "rpc"), err)
			if !*quiet {
				fmt.Fprintln(stderr, warnMsg(stderr, "hint: point -config at the daemon config dir (e.g. ~/.reticulum for rnsd)"))
				fmt.Fprintln(stderr, warnMsg(stderr, "hint: align shared_instance_type (unix on Linux by default, or tcp) and instance_name / ports"))
			}
			return 1
		}
		client.SetTimeout(*timeout)

		stats, err := client.GetInterfaceStats()
		if err != nil {
			fmt.Fprintf(stderr, "%s (%s): %v\n", errMsg(stderr, "interface stats"), client.Addr(), err)
			if !*quiet {
				fmt.Fprintln(stderr, warnMsg(stderr, "hint: start rnsd or reticulum-go first, then retry"))
				fmt.Fprintln(stderr, warnMsg(stderr, "hint: for Python rnsd use: rgostatus -config ~/.reticulum"))
			}
			return 1
		}
		rnsutil.SortInterfaceStats(&stats, *sortBy, *sortAsc)

		var linkCount, activeLinkCount *int
		if *links {
			n, err := client.GetLinkCount()
			if err != nil {
				diagErr(stderr, "link count", err)
				return 1
			}
			linkCount = &n
			m, err := client.GetActiveLinkCount()
			if err == nil {
				activeLinkCount = &m
			}
		}

		if *jsonOut {
			if err := rnsutil.WriteStatusJSON(out, stats); err != nil {
				diagErr(stderr, "json", err)
				return 1
			}
			return 0
		}
		if err := rnsutil.WriteStatusHuman(out, stats, linkCount, activeLinkCount, statusOpts); err != nil {
			diagErr(stderr, "write", err)
			return 1
		}
		if *profiling {
			prof, err := client.GetProfilingResults()
			if err != nil {
				fmt.Fprintln(out, "\n Profiling    : not available from this instance")
			} else if prof != "" {
				fmt.Fprintf(out, "\n Profiling    :\n%s\n", prof)
			} else {
				fmt.Fprintln(out, "\n Profiling    : no results")
			}
		}
		return 0
	}

	if !*monitor {
		return runOnce(stdout)
	}

	interval := max(time.Duration(*monitorInterval*float64(time.Second)), 200*time.Millisecond)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	for {
		start := time.Now()
		var buf bytes.Buffer
		code := runOnce(&buf)
		if code != 0 && buf.Len() == 0 {
			return code
		}
		fmt.Fprint(stdout, "\033[H\033[2J")
		_, _ = io.Copy(stdout, &buf)
		elapsed := time.Since(start)
		sleepFor := max(interval-elapsed, 200*time.Millisecond)
		select {
		case <-sigCh:
			fmt.Fprintln(stdout)
			return 0
		case <-time.After(sleepFor):
		}
	}
}

// runInterfaceManage implements rgostatus --attach/--detach/--reload against
// the local shared instance RPC (rnstatus equivalents, RNS 1.5.5).
func runInterfaceManage(cfg *common.ReticulumConfig, attach, detach, reload string, timeout time.Duration, stdout, stderr io.Writer) int {
	client, err := rnsutil.DialRPC(cfg, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "rpc"), err)
		return 1
	}
	client.SetTimeout(timeout)

	action := ""
	name := ""
	switch {
	case attach != "":
		action, name = "attach_interface", attach
	case detach != "":
		action, name = "detach_interface", detach
	case reload != "":
		action, name = "reload_interface", reload
	}
	result, err := client.ManageInterface(action, name)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "rpc"), err)
		return 1
	}
	past := map[string]string{
		"attach_interface": "attached",
		"detach_interface": "detached",
		"reload_interface": "reloaded",
	}[action]
	switch v := result.(type) {
	case bool:
		if v {
			fmt.Fprintf(stdout, "Interface %s was %s\n", name, past)
			return 0
		}
		fmt.Fprintf(stdout, "Could not %s interface %s\n", strings.TrimSuffix(past, "ed"), name)
		return 1
	case nil:
		fmt.Fprintf(stdout, "The interface %s does not exist\n", name)
		return 1
	default:
		fmt.Fprintf(stdout, "Unknown error while %sing interface %s\n", strings.TrimSuffix(past, "ed"), name)
		return 1
	}
}

// runTopology renders the path table grouped by receiving interface over the
// shared-instance RPC. Go-only view; Python rnstatus has no equivalent.
func runTopology(cfg *common.ReticulumConfig, nameFilter string, timeout time.Duration, stdout, stderr io.Writer) int {
	client, err := rnsutil.DialRPC(cfg, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "rpc"), err)
		return 1
	}
	client.SetTimeout(timeout)
	stats, err := client.GetInterfaceStats()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "rpc"), err)
		return 1
	}
	paths, err := client.GetPathTable(nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "rpc"), err)
		return 1
	}
	if nameFilter != "" {
		kept := paths[:0]
		for _, p := range paths {
			if strings.Contains(p.Interface, nameFilter) {
				kept = append(kept, p)
			}
		}
		paths = kept
	}
	if err := rnsutil.WriteTopologyHuman(stdout, stats, paths, time.Now()); err != nil {
		diagErr(stderr, "write", err)
		return 1
	}
	return 0
}

type statusRemoteOpts struct {
	jsonOut    bool
	links      bool
	profiling  bool
	remoteHex  string
	identPath  string
	timeoutSec float64
	statusOpts rnsutil.StatusOptions
	stdout     io.Writer
	stderr     io.Writer
}

func runStatusRemote(cfg *common.ReticulumConfig, opts statusRemoteOpts) int {
	stdout, stderr := opts.stdout, opts.stderr
	tid, err := rnsutil.ParseDestHash(opts.remoteHex)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 20
	}
	auth, err := rnsutil.LoadManagementIdentity(opts.identPath)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 20
	}

	n, err := node.New(cfg)
	if err != nil {
		diagErr(stderr, "node", err)
		return 1
	}
	if err := n.Start(); err != nil {
		diagErr(stderr, "start", err)
		return 1
	}
	defer n.Stop()

	timeout := time.Duration(opts.timeoutSec * float64(time.Second))
	ctx, cancel := rnsutil.CLIWaitContext(timeout)
	defer cancel()

	fmt.Fprintln(stdout, infoMsg(stdout, "Establishing link with remote transport instance..."))
	l, err := rnsutil.EstablishRemoteManagementLink(ctx, n.Transport(), tid, auth)
	if err != nil {
		fmt.Fprintln(stdout, errMsg(stdout, err.Error()))
		return 12
	}
	defer l.Teardown()

	fmt.Fprintln(stdout, infoMsg(stdout, "Sending request..."))
	raw, err := rnsutil.RemoteStatusRequest(ctx, l, opts.links, opts.profiling)
	if err != nil {
		fmt.Fprintln(stdout, errMsg(stdout, "The remote status request failed. Likely authentication failure."))
		return 10
	}
	stats, linkCount, profilingText, err := rnsutil.InterfaceStatsFromRemoteStatus(raw, opts.links, opts.profiling)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	rnsutil.SortInterfaceStats(&stats, opts.statusOpts.SortBy, opts.statusOpts.SortAsc)
	if opts.jsonOut {
		if err := rnsutil.WriteStatusJSON(stdout, stats); err != nil {
			diagErr(stderr, "json", err)
			return 1
		}
		return 0
	}
	if err := rnsutil.WriteStatusHuman(stdout, stats, linkCount, nil, opts.statusOpts); err != nil {
		diagErr(stderr, "write", err)
		return 1
	}
	if opts.profiling {
		if profilingText != "" {
			fmt.Fprintf(stdout, "\n Profiling    :\n%s\n", profilingText)
		} else {
			fmt.Fprintln(stdout, "\n Profiling    : not available from this instance")
		}
	}
	return 0
}
