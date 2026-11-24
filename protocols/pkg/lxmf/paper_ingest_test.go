// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// TestIngestLXMURI exercises the paper-URI ingest path matching upstream
// ingest_lxm_uri: a message packed, encrypted for the router's delivery
// destination, base64url-encoded as lxm://, then ingested locally with
// stamp enforcement skipped.
func TestIngestLXMURI(t *testing.T) {
	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()

	id := mustNewIdentity(t)
	home := t.TempDir()
	msgDir := filepath.Join(home, "messages")
	router, err := NewRouter(id, tr, RouterOptions{
		Config:      DefaultConfig(),
		StoragePath: filepath.Join(home, "storage"),
		MessagesDir: msgDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	deliveryDest, err := router.RegisterDelivery("ingest-test", nil)
	if err != nil {
		t.Fatal(err)
	}

	srcID := mustNewIdentity(t)
	destHash := deliveryDest.GetHash()
	srcHash := deliveryDestinationHashMust(t, srcID)
	msg, err := NewMessage(destHash, srcHash, []byte("paper"), []byte("ingested body"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := msg.Pack(srcID); err != nil {
		t.Fatal(err)
	}

	rid := identity.FromPublicKey(id.GetPublicKey())
	outDest, err := destination.FromHash(destHash, rid, destination.Single, tr)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := outDest.Encrypt(msg.Packed[DestinationLength:])
	if err != nil {
		t.Fatal(err)
	}
	lxmfData := append(append([]byte(nil), destHash...), enc...)

	uri, err := PaperURI(lxmfData)
	if err != nil {
		t.Fatal(err)
	}
	if !router.IngestLXMURI(uri) {
		t.Fatal("IngestLXMURI rejected valid paper message")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(msgDir)
		if err == nil {
			for _, ent := range entries {
				if ent.IsDir() {
					continue
				}
				_, got, err := ReadFromFile(filepath.Join(msgDir, ent.Name()), RecallSource)
				if err == nil && got != nil && got.ContentString() == "ingested body" {
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("ingested message not delivered to messages dir")
}

func TestIngestLXMURIInvalid(t *testing.T) {
	router := &Router{}
	for _, uri := range []string{
		"",
		"not-a-uri",
		"http://example.com",
		"lxm://",
		"lxm://!!!invalid-base64!!!",
	} {
		if router.IngestLXMURI(uri) {
			t.Fatalf("IngestLXMURI(%q) returned true", uri)
		}
	}
}

func deliveryDestinationHashMust(t *testing.T, id *identity.Identity) []byte {
	t.Helper()
	h, err := deliveryDestinationHash(id)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
