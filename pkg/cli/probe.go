// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
)

func RunProbe(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("rgoprobe", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configDir := fs.String("config", "", "path to config directory")
	size := fs.Int("s", rnsutil.DefaultProbeSize, "probe payload size in bytes")
	timeoutSec := fs.Float64("t", 0, "per-probe timeout in seconds (0 = first-hop based)")
	waitSec := fs.Float64("w", 0, "delay between probes in seconds")
	count := fs.Int("n", 1, "number of probes")
	verbose := fs.Bool("v", false, "show next-hop details")
	jsonOut := fs.Bool("json", false, "emit JSON results")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: rgoprobe [flags] <full_name> <destination_hash_hex>")
		return 2
	}
	fullName := fs.Arg(0)
	if _, _, err := destination.ParseName(fullName); err != nil {
		fmt.Fprintf(stderr, "name: %v\n", err)
		return 2
	}

	destHash, err := hex.DecodeString(fs.Arg(1))
	if err != nil || len(destHash) != 16 {
		fmt.Fprintln(stderr, "destination hash must be 32 hex characters (16 bytes)")
		return 2
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}
	cfg.ShareInstance = true

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
	var pathTimeout time.Duration
	if *timeoutSec > 0 {
		pathTimeout = time.Duration(*timeoutSec * float64(time.Second))
	} else {
		pathTimeout = rnsutil.DefaultProbeTimeout + time.Duration(tr.GetFirstHopTimeoutRPC(destHash)*float64(time.Second))
	}

	pathCtx, cancel := context.WithTimeout(context.Background(), pathTimeout)
	defer cancel()
	fmt.Fprintln(stdout, infoMsg(stdout, fmt.Sprintf("Path to %s requested", rnsutil.PrettyHex(destHash))))
	if err := rnsutil.WaitPath(pathCtx, tr, destHash); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "path request timed out"), err)
		return 1
	}

	wait := time.Duration(*waitSec * float64(time.Second))
	sent, replies := 0, 0
	type probeJSON struct {
		Index     int     `json:"index"`
		Delivered bool    `json:"delivered"`
		RTTMS     float64 `json:"rtt_ms,omitempty"`
		Hops      uint8   `json:"hops,omitempty"`
		Error     string  `json:"error,omitempty"`
	}
	var jsonResults []probeJSON

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
		if !*jsonOut {
			fmt.Fprintf(stdout, "Sent probe %d (%d bytes) to %s%s\n", i+1, *size, rnsutil.PrettyHex(destHash), more)
		}
		sent++

		res, err := rnsutil.SendProbe(ctx, tr, destHash, *size)
		cancelProbe()
		if err != nil {
			if *jsonOut {
				jsonResults = append(jsonResults, probeJSON{Index: i + 1, Error: err.Error()})
			} else {
				fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "probe failed"), err)
			}
			continue
		}
		if !res.Delivered {
			if *jsonOut {
				jsonResults = append(jsonResults, probeJSON{Index: i + 1, Delivered: false})
			} else {
				fmt.Fprintln(stdout, warnMsg(stdout, "Probe timed out"))
			}
			continue
		}
		replies++
		rtt := res.RTT
		if *jsonOut {
			jsonResults = append(jsonResults, probeJSON{
				Index:     i + 1,
				Delivered: true,
				RTTMS:     float64(rtt.Microseconds()) / 1000,
				Hops:      res.Hops,
			})
			continue
		}
		rttStr := fmt.Sprintf("%.3f milliseconds", float64(rtt.Microseconds())/1000)
		if rtt >= time.Second {
			rttStr = fmt.Sprintf("%.3f seconds", rtt.Seconds())
		}
		hopWord := "hops"
		if res.Hops == 1 {
			hopWord = "hop"
		}
		fmt.Fprintf(stdout, "%s %s\nRound-trip time is %s over %d %s\n",
			okMsg(stdout, "Valid reply from"), rnsutil.PrettyHex(destHash), rttStr, res.Hops, hopWord)
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"sent":    sent,
			"replies": replies,
			"loss_pct": func() float64 {
				if sent == 0 {
					return 0
				}
				return (1 - float64(replies)/float64(sent)) * 100
			}(),
			"probes": jsonResults,
		})
	} else {
		loss := 0.0
		if sent > 0 {
			loss = (1 - float64(replies)/float64(sent)) * 100
		}
		summary := fmt.Sprintf("Sent %d, received %d, packet loss %.2f%%", sent, replies, loss)
		switch {
		case replies == 0:
			summary = errMsg(stdout, summary)
		case loss > 0:
			summary = warnMsg(stdout, summary)
		default:
			summary = okMsg(stdout, summary)
		}
		fmt.Fprintln(stdout, summary)
	}
	if replies == 0 {
		return 1
	}
	loss := 0.0
	if sent > 0 {
		loss = (1 - float64(replies)/float64(sent)) * 100
	}
	if loss > 0 {
		return 2
	}
	return 0
}
