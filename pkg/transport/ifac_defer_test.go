// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/ifac"
)

func TestRegisterInterfaceDefersInboundIFAC(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	iface := newSimIface("defer-check")
	if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatalf("register: %v", err)
	}
	if !iface.DeferInboundIFAC() {
		t.Fatal("expected defer inbound IFAC after transport registration")
	}
}

func TestSimIFACFirstHop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node IFAC simulation in -short mode")
	}
	enableSimFastPath(t)
	id, err := ifac.New(0, "sim-net", "sim-passphrase")
	if err != nil {
		t.Fatalf("ifac.New: %v", err)
	}
	net := buildLine(t, 2)
	defer net.close()
	for _, node := range net.nodes {
		for _, ifc := range node.ifaces {
			ifc.SetIFAC(id)
		}
	}
	src := net.nodes[0]
	timeout := simConvergenceTimeout(1) + 20*time.Second
	deadline := time.Now().Add(timeout)
	var took time.Duration
	var ok int
	for time.Now().Before(deadline) {
		src.originateAnnounce(t)
		for _, node := range net.nodes {
			waitInboundDrain(t, node.tr, 50*time.Millisecond)
		}
		took, ok = waitForPaths(net.nodes[1:2], src.destHash, 2*time.Second)
		if ok == 1 {
			return
		}
	}
	if ok != 1 {
		t.Fatalf("first hop: %d/1 converged in %v (timeout %v)", ok, took, timeout)
	}
}

func TestSimIFACMaskHopRelayRoundTrip(t *testing.T) {
	id, err := ifac.New(0, "sim-net", "sim-passphrase")
	if err != nil {
		t.Fatalf("ifac.New: %v", err)
	}
	net := buildLine(t, 2)
	defer net.close()
	src := net.nodes[0]
	raw := src.announcePacket(t)
	masked, err := id.Mask(raw)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	unmasked, ok, err := id.Unmask(masked)
	if err != nil || !ok {
		t.Fatalf("unmask: ok=%v err=%v", ok, err)
	}
	unmasked[1]++
	remasked, err := id.Mask(unmasked)
	if err != nil {
		t.Fatalf("remask: %v", err)
	}
	final, ok, err := id.Unmask(remasked)
	if err != nil || !ok {
		t.Fatalf("final unmask: ok=%v err=%v", ok, err)
	}
	if final[1] != unmasked[1] {
		t.Fatalf("hop byte=%d want %d", final[1], unmasked[1])
	}
}

func TestSimIFACRelayPreprocessAcceptsForwardedAnnounce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node IFAC relay in -short mode")
	}
	enableSimFastPath(t)
	id, err := ifac.New(0, "sim-net", "sim-passphrase")
	if err != nil {
		t.Fatalf("ifac.New: %v", err)
	}
	net := buildLine(t, 3)
	defer net.close()
	for _, node := range net.nodes {
		for _, ifc := range node.ifaces {
			ifc.SetIFAC(id)
		}
	}
	src := net.nodes[0]
	raw := src.announcePacket(t)
	masked, err := id.Mask(raw)
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	midTr := net.nodes[1].tr
	job, _, accepted := midTr.preprocessInboundPacket(masked, net.nodes[1].ifaces[0])
	if !accepted {
		t.Fatal("preprocess rejected masked relay announce")
	}
	midTr.dispatchInboundPacket(job.pc.buf, job.iface, job.packetType, job.destType, job.headerType)
	putPacketCopy(job.pc)
	if !midTr.HasPath(src.destHash) {
		t.Fatal("mid did not learn path after preprocess")
	}
	unmasked, uok, err := id.Unmask(masked)
	if err != nil || !uok {
		t.Fatalf("unmask for forward: ok=%v err=%v", uok, err)
	}
	fwd := append([]byte(nil), unmasked...)
	fwd[1]++
	maskedFwd, err := id.Mask(fwd)
	if err != nil {
		t.Fatalf("mask forward: %v", err)
	}
	tailTr := net.nodes[2].tr
	tailJob, _, tailAccepted := tailTr.preprocessInboundPacket(maskedFwd, net.nodes[2].ifaces[0])
	if !tailAccepted {
		t.Fatal("tail preprocess rejected masked relay announce")
	}
	tailTr.dispatchInboundPacket(tailJob.pc.buf, tailJob.iface, tailJob.packetType, tailJob.destType, tailJob.headerType)
	putPacketCopy(tailJob.pc)
	if !tailTr.HasPath(src.destHash) {
		t.Fatalf("tail did not learn path after relay hops=%d", tailTr.HopsTo(src.destHash))
	}
}
