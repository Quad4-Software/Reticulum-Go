// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/transport"
)

type reachMockIface struct {
	common.BaseInterface
}

func (m *reachMockIface) Send([]byte, string) error { return nil }
func (m *reachMockIface) GetConn() net.Conn         { return nil }
func (m *reachMockIface) SendPathRequest([]byte) error {
	return nil
}
func (m *reachMockIface) SendLinkPacket([]byte, []byte, time.Time) error {
	return nil
}

func TestDiagnoseReachabilityNilTransport(t *testing.T) {
	dest := bytes.Repeat([]byte{0xab}, 16)
	report := DiagnoseReachability(nil, dest)
	if report.Stage != ReachStageNoInterfaces {
		t.Fatalf("stage=%s want %s", report.Stage, ReachStageNoInterfaces)
	}
}

func TestDiagnoseReachabilityNoInterfaces(t *testing.T) {
	tr := transport.NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	dest := bytes.Repeat([]byte{0xab}, 16)
	report := DiagnoseReachability(tr, dest)
	if report.Stage != ReachStageNoInterfaces {
		t.Fatalf("stage=%s want %s", report.Stage, ReachStageNoInterfaces)
	}
	if report.OnlineIfaces != 0 {
		t.Fatalf("online=%d want 0", report.OnlineIfaces)
	}
	if len(report.Hints) == 0 {
		t.Fatal("expected hints")
	}
}

func TestDiagnoseReachabilityNoPath(t *testing.T) {
	tr := transport.NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	mock := &reachMockIface{}
	mock.Name = "udp0"
	mock.Enabled = true
	mock.Online = true
	if err := tr.RegisterInterface(mock.Name, mock); err != nil {
		t.Fatalf("register: %v", err)
	}
	dest := bytes.Repeat([]byte{0xcd}, 16)
	report := DiagnoseReachability(tr, dest)
	if report.Stage != ReachStageNoPath {
		t.Fatalf("stage=%s want %s", report.Stage, ReachStageNoPath)
	}
	if report.OnlineIfaces != 1 {
		t.Fatalf("online=%d want 1", report.OnlineIfaces)
	}
	var buf bytes.Buffer
	if err := WriteReachReportHuman(&buf, report); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, ReachStageNoPath) {
		t.Fatalf("unexpected report output: %s", out)
	}
}

func TestDiagnoseReachabilityReachable(t *testing.T) {
	tr := transport.NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	mock := &reachMockIface{}
	mock.Name = "udp0"
	mock.Enabled = true
	mock.Online = true
	if err := tr.RegisterInterface(mock.Name, mock); err != nil {
		t.Fatalf("register: %v", err)
	}
	dest := bytes.Repeat([]byte{0x11}, 16)
	via := bytes.Repeat([]byte{0x22}, 16)
	tr.UpdatePath(dest, via, mock.Name, 2)
	report := DiagnoseReachability(tr, dest)
	if report.Stage != ReachStageReachable {
		t.Fatalf("stage=%s want %s", report.Stage, ReachStageReachable)
	}
	if report.Hops != 2 {
		t.Fatalf("hops=%d want 2", report.Hops)
	}
}
