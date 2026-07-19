// SPDX-License-Identifier: Apache-2.0
package librns_test

import (
	"testing"

	"quad4/reticulum-go/pkg/librns"
)

func TestDestinationCreateSurfacesPacketEvents(t *testing.T) {
	node, code := librns.NodeCreate("")
	if code != librns.OK || node == 0 {
		t.Fatalf("NodeCreate: %d %s", code, librns.LastError())
	}
	defer librns.NodeDestroy(node)

	id, code := librns.IdentityGenerate()
	if code != librns.OK || id == 0 {
		t.Fatalf("IdentityGenerate: %d", code)
	}
	defer librns.IdentityDestroy(id)

	if code := librns.NodeSetIdentity(node, id); code != librns.OK {
		t.Fatalf("NodeSetIdentity: %d", code)
	}

	dest, code := librns.DestinationCreate(node, id, "lxmf", []string{"delivery"}, true)
	if code != librns.OK || dest == 0 {
		t.Fatalf("DestinationCreate: %d %s", code, librns.LastError())
	}
	defer librns.DestinationDestroy(dest)

	if code := librns.NodeStart(node); code != librns.OK {
		t.Fatalf("NodeStart: %d %s", code, librns.LastError())
	}
	defer librns.NodeStop(node)

	if _, code := librns.NodeInterfaces(node); code != librns.OK {
		t.Fatalf("NodeInterfaces: %d", code)
	}
	if librns.EventDestinationData != 11 {
		t.Fatalf("EventDestinationData = %d want 11", librns.EventDestinationData)
	}
	if librns.Version() != "1.4" {
		t.Fatalf("Version = %s want 1.4", librns.Version())
	}
}
