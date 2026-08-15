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

// Announce registers a filtered announce handler and publishes
// two destinations with random app_data on each enter keypress.

const APP_NAME = "example_utilities"

var (
	configPath = flag.String("config", "", "path to alternative Reticulum config directory")
	listenPort = flag.Int("listen-port", 4242, "UDP interface listen port")
	targetPort = flag.Int("target-port", 4242, "UDP interface target port (for sending to peers)")
)

// Sample app_data payloads attached to announces.
var colors = []string{"Crimson", "Teal", "Amber", "Indigo", "Olive", "Slate", "Coral"}
var metals = []string{"Iron", "Copper", "Tin", "Zinc", "Nickel", "Cobalt", "Titanium"}

func main() {
	flag.Parse()
	debug.Init()

	if err := programSetup(*configPath); err != nil {
		debug.Log(debug.DebugCritical, "Failed to setup program", "error", err)
		os.Exit(1)
	}
}

func programSetup(configpath string) error {
	cfg := loadConfig(configpath)
	t := transport.NewTransport(cfg)

	id, err := identity.NewIdentity()
	if err != nil {
		return err
	}

	// Two destinations under announcesample with different aspects.
	destination1, err := destination.New(
		id,
		destination.In,
		destination.Single,
		APP_NAME,
		t,
		"announcesample",
		"colors",
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
		"metals",
	)
	if err != nil {
		return err
	}

	destination1.SetProofStrategy(destination.ProveAll)
	destination2.SetProofStrategy(destination.ProveAll)

	// Only color-aspect announces are delivered to this handler.
	announceHandler := NewExampleAnnounceHandler(
		[]string{"example_utilities.announcesample.colors"},
	)

	if err := t.Start(); err != nil {
		return err
	}

	t.RegisterAnnounceHandler(announceHandler)

	if err := initializeInterfaces(cfg, t); err != nil {
		return err
	}

	announceLoop(destination1, destination2)

	return nil
}

func announceLoop(destination1, destination2 *destination.Destination) {
	debug.Log(debug.DebugInfo, "Announce example ready. Press enter to announce, Ctrl-C to quit")

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
			color := colors[rand.Intn(len(colors))] // #nosec G404 -- demo payload only

			destination1.SetDefaultAppData([]byte(color))
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

			metal := metals[rand.Intn(len(metals))] // #nosec G404 -- demo payload only

			destination2.SetDefaultAppData([]byte(metal))
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

// ExampleAnnounceHandler filters announces by aspect name.
type ExampleAnnounceHandler struct {
	aspectFilter []string
}

// NewExampleAnnounceHandler builds a handler with an optional aspect filter.
func NewExampleAnnounceHandler(aspectFilter []string) *ExampleAnnounceHandler {
	return &ExampleAnnounceHandler{
		aspectFilter: aspectFilter,
	}
}

// AspectFilter returns the aspect filter for this handler.
func (h *ExampleAnnounceHandler) AspectFilter() []string {
	return h.aspectFilter
}

// ReceivedAnnounce runs for matching announces.
// Aspect filters are exact names, not wildcards.
func (h *ExampleAnnounceHandler) ReceivedAnnounce(destHash []byte, announcedIdentity interface{}, appData []byte, hops uint8) error {
	debug.Log(debug.DebugInfo, "Received an announce", "hash", fmt.Sprintf("%x", destHash))

	if len(appData) > 0 {
		debug.Log(debug.DebugInfo, "Announce included app data", "data", string(appData))
	}

	return nil
}

// ReceivePathResponses reports whether path responses should be delivered.
func (h *ExampleAnnounceHandler) ReceivePathResponses() bool {
	return true
}

// ========== Helper Functions ==========

func loadConfig(configpath string) *common.ReticulumConfig {
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
