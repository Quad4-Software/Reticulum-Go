package destination

import (
	"bytes"
	"crypto/sha256"
	"path/filepath"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
)

type mockTransport struct {
	config     *common.ReticulumConfig
	interfaces map[string]common.NetworkInterface
}

func (m *mockTransport) GetConfig() *common.ReticulumConfig {
	return m.config
}

func (m *mockTransport) GetInterfaces() map[string]common.NetworkInterface {
	return m.interfaces
}

func (m *mockTransport) RegisterDestination(hash []byte, dest interface{}) {
}

type mockInterface struct {
	common.BaseInterface
}

func (m *mockInterface) Send(data []byte, address string) error {
	return nil
}

func TestNewDestination(t *testing.T) {
	id, _ := identity.New()
	transport := &mockTransport{config: &common.ReticulumConfig{}}

	dest, err := New(id, IN|OUT, SINGLE, "testapp", transport, "testaspect")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if dest == nil {
		t.Fatal("New returned nil")
	}

	if dest.ExpandName() != "testapp.testaspect" {
		t.Errorf("Expected name testapp.testaspect, got %s", dest.ExpandName())
	}

	hash := dest.GetHash()
	if len(hash) != 16 {
		t.Errorf("Expected hash length 16, got %d", len(hash))
	}
}

func TestFromHash(t *testing.T) {
	id, _ := identity.New()
	transport := &mockTransport{}
	hash := make([]byte, 16)

	dest, err := FromHash(hash, id, SINGLE, transport)
	if err != nil {
		t.Fatalf("FromHash failed: %v", err)
	}
	if !bytes.Equal(dest.GetHash(), hash) {
		t.Error("Hashes don't match")
	}
}

func TestRequestHandlers(t *testing.T) {
	id, _ := identity.New()
	dest, _ := New(id, IN, SINGLE, "test", &mockTransport{})

	path := "test/path"
	response := []byte("hello")

	err := dest.RegisterRequestHandler(path, func(p string, d []byte, rid []byte, lid []byte, ri *identity.Identity, ra int64) []byte {
		return response
	}, ALLOW_ALL, nil)
	if err != nil {
		t.Fatalf("RegisterRequestHandler failed: %v", err)
	}

	result := dest.HandleRequest(path, nil, nil, nil, nil, 0)
	if !bytes.Equal(result, response) {
		t.Errorf("Expected response %q, got %q", response, result)
	}

	if !dest.DeregisterRequestHandler(path) {
		t.Error("DeregisterRequestHandler failed")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	id, _ := identity.New()
	dest, _ := New(id, IN|OUT, SINGLE, "test", &mockTransport{})

	plaintext := []byte("hello world")
	ciphertext, err := dest.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := dest.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted data doesn't match: %q vs %q", decrypted, plaintext)
	}
}

func TestRatchets(t *testing.T) {
	tmpDir := t.TempDir()
	ratchetPath := filepath.Join(tmpDir, "ratchets")

	id, _ := identity.New()
	dest, _ := New(id, IN|OUT, SINGLE, "test", &mockTransport{})

	if !dest.EnableRatchets(ratchetPath) {
		t.Fatal("EnableRatchets failed")
	}

	err := dest.RotateRatchets()
	if err != nil {
		t.Fatalf("RotateRatchets failed: %v", err)
	}

	ratchets := dest.GetRatchets()
	if len(ratchets) != 1 {
		t.Errorf("Expected 1 ratchet, got %d", len(ratchets))
	}
}

func TestPlainDestination(t *testing.T) {
	id, _ := identity.New()
	dest, _ := New(id, IN|OUT, PLAIN, "test", &mockTransport{})

	plaintext := []byte("plain text")
	ciphertext, _ := dest.Encrypt(plaintext)
	if !bytes.Equal(plaintext, ciphertext) {
		t.Error("Plain destination should not encrypt")
	}

	decrypted, _ := dest.Decrypt(ciphertext)
	if !bytes.Equal(plaintext, decrypted) {
		t.Error("Plain destination should not decrypt")
	}
}

func TestPlainDestinationHash(t *testing.T) {
	// A PLAIN destination with no identity should have a hash based only on its name
	transport := &mockTransport{}
	dest, err := New(nil, IN|OUT, PLAIN, "testapp", transport, "testaspect")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	hash := dest.GetHash()
	if len(hash) != 16 {
		t.Fatalf("Expected hash length 16, got %d", len(hash))
	}

	// Calculate manually: SHA256(SHA256("testapp.testaspect")[:10])[:16]
	name := "testapp.testaspect"
	nameHashFull := sha256.Sum256([]byte(name))
	nameHash10 := nameHashFull[:10]
	finalHashFull := sha256.Sum256(nameHash10)
	expectedHash := finalHashFull[:16]

	if !bytes.Equal(hash, expectedHash) {
		t.Errorf("Expected hash %x, got %x", expectedHash, hash)
	}
}
