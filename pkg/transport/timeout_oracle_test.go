// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/packet"
)

func almostEqual(t *testing.T, name string, got, want, eps float64) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Fatalf("%s = %v want %v", name, got, want)
	}
}

func TestOracleTimeoutConstantsMatchPythonRNS(t *testing.T) {
	if PathRequestTimeout != 15 {
		t.Fatalf("PATH_REQUEST_TIMEOUT=%d want 15", PathRequestTimeout)
	}
	if PathRequestGrace != 400*time.Millisecond {
		t.Fatalf("PATH_REQUEST_GRACE=%s want 400ms", PathRequestGrace)
	}
	if PathRequestMI != 20*time.Second {
		t.Fatalf("PATH_REQUEST_MI=%s want 20s", PathRequestMI)
	}
	if PathfinderM != 128 {
		t.Fatalf("PATHFINDER_M=%d want 128", PathfinderM)
	}
	if EstablishmentTimeoutPerHop != 6 {
		t.Fatalf("DEFAULT_PER_HOP_TIMEOUT=%d want 6", EstablishmentTimeoutPerHop)
	}
	if common.BitrateMinimum != 5 {
		t.Fatalf("MINIMUM_BITRATE=%d want 5", common.BitrateMinimum)
	}
	if packet.MTU != 500 {
		t.Fatalf("MTU=%d want 500", packet.MTU)
	}
	if PathExchangeBytes != 240 {
		t.Fatalf("PathExchangeBytes=%d want 240", PathExchangeBytes)
	}
	if PathWindowMarginSec != 10 {
		t.Fatalf("PathWindowMarginSec=%d want 10", PathWindowMarginSec)
	}
}

func TestOracleAdaptiveWindowsMatchForumFormulas(t *testing.T) {
	type row struct {
		bitrate    int64
		firstHop   float64
		pathWindow float64
		discovery  float64
		extraProof float64
	}
	rows := []row{
		{5, 806, 806, 1600.4, 800},
		{8, 506, 506, 1000.4, 500},
		{50, 86, 86.8, 160.4, 80},
		{125, 38, 40.72, 64.4, 32},
		{250, 22, 25.36, 32.4, 16},
		{550, 13.272727272727273, 16.98181818181818, 15, 7.2727272727272725},
		{1200, 9.333333333333334, 15, 15, 3.3333333333333335},
		{1_000_000, 6.004, 15, 15, 0.004},
	}
	for _, tc := range rows {
		t.Run(strconv.FormatInt(tc.bitrate, 10), func(t *testing.T) {
			br := float64(tc.bitrate)
			if minBr := float64(common.BitrateMinimum); br < minBr {
				br = minBr
			}
			first := float64(packet.MTU)*8/br + float64(EstablishmentTimeoutPerHop)
			almostEqual(t, "first_hop", first, tc.firstHop, 1e-9)
			gotWin := PathResponseWindowFrom(first, tc.bitrate)
			almostEqual(t, "path_window", gotWin.Seconds(), tc.pathWindow, 1e-9)
			gotDisc := max(mediumRoundTripTimeout(tc.bitrate), time.Duration(PathRequestTimeout)*time.Second)
			almostEqual(t, "discovery", gotDisc.Seconds(), tc.discovery, 1e-9)
			iface := &bitrateIface{}
			iface.BaseInterface = interfaces.NewBaseInterface("radio", common.IFTypeUDP, true)
			iface.Online = true
			iface.bitrate = int(tc.bitrate)
			gotExtra := ExtraLinkProofTimeout(iface)
			almostEqual(t, "extra_proof", gotExtra.Seconds(), tc.extraProof, 1e-9)
		})
	}
}

func TestOracleDiscoveryTimeoutFloorAbove550bps(t *testing.T) {
	got := mediumRoundTripTimeout(550)
	if got >= 15*time.Second {
		t.Fatalf("550 bit/s two-way MTU airtime %s should sit under the 15s floor", got)
	}
	tr := NewTransport(common.DefaultConfig())
	fast := &bitrateIface{}
	fast.BaseInterface = interfaces.NewBaseInterface("radio", common.IFTypeUDP, true)
	fast.Online = true
	fast.bitrate = 550
	if err := tr.RegisterInterface("radio", fast); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got := tr.DiscoveryTimeout(nil); got != 15*time.Second {
		t.Fatalf("550 bit/s discovery = %s want 15s floor", got)
	}
}

func TestOracleBrokenBitrateClampsToFiveNotFifty(t *testing.T) {
	got := PathResponseWindowFrom(float64(EstablishmentTimeoutPerHop), 3)
	want := PathResponseWindowFrom(float64(EstablishmentTimeoutPerHop), 5)
	if got != want {
		t.Fatalf("3 bit/s clamp = %s want 5 bit/s window %s", got, want)
	}
	floor50 := PathResponseWindowFrom(float64(EstablishmentTimeoutPerHop), 50)
	if got <= floor50 {
		t.Fatalf("5 bit/s clamp %s should exceed 50 bit/s %s", got, floor50)
	}
	iface := &bitrateIface{}
	iface.BaseInterface = interfaces.NewBaseInterface("broken", common.IFTypeUDP, true)
	iface.Online = true
	iface.bitrate = 3
	gotExtra := ExtraLinkProofTimeout(iface)
	wantExtra := ExtraLinkProofTimeout(&bitrateIface{BaseInterface: interfaces.NewBaseInterface("min", common.IFTypeUDP, true), bitrate: 5})
	if gotExtra != wantExtra {
		t.Fatalf("extra proof at 3 bit/s = %s want 5 bit/s %s", gotExtra, wantExtra)
	}
}

func TestOracleFirstHopTimeoutMatchesPythonAirtime(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	bi := &bitrateIface{}
	bi.BaseInterface = interfaces.NewBaseInterface("hf", common.IFTypeUDP, true)
	bi.Online = true
	bi.bitrate = 125
	if err := tr.RegisterInterface("hf", bi); err != nil {
		t.Fatal(err)
	}
	dest := make([]byte, 16)
	dest[0] = 0x7a
	tr.UpdatePath(dest, nil, "hf", 1)
	got := tr.FirstHopTimeout(dest)
	want := float64(packet.MTU)*8/125 + 6
	almostEqual(t, "first_hop_timeout", got, want, 1e-12)
}

func TestPythonRNSTimeoutConstants(t *testing.T) {
	exe := os.Getenv("PYTHON_INTEROP")
	if exe == "" {
		exe = "python3"
	}
	script := `
import sys
from RNS.Transport import Transport
from RNS.Reticulum import Reticulum
from RNS.Link import Link
from RNS.Packet import Packet
print(Transport.PATH_REQUEST_TIMEOUT)
print(Transport.PATH_REQUEST_GRACE)
print(Transport.PATH_REQUEST_MI)
print(Transport.PATHFINDER_M)
print(Reticulum.MINIMUM_BITRATE)
print(Reticulum.DEFAULT_PER_HOP_TIMEOUT)
print(Reticulum.MTU)
print(Link.ESTABLISHMENT_TIMEOUT_PER_HOP)
print(Packet.ENCRYPTED_MDU)
print(Packet.PLAIN_MDU)
print(Link.MDU)
`
	cmd := exec.Command(exe, "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if os.Getenv("RUN_PY_INTEROP") != "" {
			t.Fatalf("python RNS constants required: %v\n%s", err, out)
		}
		t.Skip("python RNS not available")
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 11 {
		t.Fatalf("python lines=%d output=%q", len(lines), out)
	}
	want := []string{"15", "0.4", "20", "128", "5", "6", "500", "6", "383", "464", "431"}
	for i, w := range want {
		if lines[i] != w {
			t.Fatalf("python const[%d]=%q want %q", i, lines[i], w)
		}
	}
}
