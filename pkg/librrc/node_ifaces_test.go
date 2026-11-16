// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/librrc"
)

// Full hub-client session over TCP: hub node hosts a TCP server interface,
// client node dials in through a TCP client interface.
func TestHubClientTCPLoopback(t *testing.T) {
	if testing.Short() {
		t.Skip("tcp loopback skipped in -short mode")
	}

	const (
		listen     = "127.0.0.1:42530"
		eventWait  = 15 * time.Second
		pathWait   = 20 * time.Second
		pollPeriod = 200 * time.Millisecond
	)

	hubNode, code := librrc.NodeCreate("")
	if code != librrc.OK {
		t.Fatalf("hub node: %d", code)
	}
	defer librrc.NodeDestroy(hubNode)

	cliNode, code := librrc.NodeCreate("")
	if code != librrc.OK {
		t.Fatalf("client node: %d", code)
	}
	defer librrc.NodeDestroy(cliNode)

	if code := librrc.NodeAddTCPServerInterface(hubNode, "H1", listen); code != librrc.OK {
		t.Fatalf("hub tcp server: %d %s", code, lastErr())
	}
	if code := librrc.NodeAddTCPClientInterface(cliNode, "C1", listen); code != librrc.OK {
		t.Fatalf("client tcp: %d %s", code, lastErr())
	}

	if n, code := librrc.NodeInterfaceCount(cliNode); code != librrc.OK || n != 1 {
		t.Fatalf("interface count=%d code=%d", n, code)
	}
	if names, code := librrc.NodeInterfaceNames(cliNode); code != librrc.OK || len(names) != 1 || names[0] != "C1" {
		t.Fatalf("interface names=%v code=%d", names, code)
	}

	idH, code := librrc.IdentityGenerate()
	if code != librrc.OK {
		t.Fatal(code)
	}
	defer librrc.IdentityDestroy(idH)

	idC, code := librrc.IdentityGenerate()
	if code != librrc.OK {
		t.Fatal(code)
	}
	defer librrc.IdentityDestroy(idC)

	if code := librrc.NodeSetIdentity(hubNode, idH); code != librrc.OK {
		t.Fatal(code)
	}
	if code := librrc.NodeSetIdentity(cliNode, idC); code != librrc.OK {
		t.Fatal(code)
	}
	if code := librrc.NodeStart(hubNode); code != librrc.OK {
		t.Fatal(code)
	}
	if code := librrc.NodeStart(cliNode); code != librrc.OK {
		t.Fatal(code)
	}

	hub, code := librrc.HubCreate(hubNode, idH, "tcp-hub", "1.0")
	if code != librrc.OK {
		t.Fatalf("hub create: %d", code)
	}
	defer librrc.HubDestroy(hub)
	if code := librrc.HubStart(hub); code != librrc.OK {
		t.Fatal(code)
	}

	hubHash, code := librrc.HubHash(hub)
	if code != librrc.OK || len(hubHash) != 16 {
		t.Fatalf("hub hash: %d len=%d", code, len(hubHash))
	}
	if code := librrc.IdentitySeedDestination(idH, hubHash); code != librrc.OK {
		t.Fatal(code)
	}

	// Re-announce until the TCP pipe is up and the client learns the path.
	deadline := time.Now().Add(pathWait)
	for {
		_ = librrc.HubAnnounce(hub)
		ok, code := librrc.NodeHasPath(cliNode, hubHash)
		if code == librrc.OK && ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("path timeout")
		}
		time.Sleep(pollPeriod)
	}

	client, code := librrc.ClientDial(cliNode, idC, hubHash, "bob", "tcp-test", "1.0", 20000)
	if code != librrc.OK {
		t.Fatalf("dial: %d %s", code, lastErr())
	}
	runClientSession(t, hub, client, eventWait)
}

// Config file loading: enabled interface entries are attached at create.
func TestNodeCreateConfigPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	cfg := `
[interfaces]
  [[loopback]]
    type = UDPInterface
    enabled = yes
    listen_ip = 127.0.0.1
    listen_port = 42540
    forward_ip = 127.0.0.1
    forward_port = 42541

  [[dead-tcp]]
    type = TCPClientInterface
    enabled = no
    target_host = 127.0.0.1
    target_port = 1
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	node, code := librrc.NodeCreate(cfgPath)
	if code != librrc.OK {
		t.Fatalf("node create: %d %s", code, lastErr())
	}
	defer librrc.NodeDestroy(node)

	// One enabled entry attached; the disabled TCP entry must be skipped.
	if n, code := librrc.NodeInterfaceCount(node); code != librrc.OK || n != 1 {
		t.Fatalf("interface count=%d code=%d", n, code)
	}
	if names, code := librrc.NodeInterfaceNames(node); code != librrc.OK || len(names) != 1 || names[0] != "loopback" {
		t.Fatalf("interface names=%v code=%d", names, code)
	}
}

func TestNodeCreateConfigErrors(t *testing.T) {
	if node, code := librrc.NodeCreate("/nonexistent/rrc/config"); code == librrc.OK {
		librrc.NodeDestroy(node)
		t.Fatal("expected error for missing config path")
	}

	// A directory is not a readable config file.
	dir := t.TempDir()
	if node, code := librrc.NodeCreate(dir); code == librrc.OK {
		librrc.NodeDestroy(node)
		t.Fatal("expected error for directory config path")
	}
}

func TestNodeAddInterfaceValidation(t *testing.T) {
	node, code := librrc.NodeCreate("")
	if code != librrc.OK {
		t.Fatalf("node create: %d", code)
	}
	defer librrc.NodeDestroy(node)

	if code := librrc.NodeAddTCPClientInterface(node, "bad", "not-an-addr"); code == librrc.OK {
		t.Fatal("expected invalid arg for bad target")
	}
	if code := librrc.NodeAddTCPServerInterface(node, "bad", "not-an-addr"); code == librrc.OK {
		t.Fatal("expected invalid arg for bad listen")
	}
	if code := librrc.NodeAddTCPClientInterface(node, "", "127.0.0.1:1"); code == librrc.OK {
		t.Fatal("expected invalid arg for empty name")
	}
	if code := librrc.NodeAddBackboneClientInterface(node, "bb", "not-an-addr"); code == librrc.OK {
		t.Fatal("expected invalid arg for bad backbone target")
	}
}

func lastErr() string {
	return librrc.LastError()
}
