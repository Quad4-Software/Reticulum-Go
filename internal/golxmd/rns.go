// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/backbone"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/rnsnode"
)

type TransportConfig struct {
	RNSConfigDir string
	UDPListen    string
	UDPForward   string
}

func startTransport(cfg TransportConfig, log *slog.Logger) (*transport.Transport, []interfaces.Interface, error) {
	debug.Init()
	rc := common.DefaultConfig()
	rc.EnableTransport = true
	rc.ShareInstance = false
	rnsDir := ResolveRNSConfigDir(cfg.RNSConfigDir)
	if rnsDir != "" {
		rc.ConfigPath = filepath.Join(rnsDir, "config")
		created, err := rnsnode.EnsureDefaultConfig(rnsDir)
		if err != nil {
			return nil, nil, fmt.Errorf("rns config: %w", err)
		}
		if created {
			log.Info("created default reticulum-go config", "path", rc.ConfigPath)
		}
	}
	tr := transport.NewTransport(rc)
	if err := tr.Start(); err != nil {
		return nil, nil, err
	}

	var started []interfaces.Interface
	attach := func(name string, iface interfaces.Interface) error {
		iface.SetPacketCallback(func(d []byte, ni common.NetworkInterface) { tr.HandlePacket(d, ni) })
		if err := iface.Start(); err != nil {
			return err
		}
		if ni, ok := iface.(common.NetworkInterface); ok {
			if err := tr.RegisterInterface(name, ni); err != nil {
				_ = iface.Stop()
				return err
			}
		}
		started = append(started, iface)
		return nil
	}

	if cfg.UDPListen != "" {
		listen := udpAddr(cfg.UDPListen)
		forward := udpAddr(cfg.UDPForward)
		if forward == "" {
			return nil, started, fmt.Errorf("udp-forward required with udp-listen")
		}
		udp, err := interfaces.NewUDPInterface("UDPInterface", listen, forward, true)
		if err != nil {
			return nil, started, fmt.Errorf("udp interface: %w", err)
		}
		if err := attach("UDPInterface", udp); err != nil {
			return nil, started, fmt.Errorf("start udp interface: %w", err)
		}
		log.Info("udp interface", "listen", listen, "forward", forward)
		return tr, started, nil
	}

	useUnix := common.SharedInstanceUsesUnix(rc.SharedInstanceType)
	lc, err := interfaces.NewLocalClientInterface(common.DefaultSharedInstancePort, "default", useUnix, backbone.Get())
	if err == nil {
		if aerr := attach(lc.GetName(), lc); aerr == nil {
			log.Info("attached to reticulum shared instance")
			return tr, started, nil
		}
		log.Info("shared instance not available")
	}

	if n := attachFromRNSConfig(rnsDir, tr, attach, log); n > 0 {
		log.Info("started interfaces from rns config", "count", n, "dir", rnsDir)
		return tr, started, nil
	}

	log.Info("no rns config interfaces available, using AutoInterface")

	auto, err := interfaces.NewAutoInterface("AutoInterface", &common.InterfaceConfig{
		Type:    "AutoInterface",
		Enabled: true,
		Name:    "AutoInterface",
	})
	if err != nil {
		return nil, started, fmt.Errorf("auto interface: %w", err)
	}
	if err := attach("AutoInterface", auto); err != nil {
		return nil, started, fmt.Errorf("start auto interface: %w", err)
	}
	return tr, started, nil
}

func attachFromRNSConfig(rnsDir string, tr *transport.Transport, attach func(string, interfaces.Interface) error, log *slog.Logger) int {
	if strings.TrimSpace(rnsDir) == "" {
		return 0
	}
	rcfg, err := rnsnode.LoadOfficialDir(rnsDir)
	if err != nil {
		log.Info("rns config not loaded", "dir", rnsDir, "error", err)
		return 0
	}
	ctx := &interfaces.FromConfigContext{
		I2PStoragePath: filepath.Join(rnsDir, "storage"),
		TransportID:    tr.TransportIdentityHash(),
		BackboneHub:    backbone.Get(),
		RegisterPeer: func(name string, peer common.NetworkInterface) error {
			return tr.RegisterInterface(name, peer)
		},
		ConfigDir: rnsDir,
	}
	started := 0
	for name, ic := range rcfg.Interfaces {
		if ic == nil || !ic.Enabled {
			continue
		}
		iface, err := interfaces.NewFromConfigWithContext(name, ic, ctx)
		if err != nil {
			log.Warn("skip rns interface", "name", name, "error", err)
			continue
		}
		enableDuplex(iface)
		if err := attach(iface.GetName(), iface); err != nil {
			log.Warn("start rns interface failed", "name", name, "error", err)
			continue
		}
		log.Info("rns interface up", "name", iface.GetName(), "type", ic.Type)
		started++
	}
	return started
}

func enableDuplex(iface interfaces.Interface) {
	switch v := iface.(type) {
	case *interfaces.UDPInterface:
		v.In = true
		v.Out = true
	case *interfaces.TCPClientInterface:
		v.In = true
		v.Out = true
	case *interfaces.TCPServerInterface:
		v.In = true
		v.Out = true
	case *interfaces.BackboneInterface:
		v.In = true
		v.Out = true
	case *interfaces.BackboneClientInterface:
		v.In = true
		v.Out = true
	case *interfaces.AutoInterface:
		v.In = true
		v.Out = true
	}
}

func udpAddr(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.Contains(s, ":") {
		return s
	}
	return "127.0.0.1:" + s
}
