// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"context"
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
	maxHops := fs.Int("m", -1, "max hops filter for path table (-1 = no filter)")
	rates := fs.Bool("r", false, "show announce rate info")
	drop := fs.Bool("d", false, "drop path to destination")
	dropVia := fs.Bool("D", false, "drop all paths via transport hash")
	dropQueues := fs.Bool("q", false, "drop announce queues")
	timeoutSec := fs.Float64("w", 15, "path request timeout in seconds")
	rpcTimeout := fs.Duration("timeout", 10*time.Second, "RPC timeout")
	remoteHex := fs.String("R", "", "transport identity hash of remote instance")
	mgmtIdent := fs.String("i", "", "identity file for remote management")
	remoteTimeoutSec := fs.Float64("W", 15, "timeout for remote queries")
	blackholed := fs.Bool("blackholed", false, "list blackholed identities")
	blackhole := fs.Bool("blackhole", false, "blackhole identity")
	unblackhole := fs.Bool("unblackhole", false, "lift blackhole for identity")
	bhHours := fs.Float64("for", 0, "blackhole duration in hours (0 = indefinite)")
	bhReason := fs.String("reason", "", "blackhole reason string")
	filter := fs.String("filter", "", "substring filter for blackhole list")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}

	var destHash []byte
	if fs.NArg() > 0 {
		destHash, err = rnsutil.ParseDestHash(fs.Arg(0))
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
	}

	modeCount := 0
	for _, b := range []bool{*table, *rates, *drop, *dropVia, *dropQueues, *blackholed, *blackhole, *unblackhole} {
		if b {
			modeCount++
		}
	}
	if modeCount > 1 {
		fmt.Fprintln(stderr, "specify only one of -t -r -d -D -q -blackholed -blackhole -unblackhole")
		return 2
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
			fmt.Fprintf(stderr, "rpc: %v\n", err)
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
				fmt.Fprintf(stderr, "path table: %v\n", err)
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
				fmt.Fprintf(stderr, "rate table: %v\n", err)
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
				fmt.Fprintf(stderr, "drop path: %v\n", err)
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
				fmt.Fprintln(stderr, "transport hash required for -D")
				return 2
			}
			n, err := client.DropAllVia(destHash)
			if err != nil {
				fmt.Fprintf(stderr, "drop via: %v\n", err)
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
				fmt.Fprintf(stderr, "drop queues: %v\n", err)
				return 1
			}
			fmt.Fprintln(stdout, okMsg(stdout, fmt.Sprintf("Dropping announce queues on all interfaces... (%d cleared)", n)))
			return 0

		case *blackholed:
			raw, err := client.GetBlackholedIdentities()
			if err != nil {
				fmt.Fprintf(stderr, "blackhole list: %v\n", err)
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
			filt := *filter
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
				fmt.Fprintln(stderr, "identity hash required for -blackhole")
				return 2
			}
			var until float64
			if *bhHours > 0 {
				until = float64(time.Now().Unix()) + (*bhHours)*3600
			}
			ok, err := client.BlackholeIdentity(destHash, until, *bhReason)
			if err != nil {
				fmt.Fprintf(stderr, "blackhole: %v\n", err)
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
				fmt.Fprintln(stderr, "identity hash required for -unblackhole")
				return 2
			}
			ok, err := client.UnblackholeIdentity(destHash)
			if err != nil {
				fmt.Fprintf(stderr, "unblackhole: %v\n", err)
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
		fmt.Fprintln(stderr, "usage: rgopath [flags] <destination_hash>")
		fmt.Fprintln(stderr, "  -t path table  -r rates  -d drop  -D drop via  -q drop queues")
		fmt.Fprintln(stderr, "  -R transport_id  -i identity  -W seconds")
		fmt.Fprintln(stderr, "  -blackholed / -blackhole / -unblackhole")
		return 2
	}

	n, err := node.New(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "node: %v\n", err)
		return 1
	}
	if err := n.Start(); err != nil {
		fmt.Fprintf(stderr, "start: %v\n", err)
		return 1
	}
	defer n.Stop()

	tr := n.Transport()
	timeout := time.Duration(*timeoutSec * float64(time.Second))
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	fmt.Fprintln(stdout, infoMsg(stdout, fmt.Sprintf("Path to %s requested", rnsutil.PrettyHex(destHash))))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
		fmt.Fprintf(stderr, "node: %v\n", err)
		return 1
	}
	if err := n.Start(); err != nil {
		fmt.Fprintf(stderr, "start: %v\n", err)
		return 1
	}
	defer n.Stop()

	timeout := time.Duration(opts.timeoutSec * float64(time.Second))
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
