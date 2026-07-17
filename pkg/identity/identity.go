// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/identity/store"
	"quad4/reticulum-go/pkg/securemem"
)

// Ed25519Signer is re-exported for identity callers configuring HSM-backed signing.
type Ed25519Signer = cryptography.Ed25519Signer

type Identity struct {
	privateKey      *securemem.Buf // 32-byte X25519 private scalar
	publicKey       []byte
	signingSeed     *securemem.Buf // 32-byte Ed25519 seed. Nil if externalSigner is set.
	signingKey      *securemem.Buf // 64-byte expanded Ed25519 private key
	verificationKey ed25519.PublicKey
	externalSigner  cryptography.Ed25519Signer // if non-nil, Sign uses this instead of signingSeed
	hash            []byte
	hexHash         string

	ratchets      map[string]*securemem.Buf
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
		ratchets:      make(map[string]*securemem.Buf),
		ratchetExpiry: make(map[string]int64),
		mutex:         &sync.RWMutex{},
	}

	privKey, pubKey, err := cryptography.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate X25519 keypair: %w", err)
	}
	if err := storeX25519(i, privKey); err != nil {
		securemem.WipeBytes(privKey)
		return nil, err
	}
	securemem.WipeBytes(privKey)
	i.publicKey = pubKey

	var ed25519Seed [32]byte
	if _, err := io.ReadFull(rand.Reader, ed25519Seed[:]); err != nil {
		i.Close()
		return nil, fmt.Errorf("failed to generate Ed25519 seed: %w", err)
	}
	if err := storeSigningMaterial(i, ed25519Seed[:]); err != nil {
		securemem.WipeBytes(ed25519Seed[:])
		i.Close()
		return nil, err
	}
	securemem.WipeBytes(ed25519Seed[:])
	i.cachePublicHash()

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
	if !i.hasExportablePrivate() {
		return nil, errors.New("identity has no exportable private key material")
	}
	out := make([]byte, 64)
	copy(out[:32], i.privateKey.Bytes())
	copy(out[32:], i.signingSeed.Bytes())
	return out, nil
}

func (i *Identity) Sign(data []byte) ([]byte, error) {
	if i.externalSigner != nil {
		return i.externalSigner.Sign(data)
	}
	if i.signingKey == nil || i.signingKey.Len() != ed25519.PrivateKeySize {
		return nil, errors.New("identity has no signing key")
	}
	return cryptography.Sign(i.signingKey.Bytes(), data), nil
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
	defer securemem.WipeBytes(ephemeralPrivKey)

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
	defer securemem.WipeBytes(sharedSecret)

	salt := i.GetSalt()
	debug.Log(debug.DebugAll, "Encrypt: using salt", "salt", fmt.Sprintf("%x", salt), "identity_hash", fmt.Sprintf("%x", i.Hash()))
	key, err := cryptography.DeriveIdentityKeyMaterial(sharedSecret, salt, i.GetContext())
	if err != nil {
		return nil, err
	}
	defer securemem.WipeBytes(key)

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
	if i == nil {
		return nil
	}
	if len(i.hash) != TruncatedHashLength/8 {
		i.cachePublicHash()
	}
	out := make([]byte, TruncatedHashLength/8)
	copy(out, i.hash)
	return out
}

// cachePublicHash stores the truncated destination hash of the public key material.
func (i *Identity) cachePublicHash() {
	if i == nil {
		return
	}
	var full [64]byte
	copy(full[:32], i.publicKey)
	copy(full[32:], i.verificationKey)
	sum := cryptography.Hash(full[:])
	i.hash = append([]byte(nil), sum[:TruncatedHashLength/8]...)
}

func (i *Identity) ensureRatchetMaps() {
	if i.ratchets == nil {
		i.ratchets = make(map[string]*securemem.Buf)
	}
	if i.ratchetExpiry == nil {
		i.ratchetExpiry = make(map[string]int64)
	}
}

func (i *Identity) publicKeyEqual(publicKey []byte) bool {
	if i == nil || len(publicKey) != KeySize/8 {
		return false
	}
	return bytes.Equal(i.publicKey, publicKey[:KeySize/16]) &&
		bytes.Equal(i.verificationKey, publicKey[KeySize/16:])
}

func knownDestKey(destHash []byte) string {
	var buf [64]byte
	n := hex.Encode(buf[:], destHash)
	return string(buf[:n])
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
	hashStr := knownDestKey(destHash)

	knownDestinationsLock.Lock()
	defer knownDestinationsLock.Unlock()

	if existing, ok := knownDestinations[hashStr]; ok && len(existing) >= 4 {
		if id, ok := existing[2].(*Identity); ok && id.publicKeyEqual(publicKey) {
			prevPkt, _ := existing[0].([]byte)
			prevApp, _ := existing[3].([]byte)
			if bytes.Equal(prevPkt, packet) && bytes.Equal(prevApp, appData) {
				return
			}
			existing[0] = append([]byte(nil), packet...)
			existing[3] = append([]byte(nil), appData...)
			markKnownDestinationsDirty()
			return
		}
	}

	packetCopy := append([]byte(nil), packet...)
	destHashCopy := append([]byte(nil), destHash...)
	publicKeyCopy := append([]byte(nil), publicKey...)
	appDataCopy := append([]byte(nil), appData...)

	id := FromPublicKey(publicKeyCopy)
	knownDestinations[hashStr] = []any{
		packetCopy,
		destHashCopy,
		id,
		appDataCopy,
	}
	evictKnownDestinationsIfNeededLocked()
	markKnownDestinationsDirty()
}

// knownDestMaxEntries is the soft cap applied while in-memory storage is
// active. Zero means unlimited. Set via SetKnownDestinationsMaxEntries.
var knownDestMaxEntries atomic.Int64

// SetKnownDestinationsMaxEntries installs a soft cap on known destinations.
// Zero or negative disables the cap.
func SetKnownDestinationsMaxEntries(max int) {
	if max < 0 {
		max = 0
	}
	knownDestMaxEntries.Store(int64(max))
}

func evictKnownDestinationsIfNeededLocked() {
	max := int(knownDestMaxEntries.Load())
	if max <= 0 || len(knownDestinations) <= max {
		return
	}
	excess := len(knownDestinations) - max
	for key := range knownDestinations {
		if excess <= 0 {
			return
		}
		delete(knownDestinations, key)
		excess--
	}
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

	pub := make([]byte, KeySize/16)
	ver := make([]byte, KeySize/16)
	copy(pub, publicKey[:KeySize/16])
	copy(ver, publicKey[KeySize/16:])

	id := &Identity{
		publicKey:       pub,
		verificationKey: ver,
		mutex:           &sync.RWMutex{},
	}
	id.cachePublicHash()

	return id
}

// Hex returns the truncated identity hash as lowercase hex.
func (i *Identity) Hex() string {
	return fmt.Sprintf("%x", i.Hash())
}

// String returns the truncated identity hash as lowercase hex.
func (i *Identity) String() string {
	return i.Hex()
}

// Recall returns a previously remembered public identity for hash.
// Misses return ErrIdentityNotFound.
func Recall(hash []byte) (*Identity, error) {
	hashStr := knownDestKey(hash)

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

	return nil, common.ErrIdentityNotFoundf(hash)
}

// GenerateHMACKey returns a fresh KeySize/8-byte HMAC key, or nil on RNG failure.
func (i *Identity) GenerateHMACKey() []byte {
	hmacKey := make([]byte, KeySize/8)
	if _, err := io.ReadFull(rand.Reader, hmacKey); err != nil {
		return nil
	}
	return hmacKey
}

// ComputeHMAC returns the HMAC-SHA256 of message under key.
func (i *Identity) ComputeHMAC(key, message []byte) []byte {
	return cryptography.ComputeHMAC(key, message)
}

// ValidateHMAC reports whether messageHMAC is the HMAC-SHA256 of message under key.
func (i *Identity) ValidateHMAC(key, message, messageHMAC []byte) bool {
	return cryptography.ValidateHMAC(key, message, messageHMAC)
}

func (i *Identity) GetCurrentRatchetKey() []byte {
	i.mutex.RLock()
	defer i.mutex.RUnlock()

	if len(i.ratchets) == 0 {
		debug.Log(debug.DebugTrace, "No ratchets found, generating a new one on-the-fly")
		i.mutex.RUnlock()
		newRatchet, err := i.RotateRatchet()
		i.mutex.RLock()
		if err != nil {
			debug.Log(debug.DebugCritical, "Failed to generate initial ratchet key", "error", err)
			return nil
		}
		return newRatchet
	}

	var latestKey []byte
	var latestTime int64
	for id, expiry := range i.ratchetExpiry {
		if expiry > latestTime {
			latestTime = expiry
			if buf := i.ratchets[id]; buf != nil {
				latestKey = buf.CopyOut()
			}
		}
	}

	if latestKey == nil {
		debug.Log(debug.DebugError, "Could not determine the latest ratchet key", "ratchet_count", len(i.ratchets))
	}

	return latestKey
}

func (i *Identity) Decrypt(ciphertextToken []byte, ratchets [][]byte, enforceRatchets bool, ratchetIDReceiver *common.RatchetIDReceiver) ([]byte, error) {
	if !i.hasDecryptPrivate() {
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

	sharedKey, err := cryptography.DeriveSharedSecret(i.privateKey.Bytes(), peerPubBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate shared key: %w", err)
	}

	salt := i.GetSalt()
	debug.Log(debug.DebugAll, "Decrypt: using salt", "salt", fmt.Sprintf("%x", salt), "identity_hash", fmt.Sprintf("%x", i.Hash()))
	derivedKey, err := cryptography.DeriveIdentityKeyMaterial(sharedKey, salt, i.GetContext())
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
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

// EncryptWithHMAC AES-CBC encrypts plaintext and appends an HMAC over the ciphertext.
// key must be 32 bytes (expanded) or 64 bytes (hmac||enc material).
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

// DecryptWithHMAC verifies the trailing HMAC then AES-CBC decrypts.
// key must match the material used by EncryptWithHMAC.
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
	if !i.hasExportablePrivate() {
		return errors.New("cannot save identity without private keys")
	}

	privateKeyBytes := make([]byte, 64)
	copy(privateKeyBytes[:32], i.privateKey.Bytes())
	copy(privateKeyBytes[32:], i.signingSeed.Bytes())
	defer securemem.WipeBytes(privateKeyBytes)

	if err := store.SaveIdentityBlob(path, privateKeyBytes, ""); err != nil {
		debug.Log(debug.DebugCritical, "Failed to write identity file", "error", err)
		return err
	}

	debug.Log(debug.DebugAll, "Identity saved successfully", "bytes", len(privateKeyBytes))
	return nil
}

func FromFile(path string) (*Identity, error) {
	debug.Log(debug.DebugAll, "Loading identity from file", "path", path)

	data, err := store.LoadIdentityBlob(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read identity file: %w", err)
	}

	if len(data) != 64 {
		securemem.WipeBytes(data)
		return nil, fmt.Errorf("invalid identity file: expected 64 bytes, got %d", len(data))
	}
	defer securemem.WipeBytes(data)

	privateKey := data[:32]
	signingSeed := data[32:64]

	ident := &Identity{
		ratchets:      make(map[string]*securemem.Buf),
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

	// Empty storage path means fully ephemeral: never write into the operator
	// home directory from library or test callers.
	if storagePath == "" {
		ident, err := New()
		if err != nil {
			return nil, fmt.Errorf("failed to create ephemeral transport identity: %w", err)
		}
		debug.Log(debug.DebugInfo, "Created ephemeral transport identity")
		return ident, nil
	}

	// #nosec G703 -- storage path from RETICULUM_STORAGE_PATH or caller. Operator-controlled, not remote taint

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
	if err := loadPrivateInto(i, privateKey, signingSeed); err != nil {
		return err
	}

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
	x25519PrivKey := make([]byte, 32)
	ed25519Seed := make([]byte, 32)
	copy(x25519PrivKey, privateKeyBytes[:32])
	copy(ed25519Seed, privateKeyBytes[32:])
	securemem.WipeBytes(privateKeyBytes)

	id := &Identity{
		ratchets:      make(map[string]*securemem.Buf),
		ratchetExpiry: make(map[string]int64),
		mutex:         &sync.RWMutex{},
	}
	if err := id.loadPrivateKey(x25519PrivKey, ed25519Seed); err != nil {
		securemem.WipeBytes(x25519PrivKey)
		securemem.WipeBytes(ed25519Seed)
		return nil, err
	}
	securemem.WipeBytes(x25519PrivKey)
	securemem.WipeBytes(ed25519Seed)

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
	var ed25519Seed [32]byte
	if _, err := io.ReadFull(rand.Reader, ed25519Seed[:]); err != nil {
		return nil, fmt.Errorf("failed to generate Ed25519 seed: %w", err)
	}

	var encPrivKey [32]byte
	if _, err := io.ReadFull(rand.Reader, encPrivKey[:]); err != nil {
		securemem.WipeBytes(ed25519Seed[:])
		return nil, fmt.Errorf("failed to generate X25519 private key: %w", err)
	}

	encPubKey, err := cryptography.PublicKeyFromPrivate(encPrivKey[:])
	if err != nil {
		securemem.WipeBytes(ed25519Seed[:])
		securemem.WipeBytes(encPrivKey[:])
		return nil, fmt.Errorf("failed to generate X25519 public key: %w", err)
	}

	i := &Identity{
		publicKey:     encPubKey,
		ratchets:      make(map[string]*securemem.Buf),
		ratchetExpiry: make(map[string]int64),
		mutex:         &sync.RWMutex{},
	}
	if err := storeX25519(i, encPrivKey[:]); err != nil {
		securemem.WipeBytes(ed25519Seed[:])
		securemem.WipeBytes(encPrivKey[:])
		return nil, err
	}
	securemem.WipeBytes(encPrivKey[:])
	if err := storeSigningMaterial(i, ed25519Seed[:]); err != nil {
		securemem.WipeBytes(ed25519Seed[:])
		i.Close()
		return nil, err
	}
	securemem.WipeBytes(ed25519Seed[:])

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
		ratchets:      make(map[string]*securemem.Buf),
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
	i.ensureRatchetMaps()

	debug.Log(debug.DebugAll, "Rotating ratchet for identity", "hash", i.GetHexHash())

	newRatchet := make([]byte, RatchetSize/8)
	if _, err := io.ReadFull(rand.Reader, newRatchet); err != nil {
		debug.Log(debug.DebugCritical, "Failed to generate new ratchet", "error", err)
		return nil, err
	}

	ratchetPub, err := cryptography.PublicKeyFromPrivate(newRatchet)
	if err != nil {
		securemem.WipeBytes(newRatchet)
		debug.Log(debug.DebugCritical, "Failed to generate ratchet public key", "error", err)
		return nil, err
	}

	ratchetID := i.GetRatchetID(ratchetPub)
	expiry := time.Now().Unix() + RatchetExpiry

	buf, err := securemem.New(len(newRatchet))
	if err != nil {
		securemem.WipeBytes(newRatchet)
		return nil, err
	}
	if err := buf.CopyFrom(newRatchet); err != nil {
		_ = buf.Close()
		securemem.WipeBytes(newRatchet)
		return nil, err
	}
	i.ratchets[string(ratchetID)] = buf
	i.ratchetExpiry[string(ratchetID)] = expiry

	out := append([]byte(nil), newRatchet...)
	securemem.WipeBytes(newRatchet)

	debug.Log(debug.DebugAll, "New ratchet generated", "id", fmt.Sprintf("%x", ratchetID), "expiry", expiry)

	if len(i.ratchets) > MaxRetainedRatchets {
		var oldestID string
		oldestTime := time.Now().Unix()

		for id, exp := range i.ratchetExpiry {
			if exp < oldestTime {
				oldestTime = exp
				oldestID = id
			}
		}

		if old := i.ratchets[oldestID]; old != nil {
			_ = old.Close()
		}
		delete(i.ratchets, oldestID)
		delete(i.ratchetExpiry, oldestID)
		debug.Log(debug.DebugAll, "Cleaned up oldest ratchet", "id", fmt.Sprintf("%x", []byte(oldestID)))
	}

	debug.Log(debug.DebugAll, "Current number of active ratchets", "count", len(i.ratchets))
	return out, nil
}

func (i *Identity) GetRatchets() [][]byte {
	i.mutex.RLock()
	defer i.mutex.RUnlock()

	debug.Log(debug.DebugAll, "Getting ratchets for identity", "hash", i.GetHexHash())

	ratchets := make([][]byte, 0, len(i.ratchets))
	now := time.Now().Unix()
	expired := 0

	for id, expiry := range i.ratchetExpiry {
		if expiry > now {
			if buf := i.ratchets[id]; buf != nil {
				ratchets = append(ratchets, buf.CopyOut())
			}
		} else {
			if buf := i.ratchets[id]; buf != nil {
				_ = buf.Close()
			}
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
			if buf := i.ratchets[id]; buf != nil {
				_ = buf.Close()
			}
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
