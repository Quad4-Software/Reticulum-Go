// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

import (
	"fmt"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
)

func NodeAddUDPInterface(nodeHandle uint64, name, localAddr, peerAddr string) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if name == "" || localAddr == "" || peerAddr == "" {
		return setLastError(errInvalidArg)
	}
	iface, err := interfaces.NewUDPInterface(name, localAddr, peerAddr, true)
	if err != nil {
		return setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	var ni interfaces.Interface = iface
	ni.SetPacketCallback(func(d []byte, ifc common.NetworkInterface) {
		rec.transport.HandlePacket(d, ifc)
	})
	if err := ni.Start(); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	if net, ok := ni.(common.NetworkInterface); ok {
		if err := rec.transport.RegisterInterface(name, net); err != nil {
			_ = ni.Stop()
			return setLastError(fmt.Errorf("%w: %v", errInternal, err))
		}
	}
	rec.ifaces = append(rec.ifaces, ni)
	return OK
}
