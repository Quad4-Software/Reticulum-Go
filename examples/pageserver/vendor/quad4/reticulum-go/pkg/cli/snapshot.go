// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/rnsutil"
	"quad4/reticulum-go/pkg/transport"
)

// NodeSnapshot is a path and link health dump for operators and porters.
type NodeSnapshot struct {
	TS              string                    `json:"ts"`
	TransportUptime float64                   `json:"transport_uptime"`
	TransportID     string                    `json:"transport_id,omitempty"`
	ActiveLinks     int                       `json:"active_links"`
	NetmonFlap      uint64                    `json:"netmon_flap"`
	Health          health.Snapshot           `json:"health"`
	Interfaces      []transport.InterfaceStat `json:"interfaces"`
	Paths           []snapshotPath            `json:"paths"`
	PathCount       int                       `json:"path_count"`
}

type snapshotPath struct {
	Hash      string  `json:"hash"`
	Hops      uint8   `json:"hops"`
	Via       string  `json:"via,omitempty"`
	Interface string  `json:"interface,omitempty"`
	Timestamp float64 `json:"timestamp"`
	Expires   float64 `json:"expires"`
}

func RunSnapshot(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("rgosnap", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configDir := fs.String("config", "", "path to config directory")
	maxHops := fs.Int("max-hops", 0, "omit paths with more hops than this (0 means no filter)")
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
		fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "rpc"), err)
		if !*quiet {
			fmt.Fprintln(stderr, warnMsg(stderr, "hint: point -config at the daemon config dir"))
		}
		return 1
	}
	client.SetTimeout(*timeout)

	stats, err := client.GetInterfaceStats()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "interface stats"), err)
		return 1
	}

	var mh *int
	if *maxHops > 0 {
		mh = maxHops
	}
	paths, err := client.GetPathTable(mh)
	if err != nil {
		fmt.Fprintf(stderr, "path table: %v\n", err)
		return 1
	}

	linkCount, err := client.GetLinkCount()
	if err != nil {
		fmt.Fprintf(stderr, "link count: %v\n", err)
		return 1
	}

	snap := NodeSnapshot{
		TS:              time.Now().UTC().Format(time.RFC3339Nano),
		TransportUptime: stats.TransportUptime,
		ActiveLinks:     linkCount,
		NetmonFlap:      stats.NetmonFlap,
		Health:          stats.Health,
		Interfaces:      stats.Interfaces,
		Paths:           make([]snapshotPath, 0, len(paths)),
		PathCount:       len(paths),
	}
	if len(stats.TransportID) > 0 {
		snap.TransportID = hex.EncodeToString(stats.TransportID)
	}
	if snap.ActiveLinks == 0 {
		snap.ActiveLinks = stats.ActiveLinks
	}
	for _, p := range paths {
		sp := snapshotPath{
			Hash:      hex.EncodeToString(p.Hash),
			Hops:      p.Hops,
			Interface: p.Interface,
			Timestamp: p.Timestamp,
			Expires:   p.Expires,
		}
		if len(p.Via) > 0 {
			sp.Via = hex.EncodeToString(p.Via)
		}
		snap.Paths = append(snap.Paths, sp)
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snap); err != nil {
		fmt.Fprintf(stderr, "json: %v\n", err)
		return 1
	}
	return 0
}
