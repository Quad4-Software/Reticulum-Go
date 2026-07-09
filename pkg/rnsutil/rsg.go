// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/identity"
)

const (
	// SigExt is the detached signature file extension (.rsg).
	SigExt = "rsg"
	// MsgExt is the embedded signed message extension (.rsm).
	MsgExt = "rsm"
	// EncryptExt is the encrypted file extension (.rfe).
	EncryptExt = "rfe"
	// SignatureSize is Ed25519 signature length in bytes.
	SignatureSize = 64
	// RSGHashType is the only supported content hash algorithm.
	RSGHashType = "sha256"
	// EncChunk is plaintext chunk size for .rfe encryption.
	EncChunk = 1024 * 1024 * 16
	// TokenOverhead is the per-chunk ciphertext overhead (pubkey + token).
	TokenOverhead = 96
)

var (
	errInvalidRSG    = errors.New("invalid rsg")
	errLegacyRSG     = errors.New("legacy rsg format not supported")
	errInvalidSigner = errors.New("signer mismatch")
	errInvalidHash   = errors.New("content hash mismatch")
	errInvalidSig    = errors.New("signature invalid")
	errDecryptFailed = errors.New("decrypt failed")
	rsgASCIIHeader   = []byte("#### Start of rsg data ")
	rsgASCIIFooter   = []byte(" End of rsg data ####")
	rsgASCIIRowWidth = 64
)

// RSGMeta is the metadata map embedded in an RSG envelope.
type RSGMeta struct {
	Signer []byte         `msgpack:"signer"`
	PubKey []byte         `msgpack:"pubkey"`
	Extra  map[string]any `msgpack:"-"`
}

// RSGEnvelope is the msgpack body signed by the identity.
type RSGEnvelope struct {
	HashType string         `msgpack:"hashtype"`
	Hash     []byte         `msgpack:"hash"`
	Meta     map[string]any `msgpack:"meta"`
	Message  []byte         `msgpack:"message,omitempty"`
}

// RSGResult is the outcome of validating an RSG/RSM blob.
type RSGResult struct {
	Valid    bool
	Envelope *RSGEnvelope
	Signer   *identity.Identity
}

// ContentHashSHA256 hashes message bytes or a reader with SHA-256.
func ContentHashSHA256(message any) ([]byte, error) {
	h := sha256.New()
	switch m := message.(type) {
	case []byte:
		_, _ = h.Write(m)
	case string:
		_, _ = h.Write([]byte(m))
	case io.Reader:
		if _, err := io.Copy(h, m); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported message type %T", message)
	}
	sum := h.Sum(nil)
	return sum, nil
}

// CreateRSG builds a .rsg or .rsm blob.
// When embed is true the message bytes are stored in the envelope (.rsm).
func CreateRSG(signer *identity.Identity, message any, embed bool, meta map[string]any) ([]byte, error) {
	if signer == nil {
		return nil, errors.New("nil signer")
	}
	if _, err := signer.GetPrivateKey(); err != nil {
		return nil, err
	}
	contentHash, err := ContentHashSHA256(message)
	if err != nil {
		return nil, err
	}
	envMap := map[string]any{
		"hashtype": RSGHashType,
		"hash":     contentHash,
		"meta": map[string]any{
			"signer": append([]byte(nil), signer.Hash()...),
			"pubkey": append([]byte(nil), signer.GetPublicKey()...),
		},
	}
	if embed {
		switch m := message.(type) {
		case []byte:
			envMap["message"] = append([]byte(nil), m...)
		case string:
			envMap["message"] = []byte(m)
		default:
			return nil, errors.New("embed requires []byte or string message")
		}
	}
	if meta != nil {
		metaMap := envMap["meta"].(map[string]any)
		for k, v := range meta {
			if _, exists := metaMap[k]; !exists {
				metaMap[k] = v
			}
		}
	}
	envelope, err := msgpack.Marshal(envMap)
	if err != nil {
		return nil, err
	}
	sig, err := signer.Sign(envelope)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(sig)+len(envelope))
	out = append(out, sig...)
	out = append(out, envelope...)
	return out, nil
}

// ValidateRSG validates an .rsg/.rsm blob against message content.
// requiredSigner may be an identity, a 16-byte hash, or nil.
func ValidateRSG(rsg []byte, message any, requiredSigner any) (RSGResult, error) {
	var out RSGResult
	rsg = unwrapRSGBytes(rsg)
	if len(rsg) == SignatureSize {
		return out, errLegacyRSG
	}
	if len(rsg) < SignatureSize+1 {
		return out, errInvalidRSG
	}
	sig := rsg[:SignatureSize]
	envelope := rsg[SignatureSize:]

	var envMap map[string]any
	if err := msgpack.Unmarshal(envelope, &envMap); err != nil {
		return out, errInvalidRSG
	}
	hashType, _ := envMap["hashtype"].(string)
	hashBytes, _ := asBytes(envMap["hash"])
	meta, _ := envMap["meta"].(map[string]any)
	if hashType != RSGHashType || len(hashBytes) == 0 || meta == nil {
		return out, errInvalidRSG
	}
	signerHash, _ := asBytes(meta["signer"])
	pubKey, _ := asBytes(meta["pubkey"])
	if len(signerHash) == 0 || len(pubKey) == 0 {
		return out, errInvalidRSG
	}

	var requiredHash []byte
	var signingID *identity.Identity
	switch rs := requiredSigner.(type) {
	case *identity.Identity:
		if rs != nil {
			signingID = rs
			requiredHash = rs.Hash()
		}
	case []byte:
		requiredHash = rs
	case string:
		b, err := hex.DecodeString(rs)
		if err != nil {
			return out, err
		}
		requiredHash = b
	case nil:
	default:
		return out, fmt.Errorf("invalid required signer type %T", requiredSigner)
	}

	if signingID == nil {
		signingID = identity.FromPublicKey(pubKey)
		if signingID == nil {
			return out, errInvalidRSG
		}
	}
	out.Signer = signingID
	out.Envelope = &RSGEnvelope{
		HashType: hashType,
		Hash:     hashBytes,
		Meta:     meta,
	}
	if msg, ok := asBytes(envMap["message"]); ok && len(msg) > 0 {
		out.Envelope.Message = msg
	}

	if requiredHash == nil {
		requiredHash = signingID.Hash()
	}
	if !bytes.Equal(signingID.Hash(), requiredHash) {
		return out, errInvalidSigner
	}

	contentHash, err := ContentHashSHA256(message)
	if err != nil {
		return out, err
	}
	if !bytes.Equal(hashBytes, contentHash) {
		return out, errInvalidHash
	}
	if !signingID.Verify(envelope, sig) {
		return out, errInvalidSig
	}
	out.Valid = true
	return out, nil
}

func asBytes(v any) ([]byte, bool) {
	switch b := v.(type) {
	case []byte:
		return b, true
	case string:
		return []byte(b), true
	default:
		return nil, false
	}
}

func unwrapRSGBytes(rsg []byte) []byte {
	if len(rsg) == 0 {
		return rsg
	}
	if !bytes.Contains(rsg, rsgASCIIHeader) && !bytes.HasPrefix(bytes.TrimSpace(rsg), []byte("####")) {
		// Try decode ascii encodings of binary rsg
		s := strings.TrimSpace(string(rsg))
		if decoded, err := decodeRSGText(s); err == nil && len(decoded) > SignatureSize {
			return decoded
		}
		return rsg
	}
	unwrapped := unwrapRSGString(string(rsg))
	if unwrapped == "" {
		return rsg
	}
	if decoded, err := decodeRSGText(unwrapped); err == nil {
		return decoded
	}
	return []byte(unwrapped)
}

func unwrapRSGString(wrapped string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(wrapped, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		b.WriteString(strings.TrimRight(line, "="))
	}
	return b.String()
}

func decodeRSGText(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := hex.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	pad := (8 - len(s)%8) % 8
	if b, err := base32.StdEncoding.DecodeString(s + strings.Repeat("=", pad)); err == nil {
		return b, nil
	}
	return nil, errors.New("not encoded rsg text")
}

// EncodeRSGText encodes binary rsg as hex/base64/base32.
func EncodeRSGText(rsg []byte, enc Encoding) string {
	return EncodeBytes(rsg, enc)
}

// SignFileRSG signs path contents into a .rsg blob.
func SignFileRSG(signer *identity.Identity, path string) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- operator path
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return CreateRSG(signer, f, false, nil)
}

// VerifyFileRSG verifies path against an .rsg blob.
func VerifyFileRSG(rsg []byte, path string, requiredSigner any) (RSGResult, error) {
	f, err := os.Open(path) // #nosec G304 -- operator path
	if err != nil {
		return RSGResult{}, err
	}
	defer f.Close()
	return ValidateRSG(rsg, f, requiredSigner)
}

// CreateRSM signs an embedded UTF-8 message into a .rsm blob.
func CreateRSM(signer *identity.Identity, message string, meta map[string]any) ([]byte, error) {
	return CreateRSG(signer, message, true, meta)
}

// VerifyRSM validates a .rsm blob and returns the embedded message.
func VerifyRSM(rsm []byte, requiredSigner any) (RSGResult, string, error) {
	rsm = unwrapRSGBytes(rsm)
	if len(rsm) <= SignatureSize {
		return RSGResult{}, "", errInvalidRSG
	}
	var envMap map[string]any
	if err := msgpack.Unmarshal(rsm[SignatureSize:], &envMap); err != nil {
		return RSGResult{}, "", errInvalidRSG
	}
	msg, ok := asBytes(envMap["message"])
	if !ok || len(msg) == 0 {
		return RSGResult{}, "", errors.New("no embedded message")
	}
	res, err := ValidateRSG(rsm, msg, requiredSigner)
	return res, string(msg), err
}

// EncryptFileRFE encrypts path to outPath in chunked .rfe format.
func EncryptFileRFE(id *identity.Identity, inPath, outPath string) error {
	in, err := os.Open(inPath) // #nosec G304
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 G302
	if err != nil {
		return err
	}
	defer out.Close()
	buf := make([]byte, EncChunk)
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			ct, err := id.Encrypt(buf[:n], nil)
			if err != nil {
				return err
			}
			if _, err := out.Write(ct); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

// DecryptFileRFE decrypts a .rfe file to outPath.
func DecryptFileRFE(id *identity.Identity, inPath, outPath string) error {
	in, err := os.Open(inPath) // #nosec G304
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 G302
	if err != nil {
		return err
	}
	defer out.Close()
	buf := make([]byte, EncChunk+TokenOverhead)
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			pt, err := id.Decrypt(buf[:n], nil, false, nil)
			if err != nil || pt == nil {
				return errDecryptFailed
			}
			if _, err := out.Write(pt); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
