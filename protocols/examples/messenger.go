//go:build ignore
// +build ignore

package main

import (
	"encoding/hex"
	"log"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/mf"
)

const (
	maxRetries      = 5
	retryDelay      = 2 * time.Second
	announceTimeout = 10 * time.Second
	exampleAppName  = "mf"
	exampleMessage  = "Hello from reticulum-go-protocols!"
	// Loopback bind for this example. Not a mesh listen address.
	udpListenAddr = "127.0.0.1:4242"
	udpIfaceName  = "UDPInterface"
)

func main() {
	cfg := common.DefaultConfig()
	tr := transport.NewTransport(cfg)

	if err := tr.Start(); err != nil {
		log.Fatalf("Failed to start transport: %v", err)
	}

	if err := setupInterfaces(cfg, tr); err != nil {
		log.Fatalf("Failed to setup interfaces: %v", err)
	}

	id, err := identity.NewIdentity()
	if err != nil {
		log.Fatal(err)
	}

	dest, err := destination.New(id, destination.In, destination.Single, exampleAppName, tr)
	if err != nil {
		log.Fatal(err)
	}

	if err := dest.Announce(false, nil, nil); err != nil {
		log.Printf("Warning: Failed to announce destination: %v", err)
	} else {
		log.Printf("Announced destination: %s", hex.EncodeToString(dest.GetHash()))
	}

	messenger := mf.NewMessenger(tr, dest)

	remoteId, err := identity.NewIdentity()
	if err != nil {
		log.Fatal(err)
	}
	remoteHash := remoteId.Hash()

	identity.Remember(nil, remoteHash, remoteId.GetPublicKey(), nil)

	log.Printf("Attempting to send message to %s", hex.EncodeToString(remoteHash))

	if err := sendMessageWithRetry(messenger, tr, remoteHash, exampleMessage); err != nil {
		log.Fatalf("Failed to send message after retries: %v", err)
	}

	log.Printf("Message sent successfully to %s", hex.EncodeToString(remoteHash))
}

func sendMessageWithRetry(messenger *mf.Messenger, tr *transport.Transport, destHash []byte, text string) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if tr.HasPath(destHash) {
			err := messenger.SendMessage(destHash, text)
			if err == nil {
				return nil
			}
			log.Printf("Attempt %d: Send failed (path exists but send error): %v", attempt, err)
		} else {
			log.Printf("Attempt %d: No path to destination, waiting...", attempt)
			if attempt == 1 {
				time.Sleep(announceTimeout)
			} else {
				time.Sleep(retryDelay)
			}
			continue
		}

		if attempt < maxRetries {
			time.Sleep(retryDelay)
		}
	}

	return messenger.SendMessage(destHash, text)
}

func setupInterfaces(cfg *common.ReticulumConfig, tr *transport.Transport) error {
	if len(cfg.Interfaces) == 0 {
		cfg.Interfaces = make(map[string]*common.InterfaceConfig)
		cfg.Interfaces[udpIfaceName] = &common.InterfaceConfig{
			Type:    "UDPInterface",
			Enabled: true,
			Address: udpListenAddr,
			Name:    udpIfaceName,
		}
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
		default:
			log.Printf("Skipping unknown interface type: %s", ifaceConfig.Type)
			continue
		}

		if err != nil {
			return err
		}

		iface.SetPacketCallback(func(data []byte, ni common.NetworkInterface) {
			tr.HandlePacket(data, ni)
		})

		if err := iface.Start(); err != nil {
			return err
		}

		if netIface, ok := iface.(common.NetworkInterface); ok {
			if err := tr.RegisterInterface(name, netIface); err != nil {
				return err
			}
		}

		log.Printf("Interface %s started", name)
	}

	return nil
}
