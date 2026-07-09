// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"flag"
	"fmt"
	"os"
	"time"

	"quad4/reticulum-go/pkg/rnsutil"
)


func RunStatus(args []string) int {
	fs := flag.NewFlagSet("rgostatus", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configDir := fs.String("config", "", "path to config directory")
	jsonOut := fs.Bool("json", false, "emit JSON")
	all := fs.Bool("a", false, "include all interfaces")
	nameFilter := fs.String("n", "", "filter interfaces by name substring")
	links := fs.Bool("l", false, "include link count")
	sortBy := fs.String("s", "", "sort by rate|rx|tx|rxs|txs|traffic|announce|arx|atx|prx|ptx|held")
	sortAsc := fs.Bool("r", false, "sort ascending (default descending)")
	timeout := fs.Duration("timeout", 10*time.Second, "RPC timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}
	client, err := rnsutil.DialRPC(cfg, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rpc: %v\n", err)
		fmt.Fprintln(os.Stderr, "hint: point -config at the daemon config dir (e.g. ~/.reticulum for rnsd)")
		fmt.Fprintln(os.Stderr, "hint: rnsd on Linux needs shared_instance_type = tcp to expose 127.0.0.1:37429")
		return 1
	}
	client.SetTimeout(*timeout)

	stats, err := client.GetInterfaceStats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "interface stats (%s): %v\n", client.Addr(), err)
		fmt.Fprintln(os.Stderr, "hint: start rnsd or reticulum-go first, then retry")
		fmt.Fprintln(os.Stderr, "hint: for Python rnsd use: rgostatus -config ~/.reticulum")
		return 1
	}
	rnsutil.SortInterfaceStats(&stats, *sortBy, *sortAsc)

	var linkCount *int
	if *links {
		n, err := client.GetLinkCount()
		if err != nil {
			fmt.Fprintf(os.Stderr, "link count: %v\n", err)
			return 1
		}
		linkCount = &n
	}

	if *jsonOut {
		if err := rnsutil.WriteStatusJSON(os.Stdout, stats); err != nil {
			fmt.Fprintf(os.Stderr, "json: %v\n", err)
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
	if err := rnsutil.WriteStatusHuman(os.Stdout, stats, linkCount, opts); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		return 1
	}
	return 0
}
