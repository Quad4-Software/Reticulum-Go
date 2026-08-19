// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/reticulumconfig"
	"quad4/reticulum-go/pkg/transport"
)

var (
	timeout    = flag.Int("timeout", 0, "Timeout in seconds for path, link, and request (0 = adaptive path and link)")
	logLevel   = flag.Int("log-level", -1, "Log level override (0-7). Alias for -debug")
	configPath = flag.String("config", "", "Reticulum config file (required unless -udp)")
	useUDP     = flag.Bool("udp", false, "Use local UDP interface mode")
	listenPort = flag.Int("listen-port", 4243, "UDP listen port when -udp is enabled")
	targetPort = flag.Int("target-port", 4242, "UDP target port when -udp is enabled")
	once       = flag.Bool("once", false, "Exit after first successful page response")
)

func main() {
	flag.Parse()
	if *logLevel >= debug.DebugCritical {
		debug.SetDebugLevel(*logLevel)
	}

	if len(flag.Args()) < 1 {
		fmt.Println("Usage: page-downloader [options] <dest_hash>:<page_path>")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		fmt.Println("\nExample:")
		fmt.Println("  page-downloader -config /path/to/config \\")
		fmt.Println("    92798ea245a0afcfa559348e42d628c6:/page/index.mu")
		os.Exit(1)
	}

	if !*useUDP && strings.TrimSpace(*configPath) == "" {
		debug.Log(debug.DebugCritical, "Error: -config is required unless -udp is set")
		fmt.Println("Pick a TCP or Backbone hub from https://directory.rns.recipes/")
		os.Exit(1)
	}

	debug.Init()
	debug.Log(debug.DebugInfo, "Page Downloader Starting")

	parts := strings.SplitN(flag.Args()[0], ":", 2)
	if len(parts) != 2 {
		debug.Log(debug.DebugCritical, "Error: Invalid format. Use <dest_hash>:<page_path>")
		os.Exit(1)
	}

	destHashHex := parts[0]
	pagePath := parts[1]

	destHashBytes, err := hexDecode(destHashHex)
	if err != nil {
		debug.Log(debug.DebugCritical, "Error: Invalid destination hash", "error", err)
		os.Exit(1)
	}

	if len(destHashBytes) != 16 {
		debug.Log(debug.DebugCritical, "Error: Destination hash must be 16 bytes", "got", len(destHashBytes))
		os.Exit(1)
	}

	var cfg *common.ReticulumConfig
	if *useUDP {
		cfg = common.DefaultConfig()
		cfg.ShareInstance = false
		cfg.Interfaces = map[string]*common.InterfaceConfig{
			"UDP": {
				Type:       "UDPInterface",
				Enabled:    true,
				Address:    fmt.Sprintf("0.0.0.0:%d", *listenPort),
				TargetHost: fmt.Sprintf("127.0.0.1:%d", *targetPort),
				Name:       "UDP",
			},
		}
	} else {
		cfg, err = reticulumconfig.LoadConfig(*configPath)
		if err != nil {
			debug.Log(debug.DebugCritical, "Error loading config", "error", err)
			os.Exit(1)
		}
	}

	if _, err := backbone.Init(backbone.ParseBackend(cfg.BackboneIO)); err != nil {
		debug.Log(debug.DebugCritical, "Error initialising backbone hub", "error", err)
		os.Exit(1)
	}

	trans := transport.NewTransport(cfg)
	if err := trans.Start(); err != nil {
		debug.Log(debug.DebugCritical, "Error starting transport", "error", err)
		os.Exit(1)
	}
	debug.Log(debug.DebugInfo, "Transport started")

	for name, ifaceConfig := range cfg.Interfaces {
		if ifaceConfig == nil || !ifaceConfig.Enabled {
			continue
		}
		ifaceConfig.Name = name
		iface, ierr := interfaces.NewFromConfig(name, ifaceConfig)
		if ierr != nil {
			debug.Log(debug.DebugError, "Failed to create interface", "name", name, "error", ierr)
			continue
		}

		iface.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
			if trans != nil {
				trans.HandlePacket(data, ni)
			}
		})

		if err := iface.Start(); err != nil {
			debug.Log(debug.DebugError, "Failed to start interface", "name", name, "error", err)
			continue
		}

		if netIface, ok := iface.(common.NetworkInterface); ok {
			if err := trans.RegisterInterface(name, netIface); err != nil {
				debug.Log(debug.DebugError, "Failed to register interface", "name", name, "error", err)
			}
		}
		debug.Log(debug.DebugInfo, "Interface started", "name", name)
	}

	debug.Log(debug.DebugInfo, "Target destination", "hash", fmt.Sprintf("%x", destHashBytes))

	debug.Log(debug.DebugInfo, "Looking for path", "to", destHashHex)

	if *useUDP {
		trans.UpdatePath(destHashBytes, nil, "UDP", 1)
		debug.Log(debug.DebugInfo, "Seeded direct UDP path", "to", destHashHex)
	}

	pathTimeout := time.Duration(*timeout) * time.Second
	if pathTimeout <= 0 {
		pathTimeout = trans.PathResponseWindow(destHashBytes)
	}
	pathCtx, cancelPath := context.WithTimeout(context.Background(), pathTimeout)
	defer cancelPath()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	lastReq := time.Time{}
	for {
		if trans.HasPath(destHashBytes) {
			break
		}
		if time.Since(lastReq) >= 2*time.Second {
			if err := trans.RequestPath(destHashBytes, "", nil, false); err != nil {
				debug.Log(debug.DebugError, "Failed to request path", "error", err)
			}
			lastReq = time.Now()
		}
		select {
		case <-pathCtx.Done():
			debug.Log(debug.DebugCritical, "Path to destination not found", "to", destHashHex, "timeout", pathTimeout)
			debug.Log(debug.DebugInfo, "Make sure the target destination is:")
			debug.Log(debug.DebugInfo, "  1. Running and announcing")
			debug.Log(debug.DebugInfo, "  2. Reachable on the network")
			debug.Log(debug.DebugInfo, "  3. The destination hash is correct")
			_ = trans.Close()
			os.Exit(1)
		case <-ticker.C:
		}
	}
	debug.Log(debug.DebugInfo, "Path found!")

	debug.Log(debug.DebugInfo, "Resolving destination identity", "hash", destHashHex)
	var remoteIdentity *identity.Identity
	for {
		remoteIdentity, err = identity.Recall(destHashBytes)
		if err == nil && remoteIdentity != nil {
			debug.Log(debug.DebugInfo, "Destination identity resolved", "hash", destHashHex)
			break
		}
		select {
		case <-pathCtx.Done():
			debug.Log(debug.DebugCritical, "Destination identity not found", "hash", destHashHex)
			debug.Log(debug.DebugInfo, "Wait for an announce from this destination and try again")
			_ = trans.Close()
			os.Exit(1)
		case <-time.After(50 * time.Millisecond):
		}
	}

	dest, err := destination.FromHash(destHashBytes, remoteIdentity, destination.Single, trans)
	if err != nil {
		debug.Log(debug.DebugCritical, "Error creating destination", "error", err)
		_ = trans.Close()
		os.Exit(1)
	}

	debug.Log(debug.DebugInfo, "Establishing link", "to", destHashHex)

	var linkInterface common.NetworkInterface
	if ifaceName := trans.NextHopInterface(destHashBytes); ifaceName != "" {
		if iface, ok := trans.GetInterfaces()[ifaceName]; ok {
			linkInterface = iface
		}
	}

	established := make(chan bool, 1)
	failed := make(chan bool, 1)

	l := link.NewLink(dest, trans, linkInterface, func(lnk *link.Link) {
		debug.Log(debug.DebugInfo, "Link established callback")
		established <- true
	}, func(lnk *link.Link) {
		debug.Log(debug.DebugInfo, "Link closed callback")
		failed <- true
	})

	if err := l.Establish(); err != nil {
		debug.Log(debug.DebugCritical, "Error establishing link", "error", err)
		_ = trans.Close()
		os.Exit(1)
	}

	startTime := time.Now()
	linkTimeout := l.EstablishmentTimeout() + 6*time.Second
	if *timeout > 0 {
		linkTimeout = time.Duration(*timeout) * time.Second
	}
	select {
	case <-established:
		debug.Log(debug.DebugInfo, "Link established", "elapsed", time.Since(startTime))
	case <-failed:
		debug.Log(debug.DebugCritical, "Link establishment failed")
		_ = trans.Close()
		os.Exit(1)
	case <-time.After(linkTimeout):
		debug.Log(debug.DebugCritical, "Link establishment timeout")
		_ = trans.Close()
		os.Exit(1)
	}

	debug.Log(debug.DebugInfo, "Link established!", "rtt", fmt.Sprintf("%.3fs", l.GetRTT()))
	debug.Log(debug.DebugInfo, "Requesting page", "path", pagePath)

	responseChan := make(chan []byte, 1)
	errorChan := make(chan error, 1)

	reqTimeout := time.Duration(*timeout) * time.Second
	if reqTimeout <= 0 {
		reqTimeout = time.Duration(trans.FirstHopTimeout(destHashBytes)*float64(time.Second)) + 30*time.Second
	}
	receipt, err := l.Request(pagePath, nil, reqTimeout)
	if err != nil {
		debug.Log(debug.DebugCritical, "Error sending request", "error", err)
		l.Teardown()
		_ = trans.Close()
		os.Exit(1)
	}

	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			if receipt.Concluded() {
				if receipt.GetStatus() == link.StatusActive {
					responseChan <- receipt.GetResponse()
				} else {
					errorChan <- fmt.Errorf("request failed")
				}
				return
			}
		}
	}()

	select {
	case response := <-responseChan:
		fmt.Printf("\n=== Page Content (%d bytes) ===\n", len(response))
		fmt.Println(string(response))
		fmt.Println("=== End of Page ===")
	case err := <-errorChan:
		debug.Log(debug.DebugCritical, "Request failed", "error", err)
		l.Teardown()
		_ = trans.Close()
		os.Exit(1)
	case <-time.After(reqTimeout):
		debug.Log(debug.DebugCritical, "Request timeout")
		l.Teardown()
		_ = trans.Close()
		os.Exit(1)
	}

	if *once {
		l.Teardown()
		_ = trans.Close()
		debug.Log(debug.DebugInfo, "Done (-once), exiting")
		return
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	debug.Log(debug.DebugInfo, "Press Ctrl+C to exit...")
	<-sigChan

	l.Teardown()
	_ = trans.Close()
	debug.Log(debug.DebugInfo, "Goodbye!")
}

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("hex string must have even length")
	}

	result := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		var b byte
		_, err := fmt.Sscanf(s[i:i+2], "%02x", &b)
		if err != nil {
			return nil, fmt.Errorf("invalid hex at position %d: %w", i, err)
		}
		result[i/2] = b
	}
	return result, nil
}
