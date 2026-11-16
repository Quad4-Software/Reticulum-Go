// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

import (
	"fmt"
	"net"
	"strconv"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/backbone"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
)

// NodeInterfaceCount reports how many interfaces are currently attached.
func NodeInterfaceCount(nodeHandle uint64) (int, int) {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return 0, setLastError(err)
	}
	return len(rec.ifaces), OK
}

// NodeInterfaceNames lists the names of attached interfaces.
func NodeInterfaceNames(nodeHandle uint64) ([]string, int) {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	names := make([]string, 0, len(rec.ifaces))
	for _, ni := range rec.ifaces {
		names = append(names, ni.GetName())
	}
	return names, OK
}

// NodeAddTCPClientInterface attaches an outbound TCP interface to
// host:port, for reaching TCP-facing nodes and backbones.
func NodeAddTCPClientInterface(nodeHandle uint64, name, target string) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	host, port, err := hostPort(target)
	if err != nil {
		return setLastError(err)
	}
	if name == "" {
		return setLastError(errInvalidArg)
	}
	iface, err := interfaces.NewTCPClientInterface(name, host, port, false, false, true)
	if err != nil {
		return setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	if err := attachInterface(rec, name, iface); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	return OK
}

// NodeAddTCPServerInterface attaches an inbound TCP listener on host:port.
func NodeAddTCPServerInterface(nodeHandle uint64, name, listen string) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	host, port, err := hostPort(listen)
	if err != nil {
		return setLastError(err)
	}
	if name == "" {
		return setLastError(errInvalidArg)
	}
	iface, err := interfaces.NewTCPServerInterface(name, host, port, false, false, false)
	if err != nil {
		return setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	if err := attachInterface(rec, name, iface); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	return OK
}

// NodeAddAutoInterface attaches an AutoInterface (UDP multicast discovery)
// with the standard RNS defaults.
func NodeAddAutoInterface(nodeHandle uint64, name string) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	if name == "" {
		return setLastError(errInvalidArg)
	}
	ic := &common.InterfaceConfig{
		Name:           name,
		Type:           "AutoInterface",
		Enabled:        true,
		DiscoveryPort:  29716,
		DataPort:       42671,
		DiscoveryScope: "link",
		GroupID:        "reticulum",
	}
	iface, err := interfaces.NewAutoInterface(name, ic)
	if err != nil {
		return setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	if err := attachInterface(rec, name, iface); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	return OK
}

// NodeAddBackboneClientInterface attaches a Backbone client dialing
// host:port.
func NodeAddBackboneClientInterface(nodeHandle uint64, name, target string) int {
	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		return setLastError(err)
	}
	host, port, err := hostPort(target)
	if err != nil {
		return setLastError(err)
	}
	if name == "" {
		return setLastError(err)
	}
	ic := &common.InterfaceConfig{
		Name:       name,
		Type:       "BackboneClientInterface",
		Enabled:    true,
		TargetHost: host,
		TargetPort: port,
	}
	iface, err := interfaces.NewBackboneClientInterface(name, ic, backbone.Get())
	if err != nil {
		return setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	if err := attachInterface(rec, name, iface); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	return OK
}

func hostPort(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, errInvalidArg
	}
	if host == "" {
		host = "0.0.0.0"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return "", 0, errInvalidArg
	}
	return host, port, nil
}
