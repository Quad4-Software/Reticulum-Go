// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
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

// This RNS example demonstrates setting up announce
// callbacks, which will let an application receive a
// notification when an announce relevant for it arrives

const APP_NAME = "example_utilities"

var (
	configPath = flag.String("config", "", "path to alternative Reticulum config directory")
	listenPort = flag.Int("listen-port", 4242, "UDP interface listen port")
	targetPort = flag.Int("target-port", 4242, "UDP interface target port (for sending to peers)")
)

// We initialize two lists of strings to use as app_data
var fruits = []string{"Peach", "Quince", "Date", "Tangerine", "Pomelo", "Carambola", "Grape"}
var nobleGases = []string{"Helium", "Neon", "Argon", "Krypton", "Xenon", "Radon", "Oganesson"}

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
	cfg := loadConfig(configpath)

	// Initialize Reticulum
	t := transport.NewTransport(cfg)

	// Randomly create a new identity for our example
	id, err := identity.NewIdentity()
	if err != nil {
		return err
	}

	// Using the identity we just created, we create two destinations
	// in the "example_utilities.announcesample" application space.
	//
	// Destinations are endpoints in Reticulum, that can be addressed
	// and communicated with. Destinations can also announce their
	// existence, which will let the network know they are reachable
	// and automatically create paths to them, from anywhere else
	// in the network.
	destination1, err := destination.New(
		id,
		destination.In,
		destination.Single,
		APP_NAME,
		t,
		"announcesample",
		"fruits",
	)
	if err != nil {
		return err
	}

	destination2, err := destination.New(
		id,
		destination.In,
		destination.Single,
		APP_NAME,
		t,
		"announcesample",
		"noble_gases",
	)
	if err != nil {
		return err
	}

	// We configure the destinations to automatically prove all
	// packets addressed to it. By doing this, RNS will automatically
	// generate a proof for each incoming packet and transmit it
	// back to the sender of that packet. This will let anyone that
	// tries to communicate with the destination know whether their
	// communication was received correctly.
	destination1.SetProofStrategy(destination.ProveAll)
	destination2.SetProofStrategy(destination.ProveAll)

	// We create an announce handler and configure it to only ask for
	// announces from "example_utilities.announcesample.fruits".
	// Try changing the filter and see what happens.
	announceHandler := NewExampleAnnounceHandler(
		[]string{"example_utilities.announcesample.fruits"},
	)

	// Start transport
	if err := t.Start(); err != nil {
		return err
	}

	// Register the announce handler with Transport
	t.RegisterAnnounceHandler(announceHandler)

	// Initialize interfaces
	if err := initializeInterfaces(cfg, t); err != nil {
		return err
	}

	// Everything's ready!
	// Let's hand over control to the announce loop
	announceLoop(destination1, destination2)

	return nil
}

func announceLoop(destination1, destination2 *destination.Destination) {
	// Let the user know that everything is ready
	debug.Log(debug.DebugInfo, "Announce example running, hit enter to manually send an announce (Ctrl-C to quit)")

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
			// Randomly select a fruit
			fruit := fruits[rand.Intn(len(fruits))] // #nosec G404 -- Using math/rand for non-security demo data selection

			// Send the announce including the app data
			destination1.SetDefaultAppData([]byte(fruit))
			if err := destination1.Announce(false, nil, nil); err != nil {
				debug.Log(debug.DebugError, "Failed to send announce from destination 1", "error", err)
			} else {
				debug.Log(
					debug.DebugInfo,
					"Sent announce from destination 1",
					"hash", fmt.Sprintf("%x", destination1.GetHash()),
					"name", destination1.ExpandName(),
				)
			}

			// Randomly select a noble gas
			nobleGas := nobleGases[rand.Intn(len(nobleGases))] // #nosec G404 -- Using math/rand for non-security demo data selection

			// Send the announce including the app data
			destination2.SetDefaultAppData([]byte(nobleGas))
			if err := destination2.Announce(false, nil, nil); err != nil {
				debug.Log(debug.DebugError, "Failed to send announce from destination 2", "error", err)
			} else {
				debug.Log(
					debug.DebugInfo,
					"Sent announce from destination 2",
					"hash", fmt.Sprintf("%x", destination2.GetHash()),
					"name", destination2.ExpandName(),
				)
			}
		}
	}
}

// ========== Announce Handler ==========

// ExampleAnnounceHandler defines a handler for announce messages
type ExampleAnnounceHandler struct {
	aspectFilter []string
}

// NewExampleAnnounceHandler creates a new announce handler with optional aspect filter
func NewExampleAnnounceHandler(aspectFilter []string) *ExampleAnnounceHandler {
	return &ExampleAnnounceHandler{
		aspectFilter: aspectFilter,
	}
}

// AspectFilter returns the aspect filter for this handler
func (h *ExampleAnnounceHandler) AspectFilter() []string {
	return h.aspectFilter
}

// ReceivedAnnounce is called by Reticulum's Transport
// system when an announce arrives that matches the
// configured aspect filter. Filters must be specific,
// and cannot use wildcards.
func (h *ExampleAnnounceHandler) ReceivedAnnounce(destHash []byte, announcedIdentity interface{}, appData []byte, hops uint8) error {
	debug.Log(debug.DebugInfo, "Received an announce", "hash", fmt.Sprintf("%x", destHash))

	if len(appData) > 0 {
		debug.Log(debug.DebugInfo, "The announce contained app data", "data", string(appData))
	}

	return nil
}

// ReceivePathResponses indicates whether this handler wants to receive path responses
func (h *ExampleAnnounceHandler) ReceivePathResponses() bool {
	return true
}

// ========== Helper Functions ==========

func loadConfig(configpath string) *common.ReticulumConfig {
	cfg := common.DefaultConfig()

	// Add default interface if none configured
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

	return cfg
}

func initializeInterfaces(cfg *common.ReticulumConfig, t *transport.Transport) error {
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

	return nil
}
