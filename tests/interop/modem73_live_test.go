// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live Modem73 interop against a local Go fake or a real modem73 binary.
// Set RUN_LIVE_INTEROP=1.
// Binary audio loopback (no radio): MODEM73_BIN plus PulseAudio/PipeWire pactl.

//go:build !js

package interop

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
)

func TestLiveInteropModem73FakeKISS(t *testing.T) {
	liveOrSkip(t)

	kissLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer kissLn.Close()
	ctrlLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlLn.Close()

	var (
		got []byte
		mu  sync.Mutex
	)
	go serveFakeModem73KISS(kissLn, &got, &mu)
	go serveFakeModem73Control(ctrlLn)

	kissPort := kissLn.Addr().(*net.TCPAddr).Port
	ctrlPort := ctrlLn.Addr().(*net.TCPAddr).Port

	m, err := interfaces.NewModem73Interface("live-m73", true, interfaces.Modem73Options{
		TargetHost:  "127.0.0.1",
		TargetPort:  kissPort,
		ControlHost: "127.0.0.1",
		ControlPort: ctrlPort,
		ShortFrames: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Detach()

	waitIfaceOnline(t, m, 3*time.Second)
	payload := []byte("live-modem73")
	deadline := time.Now().Add(3 * time.Second)
	var sendErr error
	for time.Now().Before(deadline) {
		sendErr = m.ProcessOutgoing(payload)
		if sendErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if sendErr != nil {
		t.Fatal(sendErr)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := bytes.Equal(got, payload)
		mu.Unlock()
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("got=%q", got)
}

func TestLiveInteropModem73BinaryIfPresent(t *testing.T) {
	liveOrSkip(t)
	if _, err := exec.LookPath("modem73"); err != nil {
		t.Skip("modem73 not on PATH")
	}
	host := envOr("MODEM73_CONTROL_HOST", "127.0.0.1")
	port := envOrInt("MODEM73_CONTROL_PORT", 8073)
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, itoa(port)), 2*time.Second)
	if err != nil {
		t.Skipf("modem73 control not reachable: %v", err)
	}
	_ = conn.Close()

	dataHost := envOr("MODEM73_TARGET_HOST", "127.0.0.1")
	dataPort := envOrInt("MODEM73_TARGET_PORT", 8001)
	m, err := interfaces.NewModem73Interface("live-m73-bin", true, interfaces.Modem73Options{
		TargetHost:  dataHost,
		TargetPort:  dataPort,
		ControlHost: host,
		ControlPort: port,
		ShortFrames: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Detach()
	waitIfaceOnline(t, m, 5*time.Second)
}

// TestLiveInteropModem73BinaryAudioLoop starts two headless modem73 processes
// cross-wired through Pulse null sinks and exchanges one KISS payload through
// Modem73Interface without RF hardware.
func TestLiveInteropModem73BinaryAudioLoop(t *testing.T) {
	liveOrSkip(t)

	bin := modem73BinaryPath()
	if bin == "" {
		t.Skip("set MODEM73_BIN or put modem73 on PATH")
	}
	if _, err := exec.LookPath("pactl"); err != nil {
		t.Skip("pactl required for null-sink audio loopback")
	}
	if err := ensureModem73NullSinks(); err != nil {
		t.Fatalf("null sinks: %v", err)
	}
	devs, err := parseModem73AudioDevices(bin)
	if err != nil {
		t.Fatal(err)
	}

	cfgDir := t.TempDir()
	ports, err := freeTCPPorts(4)
	if err != nil {
		t.Fatal(err)
	}
	kissA, ctrlA, kissB, ctrlB := ports[0], ports[1], ports[2], ports[3]

	cmdA := exec.Command(bin,
		"--headless", "--ptt", "none", "--no-csma", "--no-frag", "--short",
		"-p", itoa(kissA), "--control-port", itoa(ctrlA),
		"--output-device", devs.outA, "--input-device", devs.inB,
		"-c", "GOTESTA", "--config", filepath.Join(cfgDir, "a.conf"),
	)
	cmdB := exec.Command(bin,
		"--headless", "--ptt", "none", "--no-csma", "--no-frag", "--short",
		"-p", itoa(kissB), "--control-port", itoa(ctrlB),
		"--output-device", devs.outB, "--input-device", devs.inA,
		"-c", "GOTESTB", "--config", filepath.Join(cfgDir, "b.conf"),
	)
	cmdA.Stdout = &bytes.Buffer{}
	cmdA.Stderr = cmdA.Stdout
	cmdB.Stdout = &bytes.Buffer{}
	cmdB.Stderr = cmdB.Stdout
	if err := cmdA.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmdA.Process.Kill(); _ = cmdA.Wait() }()
	if err := cmdB.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmdB.Process.Kill(); _ = cmdB.Wait() }()

	waitTCP(t, "127.0.0.1", kissA, 8*time.Second)
	waitTCP(t, "127.0.0.1", ctrlA, 8*time.Second)
	waitTCP(t, "127.0.0.1", kissB, 8*time.Second)
	waitTCP(t, "127.0.0.1", ctrlB, 8*time.Second)

	tx, err := interfaces.NewModem73Interface("live-m73-tx", true, interfaces.Modem73Options{
		TargetHost:  "127.0.0.1",
		TargetPort:  kissA,
		ControlHost: "127.0.0.1",
		ControlPort: ctrlA,
		ShortFrames: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	rx, err := interfaces.NewModem73Interface("live-m73-rx", true, interfaces.Modem73Options{
		TargetHost:  "127.0.0.1",
		TargetPort:  kissB,
		ControlHost: "127.0.0.1",
		ControlPort: ctrlB,
		ShortFrames: "off",
	})
	if err != nil {
		t.Fatal(err)
	}

	var (
		got []byte
		mu  sync.Mutex
	)
	rx.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		mu.Lock()
		got = append([]byte(nil), data...)
		mu.Unlock()
	})

	if err := tx.Start(); err != nil {
		t.Fatal(err)
	}
	defer tx.Detach()
	if err := rx.Start(); err != nil {
		t.Fatal(err)
	}
	defer rx.Detach()

	waitIfaceOnline(t, tx, 8*time.Second)
	waitIfaceOnline(t, rx, 8*time.Second)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if tx.GetMTU() > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if tx.GetMTU() <= 0 {
		t.Fatalf("TX MTU not synced from modem73 control (logs A=%s B=%s)",
			cmdA.Stdout.(*bytes.Buffer).String(), cmdB.Stdout.(*bytes.Buffer).String())
	}

	payload := []byte("go-modem73-audio-e2e")
	sendDeadline := time.Now().Add(5 * time.Second)
	var sendErr error
	for time.Now().Before(sendDeadline) {
		sendErr = tx.ProcessOutgoing(payload)
		if sendErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if sendErr != nil {
		t.Fatalf("send: %v", sendErr)
	}

	rxDeadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(rxDeadline) {
		mu.Lock()
		ok := bytes.Equal(got, payload)
		mu.Unlock()
		if ok {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("payload not received got=%q logs A=%s B=%s",
		got, cmdA.Stdout.(*bytes.Buffer).String(), cmdB.Stdout.(*bytes.Buffer).String())
}

type modem73AudioDevs struct {
	outA, outB, inA, inB string
}

func modem73BinaryPath() string {
	if p := os.Getenv("MODEM73_BIN"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath("modem73"); err == nil {
		return p
	}
	return ""
}

func ensureModem73NullSinks() error {
	out, err := exec.Command("pactl", "list", "short", "sinks").CombinedOutput()
	if err != nil {
		return fmt.Errorf("pactl list sinks: %w (%s)", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "\tm73_a\t") {
		if out, err := exec.Command("pactl", "load-module", "module-null-sink",
			"sink_name=m73_a", "sink_properties=device.description=m73_a",
			"rate=48000", "channels=1").CombinedOutput(); err != nil {
			return fmt.Errorf("create m73_a: %w (%s)", err, out)
		}
	}
	if !strings.Contains(text, "\tm73_b\t") {
		if out, err := exec.Command("pactl", "load-module", "module-null-sink",
			"sink_name=m73_b", "sink_properties=device.description=m73_b",
			"rate=48000", "channels=1").CombinedOutput(); err != nil {
			return fmt.Errorf("create m73_b: %w (%s)", err, out)
		}
	}
	time.Sleep(300 * time.Millisecond)
	return nil
}

func parseModem73AudioDevices(bin string) (modem73AudioDevs, error) {
	out, err := exec.Command(bin, "--list-audio").CombinedOutput()
	if err != nil {
		return modem73AudioDevs{}, fmt.Errorf("list-audio: %w (%s)", err, out)
	}
	section := ""
	inDevs := map[string]string{}
	outDevs := map[string]string{}
	re := regexp.MustCompile(`^\s*(\d+)\s+-\s+(.+)$`)
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Input") {
			section = "in"
			continue
		}
		if strings.HasPrefix(line, "Output") {
			section = "out"
			continue
		}
		m := re.FindStringSubmatch(line)
		if m == nil || section == "" {
			continue
		}
		name := strings.TrimSpace(m[2])
		if section == "in" {
			inDevs[name] = m[1]
		} else {
			outDevs[name] = m[1]
		}
	}
	devs := modem73AudioDevs{
		outA: outDevs["m73_a"],
		outB: outDevs["m73_b"],
		inA:  inDevs["Monitor of m73_a"],
		inB:  inDevs["Monitor of m73_b"],
	}
	if devs.outA == "" || devs.outB == "" || devs.inA == "" || devs.inB == "" {
		return modem73AudioDevs{}, fmt.Errorf("missing m73 null-sink devices in list-audio:\n%s", out)
	}
	return devs, nil
}

func freeTCPPorts(n int) ([]int, error) {
	ports := make([]int, 0, n)
	listeners := make([]net.Listener, 0, n)
	defer func() {
		for _, ln := range listeners {
			_ = ln.Close()
		}
	}()
	for i := 0; i < n; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, ln)
		ports = append(ports, ln.Addr().(*net.TCPAddr).Port)
	}
	for _, ln := range listeners {
		_ = ln.Close()
	}
	listeners = nil
	return ports, nil
}

func waitTCP(t *testing.T, host string, port int, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var last error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		last = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("waitTCP %s: %v", addr, last)
}

func serveFakeModem73KISS(ln net.Listener, got *[]byte, mu *sync.Mutex) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(conn net.Conn) {
			defer conn.Close()
			var (
				inFrame bool
				escape  bool
				haveCmd bool
				cmd     byte
				data    []byte
			)
			buf := make([]byte, 4096)
			for {
				n, err := conn.Read(buf)
				for _, b := range buf[:n] {
					if inFrame && b == 0xC0 && haveCmd && cmd == 0x00 {
						inFrame = false
						mu.Lock()
						*got = append([]byte(nil), data...)
						mu.Unlock()
						data = data[:0]
						escape = false
						haveCmd = false
						continue
					}
					if b == 0xC0 {
						inFrame = true
						haveCmd = false
						data = data[:0]
						escape = false
						continue
					}
					if !inFrame {
						continue
					}
					if !haveCmd {
						cmd = b & 0x0F
						haveCmd = true
						continue
					}
					if cmd != 0x00 {
						continue
					}
					if b == 0xDB {
						escape = true
						continue
					}
					if escape {
						switch b {
						case 0xDC:
							b = 0xC0
						case 0xDD:
							b = 0xDB
						}
						escape = false
					}
					data = append(data, b)
				}
				if err != nil {
					return
				}
			}
		}(c)
	}
}

func serveFakeModem73Control(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(conn net.Conn) {
			defer conn.Close()
			for {
				msg, err := interfaces.Modem73ReadControl(conn)
				if err != nil {
					return
				}
				if msg["cmd"] == "get_config" {
					_ = interfaces.Modem73WriteControl(conn, map[string]any{
						"ok":           true,
						"payload_size": float64(600),
						"modem_type":   float64(0),
						"modulation":   "QPSK",
						"code_rate":    "1/2",
						"frame_size":   float64(1),
					})
					continue
				}
				if msg["cmd"] == "tx" {
					if s, ok := msg["data"].(string); ok {
						_, _ = base64.StdEncoding.DecodeString(s)
					}
				}
			}
		}(c)
	}
}

func waitIfaceOnline(t *testing.T, iface common.NetworkInterface, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if iface.IsOnline() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("interface not online")
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envOrInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 && v != "0" {
		return def
	}
	return n
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
