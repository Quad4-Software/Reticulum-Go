// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
)


func RunProbe(args []string) int {
	fs := flag.NewFlagSet("rgoprobe", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configDir := fs.String("config", "", "path to config directory")
	size := fs.Int("s", rnsutil.DefaultProbeSize, "probe payload size in bytes")
	timeoutSec := fs.Float64("t", 0, "per-probe timeout in seconds (0 = first-hop based)")
	waitSec := fs.Float64("w", 0, "delay between probes in seconds")
	count := fs.Int("n", 1, "number of probes")
	verbose := fs.Bool("v", false, "show next-hop details")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: rgoprobe [flags] <full_name> <destination_hash_hex>")
		return 2
	}
	fullName := fs.Arg(0)
	if _, _, err := destination.ParseName(fullName); err != nil {
		fmt.Fprintf(os.Stderr, "name: %v\n", err)
		return 2
	}

	destHash, err := hex.DecodeString(fs.Arg(1))
	if err != nil || len(destHash) != 16 {
		fmt.Fprintln(os.Stderr, "destination hash must be 32 hex characters (16 bytes)")
		return 2
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}
	cfg.ShareInstance = true
	if cfg.SharedInstanceType == "" {
		cfg.SharedInstanceType = common.SharedInstanceTCP
	}

	n, err := node.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "node: %v\n", err)
		return 1
	}
	if err := n.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start: %v\n", err)
		return 1
	}
	defer n.Stop()

	tr := n.Transport()
	var pathTimeout time.Duration
	if *timeoutSec > 0 {
		pathTimeout = time.Duration(*timeoutSec * float64(time.Second))
	} else {
		pathTimeout = rnsutil.DefaultProbeTimeout + time.Duration(tr.GetFirstHopTimeoutRPC(destHash)*float64(time.Second))
	}

	pathCtx, cancel := context.WithTimeout(context.Background(), pathTimeout)
	defer cancel()
	fmt.Fprintf(os.Stdout, "Path to %s requested\n", rnsutil.PrettyHex(destHash))
	if err := rnsutil.WaitPath(pathCtx, tr, destHash); err != nil {
		fmt.Fprintf(os.Stderr, "path request timed out: %v\n", err)
		return 1
	}

	wait := time.Duration(*waitSec * float64(time.Second))
	sent, replies := 0, 0
	for i := 0; i < *count; i++ {
		if i > 0 && wait > 0 {
			time.Sleep(wait)
		}
		var probeTimeout time.Duration
		if *timeoutSec > 0 {
			probeTimeout = time.Duration(*timeoutSec * float64(time.Second))
		} else {
			probeTimeout = rnsutil.DefaultProbeTimeout + time.Duration(tr.GetFirstHopTimeoutRPC(destHash)*float64(time.Second))
		}
		ctx, cancelProbe := context.WithTimeout(context.Background(), probeTimeout)

		more := ""
		if *verbose {
			if nh := tr.NextHop(destHash); len(nh) > 0 {
				more += " via " + rnsutil.PrettyHex(nh)
			}
			if ifn := tr.NextHopInterface(destHash); ifn != "" && ifn != "None" {
				more += " on " + ifn
			}
		}
		fmt.Fprintf(os.Stdout, "Sent probe %d (%d bytes) to %s%s\n", i+1, *size, rnsutil.PrettyHex(destHash), more)
		sent++

		res, err := rnsutil.SendProbe(ctx, tr, destHash, *size)
		cancelProbe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "probe failed: %v\n", err)
			continue
		}
		if !res.Delivered {
			fmt.Fprintln(os.Stdout, "Probe timed out")
			continue
		}
		replies++
		rtt := res.RTT
		rttStr := fmt.Sprintf("%.3f milliseconds", float64(rtt.Microseconds())/1000)
		if rtt >= time.Second {
			rttStr = fmt.Sprintf("%.3f seconds", rtt.Seconds())
		}
		hopWord := "hops"
		if res.Hops == 1 {
			hopWord = "hop"
		}
		fmt.Fprintf(os.Stdout, "Valid reply from %s\nRound-trip time is %s over %d %s\n",
			rnsutil.PrettyHex(destHash), rttStr, res.Hops, hopWord)
	}

	loss := 0.0
	if sent > 0 {
		loss = (1 - float64(replies)/float64(sent)) * 100
	}
	fmt.Fprintf(os.Stdout, "Sent %d, received %d, packet loss %.2f%%\n", sent, replies, loss)
	if replies == 0 {
		return 1
	}
	if loss > 0 {
		return 2
	}
	return 0
}
