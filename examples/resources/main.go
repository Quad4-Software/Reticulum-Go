// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/resource"
	"quad4/reticulum-go/pkg/transport"
)

// Minimal link resource transfer. Server accepts resources on an established
// link. Client sends one payload then exits. For a file browser style demo
// see examples/filetransfer.

const APP_NAME = "example_utilities"

var (
	isServer   = flag.Bool("server", false, "wait for links and accept resources")
	configPath = flag.String("config", "", "path to alternative Reticulum config directory")
	destHash   = flag.String("destination", "", "hexadecimal hash of the server destination")
	payload    = flag.String("payload", "hello from resources example", "bytes to send as a resource")
	listenPort = flag.Int("listen-port", 4242, "UDP interface listen port")
	targetPort = flag.Int("target-port", 4242, "UDP interface target port for client")
)

func main() {
	flag.Parse()
	debug.Init()

	if *isServer {
		if err := server(*configPath); err != nil {
			debug.Log(debug.DebugCritical, "Server error", "error", err)
			os.Exit(1)
		}
		return
	}
	if *destHash == "" {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "\nError: destination hash required for client mode\n")
		os.Exit(1)
	}
	if err := client(*destHash, *configPath, []byte(*payload)); err != nil {
		debug.Log(debug.DebugCritical, "Client error", "error", err)
		os.Exit(1)
	}
}

func server(configpath string) error {
	cfg := loadConfig(configpath)
	t := transport.NewTransport(cfg)

	id, err := identity.NewIdentity()
	if err != nil {
		return err
	}
	dest, err := destination.New(id, destination.In, destination.Single, APP_NAME, t, "resourceexample")
	if err != nil {
		return err
	}
	dest.SetLinkEstablishedCallback(func(l interface{}) {
		lnk, ok := l.(*link.Link)
		if !ok {
			return
		}
		debug.Log(debug.DebugInfo, "Client connected")
		lnk.SetResourceCallback(func(any) bool { return true })
		lnk.SetResourceConcludedCallback(func(got any) {
			switch v := got.(type) {
			case link.IncomingResource:
				debug.Log(debug.DebugInfo, "Resource received", "bytes", len(v.Data), "hash", fmt.Sprintf("%x", v.Hash))
				fmt.Printf("payload: %q\n", string(v.Data))
			case []byte:
				debug.Log(debug.DebugInfo, "Resource received", "bytes", len(v))
				fmt.Printf("payload: %q\n", string(v))
			default:
				debug.Log(debug.DebugInfo, "Resource received", "type", fmt.Sprintf("%T", got))
			}
		})
		lnk.SetLinkClosedCallback(func(*link.Link) {
			debug.Log(debug.DebugInfo, "Client disconnected")
		})
	})

	if err := t.Start(); err != nil {
		return err
	}
	if err := initializeInterfaces(cfg, t); err != nil {
		return err
	}

	debug.Log(debug.DebugInfo, "Resource example server waiting", "hash", fmt.Sprintf("%x", dest.GetHash()))
	debug.Log(debug.DebugInfo, "Hit enter to announce (Ctrl-C to quit)")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	inputChan := make(chan struct{})
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			if _, err := reader.ReadString('\n'); err != nil {
				return
			}
			inputChan <- struct{}{}
		}
	}()

	for {
		select {
		case <-sigChan:
			return nil
		case <-inputChan:
			if err := dest.Announce(false, nil, nil); err != nil {
				debug.Log(debug.DebugError, "Announce failed", "error", err)
			} else {
				debug.Log(debug.DebugInfo, "Sent announce", "hash", fmt.Sprintf("%x", dest.GetHash()))
			}
		}
	}
}

func client(destinationHexHash, configpath string, data []byte) error {
	destinationHash, err := hex.DecodeString(destinationHexHash)
	if err != nil {
		return fmt.Errorf("invalid destination hash: %w", err)
	}

	cfg := loadConfig(configpath)
	if ifaceCfg, ok := cfg.Interfaces["UDPInterface"]; ok && ifaceCfg.TargetHost == "" {
		ifaceCfg.TargetHost = fmt.Sprintf("127.0.0.1:%d", *targetPort)
	}

	t := transport.NewTransport(cfg)
	if err := t.Start(); err != nil {
		return err
	}
	if err := initializeInterfaces(cfg, t); err != nil {
		return err
	}

	if !t.HasPath(destinationHash) {
		debug.Log(debug.DebugInfo, "Requesting path to server...")
		if err := t.RequestPath(destinationHash, "", nil, false); err != nil {
			return err
		}
		deadline := time.After(30 * time.Second)
		for !t.HasPath(destinationHash) {
			select {
			case <-deadline:
				return fmt.Errorf("timeout waiting for path")
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	serverIdentity, err := identity.Recall(destinationHash)
	if err != nil {
		return err
	}
	serverDest, err := destination.New(serverIdentity, destination.Out, destination.Single, APP_NAME, t, "resourceexample")
	if err != nil {
		return err
	}

	established := make(chan *link.Link, 1)
	l := link.NewLink(serverDest, t, nil, func(lnk *link.Link) {
		established <- lnk
	}, func(*link.Link) {
		debug.Log(debug.DebugInfo, "Link closed")
	})
	if err := l.Establish(); err != nil {
		return err
	}
	l.Start()

	var active *link.Link
	select {
	case active = <-established:
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for link")
	}

	res, err := resource.New(data, false)
	if err != nil {
		return err
	}
	debug.Log(debug.DebugInfo, "Sending resource", "bytes", len(data))
	if err := active.SendResource(res); err != nil {
		return err
	}
	debug.Log(debug.DebugInfo, "Resource transfer complete")
	active.Teardown()
	return nil
}

func loadConfig(configpath string) *common.ReticulumConfig {
	_ = configpath
	cfg := common.DefaultConfig()
	if len(cfg.Interfaces) == 0 {
		cfg.Interfaces = make(map[string]*common.InterfaceConfig)
		cfg.Interfaces["UDPInterface"] = &common.InterfaceConfig{
			Type:    "UDPInterface",
			Enabled: true,
			Address: fmt.Sprintf("0.0.0.0:%d", *listenPort),
			Name:    "UDPInterface",
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
			iface, err = interfaces.NewUDPInterface(name, ifaceConfig.Address, ifaceConfig.TargetHost, ifaceConfig.Enabled)
		default:
			debug.Log(debug.DebugError, "Unknown interface type", "type", ifaceConfig.Type)
			continue
		}
		if err != nil {
			return err
		}
		if err := iface.Start(); err != nil {
			return err
		}
		t.RegisterInterface(name, iface)
	}
	return nil
}
