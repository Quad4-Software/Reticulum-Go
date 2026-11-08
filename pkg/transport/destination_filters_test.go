// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import "testing"

func TestDestinationFilters(t *testing.T) {
	f := newDestinationFilters()
	f.wants("app.aspect", "req1")
	if !f.announceAllowed(nameHashForType("app.aspect"), 3, true, false) {
		t.Fatal("client-required type should be allowed under required_only")
	}
	if f.announceAllowed(nameHashForType("other.aspect"), 3, true, false) {
		t.Fatal("unregistered type should be dropped under required_only")
	}
	if !f.announceAllowed(nameHashForType("other.aspect"), 1, true, true) {
		t.Fatal("local-client hops==1 exception should allow")
	}
	if f.announceAllowed(nameHashForType("other.aspect"), 1, true, false) {
		t.Fatal("non-local-client hops==1 should still be dropped")
	}
	if !f.unwants("app.aspect", "req1") {
		t.Fatal("unwants should return true for registered requester")
	}
	if f.announceAllowed(nameHashForType("app.aspect"), 3, true, false) {
		t.Fatal("type should be dropped after last requester leaves")
	}
	f.allow("allowed.aspect")
	if !f.announceAllowed(nameHashForType("allowed.aspect"), 5, true, false) {
		t.Fatal("allowed type should pass required_only")
	}
	f.block("blocked.aspect")
	if f.announceAllowed(nameHashForType("blocked.aspect"), 1, false, true) {
		t.Fatal("blocked type must always be dropped")
	}
	f.wants("blocked.aspect", "req2")
	if f.wants("blocked.aspect", "req3") {
		t.Fatal("wants on blocked type should fail")
	}
	if !f.unwantsAll("req2") {
		t.Fatal("unwantsAll should return true")
	}
	if f.announceAllowed(nameHashForType("app.aspect"), 3, true, false) {
		t.Fatal("unwantsAll should clear all requesters")
	}
}
