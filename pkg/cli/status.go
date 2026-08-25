// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/discovery"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
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
	queues := fs.Bool("Q", false, "show inbound queue pressure (RNS 1.5.0, use -Q not -q)")
	discovered := fs.Bool("d", false, "list discovered interfaces")
	discoveredDetail := fs.Bool("D", false, "show details and config entries for discovered interfaces")
	sortBy := fs.String("s", "", "sort by rate|rx|tx|rxs|txs|traffic|announce|arx|atx|prx|ptx|held|pvs|ivs|flt|arxc|atxc|prxc|ptxc|gravity")
	sortAsc := fs.Bool("r", false, "sort ascending (default descending)")
	timeout := fs.Duration("timeout", 10*time.Second, "RPC timeout")
	remoteHex := fs.String("R", "", "transport identity hash of remote instance")
	mgmtIdent := fs.String("i", "", "identity file for remote management")
	remoteTimeoutSec := fs.Float64("W", 0, "timeout for remote queries in seconds (0 = adaptive from interface bitrate)")
	quiet := fs.Bool("q", false, "quiet: suppress stderr hints")
	bindFlagUsage(fs, "rgostatus - interface and transport status",
		"Queries a running shared instance over RPC. Use -R for remote instance status.",
		[]helpLine{
			{Cmd: "rgostatus [flags]"},
			{Cmd: "reticulum-go status [flags]"},
		},
		"rgostatus",
		"rgostatus -json -l",
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

	statusOpts := rnsutil.StatusOptions{
		NameFilter:     *nameFilter,
		SortBy:         *sortBy,
		SortAsc:        *sortAsc,
		ShowAll:        *all,
		AnnounceStats:  *announceStats,
		PRStats:        *prStats,
		ShowBlockedIPs: *blockedIPs,
		QueueStats:     *queues,
		TrafficTotals:  *trafficTotals,
		BurstFilter:    *burstFilter,
	}

	if *discovered || *discoveredDetail {
		storageDir := ""
		if cfg.ConfigPath != "" {
			storageDir = filepath.Join(filepath.Dir(cfg.ConfigPath), "storage")
		}
		list, err := discovery.ListDiscoveredInterfaces(storageDir, discovery.ListOptions{
			NameFilter: *nameFilter,
		})
		if err != nil {
			fmt.Fprintf(stderr, "discovered interfaces: %v\n", err)
			return 1
		}
		if *jsonOut {
			if err := rnsutil.WriteDiscoveredJSON(stdout, list); err != nil {
				diagErr(stderr, "json", err)
				return 1
			}
			return 0
		}
		if err := rnsutil.WriteDiscoveredHuman(stdout, list, *discoveredDetail); err != nil {
			diagErr(stderr, "write", err)
			return 1
		}
		return 0
	}

	if *remoteHex != "" {
		return runStatusRemote(cfg, statusRemoteOpts{
			jsonOut:    *jsonOut,
			links:      *links,
			remoteHex:  *remoteHex,
			identPath:  *mgmtIdent,
			timeoutSec: *remoteTimeoutSec,
			statusOpts: statusOpts,
			stdout:     stdout,
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
		if err := rnsutil.WriteStatusJSON(stdout, stats); err != nil {
			diagErr(stderr, "json", err)
			return 1
		}
		return 0
	}
	if err := rnsutil.WriteStatusHuman(stdout, stats, linkCount, activeLinkCount, statusOpts); err != nil {
		diagErr(stderr, "write", err)
		return 1
	}
	return 0
}

type statusRemoteOpts struct {
	jsonOut    bool
	links      bool
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
	raw, err := rnsutil.RemoteStatusRequest(ctx, l, opts.links)
	if err != nil {
		fmt.Fprintln(stdout, errMsg(stdout, "The remote status request failed. Likely authentication failure."))
		return 10
	}
	stats, linkCount, err := rnsutil.InterfaceStatsFromRemoteStatus(raw)
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
	return 0
}
