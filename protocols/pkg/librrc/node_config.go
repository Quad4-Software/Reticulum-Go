// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

import (
	"log/slog"
	"path/filepath"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/backbone"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
)

// attachInterface wires an interface to the node transport and starts it.
// Mirrors the gorrcd attach path: packet callback, duplex enable, start,
// register.
func attachInterface(rec *nodeRecord, name string, ni interfaces.Interface) error {
	ni.SetPacketCallback(func(d []byte, ifc common.NetworkInterface) {
		rec.transport.HandlePacket(d, ifc)
	})
	enableDuplex(ni)
	if err := ni.Start(); err != nil {
		return err
	}
	if net, ok := ni.(common.NetworkInterface); ok {
		if err := rec.transport.RegisterInterface(name, net); err != nil {
			_ = ni.Stop()
			return err
		}
	}
	rec.ifaces = append(rec.ifaces, ni)
	return nil
}

// attachConfiguredInterfaces instantiates every enabled [interfaces] entry
// from the loaded Reticulum-Go config. Individual failures are logged and
// skipped, matching gorrcd behavior. The node itself stays usable.
func attachConfiguredInterfaces(rec *nodeRecord, configDir string) {
	rcfg := rec.cfg
	if rcfg == nil {
		return
	}
	ctx := &interfaces.FromConfigContext{
		I2PStoragePath: filepath.Join(configDir, "storage"),
		TransportID:    rec.transport.TransportIdentityHash(),
		BackboneHub:    backbone.Get(),
		RegisterPeer: func(name string, peer common.NetworkInterface) error {
			return rec.transport.RegisterInterface(name, peer)
		},
		ConfigDir: configDir,
	}
	for name, ic := range rcfg.Interfaces {
		if ic == nil || !ic.Enabled {
			continue
		}
		iface, err := interfaces.NewFromConfigWithContext(name, ic, ctx)
		if err != nil {
			slog.Warn("librrc: skip rns interface", "name", name, "error", err)
			continue
		}
		if err := attachInterface(rec, iface.GetName(), iface); err != nil {
			slog.Warn("librrc: start rns interface failed", "name", name, "error", err)
			continue
		}
		slog.Info("librrc: rns interface up", "name", iface.GetName(), "type", ic.Type)
	}
}

// enableDuplex marks client/server interfaces as full-duplex, same switch
// gorrcd applies to config-loaded interfaces.
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
