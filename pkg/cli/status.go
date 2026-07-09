// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"flag"
	"fmt"
	"time"

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
	links := fs.Bool("l", false, "include link count")
	sortBy := fs.String("s", "", "sort by rate|rx|tx|rxs|txs|traffic|announce|arx|atx|prx|ptx|held")
	sortAsc := fs.Bool("r", false, "sort ascending (default descending)")
	timeout := fs.Duration("timeout", 10*time.Second, "RPC timeout")
	quiet := fs.Bool("q", false, "quiet: suppress stderr hints")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}
	client, err := rnsutil.DialRPC(cfg, nil)
	if err != nil {
		fmt.Fprintf(stderr, "rpc: %v\n", err)
		if !*quiet {
			fmt.Fprintln(stderr, "hint: point -config at the daemon config dir (e.g. ~/.reticulum for rnsd)")
			fmt.Fprintln(stderr, "hint: rnsd on Linux needs shared_instance_type = tcp to expose 127.0.0.1:37429")
		}
		return 1
	}
	client.SetTimeout(*timeout)

	stats, err := client.GetInterfaceStats()
	if err != nil {
		fmt.Fprintf(stderr, "interface stats (%s): %v\n", client.Addr(), err)
		if !*quiet {
			fmt.Fprintln(stderr, "hint: start rnsd or reticulum-go first, then retry")
			fmt.Fprintln(stderr, "hint: for Python rnsd use: rgostatus -config ~/.reticulum")
		}
		return 1
	}
	rnsutil.SortInterfaceStats(&stats, *sortBy, *sortAsc)

	var linkCount *int
	if *links {
		n, err := client.GetLinkCount()
		if err != nil {
			fmt.Fprintf(stderr, "link count: %v\n", err)
			return 1
		}
		linkCount = &n
	}

	if *jsonOut {
		if err := rnsutil.WriteStatusJSON(stdout, stats); err != nil {
			fmt.Fprintf(stderr, "json: %v\n", err)
			return 1
		}
		return 0
	}
	opts := rnsutil.StatusOptions{
		NameFilter: *nameFilter,
		SortBy:     *sortBy,
		SortAsc:    *sortAsc,
		ShowAll:    *all,
	}
	if err := rnsutil.WriteStatusHuman(stdout, stats, linkCount, opts); err != nil {
		fmt.Fprintf(stderr, "write: %v\n", err)
		return 1
	}
	return 0
}
