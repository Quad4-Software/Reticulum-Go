// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/debug"
)

// Ed25519Signer is re-exported for identity callers configuring HSM-backed signing.
type Ed25519Signer = cryptography.Ed25519Signer

type Identity struct {
	privateKey      []byte
	publicKey       []byte
	signingSeed     []byte // 32-byte Ed25519 seed; nil if externalSigner is set
	signingKey      ed25519.PrivateKey
	verificationKey ed25519.PublicKey
	externalSigner  cryptography.Ed25519Signer // if non-nil, Sign uses this instead of signingSeed
	hash            []byte
	hexHash         string

	ratchets      map[string][]byte
	ratchetExpiry map[string]int64
	mutex         *sync.RWMutex
}

var (
	knownDestinations     = make(map[string][]any)
	knownDestinationsLock sync.RWMutex
	knownRatchets         = make(map[string][]byte)
	ratchetPersistLock    sync.Mutex
)

func New() (*Identity, error) {
	i := &Identity{
		ratchets:      make(map[string][]byte),
		ratchetExpiry: make(map[string]int64),
		mutex:         &sync.RWMutex{},
	}

	// Generate keypairs using cryptography package
	privKey, pubKey, err := cryptography.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate X25519 keypair: %v", err)
	}
	i.privateKey = privKey
	i.publicKey = pubKey

	// Generate 32-byte Ed25519 seed
	var ed25519Seed [32]byte
	if _, err := io.ReadFull(rand.Reader, ed25519Seed[:]); err != nil {
		return nil, fmt.Errorf("failed to generate Ed25519 seed: %v", err)
	}

	// Derive Ed25519 keypair from seed
	privKeyEd := ed25519.NewKeyFromSeed(ed25519Seed[:])
	pubKeyEd := privKeyEd.Public().(ed25519.PublicKey)

	i.signingSeed = ed25519Seed[:]
	i.signingKey = privKeyEd
	i.verificationKey = pubKeyEd

	return i, nil
}

func (i *Identity) GetPublicKey() []byte {
	// Combine encryption and signing public keys in correct order
	fullKey := make([]byte, 64)
	copy(fullKey[:32], i.publicKey)       // First 32 bytes: X25519 encryption key
	copy(fullKey[32:], i.verificationKey) // Last 32 bytes: Ed25519 verification key
	return fullKey
}

func (i *Identity) GetPrivateKey() ([]byte, error) {
	if i.externalSigner != nil {
		return nil, ErrSigningMaterialNotExportable
	}
	if i.privateKey == nil || len(i.signingSeed) != ed25519.SeedSize {
		return nil, errors.New("identity has no exportable private key material")
	}
	out := make([]byte, 64)
	copy(out[:32], i.privateKey)
	copy(out[32:], i.signingSeed)
	return out, nil
}

func (i *Identity) Sign(data []byte) ([]byte, error) {
	if i.externalSigner != nil {
		return i.externalSigner.Sign(data)
	}
	if len(i.signingKey) != ed25519.PrivateKeySize {
		return nil, errors.New("identity has no signing key")
	}
	return cryptography.Sign(i.signingKey, data), nil
}

func (i *Identity) Verify(data []byte, signature []byte) bool {
	return cryptography.Verify(i.verificationKey, data, signature)
}

func (i *Identity) Encrypt(plaintext []byte, ratchet []byte) ([]byte, error) {
	// Generate ephemeral keypair
	ephemeralPrivKey, ephemeralPubKey, err := cryptography.GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	// Use ratchet key if provided, otherwise use identity public key
	targetKey := i.publicKey
	if ratchet != nil {
		targetKey = ratchet
	}

	// Generate shared secret
	sharedSecret, err := cryptography.DeriveSharedSecret(ephemeralPrivKey, targetKey)
	if err != nil {
		return nil, err
	}

	salt := i.GetSalt()
	debug.Log(debug.DebugAll, "Encrypt: using salt", "salt", fmt.Sprintf("%x", salt), "identity_hash", fmt.Sprintf("%x", i.Hash()))
	key, err := cryptography.DeriveIdentityKeyMaterial(sharedSecret, salt, i.GetContext())
	if err != nil {
		return nil, err
	}

	hmacKey := key[:32]
	encryptionKey := key[32:64]

	// Encrypt data
	ciphertext, err := cryptography.EncryptAES256CBC(encryptionKey, plaintext)
	if err != nil {
		return nil, err
	}

	// Calculate HMAC over ciphertext only (iv + encrypted_data)
	mac := cryptography.ComputeHMAC(hmacKey, ciphertext)

	// Combine components
	token := make([]byte, 0, len(ephemeralPubKey)+len(ciphertext)+len(mac))
	token = append(token, ephemeralPubKey...)
	token = append(token, ciphertext...)
	token = append(token, mac...)

	return token, nil
}

func (i *Identity) Hash() []byte {
	hash := cryptography.Hash(i.GetPublicKey())
	return hash[:TruncatedHashLength/8]
}

func TruncatedHash(data []byte) []byte {
	fullHash := cryptography.Hash(data)
	return fullHash[:TruncatedHashLength/8]
}

func GetRandomHash() []byte {
	randomData := make([]byte, TruncatedHashLength/8)
	_, err := rand.Read(randomData) // #nosec G104
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to read random data for hash", "error", err)
		return nil // Or handle the error appropriately
	}
	return TruncatedHash(randomData)
}

func Remember(packet []byte, destHash []byte, publicKey []byte, appData []byte) {
	hashStr := hex.EncodeToString(destHash)
	packetCopy := append([]byte(nil), packet...)
	destHashCopy := append([]byte(nil), destHash...)
	publicKeyCopy := append([]byte(nil), publicKey...)
	appDataCopy := append([]byte(nil), appData...)

	// Store destination data as [packet, destHash, identity, appData]
	id := FromPublicKey(publicKeyCopy)
	knownDestinationsLock.Lock()
	knownDestinations[hashStr] = []any{
		packetCopy,
		destHashCopy,
		id,
		appDataCopy,
	}
	knownDestinationsLock.Unlock()
}

func ValidateAnnounce(packet []byte, destHash []byte, publicKey []byte, signature []byte, appData []byte) bool {
	if len(publicKey) != KeySize/8 {
		return false
	}

	// Split public key into encryption and verification keys
	announced := &Identity{
		publicKey:       publicKey[:KeySize/16],
		verificationKey: publicKey[KeySize/16:],
	}

	// Verify signature
	signedData := make([]byte, 0, len(destHash)+len(publicKey)+len(appData))
	signedData = append(signedData, destHash...)
	signedData = append(signedData, publicKey...)
	signedData = append(signedData, appData...)

	if !announced.Verify(signedData, signature) {
		return false
	}

	// Store in known destinations
	Remember(packet, destHash, publicKey, appData)
	return true
}

func FromPublicKey(publicKey []byte) *Identity {
	if len(publicKey) != KeySize/8 {
		return nil
	}

	id := &Identity{
		publicKey:       publicKey[:KeySize/16],
		verificationKey: publicKey[KeySize/16:],
		ratchets:        make(map[string][]byte),
		ratchetExpiry:   make(map[string]int64),
		mutex:           &sync.RWMutex{},
	}

	hash := cryptography.Hash(id.GetPublicKey())
	id.hash = hash[:TruncatedHashLength/8]

	return id
}

func (i *Identity) Hex() string {
	return fmt.Sprintf("%x", i.Hash())
}

func (i *Identity) String() string {
	return i.Hex()
}

func Recall(hash []byte) (*Identity, error) {
	hashStr := hex.EncodeToString(hash)

	knownDestinationsLock.RLock()
	data, exists := knownDestinations[hashStr]
	knownDestinationsLock.RUnlock()

	if exists {
		// data is [packet, destHash, identity, appData]
		if len(data) >= 3 {
			if id, ok := data[2].(*Identity); ok {
				return id, nil
			}
		}
	}

	return nil, fmt.Errorf("identity not found for hash %x", hash)
}

func (i *Identity) GenerateHMACKey() []byte {
	hmacKey := make([]byte, KeySize/8)
	if _, err := io.ReadFull(rand.Reader, hmacKey); err != nil {
		return nil
	}
	return hmacKey
}

func (i *Identity) ComputeHMAC(key, message []byte) []byte {
	return cryptography.ComputeHMAC(key, message)
}

func (i *Identity) ValidateHMAC(key, message, messageHMAC []byte) bool {
	return cryptography.ValidateHMAC(key, message, messageHMAC)
}

func (i *Identity) GetCurrentRatchetKey() []byte {
	i.mutex.RLock()
	defer i.mutex.RUnlock()

	if len(i.ratchets) == 0 {
		// If no ratchets exist, generate one.
		// This should ideally be handled by an explicit setup process.
		debug.Log(debug.DebugTrace, "No ratchets found, generating a new one on-the-fly")
		// Temporarily unlock to call RotateRatchet, which locks internally.
		i.mutex.RUnlock()
		newRatchet, err := i.RotateRatchet()
		i.mutex.RLock()
		if err != nil {
			debug.Log(debug.DebugCritical, "Failed to generate initial ratchet key", "error", err)
			return nil
		}
		return newRatchet
	}

	// Return the most recently generated ratchet key
	var latestKey []byte
	var latestTime int64
	for id, expiry := range i.ratchetExpiry {
		if expiry > latestTime {
			latestTime = expiry
			latestKey = i.ratchets[id]
		}
	}

	if latestKey == nil {
		debug.Log(debug.DebugError, "Could not determine the latest ratchet key", "ratchet_count", len(i.ratchets))
	}

	return latestKey
}

func (i *Identity) Decrypt(ciphertextToken []byte, ratchets [][]byte, enforceRatchets bool, ratchetIDReceiver *common.RatchetIDReceiver) ([]byte, error) {
	if i.privateKey == nil {
		debug.Log(debug.DebugCritical, "Decryption failed: identity has no private key")
		return nil, errors.New("decryption failed because identity does not hold a private key")
	}

	debug.Log(debug.DebugAll, "Starting decryption for identity", "hash", i.GetHexHash())
	if len(ratchets) > 0 {
		debug.Log(debug.DebugAll, "Attempting decryption with ratchets", "count", len(ratchets))
	}

	if len(ciphertextToken) <= KeySize/8/2 {
		return nil, errors.New("decryption failed because the token size was invalid")
	}

	// Extract components: ephemeralPubKey(32) + ciphertext + mac(32)
	if len(ciphertextToken) < 32+32+32 { // minimum sizes
		return nil, errors.New("token too short")
	}

	peerPubBytes := ciphertextToken[:32]
	ciphertext := ciphertextToken[32 : len(ciphertextToken)-32]
	mac := ciphertextToken[len(ciphertextToken)-32:]

	// Try decryption with ratchets first if provided
	if len(ratchets) > 0 {
		for _, ratchet := range ratchets {
			if decrypted, ratchetID, err := i.tryRatchetDecryption(peerPubBytes, ciphertext, mac, ratchet); err == nil {
				if ratchetIDReceiver != nil {
					ratchetIDReceiver.LatestRatchetID = ratchetID
				}
				return decrypted, nil
			}
		}

		if enforceRatchets {
			if ratchetIDReceiver != nil {
				ratchetIDReceiver.LatestRatchetID = nil
			}
			return nil, errors.New("decryption with ratchet enforcement failed")
		}
	}

	sharedKey, err := cryptography.DeriveSharedSecret(i.privateKey, peerPubBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate shared key: %v", err)
	}

	salt := i.GetSalt()
	debug.Log(debug.DebugAll, "Decrypt: using salt", "salt", fmt.Sprintf("%x", salt), "identity_hash", fmt.Sprintf("%x", i.Hash()))
	derivedKey, err := cryptography.DeriveIdentityKeyMaterial(sharedKey, salt, i.GetContext())
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %v", err)
	}

	hmacKey := derivedKey[:32]
	encryptionKey := derivedKey[32:64]

	// Validate HMAC over ciphertext only (iv + encrypted_data)
	if !cryptography.ValidateHMAC(hmacKey, ciphertext, mac) {
		return nil, errors.New("invalid HMAC")
	}

	plaintext, err := cryptography.DecryptAES256CBC(encryptionKey, ciphertext)
	if err != nil {
		return nil, err
	}

	if ratchetIDReceiver != nil {
		ratchetIDReceiver.LatestRatchetID = nil
	}

	debug.Log(debug.DebugAll, "Decryption completed successfully")
	return plaintext, nil
}

// Helper function to attempt decryption using a ratchet
func (i *Identity) tryRatchetDecryption(peerPubBytes, ciphertext, mac, ratchet []byte) (plaintext, ratchetID []byte, err error) {
	// Convert ratchet to private key
	ratchetPriv := ratchet

	// Get ratchet ID
	ratchetPubBytes, err := cryptography.PublicKeyFromPrivate(ratchetPriv)
	if err != nil {
		debug.Log(debug.DebugAll, "Failed to generate ratchet public key", "error", err)
		return nil, nil, err
	}
	ratchetID = i.GetRatchetID(ratchetPubBytes)

	sharedSecret, err := cryptography.DeriveSharedSecret(ratchet, peerPubBytes)
	if err != nil {
		return nil, nil, err
	}

	key, err := cryptography.DeriveIdentityKeyMaterial(sharedSecret, i.GetSalt(), i.GetContext())
	if err != nil {
		return nil, nil, err
	}

	hmacKey := key[:32]
	encryptionKey := key[32:64]

	// Validate HMAC over ciphertext only (iv + encrypted_data)
	if !cryptography.ValidateHMAC(hmacKey, ciphertext, mac) {
		return nil, nil, errors.New("invalid HMAC")
	}

	plaintext, err = cryptography.DecryptAES256CBC(encryptionKey, ciphertext)
	if err != nil {
		return nil, nil, err
	}

	return plaintext, ratchetID, nil
}

func (i *Identity) EncryptWithHMAC(plaintext []byte, key []byte) ([]byte, error) {
	var hmacKey, encryptionKey []byte
	var err error
	if len(key) == 64 {
		hmacKey = key[:32]
		encryptionKey = key[32:64]
	} else if len(key) == 32 {
		hmacKey, encryptionKey, err = cryptography.ExpandEncryptWithHMACKeyMaterial(key)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("invalid key length for EncryptWithHMAC")
	}

	ciphertext, err := cryptography.EncryptAES256CBC(encryptionKey, plaintext)
	if err != nil {
		return nil, err
	}

	mac := cryptography.ComputeHMAC(hmacKey, ciphertext)
	return append(ciphertext, mac...), nil
}

func (i *Identity) DecryptWithHMAC(data []byte, key []byte) ([]byte, error) {
	if len(data) < cryptography.SHA256Size {
		return nil, errors.New("data too short")
	}

	var hmacKey, encryptionKey []byte
	var err error
	if len(key) == 64 {
		hmacKey = key[:32]
		encryptionKey = key[32:64]
	} else if len(key) == 32 {
		hmacKey, encryptionKey, err = cryptography.ExpandEncryptWithHMACKeyMaterial(key)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("invalid key length for DecryptWithHMAC")
	}

	macStart := len(data) - cryptography.SHA256Size
	ciphertext := data[:macStart]
	messageMAC := data[macStart:]

	if !cryptography.ValidateHMAC(hmacKey, ciphertext, messageMAC) {
		return nil, errors.New("invalid HMAC")
	}

	return cryptography.DecryptAES256CBC(encryptionKey, ciphertext)
}

func (i *Identity) ToFile(path string) error {
	debug.Log(debug.DebugAll, "Saving identity to file", "hash", i.GetHexHash(), "path", path)

	if i.externalSigner != nil {
		return ErrSigningMaterialNotExportable
	}
	if i.privateKey == nil || len(i.signingSeed) != ed25519.SeedSize {
		return errors.New("cannot save identity without private keys")
	}

	privateKeyBytes := make([]byte, 64)
	copy(privateKeyBytes[:32], i.privateKey)
	copy(privateKeyBytes[32:], i.signingSeed)

	// Write raw bytes to file
	// #nosec G304 G703 -- path is caller-chosen identity storage; not derived from network input here
	file, err := os.Create(path)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to create identity file", "error", err)
		return err
	}
	defer file.Close()

	if _, err := file.Write(privateKeyBytes); err != nil {
		debug.Log(debug.DebugCritical, "Failed to write identity data", "error", err)
		return err
	}

	debug.Log(debug.DebugAll, "Identity saved successfully", "bytes", len(privateKeyBytes))
	return nil
}

func FromFile(path string) (*Identity, error) {
	debug.Log(debug.DebugAll, "Loading identity from file", "path", path)

	// Read the private key bytes from file
	// #nosec G304 G703 -- path is caller-chosen identity storage; not derived from network input here
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file: %w", err)
	}

	if len(data) != 64 {
		return nil, fmt.Errorf("invalid identity file: expected 64 bytes, got %d", len(data))
	}

	// Parse the private keys
	// Format: [X25519 PrivKey (32 bytes)][Ed25519 PrivKey (32 bytes)]
	privateKey := data[:32]
	signingSeed := data[32:64]

	// Create identity with initialized maps and mutex
	ident := &Identity{
		ratchets:      make(map[string][]byte),
		ratchetExpiry: make(map[string]int64),
		mutex:         &sync.RWMutex{},
	}

	if err := ident.loadPrivateKey(privateKey, signingSeed); err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	debug.Log(debug.DebugInfo, "Identity loaded from file", "hash", ident.GetHexHash())
	return ident, nil
}

func LoadOrCreateTransportIdentity(customPath string) (*Identity, error) {
	storagePath := customPath
	if storagePath == "" {
		storagePath = os.Getenv("RETICULUM_STORAGE_PATH")
	}

	if storagePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		storagePath = fmt.Sprintf("%s/.reticulum/storage", homeDir)
	}

	// #nosec G703 -- storage path from RETICULUM_STORAGE_PATH or ~/.reticulum/storage; operator-controlled, not remote taint
	if err := os.MkdirAll(storagePath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	transportIdentityPath := fmt.Sprintf("%s/transport_identity", storagePath)

	if ident, err := FromFile(transportIdentityPath); err == nil {
		debug.Log(debug.DebugInfo, "Loaded transport identity from storage")
		return ident, nil
	}

	debug.Log(debug.DebugInfo, "No valid transport identity in storage, creating new one")
	ident, err := New()
	if err != nil {
		return nil, fmt.Errorf("failed to create transport identity: %w", err)
	}

	if err := ident.ToFile(transportIdentityPath); err != nil {
		return nil, fmt.Errorf("failed to save transport identity: %w", err)
	}

	debug.Log(debug.DebugInfo, "Created and saved transport identity")
	return ident, nil
}

func (i *Identity) loadPrivateKey(privateKey, signingSeed []byte) error {
	if len(privateKey) != 32 || len(signingSeed) != 32 {
		return errors.New("invalid private key length")
	}

	// Load X25519 private key
	i.privateKey = make([]byte, 32)
	copy(i.privateKey, privateKey)

	// Load Ed25519 signing seed
	i.signingSeed = make([]byte, 32)
	copy(i.signingSeed, signingSeed)

	var err error
	i.publicKey, err = cryptography.PublicKeyFromPrivate(i.privateKey)
	if err != nil {
		return fmt.Errorf("failed to derive X25519 public key: %w", err)
	}

	signingKey := ed25519.NewKeyFromSeed(i.signingSeed)
	i.signingKey = signingKey
	i.verificationKey = signingKey.Public().(ed25519.PublicKey)

	publicKeyBytes := make([]byte, 0, len(i.publicKey)+len(i.verificationKey))
	publicKeyBytes = append(publicKeyBytes, i.publicKey...)
	publicKeyBytes = append(publicKeyBytes, i.verificationKey...)
	i.hash = TruncatedHash(publicKeyBytes)[:TruncatedHashLength/8]
	i.hexHash = hex.EncodeToString(i.hash)

	debug.Log(debug.DebugVerbose, "Private key loaded successfully", "hash", i.GetHexHash())
	return nil
}

func RecallIdentity(path string) (*Identity, error) {
	debug.Log(debug.DebugAll, "Attempting to recall identity", "path", path)

	file, err := os.Open(path) // #nosec G304
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to open identity file", "error", err)
		return nil, err
	}
	defer file.Close()

	// Read raw bytes
	// Format: [X25519 PrivKey (32 bytes)][Ed25519 PrivKey (32 bytes)]
	privateKeyBytes := make([]byte, 64)
	n, err := io.ReadFull(file, privateKeyBytes)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to read identity data", "error", err)
		return nil, err
	}
	if n != 64 {
		return nil, fmt.Errorf("invalid identity file: expected 64 bytes, got %d", n)
	}

	// Extract keys
	x25519PrivKey := privateKeyBytes[:32]
	ed25519Seed := privateKeyBytes[32:]

	x25519PubKey, err := cryptography.PublicKeyFromPrivate(x25519PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive X25519 public key: %v", err)
	}

	ed25519PrivKey := ed25519.NewKeyFromSeed(ed25519Seed)
	ed25519PubKey := ed25519PrivKey.Public().(ed25519.PublicKey)

	id := &Identity{
		privateKey:      x25519PrivKey,
		publicKey:       x25519PubKey,
		signingSeed:     ed25519Seed,
		signingKey:      ed25519PrivKey,
		verificationKey: ed25519PubKey,
		ratchets:        make(map[string][]byte),
		ratchetExpiry:   make(map[string]int64),
		mutex:           &sync.RWMutex{},
	}

	combinedPub := make([]byte, KeySize/8)
	copy(combinedPub[:KeySize/16], id.publicKey)
	copy(combinedPub[KeySize/16:], id.verificationKey)
	fullHash := cryptography.Hash(combinedPub)
	id.hash = fullHash[:TruncatedHashLength/8]

	debug.Log(debug.DebugAll, "Successfully recalled identity", "hash", id.GetHexHash())
	return id, nil
}

func HashFromString(hash string) ([]byte, error) {
	if len(hash) != 32 {
		return nil, fmt.Errorf("invalid hash length: expected 32, got %d", len(hash))
	}

	return hex.DecodeString(hash)
}

func (i *Identity) GetSalt() []byte {
	if i.hash == nil {
		return nil
	}
	out := make([]byte, len(i.hash))
	copy(out, i.hash)
	return out
}

func (i *Identity) GetContext() []byte {
	return nil
}

func (i *Identity) GetRatchetID(ratchetPubBytes []byte) []byte {
	hash := cryptography.Hash(ratchetPubBytes)
	return hash[:NameHashLength/8]
}

func GetKnownDestination(hash string) ([]any, bool) {
	knownDestinationsLock.RLock()
	data, exists := knownDestinations[hash]
	knownDestinationsLock.RUnlock()
	if exists {
		copied := make([]any, len(data))
		copy(copied, data)
		for i := range copied {
			if b, ok := copied[i].([]byte); ok {
				copied[i] = append([]byte(nil), b...)
			}
		}
		return copied, true
	}
	return nil, false
}

func (i *Identity) GetHexHash() string {
	if i.hexHash == "" {
		i.hexHash = hex.EncodeToString(i.Hash())
	}
	return i.hexHash
}

func (i *Identity) GetRatchetKey(id string) ([]byte, bool) {
	ratchetPersistLock.Lock()
	defer ratchetPersistLock.Unlock()

	key, exists := knownRatchets[id]
	if !exists {
		return nil, false
	}
	return append([]byte(nil), key...), true
}

func (i *Identity) SetRatchetKey(id string, key []byte) {
	ratchetPersistLock.Lock()
	defer ratchetPersistLock.Unlock()

	knownRatchets[id] = append([]byte(nil), key...)
}

// NewIdentity creates a new Identity instance with fresh keys
func NewIdentity() (*Identity, error) {
	// Generate 32-byte Ed25519 seed
	var ed25519Seed [32]byte
	if _, err := io.ReadFull(rand.Reader, ed25519Seed[:]); err != nil {
		return nil, fmt.Errorf("failed to generate Ed25519 seed: %v", err)
	}

	// Derive Ed25519 keypair from seed
	privKey := ed25519.NewKeyFromSeed(ed25519Seed[:])
	pubKey := privKey.Public().(ed25519.PublicKey)

	// Generate X25519 encryption keypair
	var encPrivKey [32]byte
	if _, err := io.ReadFull(rand.Reader, encPrivKey[:]); err != nil {
		return nil, fmt.Errorf("failed to generate X25519 private key: %v", err)
	}

	encPubKey, err := cryptography.PublicKeyFromPrivate(encPrivKey[:])
	if err != nil {
		return nil, fmt.Errorf("failed to generate X25519 public key: %v", err)
	}

	i := &Identity{
		privateKey:      encPrivKey[:],
		publicKey:       encPubKey,
		signingSeed:     ed25519Seed[:],
		signingKey:      privKey,
		verificationKey: pubKey,
		ratchets:        make(map[string][]byte),
		ratchetExpiry:   make(map[string]int64),
		mutex:           &sync.RWMutex{},
	}

	combinedPub := make([]byte, KeySize/8)
	copy(combinedPub[:KeySize/16], i.publicKey)
	copy(combinedPub[KeySize/16:], i.verificationKey)
	fullHash := cryptography.Hash(combinedPub)
	i.hash = fullHash[:TruncatedHashLength/8]

	return i, nil
}

// FromBytes creates an Identity from a 64-byte private key representation
func FromBytes(data []byte) (*Identity, error) {
	if len(data) != 64 {
		return nil, fmt.Errorf("invalid identity data: expected 64 bytes, got %d", len(data))
	}

	privateKey := data[:32]
	signingSeed := data[32:64]

	ident := &Identity{
		ratchets:      make(map[string][]byte),
		ratchetExpiry: make(map[string]int64),
		mutex:         &sync.RWMutex{},
	}

	if err := ident.loadPrivateKey(privateKey, signingSeed); err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	return ident, nil
}

func (i *Identity) RotateRatchet() ([]byte, error) {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	debug.Log(debug.DebugAll, "Rotating ratchet for identity", "hash", i.GetHexHash())

	// Generate new ratchet key
	newRatchet := make([]byte, RatchetSize/8)
	if _, err := io.ReadFull(rand.Reader, newRatchet); err != nil {
		debug.Log(debug.DebugCritical, "Failed to generate new ratchet", "error", err)
		return nil, err
	}

	ratchetPub, err := cryptography.PublicKeyFromPrivate(newRatchet)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to generate ratchet public key", "error", err)
		return nil, err
	}

	ratchetID := i.GetRatchetID(ratchetPub)
	expiry := time.Now().Unix() + RatchetExpiry

	// Store new ratchet
	i.ratchets[string(ratchetID)] = newRatchet
	i.ratchetExpiry[string(ratchetID)] = expiry

	debug.Log(debug.DebugAll, "New ratchet generated", "id", fmt.Sprintf("%x", ratchetID), "expiry", expiry)

	// Cleanup old ratchets if we exceed max retained
	if len(i.ratchets) > MaxRetainedRatchets {
		var oldestID string
		oldestTime := time.Now().Unix()

		for id, exp := range i.ratchetExpiry {
			if exp < oldestTime {
				oldestTime = exp
				oldestID = id
			}
		}

		delete(i.ratchets, oldestID)
		delete(i.ratchetExpiry, oldestID)
		debug.Log(debug.DebugAll, "Cleaned up oldest ratchet", "id", fmt.Sprintf("%x", []byte(oldestID)))
	}

	debug.Log(debug.DebugAll, "Current number of active ratchets", "count", len(i.ratchets))
	return newRatchet, nil
}

func (i *Identity) GetRatchets() [][]byte {
	i.mutex.RLock()
	defer i.mutex.RUnlock()

	debug.Log(debug.DebugAll, "Getting ratchets for identity", "hash", i.GetHexHash())

	ratchets := make([][]byte, 0, len(i.ratchets))
	now := time.Now().Unix()
	expired := 0

	// Return only non-expired ratchets
	for id, expiry := range i.ratchetExpiry {
		if expiry > now {
			ratchets = append(ratchets, i.ratchets[id])
		} else {
			// Clean up expired ratchets
			delete(i.ratchets, id)
			delete(i.ratchetExpiry, id)
			expired++
		}
	}

	debug.Log(debug.DebugAll, "Retrieved active ratchets", "active", len(ratchets), "expired", expired)
	return ratchets
}

func (i *Identity) CleanupExpiredRatchets() {
	i.mutex.Lock()
	defer i.mutex.Unlock()

	debug.Log(debug.DebugAll, "Starting ratchet cleanup for identity", "hash", i.GetHexHash())

	now := time.Now().Unix()
	cleaned := 0
	for id, expiry := range i.ratchetExpiry {
		if expiry <= now {
			delete(i.ratchets, id)
			delete(i.ratchetExpiry, id)
			cleaned++
		}
	}

	debug.Log(debug.DebugAll, "Cleaned up expired ratchets", "cleaned", cleaned, "remaining", len(i.ratchets))
}

// ValidateAnnounce validates an announce packet's signature
func (i *Identity) ValidateAnnounce(data []byte, destHash []byte, appData []byte) bool {
	if i == nil || len(data) < ed25519.SignatureSize {
		return false
	}

	signatureStart := len(data) - ed25519.SignatureSize
	signature := data[signatureStart:]
	signedData := append(destHash, i.GetPublicKey()...)
	signedData = append(signedData, appData...)

	return cryptography.Verify(i.verificationKey, signedData, signature)
}

// GetNameHash returns a 10-byte hash derived from the identity's public key
func (i *Identity) GetNameHash() []byte {
	if i == nil || i.publicKey == nil {
		return nil
	}

	fullHash := cryptography.Hash(i.GetPublicKey())
	return fullHash[:NameHashLength/8]
}

// GetEncryptionKey returns the X25519 public key used for encryption
func (i *Identity) GetEncryptionKey() []byte {
	return i.publicKey
}

// GetSigningKey returns the Ed25519 public key used for signing
func (i *Identity) GetSigningKey() []byte {
	return i.verificationKey
}
