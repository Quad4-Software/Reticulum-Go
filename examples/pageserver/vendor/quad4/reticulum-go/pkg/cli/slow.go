// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"encoding/hex"
	"flag"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/rnsutil"
	"quad4/reticulum-go/pkg/transport"
)

func RunSlow(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("rgoslow", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configDir := fs.String("config", "", "path to config directory")
	jsonOut := fs.Bool("json", false, "emit JSON report")
	all := fs.Bool("a", false, "include hidden/local peer interfaces")
	nameFilter := fs.String("n", "", "filter interfaces by name substring")
	links := fs.Bool("l", false, "include active link count")
	destHex := fs.String("dest", "", "focus analysis on destination hash (32 hex chars)")
	topN := fs.Int("top", 12, "max interfaces to rank")
	highHop := fs.Int("high-hop", 6, "hop count treated as high")
	timeout := fs.Duration("timeout", 30*time.Second, "RPC timeout")
	monitor := fs.Bool("m", false, "continuously refresh report")
	interval := fs.Duration("I", 2*time.Second, "monitor refresh interval")
	quiet := fs.Bool("q", false, "quiet: suppress stderr hints")
	withPaths := fs.Bool("paths", false, "include full path-table analysis (can be slow on large tables)")

	bindFlagUsageBullets(fs, "rgoslow - find interfaces, paths, and transports slowing transfers",
		`Queries a running shared instance (Go reticulum-go or Python rnsd) over RPC
and ranks congestion signals that commonly stall resource transfers:`,
		[]string{
			"bitrate caps and utilization",
			"announce / path-request bursts and held announces",
			"bandwidth gates",
			"socket RTT (Go daemon)",
			"high-hop paths and congested egress hubs",
		},
		[]helpLine{
			{Cmd: "rgoslow [flags]"},
			{Cmd: "reticulum-go slow [flags]"},
		},
		"rgoslow",
		"rgoslow -config ~/.reticulum",
		"rgoslow -dest 06a54b505bb67b25ef3f8097e8001edc",
		"rgoslow -json -l",
		"rgoslow -paths",
		"rgoslow -m -I 3s",
	)

	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		diagErr(stderr, "config", err)
		return 1
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

	analyzeOpts := rnsutil.SlowAnalyzeOptions{
		HighHopThreshold: *highHop,
		TopN:             *topN,
		NameFilter:       *nameFilter,
		ShowAll:          *all,
	}

	runOnce := func() int {
		stats, err := client.GetInterfaceStats()
		if err != nil {
			fmt.Fprintf(stderr, "%s (%s): %v\n", errMsg(stderr, "interface stats"), client.Addr(), err)
			if !*quiet {
				fmt.Fprintln(stderr, warnMsg(stderr, "hint: start rnsd or reticulum-go first, then retry"))
			}
			return 1
		}
		var paths []transport.PathTableEntry
		if *withPaths {
			paths, err = client.GetPathTable(nil)
			if err != nil {
				fmt.Fprintf(stderr, "%s: %v\n", warnMsg(stderr, "path table"), err)
				if !*quiet {
					fmt.Fprintln(stderr, warnMsg(stderr, "hint: continuing with interface analysis only"))
				}
				paths = nil
			}
		}
		var linkCount *int
		if *links {
			n, err := client.GetLinkCount()
			if err != nil {
				diagErr(stderr, "link count", err)
				return 1
			}
			linkCount = &n
		}

		rep := rnsutil.AnalyzeSlow(stats, paths, linkCount, client.Addr(), analyzeOpts)
		if !*withPaths {
			rep.Recommendations = append([]string{
				"Path-table scan skipped (default). Use -paths for hop/egress hotspots, or -dest <hash> for one destination",
			}, rep.Recommendations...)
		}
		if *destHex != "" {
			focus, err := focusDestination(client, paths, *destHex)
			if err != nil {
				diagErr(stderr, "dest", err)
				return 1
			}
			rep.Destination = focus
			if focus.HasPath && focus.Interface != "" {
				for _, row := range rep.Interfaces {
					if row.Name == focus.Interface && row.Score >= 25 {
						rep.Recommendations = append([]string{
							fmt.Sprintf("Destination egress %s is constrained: %s", row.Name, row.Why),
						}, rep.Recommendations...)
						break
					}
				}
				if focus.Hops >= analyzeOpts.HighHopThreshold {
					rep.Recommendations = append([]string{
						fmt.Sprintf("This destination is %d hops away: resource chunks need many round trips", focus.Hops),
					}, rep.Recommendations...)
				}
			}
		}

		if *jsonOut {
			if err := rnsutil.WriteSlowJSON(stdout, rep); err != nil {
				diagErr(stderr, "json", err)
				return 1
			}
			return 0
		}
		if err := rnsutil.WriteSlowHuman(stdout, rep); err != nil {
			diagErr(stderr, "write", err)
			return 1
		}
		return 0
	}

	if !*monitor {
		return runOnce()
	}
	for {
		if code := runOnce(); code != 0 {
			return code
		}
		time.Sleep(*interval)
		fmt.Fprintln(stdout)
	}
}

func focusDestination(client *rnsutil.RPCClient, paths []transport.PathTableEntry, destHex string) (*rnsutil.DestFocus, error) {
	raw, err := hex.DecodeString(destHex)
	if err != nil || len(raw) != 16 {
		return nil, fmt.Errorf("destination hash must be 32 hex characters")
	}
	focus := &rnsutil.DestFocus{Hash: rnsutil.PrettyHex(raw)}
	now := float64(time.Now().Unix())
	for _, p := range paths {
		if len(p.Hash) == 16 && string(p.Hash) == string(raw) {
			focus.HasPath = true
			focus.Hops = int(p.Hops)
			focus.Via = rnsutil.PrettyHex(p.Via)
			focus.Interface = p.Interface
			if p.Expires > now {
				focus.ExpiresInS = p.Expires - now
			}
			break
		}
	}
	if to, err := client.GetFirstHopTimeout(raw); err == nil {
		focus.FirstHopTimeoutS = to
	}
	if nhIf, err := client.GetNextHopIfName(raw); err == nil && nhIf != "" {
		focus.Interface = nhIf
		focus.HasPath = true
	}
	if via, err := client.GetNextHop(raw); err == nil && len(via) > 0 {
		focus.Via = rnsutil.PrettyHex(via)
		focus.HasPath = true
	}
	return focus, nil
}
