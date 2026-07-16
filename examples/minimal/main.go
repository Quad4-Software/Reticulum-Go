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

// This RNS example demonstrates a minimal setup, that
// will start up the Reticulum Network Stack, generate a
// new destination, and let the user send an announce.

const (
	// Let's define an app name. We'll use this for all
	// destinations we create. Since this basic example
	// is part of a range of example utilities, we'll put
	// them all within the app namespace "example_utilities"
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
	// Initialize config
	cfg := common.DefaultConfig()

	// Add a simple UDP interface for local testing
	if len(cfg.Interfaces) == 0 {
		cfg.Interfaces = make(map[string]*common.InterfaceConfig)

		// Build target address if target port is different from listen port
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

	// We must first initialise Reticulum/Transport
	t := transport.NewTransport(cfg)
	debug.Log(debug.DebugInfo, "Transport initialized")

	// Randomly create a new identity for our example
	id, err := identity.NewIdentity()
	if err != nil {
		return fmt.Errorf("failed to create identity: %w", err)
	}
	debug.Log(debug.DebugInfo, "Created new identity", "hash", id.GetHexHash())

	// Using the identity we just created, we create a destination.
	// Destinations are endpoints in Reticulum, that can be addressed
	// and communicated with. Destinations can also announce their
	// existence, which will let the network know they are reachable
	// and automatically create paths to them, from anywhere else
	// in the network.
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

	// We configure the destination to automatically prove all
	// packets addressed to it. By doing this, RNS will automatically
	// generate a proof for each incoming packet and transmit it
	// back to the sender of that packet. This will let anyone that
	// tries to communicate with the destination know whether their
	// communication was received correctly.
	dest.SetProofStrategy(destination.ProveAll)

	debug.Log(debug.DebugInfo, "Destination created", "hash", fmt.Sprintf("%x", dest.GetHash()))

	// Start transport
	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	// Initialize and start interfaces
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

		// Set packet callback
		iface.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
			t.HandlePacket(data, ni)
		})

		// Start interface
		if err := iface.Start(); err != nil {
			debug.Log(debug.DebugError, "Failed to start interface", "name", name, "error", err)
			continue
		}

		// Register with transport
		if netIface, ok := iface.(common.NetworkInterface); ok {
			if err := t.RegisterInterface(name, netIface); err != nil {
				debug.Log(debug.DebugError, "Failed to register interface", "error", err)
			}
		}

		debug.Log(debug.DebugInfo, "Interface started successfully", "name", name)
	}

	// Everything's ready!
	// Let's hand over control to the announce loop
	announceLoop(dest)

	return nil
}

func announceLoop(dest *destination.Destination) {
	// Let the user know that everything is ready
	debug.Log(
		debug.DebugInfo,
		"Minimal example running, hit enter to manually send an announce (Ctrl-C to quit)",
		"hash", fmt.Sprintf("%x", dest.GetHash()),
	)

	// Set up signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create a channel for user input
	inputChan := make(chan struct{})
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				// EOF or error, stop reading
				return
			}
			if len(line) > 0 {
				inputChan <- struct{}{}
			}
		}
	}()

	// We enter a loop that runs until the users exits.
	// If the user hits enter, we will announce our server
	// destination on the network, which will let clients
	// know how to create messages directed towards it.
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
