// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	rlink "quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/rnsutil"
	"quad4/reticulum-go/pkg/transport"
)

func RunSpeedtest(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("rgospeed", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configDir := fs.String("config", "", "path to config directory")
	identityPath := fs.String("identity", "", "path to identity file (listen mode)")
	listenMode := fs.Bool("l", false, "listen as speedtest server")
	daemon := fs.Bool("daemon", false, "listen forever (implies -l -m, default announce every 120s)")
	loopback := fs.Bool("loopback", false, "in-process pipe speedtest (CI / local smoke)")
	printID := fs.Bool("p", false, "print identity and destination hash then exit")
	multi := fs.Bool("m", false, "listen: serve multiple clients (default: one then exit)")
	ifaceSel := fs.String("iface", "all", "interfaces to use: all, or comma-separated config names")
	dataCap := fs.Int64("bytes", rlink.DefaultSpeedtestDataCap, "plaintext bytes to transfer")
	minBps := fs.Float64("min-bps", 0, "fail if sustained rate is below this (0 disables; loopback default 1e6)")
	timeoutSec := fs.Float64("timeout", 60, "overall timeout in seconds")
	announceSec := fs.Float64("announce", 0, "listen: announce interval seconds (0 = once, <0 = never)")
	jsonOut := fs.Bool("json", false, "emit JSON result lines on stdout")
	quiet := fs.Bool("q", false, "suppress progress logs")

	fs.Usage = func() {
		fmt.Fprintf(stderr, `rgospeed - link throughput test (RNS Speedtest-style)

Modes:
  -loopback              in-process pipe (CI liveness)
  -l                     listen as speedtest.server (one client then exit)
  -daemon                persistent server for docker / VPS (implies -l -m)
  <destination_hash>     connect and blast to a listening server

Usage:
  reticulum-go speedtest -loopback [flags]
  reticulum-go speedtest -l [flags]
  reticulum-go speedtest -daemon [flags]
  reticulum-go speedtest [flags] <destination_hash>
  rgospeed ...

Flags:
`)
		fs.PrintDefaults()
		fmt.Fprintf(stderr, `
Examples:
  reticulum-go speedtest -loopback
  reticulum-go speedtest -daemon -iface tcp -json
  reticulum-go speedtest -l -iface tcp,udp -bytes 1048576
  reticulum-go speedtest -iface tcp -bytes 1048576 <server_dest_hash>
`)
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if !*quiet {
		debug.SetDebugLevel(debug.DebugCritical)
	}

	if *daemon {
		*listenMode = true
		*multi = true
		if *announceSec == 0 {
			*announceSec = 120
		}
	}

	timeout := time.Duration(*timeoutSec * float64(time.Second))
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	spOpt := rlink.SpeedtestOptions{
		DataCap: *dataCap,
		Timeout: timeout,
	}
	useLoopback := *loopback || (fs.NArg() == 0 && !*listenMode && !*printID)
	if useLoopback && *minBps == 0 {
		spOpt.EnforceFloor = true
		spOpt.MinBytesPerSec = rlink.DefaultSpeedtestMinBytesPerSec
	} else if *minBps > 0 {
		spOpt.EnforceFloor = true
		spOpt.MinBytesPerSec = *minBps
	}

	if useLoopback {
		if *listenMode {
			fmt.Fprintln(stderr, "usage: speedtest -loopback [flags]")
			return 2
		}
		return emitSpeedResult(stdout, stderr, *jsonOut, "loopback", "", spOpt, func() (rlink.SpeedtestResult, error) {
			return rlink.RunLoopbackSpeedtest(spOpt)
		})
	}

	cfg, err := rnsutil.LoadConfigDir(*configDir)
	if err != nil {
		fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}
	// Speedtest must own its interfaces. Attaching to an ambient shared
	// instance would skip configured UDP/TCP links.
	cfg.ShareInstance = false

	activeIfaces, err := rnsutil.SelectInterfaces(cfg, *ifaceSel)
	if err != nil {
		fmt.Fprintf(stderr, "iface: %v\n", err)
		return 2
	}

	idPath := *identityPath
	if idPath == "" {
		idPath = rnsutil.SpeedtestIdentityPath(rnsutil.StorageDir(cfg))
	}
	id, err := rnsutil.PrepareSpeedtestIdentity(idPath)
	if err != nil {
		fmt.Fprintf(stderr, "identity: %v\n", err)
		return 2
	}

	if *printID {
		destHash := destination.Hash(id, rnsutil.SpeedtestAppName, rnsutil.SpeedtestAspect)
		fmt.Fprintf(stdout, "%s : %s\n", infoMsg(stdout, "Identity"), hex.EncodeToString(id.Hash()))
		fmt.Fprintf(stdout, "%s : %s\n", infoMsg(stdout, "Listening on"), hex.EncodeToString(destHash))
		return 0
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

	fmt.Fprintf(stdout, "%s interfaces=%v\n", infoMsg(stdout, "speedtest using"), activeIfaces)

	switch {
	case *listenMode:
		return runSpeedtestListen(tr, id, spOpt, !*multi, *announceSec, *jsonOut, stdout, stderr)
	default:
		if fs.NArg() != 1 {
			fmt.Fprintln(stderr, "usage: speedtest [flags] <destination_hash>")
			fmt.Fprintln(stderr, "       speedtest -l|-daemon [flags]")
			fmt.Fprintln(stderr, "       speedtest -loopback [flags]")
			return 2
		}
		destHash, err := rnsutil.ParseDestHash(fs.Arg(0))
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
		return runSpeedtestClient(tr, destHash, spOpt, timeout, *jsonOut, stdout, stderr)
	}
}

func runSpeedtestListen(
	tr *transport.Transport,
	id *identity.Identity,
	spOpt rlink.SpeedtestOptions,
	once bool,
	announceSec float64,
	jsonOut bool,
	stdout, stderr io.Writer,
) int {
	dest, err := destination.New(id, destination.In, destination.Single, rnsutil.SpeedtestAppName, tr, rnsutil.SpeedtestAspect)
	if err != nil {
		fmt.Fprintf(stderr, "destination: %v\n", err)
		return 1
	}
	dest.AcceptsLinks(true)

	fmt.Fprintf(stdout, "%s %s\n", infoMsg(stdout, "Listening on"), hex.EncodeToString(dest.GetHash()))
	if once {
		fmt.Fprintln(stdout, infoMsg(stdout, "mode=oneshot (exit after one client)"))
	} else {
		fmt.Fprintln(stdout, infoMsg(stdout, "mode=daemon (serve until signal)"))
	}

	incoming := make(chan *rlink.Link, 1)
	dest.SetLinkEstablishedCallback(func(v any) {
		lnk, ok := v.(*rlink.Link)
		if !ok || lnk == nil {
			return
		}
		lnk.Start()
		select {
		case incoming <- lnk:
		default:
			fmt.Fprintln(stderr, warnMsg(stderr, "busy, rejecting extra link"))
			lnk.Teardown()
		}
	})

	if announceSec >= 0 {
		if err := dest.Announce(false, nil, nil); err != nil {
			fmt.Fprintf(stderr, "announce: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, infoMsg(stdout, "announce sent"))
	}
	var announceStop chan struct{}
	if announceSec > 0 {
		announceStop = make(chan struct{})
		go func() {
			t := time.NewTicker(time.Duration(announceSec * float64(time.Second)))
			defer t.Stop()
			for {
				select {
				case <-announceStop:
					return
				case <-t.C:
					if err := dest.Announce(false, nil, nil); err != nil {
						fmt.Fprintf(stderr, "announce: %v\n", err)
						continue
					}
					fmt.Fprintln(stdout, infoMsg(stdout, "announce refreshed"))
				}
			}
		}()
		defer close(announceStop)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)

	for {
		fmt.Fprintln(stdout, infoMsg(stdout, "Waiting for client link..."))
		var lnk *rlink.Link
		select {
		case lnk = <-incoming:
		case <-sig:
			fmt.Fprintln(stdout, "interrupted")
			return 130
		}

		ifaceName := linkIfaceName(lnk)
		fmt.Fprintf(stdout, "%s iface=%s link=%s\n",
			infoMsg(stdout, "Client linked, receiving..."),
			ifaceName,
			hex.EncodeToString(lnk.GetLinkID()),
		)
		recvOpt := spOpt
		recvOpt.EnforceFloor = false
		res, err := rlink.ReceiveOnLink(lnk, recvOpt)
		if err == nil {
			if ackErr := rlink.SendSpeedAck(lnk, res.BytesRecv); ackErr != nil {
				fmt.Fprintf(stderr, "ack: %v\n", ackErr)
			}
		}
		code := emitSpeedResultValues(stdout, stderr, jsonOut, "server", ifaceName, res, err)
		lnk.Teardown()
		if once {
			return code
		}
	}
}

func runSpeedtestClient(
	tr *transport.Transport,
	destHash []byte,
	spOpt rlink.SpeedtestOptions,
	pathTimeout time.Duration,
	jsonOut bool,
	stdout, stderr io.Writer,
) int {
	ctx, cancel := context.WithTimeout(context.Background(), pathTimeout)
	defer cancel()

	fmt.Fprintln(stdout, infoMsg(stdout, fmt.Sprintf("Path to %s...", rnsutil.PrettyHex(destHash))))
	lnk, err := rnsutil.EstablishSpeedtestLink(ctx, tr, destHash)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "link failed"), err)
		return 1
	}
	defer lnk.Teardown()
	lnk.Start()

	ifaceName := linkIfaceName(lnk)
	fmt.Fprintf(stdout, "%s iface=%s\n", infoMsg(stdout, "Link established, sending..."), ifaceName)
	blastOpt := spOpt
	blastOpt.EnforceFloor = false
	if blastOpt.SendPace <= 0 {
		// Pace UDP/TCP blasts so the peer socket is not overrun.
		blastOpt.SendPace = 100 * time.Microsecond
	}
	res, err := rlink.BlastOnLink(lnk, blastOpt)
	if err != nil {
		return emitSpeedResultValues(stdout, stderr, jsonOut, "client", ifaceName, res, err)
	}

	ackTimeout := min(spOpt.Timeout, 30*time.Second)
	confirmed, ackErr := rlink.WaitSpeedAck(lnk, ackTimeout)
	if ackErr != nil {
		fmt.Fprintf(stderr, "%s: %v (TX stats still valid)\n", warnMsg(stderr, "no server ack"), ackErr)
	} else {
		res.ConfirmedRecv = confirmed
		res.BytesRecv = confirmed
	}

	if spOpt.EnforceFloor && res.BytesPerSec < spOpt.MinBytesPerSec {
		err = fmt.Errorf("speedtest: %.0f B/s below floor %.0f B/s", res.BytesPerSec, spOpt.MinBytesPerSec)
	}
	return emitSpeedResultValues(stdout, stderr, jsonOut, "client", ifaceName, res, err)
}

func linkIfaceName(lnk *rlink.Link) string {
	if lnk == nil {
		return ""
	}
	ni := lnk.LinkedNetworkInterface()
	if ni == nil {
		return ""
	}
	return ni.GetName()
}

func emitSpeedResult(stdout, stderr io.Writer, jsonOut bool, mode, iface string, spOpt rlink.SpeedtestOptions, run func() (rlink.SpeedtestResult, error)) int {
	res, err := run()
	return emitSpeedResultValues(stdout, stderr, jsonOut, mode, iface, res, err)
}

func emitSpeedResultValues(stdout, stderr io.Writer, jsonOut bool, mode, iface string, res rlink.SpeedtestResult, err error) int {
	out := map[string]any{
		"mode":          mode,
		"bytes_sent":    res.BytesSent,
		"bytes_recv":    res.BytesRecv,
		"duration_sec":  res.Duration.Seconds(),
		"bytes_per_sec": res.BytesPerSec,
		"mdu":           res.MDU,
		"ok":            err == nil,
	}
	if iface != "" {
		out["interface"] = iface
	}
	if res.ConfirmedRecv > 0 {
		out["confirmed_recv"] = res.ConfirmedRecv
	}
	if err != nil {
		out["error"] = err.Error()
	}

	// Always emit a one-line summary on stdout for docker logs / journald.
	ok := err == nil
	fmt.Fprintf(stdout, "speedtest_result mode=%s ok=%t iface=%q bps=%.0f sent=%d recv=%d duration=%s mdu=%d\n",
		mode, ok, iface, res.BytesPerSec, res.BytesSent, res.BytesRecv, res.Duration.Round(time.Millisecond), res.MDU)

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
	} else {
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", errMsg(stderr, "speedtest failed"), err)
		} else {
			fmt.Fprintf(stdout, "%s (%s)\n", okMsg(stdout, "speedtest ok"), mode)
		}
		if iface != "" {
			fmt.Fprintf(stdout, "  interface   : %s\n", iface)
		}
		if res.BytesSent > 0 {
			fmt.Fprintf(stdout, "  sent        : %d bytes\n", res.BytesSent)
		}
		if res.BytesRecv > 0 {
			fmt.Fprintf(stdout, "  received    : %d bytes\n", res.BytesRecv)
		}
		if res.ConfirmedRecv > 0 && mode == "client" {
			fmt.Fprintf(stdout, "  confirmed   : %d bytes (server ack)\n", res.ConfirmedRecv)
		}
		fmt.Fprintf(stdout, "  mdu         : %d\n", res.MDU)
		fmt.Fprintf(stdout, "  duration    : %s\n", res.Duration.Round(time.Millisecond))
		fmt.Fprintf(stdout, "  throughput  : %.2f MB/s (%.0f B/s)\n", res.BytesPerSec/1e6, res.BytesPerSec)
	}
	if err != nil {
		return 1
	}
	return 0
}
