// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

func TestPackContainer_RoundTrip(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)

	msg, err := NewMessage(dst.Hash(), src.Hash(), []byte(testTitle), []byte(testContent), map[byte]any{
		FieldRenderer: []byte{RendererMarkdown},
	})
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := msg.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	msg.State = StateSent
	msg.Method = MethodOpportunistic

	data, err := PackContainer(msg)
	if err != nil {
		t.Fatalf("PackContainer: %v", err)
	}

	c, got, err := UnpackContainer(data, RecallSource)
	if err != nil {
		t.Fatalf("UnpackContainer: %v", err)
	}
	if c.State != StateSent {
		t.Errorf("state mismatch: %d", c.State)
	}
	if c.Method != MethodOpportunistic {
		t.Errorf("method mismatch: %d", c.Method)
	}
	if !c.TransportEncrypted {
		t.Errorf("expected transport_encrypted true")
	}
	if c.TransportEncryption != EncryptionDescriptionEC {
		t.Errorf("transport encryption: %q", c.TransportEncryption)
	}
	if !got.SignatureValidated {
		t.Errorf("decoded message signature not validated")
	}
	if !bytes.Equal(got.Hash, msg.Hash) {
		t.Errorf("hash mismatch")
	}
	if got.State != StateSent || got.Method != MethodOpportunistic {
		t.Errorf("metadata not restored on decoded message")
	}
}

func TestPackContainer_RejectsUnpacked(t *testing.T) {
	msg, err := NewMessage(make([]byte, DestinationLength), make([]byte, DestinationLength), nil, nil, nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := PackContainer(msg); err == nil {
		t.Fatal("expected error for unpacked message")
	}
}

func TestWriteToDirectoryAndReadFromFile(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)

	msg, err := NewMessage(dst.Hash(), src.Hash(), []byte("disk"), []byte("storage"), nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := msg.Pack(src); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	msg.State = StateDelivered
	msg.Method = MethodDirect

	dir := t.TempDir()
	path, err := WriteToDirectory(msg, dir)
	if err != nil {
		t.Fatalf("WriteToDirectory: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("unexpected dir: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected mode 0600, got %v", info.Mode().Perm())
	}

	c, got, err := ReadFromFile(path, RecallSource)
	if err != nil {
		t.Fatalf("ReadFromFile: %v", err)
	}
	if got.ContentString() != "storage" {
		t.Errorf("content mismatch: %q", got.ContentString())
	}
	if c.State != StateDelivered || c.Method != MethodDirect {
		t.Errorf("metadata not restored")
	}
}
