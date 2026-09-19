// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"

	"github.com/Quad4-Software/Reticulum-Go/pkg/cryptography"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/health"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/protect"
	"github.com/Quad4-Software/Reticulum-Go/pkg/securemem"
)

func encryptedPayloadLen(plainLen int) int {
	padding := aes.BlockSize - plainLen%aes.BlockSize
	return aes.BlockSize + plainLen + padding + sha256.Size
}
func (l *Link) sealEncryptedHT1Locked(p *packet.Packet, data []byte) error {
	n := encryptedPayloadLen(len(data))
	payload, err := p.PrepareHT1Buffer(l.linkID, n)
	if err != nil {
		return err
	}
	out, err := l.encryptLockedInto(data, payload)
	if err != nil {
		return err
	}
	if len(out) != n {
		return errors.New("encrypted payload length mismatch")
	}
	return p.CommitPacked()
}
func encryptWithKeys(sessionKey, hmacKey, data []byte, block cipher.Block) ([]byte, error) {
	return encryptWithKeysInto(sessionKey, hmacKey, data, block, nil, nil)
}
func encryptWithKeysInto(sessionKey, hmacKey, data []byte, block cipher.Block, mac hash.Hash, dst []byte) ([]byte, error) {
	if block == nil && len(sessionKey) == 0 {
		return nil, errors.New("no session keys available")
	}
	if mac == nil && len(hmacKey) == 0 {
		return nil, errors.New("no session keys available")
	}

	if block == nil {
		var err error
		block, err = aes.NewCipher(sessionKey)
		if err != nil {
			return nil, err
		}
	}

	padding := aes.BlockSize - len(data)%aes.BlockSize
	ctLen := len(data) + padding
	n := aes.BlockSize + ctLen + sha256.Size
	if cap(dst) < n {
		dst = make([]byte, n)
	} else {
		dst = dst[:n]
	}
	if _, err := io.ReadFull(rand.Reader, dst[:aes.BlockSize]); err != nil {
		return nil, err
	}
	copy(dst[aes.BlockSize:aes.BlockSize+len(data)], data)
	padByte := byte(padding)
	for i := aes.BlockSize + len(data); i < aes.BlockSize+ctLen; i++ {
		dst[i] = padByte
	}
	if err := cryptography.EncryptCBC(block, dst[:aes.BlockSize], dst[aes.BlockSize:aes.BlockSize+ctLen]); err != nil {
		return nil, err
	}

	signed := dst[:aes.BlockSize+ctLen]
	if mac == nil {
		mac = hmac.New(sha256.New, hmacKey)
	} else {
		mac.Reset()
	}
	mac.Write(signed)
	return mac.Sum(signed), nil
}
func decryptWithKeys(sessionKey, hmacKey, data []byte, block cipher.Block, mac hash.Hash) ([]byte, error) {
	if block == nil && len(sessionKey) == 0 {
		return nil, errors.New("no session keys available")
	}
	if mac == nil && len(hmacKey) == 0 {
		return nil, errors.New("no session keys available")
	}
	if len(data) < aes.BlockSize+aes.BlockSize+32 {
		return nil, errors.New("data too short")
	}

	signedParts := data[:len(data)-32]
	receivedMac := data[len(data)-32:]

	var expected [sha256.Size]byte
	if mac == nil {
		mac = hmac.New(sha256.New, hmacKey)
	} else {
		mac.Reset()
	}
	mac.Write(signedParts)
	mac.Sum(expected[:0])
	if !hmac.Equal(receivedMac, expected[:]) {
		return nil, errHMACVerificationFailed
	}

	if block == nil {
		var err error
		block, err = aes.NewCipher(sessionKey)
		if err != nil {
			return nil, err
		}
	}
	if len(signedParts) < aes.BlockSize {
		return nil, errors.New("ciphertext is too short")
	}
	iv := signedParts[:aes.BlockSize]
	ciphertext := signedParts[aes.BlockSize:]
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext is not a multiple of the block size")
	}
	plaintext := make([]byte, len(ciphertext))
	if err := cryptography.DecryptCBC(block, iv, ciphertext, plaintext); err != nil {
		return nil, err
	}
	return cryptography.RemovePKCS7Padding(plaintext)
}

// snapshotSessionKeysLocked copies session/hmac key bytes into dst while the
// link mutex is held. Prefer this over retaining securemem.Bytes() across the
// unlocked AES work so handshake writers are not starved and closed buffers
// cannot be used after unlock.
func snapshotSessionKeysLocked(l *Link, sessionDst, hmacDst []byte) bool {
	if l.sessionKey == nil || l.hmacKey == nil {
		return false
	}
	sk := bufBytes(l.sessionKey)
	hk := bufBytes(l.hmacKey)
	if len(sk) == 0 || len(hk) == 0 || len(sessionDst) < len(sk) || len(hmacDst) < len(hk) {
		return false
	}
	copy(sessionDst, sk)
	copy(hmacDst, hk)
	return true
}
func (l *Link) refreshAESBlockLocked() {
	l.aesBlock = nil
	l.hmacSendMu.Lock()
	l.hmacSend = nil
	l.hmacSendMu.Unlock()
	l.hmacRecvMu.Lock()
	l.hmacRecv = nil
	l.hmacRecvMu.Unlock()
	if l.sessionKey == nil || l.hmacKey == nil {
		return
	}
	sk := bufBytes(l.sessionKey)
	hk := bufBytes(l.hmacKey)
	var key []byte
	if l.mode == ModeAES128CBC {
		if len(sk) < 16 || len(hk) < 16 {
			return
		}
		key = sk[:16]
		hk = hk[:16]
	} else if len(sk) >= 32 {
		key = sk[:32]
	} else if len(sk) >= 16 {
		key = sk[:16]
	} else {
		return
	}
	if len(hk) == 0 {
		return
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return
	}
	l.aesBlock = block
	l.hmacSendMu.Lock()
	l.hmacSend = hmac.New(sha256.New, hk)
	l.hmacSendMu.Unlock()
	l.hmacRecvMu.Lock()
	l.hmacRecv = hmac.New(sha256.New, hk)
	l.hmacRecvMu.Unlock()
}
func (l *Link) encrypt(data []byte) ([]byte, error) {
	l.mutex.RLock()
	block := l.aesBlock
	mac := l.hmacSend
	mode := l.mode
	var sessionKey, hmacKey [32]byte
	ok := block != nil && mac != nil
	if !ok {
		ok = snapshotSessionKeysLocked(l, sessionKey[:], hmacKey[:])
	}
	l.mutex.RUnlock()
	if !ok {
		return nil, errors.New("no session keys available")
	}
	if block != nil && mac != nil {
		l.hmacSendMu.Lock()
		out, err := encryptWithKeysInto(nil, nil, data, block, mac, nil)
		l.hmacSendMu.Unlock()
		return out, err
	}
	defer securemem.WipeBytes(sessionKey[:])
	defer securemem.WipeBytes(hmacKey[:])
	if mode == ModeAES128CBC {
		return encryptWithKeys(sessionKey[:16], hmacKey[:16], data, block)
	}
	return encryptWithKeys(sessionKey[:], hmacKey[:], data, block)
}
func (l *Link) encryptLockedInto(data, dst []byte) ([]byte, error) {
	if l.aesBlock != nil && l.hmacSend != nil {
		l.hmacSendMu.Lock()
		out, err := encryptWithKeysInto(nil, nil, data, l.aesBlock, l.hmacSend, dst)
		l.hmacSendMu.Unlock()
		return out, err
	}
	var sessionKey, hmacKey [32]byte
	if !snapshotSessionKeysLocked(l, sessionKey[:], hmacKey[:]) {
		return nil, errors.New("no session keys available")
	}
	defer securemem.WipeBytes(sessionKey[:])
	defer securemem.WipeBytes(hmacKey[:])
	if l.mode == ModeAES128CBC {
		return encryptWithKeysInto(sessionKey[:16], hmacKey[:16], data, l.aesBlock, nil, dst)
	}
	return encryptWithKeysInto(sessionKey[:], hmacKey[:], data, l.aesBlock, nil, dst)
}

// encryptLocked encrypts data while the link mutex is already held by the caller.
func (l *Link) encryptLocked(data []byte) ([]byte, error) {
	return l.encryptLockedInto(data, nil)
}
func (l *Link) decrypt(data []byte) ([]byte, error) {
	if len(data) < aes.BlockSize+aes.BlockSize+32 {
		debug.Log(debug.DebugError, "Decrypt failed: data too short", "length", len(data))
		return nil, errors.New("data too short")
	}
	d, release := protect.AdmitCrypto(l.attachedIfaceName())
	if !d.Allow {
		return nil, errors.New("dos_protection refused crypto")
	}
	defer release()
	l.mutex.RLock()
	block := l.aesBlock
	mac := l.hmacRecv
	mode := l.mode
	var sessionKey, hmacKey [32]byte
	ok := block != nil && mac != nil
	if !ok {
		ok = snapshotSessionKeysLocked(l, sessionKey[:], hmacKey[:])
	}
	l.mutex.RUnlock()
	if !ok {
		debug.Log(debug.DebugError, "Decrypt failed: no session keys", "link_id", fmt.Sprintf("%x", l.linkID))
		return nil, errors.New("no session keys available")
	}

	var plaintext []byte
	var err error
	if block != nil && mac != nil {
		l.hmacRecvMu.Lock()
		plaintext, err = decryptWithKeys(nil, nil, data, block, mac)
		l.hmacRecvMu.Unlock()
	} else {
		defer securemem.WipeBytes(sessionKey[:])
		defer securemem.WipeBytes(hmacKey[:])
		sk, hk := sessionKey[:], hmacKey[:]
		if mode == ModeAES128CBC {
			sk = sessionKey[:16]
			hk = hmacKey[:16]
		}
		plaintext, err = decryptWithKeys(sk, hk, data, block, nil)
	}
	if err != nil {
		ifaceName := l.attachedIfaceName()
		switch {
		case errors.Is(err, errHMACVerificationFailed):
			health.Inc(ifaceName, health.KindHMACFail)
		case cryptography.IsPaddingError(err):
			health.Inc(ifaceName, health.KindPaddingFail)
		}
		debug.Log(debug.DebugError, "Decrypt failed", "link_id", fmt.Sprintf("%x", l.linkID), "error", err)
		return nil, err
	}
	return plaintext, nil
}

// Validate checks a signature the way Python Link.validate does: against
// peer_sig_pub. For initiators that is the destination identity signing key.
func (l *Link) Validate(signature, message []byte) bool {
	l.mutex.RLock()
	peerSig := append([]byte(nil), l.peerSigPub...)
	remote := l.remoteIdentity
	l.mutex.RUnlock()

	if len(peerSig) == ed25519.PublicKeySize {
		return ed25519.Verify(peerSig, message, signature)
	}
	if remote != nil {
		return remote.Verify(message, signature)
	}
	return false
}
func (l *Link) generateEphemeralKeys() error {
	priv, pub, err := cryptography.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate X25519 keypair: %w", err)
	}
	if err := setSecBuf(&l.prv, priv); err != nil {
		securemem.WipeBytes(priv)
		return err
	}
	securemem.WipeBytes(priv)
	l.pub = pub

	pubKey, privKey, err := cryptography.GenerateSigningKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate Ed25519 keypair: %w", err)
	}
	if err := setSecBuf(&l.sigPriv, privKey); err != nil {
		securemem.WipeBytes(privKey)
		return err
	}
	securemem.WipeBytes(privKey)
	l.sigPub = pubKey

	return nil
}
