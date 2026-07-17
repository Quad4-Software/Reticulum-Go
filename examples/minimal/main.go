// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/transport"
)

// Minimal boots Transport, creates a destination, and announces on enter.

const (
	// Shared app namespace for these demos.
	APP_NAME = "example_utilities"
)

var (
	configPath = flag.String("config", "", "path to alternative Reticulum config directory")
	listenPort = flag.Int("listen-port", 4242, "UDP interface listen port")
	targetPort = flag.Int("target-port", 4242, "UDP interface target port (for sending to peers)")
)

func main() {
	flag.Parse()
	debug.Init()

	if err := programSetup(*configPath); err != nil {
		debug.Log(debug.DebugCritical, "Failed to setup program", "error", err)
		os.Exit(1)
	}
}

func programSetup(configpath string) error {
	cfg := common.DefaultConfig()

	if len(cfg.Interfaces) == 0 {
		cfg.Interfaces = make(map[string]*common.InterfaceConfig)

		targetHost := ""
		if *targetPort != *listenPort {
			targetHost = fmt.Sprintf("127.0.0.1:%d", *targetPort)
		}

		cfg.Interfaces["UDPInterface"] = &common.InterfaceConfig{
			Type:       "UDPInterface",
			Enabled:    true,
			Address:    fmt.Sprintf("0.0.0.0:%d", *listenPort),
			TargetHost: targetHost,
			Name:       "UDPInterface",
		}
	}

	t := transport.NewTransport(cfg)
	debug.Log(debug.DebugInfo, "Transport initialized")

	id, err := identity.NewIdentity()
	if err != nil {
		return fmt.Errorf("failed to create identity: %w", err)
	}
	debug.Log(debug.DebugInfo, "Created new identity", "hash", id.GetHexHash())

	// Addressable endpoint peers can reach after it announces.
	dest, err := destination.New(
		id,
		destination.In,
		destination.Single,
		APP_NAME,
		t,
		"minimalsample",
	)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}

	// Auto-prove inbound packets so senders get delivery confirmation.
	dest.SetProofStrategy(destination.ProveAll)

	debug.Log(debug.DebugInfo, "Destination created", "hash", fmt.Sprintf("%x", dest.GetHash()))

	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	for name, ifaceConfig := range cfg.Interfaces {
		if !ifaceConfig.Enabled {
			continue
		}

		var iface interfaces.Interface
		var err error

		switch ifaceConfig.Type {
		case "UDPInterface":
			iface, err = interfaces.NewUDPInterface(
				name,
				ifaceConfig.Address,
				ifaceConfig.TargetHost,
				ifaceConfig.Enabled,
			)
		case "TCPClientInterface":
			iface, err = interfaces.NewTCPClientInterface(
				name,
				ifaceConfig.TargetHost,
				ifaceConfig.TargetPort,
				ifaceConfig.KISSFraming,
				ifaceConfig.I2PTunneled,
				ifaceConfig.Enabled,
			)
		default:
			debug.Log(debug.DebugError, "Unknown interface type", "type", ifaceConfig.Type)
			continue
		}

		if err != nil {
			debug.Log(debug.DebugError, "Failed to create interface", "name", name, "error", err)
			continue
		}

		iface.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
			t.HandlePacket(data, ni)
		})

		if err := iface.Start(); err != nil {
			debug.Log(debug.DebugError, "Failed to start interface", "name", name, "error", err)
			continue
		}

		if netIface, ok := iface.(common.NetworkInterface); ok {
			if err := t.RegisterInterface(name, netIface); err != nil {
				debug.Log(debug.DebugError, "Failed to register interface", "error", err)
			}
		}

		debug.Log(debug.DebugInfo, "Interface started successfully", "name", name)
	}

	announceLoop(dest)

	return nil
}

func announceLoop(dest *destination.Destination) {
	debug.Log(
		debug.DebugInfo,
		"Minimal example ready. Press enter to announce, Ctrl-C to quit",
		"hash", fmt.Sprintf("%x", dest.GetHash()),
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	inputChan := make(chan struct{})
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if len(line) > 0 {
				inputChan <- struct{}{}
			}
		}
	}()

	for {
		select {
		case <-sigChan:
			debug.Log(debug.DebugInfo, "Shutting down...")
			return
		case <-inputChan:
			if err := dest.Announce(false, nil, nil); err != nil {
				debug.Log(debug.DebugError, "Failed to send announce", "error", err)
			} else {
				debug.Log(debug.DebugInfo, "Sent announce", "hash", fmt.Sprintf("%x", dest.GetHash()))
			}
		}
	}
}
