// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"quad4/reticulum-go/pkg/interfaces"
)

func checkUDP() Result {
	a, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		a, err = net.ListenPacket("udp", "127.0.0.1:0")
	}
	if err != nil {
		return loopbackResult("network/udp", err)
	}
	defer a.Close()
	b, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		b, err = net.ListenPacket("udp", "127.0.0.1:0")
	}
	if err != nil {
		return loopbackResult("network/udp", err)
	}
	defer b.Close()

	payload := []byte("rns-selfcheck-udp")
	if _, err := a.WriteTo(payload, b.LocalAddr()); err != nil {
		return result("network/udp", SeverityFail, "send: "+err.Error())
	}
	_ = b.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, _, err := b.ReadFrom(buf)
	if err != nil {
		return result("network/udp", SeverityFail, "recv: "+err.Error())
	}
	if string(buf[:n]) != string(payload) {
		return result("network/udp", SeverityFail, "payload mismatch")
	}

	ui, err := interfaces.NewUDPInterface("selfcheck-udp", "127.0.0.1:0", "127.0.0.1:9", true)
	if err != nil {
		return result("network/udp", SeverityFail, err.Error())
	}
	if err := ui.Start(); err != nil {
		return result("network/udp", SeverityFail, "start: "+err.Error())
	}
	defer ui.Stop()
	if !ui.IsOnline() {
		return result("network/udp", SeverityFail, "not online after start")
	}
	return result("network/udp", SeverityPass, "loopback send/recv and interface start")
}

func checkTCP() Result {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return loopbackResult("network/tcp", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	srv, err := interfaces.NewTCPServerInterface("selfcheck-tcp", "127.0.0.1", port, false, false, false)
	if err != nil {
		return result("network/tcp", SeverityFail, err.Error())
	}
	if err := srv.Start(); err != nil {
		return result("network/tcp", SeverityFail, "start: "+err.Error())
	}
	defer srv.Stop()
	return result("network/tcp", SeverityPass, fmt.Sprintf("bound 127.0.0.1:%d", port))
}

func checkLocalInterface() Result {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return loopbackResult("network/local", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	srv, err := interfaces.NewLocalServerInterface(port, "", false, func(*interfaces.LocalClientInterface) {}, nil)
	if err != nil {
		return result("network/local", SeverityFail, err.Error())
	}
	if err := srv.Start(); err != nil {
		return result("network/local", SeverityFail, "start: "+err.Error())
	}
	defer srv.Stop()
	return result("network/local", SeverityPass, fmt.Sprintf("port %d", port))
}

func checkQUIC() Result {
	ln, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		ln, err = net.ListenPacket("udp", "127.0.0.1:0")
	}
	if err != nil {
		return loopbackResult("network/quic", err)
	}
	port := ln.LocalAddr().(*net.UDPAddr).Port
	_ = ln.Close()

	srv, err := interfaces.NewQUICServerInterface("selfcheck-quic", "127.0.0.1", port, interfaces.QUICServerOptions{})
	if err != nil {
		if isUnsupported(err) {
			return result("network/quic", SeveritySkip, err.Error())
		}
		return result("network/quic", SeverityFail, err.Error())
	}
	if err := srv.Start(); err != nil {
		return result("network/quic", SeverityFail, "start: "+err.Error())
	}
	defer srv.Stop()
	return result("network/quic", SeverityPass, fmt.Sprintf("bound 127.0.0.1:%d", port))
}

func checkHTTPS() Result {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return loopbackResult("network/https", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	srv, err := interfaces.NewHTTPSServerInterface("selfcheck-https", "127.0.0.1", port, interfaces.HTTPSServerOptions{})
	if err != nil {
		if isUnsupported(err) {
			return result("network/https", SeveritySkip, err.Error())
		}
		return result("network/https", SeverityFail, err.Error())
	}
	if err := srv.Start(); err != nil {
		return result("network/https", SeverityFail, "start: "+err.Error())
	}
	defer srv.Stop()
	return result("network/https", SeverityPass, fmt.Sprintf("bound 127.0.0.1:%d", port))
}

func checkVSOCK() Result {
	if runtime.GOOS != "linux" {
		return result("network/vsock", SeveritySkip, "linux only")
	}
	if _, err := os.Stat("/dev/vsock"); err != nil {
		return result("network/vsock", SeveritySkip, "no /dev/vsock")
	}
	srv, err := interfaces.NewVSOCKServerInterface("selfcheck-vsock", 0)
	if err != nil {
		if isUnsupported(err) {
			return result("network/vsock", SeveritySkip, err.Error())
		}
		return result("network/vsock", SeverityWarn, err.Error())
	}
	_ = srv
	return result("network/vsock", SeverityPass, "device present")
}

func checkPipe() Result {
	if _, err := exec.LookPath("cat"); err != nil {
		return result("network/pipe", SeveritySkip, "cat not on PATH")
	}
	pi, err := interfaces.NewPipeInterface("selfcheck-pipe", "cat", true, time.Second, false)
	if err != nil {
		return result("network/pipe", SeverityFail, err.Error())
	}
	if err := pi.Start(); err != nil {
		return result("network/pipe", SeverityFail, "start: "+err.Error())
	}
	defer pi.Stop()
	return result("network/pipe", SeverityPass, "cat HDLC pipe")
}

func checkSerial() Result {
	_, err := interfaces.NewSerialInterface("selfcheck-serial", false, interfaces.SerialOptions{
		Device: "/dev/null",
	})
	if err != nil {
		if isUnsupported(err) {
			return result("network/serial", SeveritySkip, err.Error())
		}
		return result("network/serial", SeveritySkip, err.Error())
	}
	return result("network/serial", SeverityPass, "constructor available")
}

func isUnsupported(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "not supported") ||
		strings.Contains(s, "unsupported") ||
		strings.Contains(s, "not available")
}

func loopbackResult(name string, err error) Result {
	if err == nil {
		return result(name, SeverityFail, "unknown loopback error")
	}
	if runtime.GOOS == "haiku" && isUnsupported(err) {
		return result(name, SeveritySkip, err.Error())
	}
	return result(name, SeverityFail, err.Error())
}

func haikuLoopbackOK() bool {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
