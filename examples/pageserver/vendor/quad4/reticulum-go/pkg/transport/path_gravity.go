// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"quad4/reticulum-go/pkg/common"
)

// interfaceGravity returns configured gravity for iface (RNS 1.4.1).
func interfaceGravity(iface common.NetworkInterface) int {
	if iface == nil {
		return 0
	}
	type g interface{ GetGravity() int }
	if v, ok := iface.(g); ok {
		return v.GetGravity()
	}
	return 0
}

// pathingAffinity is Go-unique effective gravity used for announce contests.
// It starts from configured gravity and applies a bounded live penalty when
// the transport has recently seen path failures via this interface. Operators
// still configure the same gravity numbers as Python.
func (t *Transport) pathingAffinity(iface common.NetworkInterface) int {
	g := interfaceGravity(iface)
	if t == nil || iface == nil {
		return g
	}
	pen := t.ifacePathPenalty(iface.GetName())
	if pen <= 0 {
		return g
	}
	if pen > 32 {
		pen = 32
	}
	g -= pen
	return g
}

func (t *Transport) ifacePathPenalty(name string) int {
	if t == nil || name == "" {
		return 0
	}
	t.rebalanceMu.Lock()
	defer t.rebalanceMu.Unlock()
	e := t.ifacePenalties[name]
	if e == nil {
		return 0
	}
	return e.penalty
}

func (t *Transport) noteIfacePathFailure(iface common.NetworkInterface) {
	if t == nil || iface == nil {
		return
	}
	name := iface.GetName()
	if name == "" {
		return
	}
	t.rebalanceMu.Lock()
	defer t.rebalanceMu.Unlock()
	if t.ifacePenalties == nil {
		t.ifacePenalties = make(map[string]*ifacePenalty)
	}
	e := t.ifacePenalties[name]
	if e == nil {
		e = &ifacePenalty{}
		t.ifacePenalties[name] = e
	}
	if e.penalty < 32 {
		e.penalty++
	}
}

func (t *Transport) noteIfacePathSuccess(iface common.NetworkInterface) {
	if t == nil || iface == nil {
		return
	}
	name := iface.GetName()
	t.rebalanceMu.Lock()
	defer t.rebalanceMu.Unlock()
	e := t.ifacePenalties[name]
	if e == nil || e.penalty == 0 {
		return
	}
	e.penalty--
	if e.penalty == 0 {
		delete(t.ifacePenalties, name)
	}
}

type ifacePenalty struct {
	penalty int
}
