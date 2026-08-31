// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"flag"
	"fmt"
	"io"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
)

func RunPath(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("rgopath", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configDir := fs.String("config", "", "path to config directory")
	table := fs.Bool("t", false, "show path table")
	jsonOut := fs.Bool("json", false, "emit JSON")
	fs.BoolVar(jsonOut, "j", false, "emit JSON (Python rnpath alias)")
	maxHops := fs.Int("m", -1, "max hops filter for path table (-1 = no filter)")
	rates := fs.Bool("r", false, "show announce rate info")
	drop := fs.Bool("d", false, "drop path to destination")
	// Match Python rnpath: -D drop-announces, -x drop-via. Keep -q as Go alias for -D.
	dropQueues := fs.Bool("D", false, "drop all queued announces")
	fs.BoolVar(dropQueues, "q", false, "drop all queued announces (Go alias for -D)")
	dropVia := fs.Bool("x", false, "drop all paths via specified transport instance")
	timeoutSec := fs.Float64("w", 0, "path request timeout in seconds (0 = adaptive from interface bitrate)")
	rpcTimeout := fs.Duration("timeout", 10*time.Second, "RPC timeout")
	remoteHex := fs.String("R", "", "transport identity hash of remote instance")
	mgmtIdent := fs.String("i", "", "identity file for remote management")
	remoteTimeoutSec := fs.Float64("W", 0, "timeout for remote queries in seconds (0 = adaptive from interface bitrate)")
	blackholed := fs.Bool("b", false, "list blackholed identities")
	fs.BoolVar(blackholed, "blackholed", false, "list blackholed identities")
	blackhole := fs.Bool("B", false, "blackhole identity")
	fs.BoolVar(blackhole, "blackhole", false, "blackhole identity")
	unblackhole := fs.Bool("U", false, "lift blackhole for identity")
	fs.BoolVar(unblackhole, "unblackhole", false, "lift blackhole for identity")
	remoteBHList := fs.Bool("p", false, "view published blackhole list for remote transport instance")
	bhHours := fs.Float64("duration", 0, "blackhole duration in hours (0 = indefinite)")
	fs.Float64Var(bhHours, "for", 0, "blackhole duration in hours (Go alias for -duration)")
	bhReason := fs.String("reason", "", "blackhole reason string")
	filter := fs.String("filter", "", "substring filter for blackhole list")
	bindFlagUsage(fs, "rgopath - path table and routing control",
		"Inspect or modify the path table via shared-instance RPC.",
		[]helpLine{
			{Cmd: "rgopath [flags]"},
			{Cmd: "rgopath [flags] <destination_hash>"},
			{Cmd: "rgopath -p <transport_hash> [list_filter]"},
			{Cmd: "reticulum-go path [flags]"},
		},
		"rgopath -t",
		"rgopath -t -json",
		"rgopath -d <dest_hash>",
		"rgopath -D",
		"rgopath -x <transport_hash>",
		"rgopath -p <transport_hash>",
	)

	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		diagErr(stderr, "config", err)
		return 1
	}

	var destHash []byte
	listFilter := *filter
	if fs.NArg() > 0 {
		destHash, err = rnsutil.ParseDestHash(fs.Arg(0))
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
	}
	if fs.NArg() > 1 && listFilter == "" {
		listFilter = fs.Arg(1)
	}

	modeCount := 0
	for _, b := range []bool{*table, *rates, *drop, *dropVia, *dropQueues, *blackholed, *blackhole, *unblackhole, *remoteBHList} {
		if b {
			modeCount++
		}
	}
	if modeCount > 1 {
		fmt.Fprintln(stderr, "specify only one of -t -r -d -D -x -b -B -U -p")
		return 2
	}

	if *remoteBHList {
		return runPathPublishedBlackhole(cfg, destHash, listFilter, *jsonOut, *remoteTimeoutSec, stdout, stderr)
	}

	if *remoteHex != "" {
		return runPathRemote(cfg, destHash, pathRemoteOpts{
			table:       *table,
			rates:       *rates,
			jsonOut:     *jsonOut,
			maxHops:     *maxHops,
			drop:        *drop,
			dropVia:     *dropVia,
			dropQueues:  *dropQueues,
			blackholed:  *blackholed,
			blackhole:   *blackhole,
			unblackhole: *unblackhole,
			remoteHex:   *remoteHex,
			identPath:   *mgmtIdent,
			timeoutSec:  *remoteTimeoutSec,
			stdout:      stdout,
			stderr:      stderr,
		})
	}

	needsRPC := *table || *rates || *drop || *dropVia || *dropQueues || *blackholed || *blackhole || *unblackhole
	if needsRPC {
		client, err := rnsutil.DialRPC(cfg, nil)
		if err != nil {
			diagErr(stderr, "rpc", err)
			return 1
		}
		client.SetTimeout(*rpcTimeout)

		switch {
		case *table:
			var mh *int
			if *maxHops >= 0 {
				mh = maxHops
			}
			paths, err := client.GetPathTable(mh)
			if err != nil {
				diagErr(stderr, "path table", err)
				return 1
			}
			if *jsonOut {
				if err := rnsutil.WritePathTableJSON(stdout, paths); err != nil {
					fmt.Fprintf(stderr, "%v\n", err)
					return 1
				}
				return 0
			}
			n, err := rnsutil.WritePathTableHuman(stdout, paths, destHash)
			if err != nil {
				fmt.Fprintf(stderr, "%v\n", err)
				return 1
			}
			if len(destHash) > 0 && n == 0 {
				fmt.Fprintln(stdout, warnMsg(stdout, "No path known"))
				return 1
			}
			return 0

		case *rates:
			table, err := client.GetRateTable()
			if err != nil {
				diagErr(stderr, "rate table", err)
				return 1
			}
			if *jsonOut {
				if err := rnsutil.WriteRateTableJSON(stdout, table); err != nil {
					fmt.Fprintf(stderr, "%v\n", err)
					return 1
				}
				return 0
			}
			n, err := rnsutil.WriteRateTableHuman(stdout, table, destHash)
			if err != nil {
				fmt.Fprintf(stderr, "%v\n", err)
				return 1
			}
			if len(destHash) > 0 && n == 0 {
				fmt.Fprintln(stdout, warnMsg(stdout, "No information available"))
				return 1
			}
			return 0

		case *drop:
			if len(destHash) == 0 {
				fmt.Fprintln(stderr, "destination hash required for -d")
				return 2
			}
			ok, err := client.DropPath(destHash)
			if err != nil {
				diagErr(stderr, "drop path", err)
				return 1
			}
			if !ok {
				fmt.Fprintln(stdout, warnMsg(stdout, fmt.Sprintf("Unable to drop path to %s. Does it exist?", rnsutil.PrettyHex(destHash))))
				return 1
			}
			fmt.Fprintln(stdout, okMsg(stdout, fmt.Sprintf("Dropped path to %s", rnsutil.PrettyHex(destHash))))
			return 0

		case *dropVia:
			if len(destHash) == 0 {
				fmt.Fprintln(stderr, "transport hash required for -x")
				return 2
			}
			n, err := client.DropAllVia(destHash)
			if err != nil {
				diagErr(stderr, "drop via", err)
				return 1
			}
			if n == 0 {
				fmt.Fprintln(stdout, warnMsg(stdout, fmt.Sprintf("Unable to drop paths via %s. Does the transport instance exist?", rnsutil.PrettyHex(destHash))))
				return 1
			}
			fmt.Fprintln(stdout, okMsg(stdout, fmt.Sprintf("Dropped all paths via %s (%d)", rnsutil.PrettyHex(destHash), n)))
			return 0

		case *dropQueues:
			n, err := client.DropAnnounceQueues()
			if err != nil {
				diagErr(stderr, "drop queues", err)
				return 1
			}
			fmt.Fprintln(stdout, okMsg(stdout, fmt.Sprintf("Dropping announce queues on all interfaces... (%d cleared)", n)))
			return 0

		case *blackholed:
			raw, err := client.GetBlackholedIdentities()
			if err != nil {
				diagErr(stderr, "blackhole list", err)
				return 1
			}
			entries := rnsutil.NormalizeBlackholeRPC(raw)
			if *jsonOut {
				if err := rnsutil.WriteBlackholeJSON(stdout, entries); err != nil {
					fmt.Fprintf(stderr, "%v\n", err)
					return 1
				}
				return 0
			}
			filt := listFilter
			if filt == "" && len(destHash) > 0 {
				filt = rnsutil.HexHash(destHash)
			}
			if err := rnsutil.WriteBlackholeHuman(stdout, entries, filt); err != nil {
				fmt.Fprintf(stderr, "%v\n", err)
				return 1
			}
			if len(entries) == 0 {
				fmt.Fprintln(stdout, "No blackholed identity data available")
			}
			return 0

		case *blackhole:
			if len(destHash) == 0 {
				fmt.Fprintln(stderr, "identity hash required for -B")
				return 2
			}
			var until float64
			if *bhHours > 0 {
				until = float64(time.Now().Unix()) + (*bhHours)*3600
			}
			ok, err := client.BlackholeIdentity(destHash, until, *bhReason)
			if err != nil {
				diagErr(stderr, "blackhole", err)
				return 1
			}
			if ok {
				fmt.Fprintln(stdout, okMsg(stdout, fmt.Sprintf("Blackholed identity %s", rnsutil.HexHash(destHash))))
			} else {
				fmt.Fprintln(stdout, warnMsg(stdout, fmt.Sprintf("Identity %s already blackholed", rnsutil.HexHash(destHash))))
			}
			return 0

		case *unblackhole:
			if len(destHash) == 0 {
				fmt.Fprintln(stderr, "identity hash required for -U")
				return 2
			}
			ok, err := client.UnblackholeIdentity(destHash)
			if err != nil {
				diagErr(stderr, "unblackhole", err)
				return 1
			}
			if ok {
				fmt.Fprintln(stdout, okMsg(stdout, fmt.Sprintf("Lifted blackhole for identity %s", rnsutil.HexHash(destHash))))
			} else {
				fmt.Fprintln(stdout, warnMsg(stdout, fmt.Sprintf("Identity %s not blackholed", rnsutil.HexHash(destHash))))
			}
			return 0
		}
	}

	// Default: request path (node-attached, like rnpath without -t).
	if len(destHash) == 0 {
		usageErr(stderr, "rgopath [flags] <destination_hash>")
		fmt.Fprintln(stderr, "  -t path table  -r rates  -d drop  -D drop announce queues  -x drop via")
		fmt.Fprintln(stderr, "  -b list blackholes  -B blackhole  -U unblackhole  -p published list")
		fmt.Fprintln(stderr, "  -R transport_id  -i identity  -W seconds")
		return 2
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

	tr := n.Transport()
	timeout := time.Duration(*timeoutSec * float64(time.Second))
	fmt.Fprintln(stdout, infoMsg(stdout, fmt.Sprintf("Path to %s requested", rnsutil.PrettyHex(destHash))))
	ctx, cancel := rnsutil.CLIWaitContext(timeout)
	defer cancel()
	if err := rnsutil.WaitPath(ctx, tr, destHash); err != nil {
		fmt.Fprintln(stdout, errMsg(stdout, "Path request timed out"))
		report := rnsutil.DiagnoseReachability(tr, destHash)
		_ = rnsutil.WriteReachReportHuman(stdout, report)
		return 12
	}
	hops := tr.HopsTo(destHash)
	via := tr.NextHop(destHash)
	iface := tr.NextHopInterface(destHash)
	hopWord := "hops"
	if hops == 1 {
		hopWord = "hop"
	}
	fmt.Fprintln(stdout, okMsg(stdout, fmt.Sprintf("Path found: %d %s via %s on %s",
		hops, hopWord, rnsutil.PrettyHex(via), iface)))
	return 0
}

func runPathPublishedBlackhole(cfg *common.ReticulumConfig, transportHash []byte, listFilter string, jsonOut bool, timeoutSec float64, stdout, stderr io.Writer) int {
	if len(transportHash) == 0 {
		fmt.Fprintln(stderr, "transport identity hash required for -p")
		return 2
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

	timeout := time.Duration(timeoutSec * float64(time.Second))
	ctx, cancel := rnsutil.CLIWaitContext(timeout)
	defer cancel()

	fmt.Fprintln(stdout, infoMsg(stdout, "Sending request..."))
	entries, err := rnsutil.FetchPublishedBlackholeList(ctx, n.Transport(), transportHash)
	if err != nil {
		fmt.Fprintln(stdout, errMsg(stdout, "The remote request failed."))
		fmt.Fprintf(stderr, "%v\n", err)
		return 10
	}
	if len(entries) == 0 {
		fmt.Fprintln(stdout, "No blackholed identity data available")
		return 20
	}
	if jsonOut {
		if err := rnsutil.WriteBlackholeJSON(stdout, entries); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		return 0
	}
	if err := rnsutil.WriteBlackholeHuman(stdout, entries, listFilter); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	return 0
}

type pathRemoteOpts struct {
	table       bool
	rates       bool
	jsonOut     bool
	maxHops     int
	drop        bool
	dropVia     bool
	dropQueues  bool
	blackholed  bool
	blackhole   bool
	unblackhole bool
	remoteHex   string
	identPath   string
	timeoutSec  float64
	stdout      io.Writer
	stderr      io.Writer
}

func runPathRemote(cfg *common.ReticulumConfig, destHash []byte, opts pathRemoteOpts) int {
	stdout, stderr := opts.stdout, opts.stderr
	if opts.drop {
		fmt.Fprintln(stdout, "Dropping path on remote instances not yet implemented")
		return 255
	}
	if opts.dropVia {
		fmt.Fprintln(stdout, "Dropping all paths via specific transport instance on remote instances yet not implemented")
		return 255
	}
	if opts.dropQueues {
		fmt.Fprintln(stdout, "Dropping announce queues on remote instances not yet implemented")
		return 255
	}
	if opts.blackholed || opts.blackhole || opts.unblackhole {
		fmt.Fprintln(stdout, "Blackholing identity on remote instances not yet implemented")
		return 255
	}
	if !opts.table && !opts.rates {
		fmt.Fprintln(stdout, "Requesting paths on remote instances not implemented")
		return 255
	}

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
	var mh *int
	if opts.maxHops >= 0 {
		mh = &opts.maxHops
	}
	cmd := "table"
	if opts.rates {
		cmd = "rates"
	}
	raw, err := rnsutil.RemotePathRequest(ctx, l, cmd, destHash, mh)
	if err != nil {
		fmt.Fprintln(stdout, errMsg(stdout, "The remote request failed. Likely authentication failure."))
		return 10
	}

	if opts.rates {
		table := rnsutil.RateTableFromResponse(raw)
		if opts.jsonOut {
			if err := rnsutil.WriteRateTableJSON(stdout, table); err != nil {
				fmt.Fprintf(stderr, "%v\n", err)
				return 1
			}
			return 0
		}
		nshown, err := rnsutil.WriteRateTableHuman(stdout, table, destHash)
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		if len(destHash) > 0 && nshown == 0 {
			fmt.Fprintln(stdout, warnMsg(stdout, "No information available"))
			return 1
		}
		return 0
	}

	table := rnsutil.PathTableFromResponse(raw)
	if opts.jsonOut {
		if err := rnsutil.WritePathTableJSON(stdout, table); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		return 0
	}
	nshown, err := rnsutil.WritePathTableHuman(stdout, table, destHash)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if len(destHash) > 0 && nshown == 0 {
		fmt.Fprintln(stdout, warnMsg(stdout, "No path known"))
		return 1
	}
	return 0
}
