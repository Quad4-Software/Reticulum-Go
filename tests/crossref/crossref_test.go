// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package crossref

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"

	"quad4/bzip2/pkg/bzip2"
	"quad4/reticulum-go/pkg/buffer"
	"quad4/reticulum-go/pkg/channel"
	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/resource"
)

type TestVectors struct {
	FormatVersion          int                            `json:"format_version"`
	Generator              string                         `json:"generator"`
	Identity               []IdentityVector               `json:"identity"`
	DestinationHash        []DestHashVector               `json:"destination_hash"`
	HKDF                   []HKDFVector                   `json:"hkdf"`
	HMAC                   []HMACVector                   `json:"hmac"`
	Token                  []TokenVector                  `json:"token"`
	PacketHeader           []PacketHeaderVector           `json:"packet_header"`
	Announce               []AnnounceVector               `json:"announce"`
	Encryption             []EncryptionVector             `json:"encryption"`
	Hash                   []HashVector                   `json:"hash"`
	ECDH                   []ECDHVector                   `json:"ecdh"`
	RatchetID              []RatchetIDVector              `json:"ratchet_id"`
	PacketWire             []PacketWireVector             `json:"packet_wire"`
	PacketHash             []PacketHashVector             `json:"packet_hash"`
	AES                    []AESVector                    `json:"aes"`
	CrossSign              []CrossSignVector              `json:"cross_sign"`
	IdentityFile           []IdentityFileVector           `json:"identity_file"`
	LinkSignalling         []LinkSignallingVector         `json:"link_signalling"`
	LinkKeyDerivation      []LinkKeyDerivationVector      `json:"link_key_derivation"`
	LinkRequest            []LinkRequestVector            `json:"link_request"`
	LinkProof              []LinkProofVector              `json:"link_proof"`
	ResourceAdvertisement  []ResourceAdvertisementVector  `json:"resource_advertisement"`
	ChannelEnvelope        []ChannelEnvelopeVector        `json:"channel_envelope"`
	BufferStream           []BufferStreamVector           `json:"buffer_stream"`
	ResourceHash           []ResourceHashVector           `json:"resource_hash"`
	LinkEncryption         []LinkEncryptionVector         `json:"link_encryption"`
	PathRequest            []PathRequestVector            `json:"path_request"`
	ReceiptProof           []ReceiptProofVector           `json:"receipt_proof"`
	LRProofPacket          []LRProofPacketVector          `json:"lrproof_packet"`
	PythonPacket           []PythonPacketVector           `json:"python_packet"`
	ResourceContext        []ResourceContextVector        `json:"resource_context"`
	ResourceMetadataPrefix []ResourceMetadataPrefixVector `json:"resource_metadata_prefix"`
	BufferCompressed       []BufferCompressedVector       `json:"buffer_compressed"`
	ResourceReq            []ResourceReqVector            `json:"resource_req"`
	ResourceHMU            []ResourceHMUVector            `json:"resource_hmu"`
	ResourcePRF            []ResourcePRFVector            `json:"resource_prf"`
	ResourceICLRCL         []ResourceICLRCLVector         `json:"resource_icl_rcl"`
	LRRTT                  []LRRTTVector                  `json:"lrrtt"`
	DestinationType        []DestinationTypeVector        `json:"destination_type"`
	CacheRequest           []CacheRequestVector           `json:"cache_request"`
}

type ResourceReqVector struct {
	DataHex            string `json:"data_hex"`
	HMUPartHex         string `json:"hmu_part_hex"`
	HashmapExhausted   bool   `json:"hashmap_exhausted"`
	ResourceHashHex    string `json:"resource_hash_hex"`
	LastMapHashHex     string `json:"last_map_hash_hex"`
	RequestedHashesHex string `json:"requested_hashes_hex"`
}

type ResourceHMUVector struct {
	DataHex         string `json:"data_hex"`
	ResourceHashHex string `json:"resource_hash_hex"`
	Segment         int    `json:"segment"`
	HashmapHex      string `json:"hashmap_hex"`
}

type ResourcePRFVector struct {
	DataHex         string `json:"data_hex"`
	ResourceHashHex string `json:"resource_hash_hex"`
	ProofHex        string `json:"proof_hex"`
	ProofDataHex    string `json:"proof_data_hex"`
}

type ResourceICLRCLVector struct {
	PayloadHex      string `json:"payload_hex"`
	ResourceHashHex string `json:"resource_hash_hex"`
}

type LRRTTVector struct {
	PayloadHex string  `json:"payload_hex"`
	RTT        float64 `json:"rtt"`
}

type DestinationTypeVector struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type CacheRequestVector struct {
	PayloadHex    string `json:"payload_hex"`
	PacketHashHex string `json:"packet_hash_hex"`
	Context       int    `json:"context"`
}

type IdentityVector struct {
	PrivateKeyHex  string `json:"private_key_hex"`
	PublicKeyHex   string `json:"public_key_hex"`
	HashHex        string `json:"hash_hex"`
	Hexhash        string `json:"hexhash"`
	SignMessageHex string `json:"sign_message_hex"`
	SignatureHex   string `json:"signature_hex"`
}

type DestHashVector struct {
	AppName           string   `json:"app_name"`
	Aspects           []string `json:"aspects"`
	ExpandName        string   `json:"expand_name"`
	NameHash10Hex     string   `json:"name_hash_10_hex"`
	IdentityHashHex   string   `json:"identity_hash_hex"`
	SingleDestHashHex string   `json:"single_dest_hash_hex"`
	PlainDestHashHex  string   `json:"plain_dest_hash_hex"`
}

type HKDFVector struct {
	SecretHex  string `json:"secret_hex"`
	SaltHex    string `json:"salt_hex"`
	ContextHex string `json:"context_hex"`
	Length     int    `json:"length"`
	DerivedHex string `json:"derived_hex"`
}

type HMACVector struct {
	KeyHex     string `json:"key_hex"`
	MessageHex string `json:"message_hex"`
	HMACHex    string `json:"hmac_hex"`
}

type TokenVector struct {
	KeyHex        string `json:"key_hex"`
	PlaintextHex  string `json:"plaintext_hex"`
	TokenHex      string `json:"token_hex"`
	IVHex         string `json:"iv_hex"`
	CiphertextHex string `json:"ciphertext_hex"`
	HMACHex       string `json:"hmac_hex"`
	TokenOverhead int    `json:"token_overhead"`
}

type PacketHeaderVector struct {
	HeaderType      int `json:"header_type"`
	ContextFlag     int `json:"context_flag"`
	TransportType   int `json:"transport_type"`
	DestinationType int `json:"destination_type"`
	PacketType      int `json:"packet_type"`
	FlagsByte       int `json:"flags_byte"`
}

type AnnounceVector struct {
	HasRatchet    bool   `json:"has_ratchet"`
	PublicKeyHex  string `json:"public_key_hex"`
	NameHashHex   string `json:"name_hash_hex"`
	RandomHashHex string `json:"random_hash_hex"`
	RatchetHex    string `json:"ratchet_hex,omitempty"`
	DestHashHex   string `json:"dest_hash_hex"`
	AppDataHex    string `json:"app_data_hex"`
	SignedDataHex string `json:"signed_data_hex"`
	SignatureHex  string `json:"signature_hex"`
	PayloadHex    string `json:"payload_hex"`
}

type EncryptionVector struct {
	PrivateKeyHex string `json:"private_key_hex"`
	PublicKeyHex  string `json:"public_key_hex"`
	PlaintextHex  string `json:"plaintext_hex"`
	CiphertextHex string `json:"ciphertext_hex"`
}

type HashVector struct {
	InputHex         string `json:"input_hex"`
	FullHashHex      string `json:"full_hash_hex"`
	TruncatedHashHex string `json:"truncated_hash_hex"`
}

type ECDHVector struct {
	PrivateAHex     string `json:"private_a_hex"`
	PublicAHex      string `json:"public_a_hex"`
	PrivateBHex     string `json:"private_b_hex"`
	PublicBHex      string `json:"public_b_hex"`
	SharedSecretHex string `json:"shared_secret_hex"`
}

type RatchetIDVector struct {
	RatchetPrivateHex string `json:"ratchet_private_hex"`
	RatchetPublicHex  string `json:"ratchet_public_hex"`
	RatchetIDHex      string `json:"ratchet_id_hex"`
}

type PacketWireVector struct {
	RawHex          string `json:"raw_hex"`
	HeaderType      int    `json:"header_type"`
	PacketType      int    `json:"packet_type"`
	TransportType   int    `json:"transport_type"`
	DestinationType int    `json:"destination_type"`
	ContextFlag     int    `json:"context_flag"`
	Context         int    `json:"context"`
	Hops            int    `json:"hops"`
	DestHashHex     string `json:"dest_hash_hex"`
	TransportIDHex  string `json:"transport_id_hex"`
	DataHex         string `json:"data_hex"`
}

type PacketHashVector struct {
	RawHex           string `json:"raw_hex"`
	HeaderType       int    `json:"header_type"`
	PacketHashHex    string `json:"packet_hash_hex"`
	TruncatedHashHex string `json:"truncated_hash_hex"`
}

type AESVector struct {
	KeyHex        string `json:"key_hex"`
	IVHex         string `json:"iv_hex"`
	PlaintextHex  string `json:"plaintext_hex"`
	PaddedHex     string `json:"padded_hex"`
	CiphertextHex string `json:"ciphertext_hex"`
}

type CrossSignVector struct {
	SignerIndex        int    `json:"signer_index"`
	SignerPublicKeyHex string `json:"signer_public_key_hex"`
	DataHex            string `json:"data_hex"`
	SignatureHex       string `json:"signature_hex"`
}

type IdentityFileVector struct {
	FileBytesHex string `json:"file_bytes_hex"`
	PublicKeyHex string `json:"public_key_hex"`
	HashHex      string `json:"hash_hex"`
}

type LinkSignallingVector struct {
	MTU           int    `json:"mtu"`
	Mode          int    `json:"mode"`
	SignallingHex string `json:"signalling_hex"`
	DecodedMTU    int    `json:"decoded_mtu"`
	DecodedMode   int    `json:"decoded_mode"`
}

type LinkKeyDerivationVector struct {
	InitiatorPrvHex string `json:"initiator_prv_hex"`
	InitiatorPubHex string `json:"initiator_pub_hex"`
	ResponderPrvHex string `json:"responder_prv_hex"`
	ResponderPubHex string `json:"responder_pub_hex"`
	SharedKeyHex    string `json:"shared_key_hex"`
	LinkIDHex       string `json:"link_id_hex"`
	Mode            int    `json:"mode"`
	DerivedKeyHex   string `json:"derived_key_hex"`
	HMACKeyHex      string `json:"hmac_key_hex"`
	SessionKeyHex   string `json:"session_key_hex"`
}

type LinkRequestVector struct {
	X25519PubHex  string `json:"x25519_pub_hex"`
	Ed25519PubHex string `json:"ed25519_pub_hex"`
	SignallingHex string `json:"signalling_hex"`
	PayloadHex    string `json:"payload_hex"`
	ECPubSize     int    `json:"ecpubsize"`
	PayloadLen    int    `json:"payload_len"`
}

type LinkProofVector struct {
	SignerPublicKeyHex string `json:"signer_public_key_hex"`
	LinkIDHex          string `json:"link_id_hex"`
	X25519PubHex       string `json:"x25519_pub_hex"`
	Ed25519PubHex      string `json:"ed25519_pub_hex"`
	SignallingHex      string `json:"signalling_hex"`
	SignedDataHex      string `json:"signed_data_hex"`
	SignatureHex       string `json:"signature_hex"`
	ProofPayloadHex    string `json:"proof_payload_hex"`
}

type ResourceAdvertisementVector struct {
	PackedHex       string `json:"packed_hex"`
	TransferSize    int64  `json:"transfer_size"`
	DataSize        int64  `json:"data_size"`
	Parts           int    `json:"parts"`
	HashHex         string `json:"hash_hex"`
	RandomHashHex   string `json:"random_hash_hex"`
	OriginalHashHex string `json:"original_hash_hex"`
	SegmentIndex    int    `json:"segment_index"`
	TotalSegments   int    `json:"total_segments"`
	RequestIDHex    string `json:"request_id_hex"`
	Flags           int    `json:"flags"`
	Encrypted       bool   `json:"encrypted"`
	Compressed      bool   `json:"compressed"`
	Split           bool   `json:"split"`
	IsRequest       bool   `json:"is_request"`
	IsResponse      bool   `json:"is_response"`
	HasMetadata     bool   `json:"has_metadata"`
	HashmapHex      string `json:"hashmap_hex"`
}

type ChannelEnvelopeVector struct {
	EnvelopeHex string `json:"envelope_hex"`
	MsgType     int    `json:"msgtype"`
	Sequence    int    `json:"sequence"`
	Length      int    `json:"length"`
	DataHex     string `json:"data_hex"`
}

type BufferStreamVector struct {
	PackedHex  string `json:"packed_hex"`
	StreamID   int    `json:"stream_id"`
	EOF        bool   `json:"eof"`
	Compressed bool   `json:"compressed"`
	DataHex    string `json:"data_hex"`
}

type ResourceHashVector struct {
	DataHex          string `json:"data_hex"`
	RandomHashHex    string `json:"random_hash_hex"`
	ResourceHashHex  string `json:"resource_hash_hex"`
	TruncatedHashHex string `json:"truncated_hash_hex"`
	ProofHashHex     string `json:"proof_hash_hex"`
	MapHashHex       string `json:"map_hash_hex"`
	MapHashLen       int    `json:"maphash_len"`
}

type LinkEncryptionVector struct {
	DerivedKeyHex string `json:"derived_key_hex"`
	PlaintextHex  string `json:"plaintext_hex"`
	IVHex         string `json:"iv_hex"`
	CiphertextHex string `json:"ciphertext_hex"`
	HMACHex       string `json:"hmac_hex"`
	TokenHex      string `json:"token_hex"`
}

type PathRequestVector struct {
	DataHex        string `json:"data_hex"`
	DestHashHex    string `json:"dest_hash_hex"`
	RequestorIDHex string `json:"requestor_id_hex"`
	TagHex         string `json:"tag_hex"`
}

type ReceiptProofVector struct {
	PacketHashHex string `json:"packet_hash_hex"`
	SignatureHex  string `json:"signature_hex"`
	ProofHex      string `json:"proof_hex"`
	PublicKeyHex  string `json:"public_key_hex"`
	ExplLength    int    `json:"expl_length"`
}

type LRProofPacketVector struct {
	RawHex          string `json:"raw_hex"`
	LinkIDHex       string `json:"link_id_hex"`
	Context         int    `json:"context"`
	ProofPayloadHex string `json:"proof_payload_hex"`
	HeaderType      int    `json:"header_type"`
	PacketType      int    `json:"packet_type"`
	DestinationType int    `json:"destination_type"`
}

type PythonPacketVector struct {
	RawHex      string `json:"raw_hex"`
	PacketType  int    `json:"packet_type"`
	HeaderType  int    `json:"header_type"`
	DestHashHex string `json:"dest_hash_hex"`
	DataHex     string `json:"data_hex"`
	DataLen     int    `json:"data_len"`
}

type ResourceContextVector struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type ResourceMetadataPrefixVector struct {
	MetadataHex  string `json:"metadata_hex"`
	PrefixHex    string `json:"prefix_hex"`
	FullHex      string `json:"full_hex"`
	MetadataSize int    `json:"metadata_size"`
}

type BufferCompressedVector struct {
	PackedHex       string `json:"packed_hex"`
	StreamID        int    `json:"stream_id"`
	Compressed      bool   `json:"compressed"`
	EOF             bool   `json:"eof"`
	OriginalDataHex string `json:"original_data_hex"`
	CompressedHex   string `json:"compressed_hex"`
}

func loadVectors(t *testing.T) *TestVectors {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	path := filepath.Join(dir, "test_vectors.json")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("Skipping: test_vectors.json not found (run generate_vectors.py or task test-crossref)")
	}

	var v TestVectors
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("Failed to parse test vectors: %v", err)
	}

	if v.FormatVersion < 1 || v.FormatVersion > 5 {
		t.Fatalf("Unsupported test vector format version: %d", v.FormatVersion)
	}

	return &v
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	if s == "" {
		return nil
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("Invalid hex string %q: %v", s, err)
	}
	return b
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case uint8:
		return int64(x)
	case uint16:
		return int64(x)
	case uint32:
		return int64(x)
	case uint64:
		return int64(x)
	default:
		return 0
	}
}

func TestIdentityKeyDerivation(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.Identity {
		t.Run(fmt.Sprintf("identity_%d", i), func(t *testing.T) {
			privKeyBytes := mustHex(t, vec.PrivateKeyHex)

			id, err := identity.FromBytes(privKeyBytes)
			if err != nil {
				t.Fatalf("FromBytes failed: %v", err)
			}

			gotPub := hex.EncodeToString(id.GetPublicKey())
			if gotPub != vec.PublicKeyHex {
				t.Errorf("Public key mismatch:\n  got:  %s\n  want: %s", gotPub, vec.PublicKeyHex)
			}

			gotHash := hex.EncodeToString(id.Hash())
			if gotHash != vec.HashHex {
				t.Errorf("Identity hash mismatch:\n  got:  %s\n  want: %s", gotHash, vec.HashHex)
			}

			if id.GetHexHash() != vec.Hexhash {
				t.Errorf("Hex hash mismatch:\n  got:  %s\n  want: %s", id.GetHexHash(), vec.Hexhash)
			}
		})
	}
}

func TestIdentitySignature(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.Identity {
		t.Run(fmt.Sprintf("sign_%d", i), func(t *testing.T) {
			privKeyBytes := mustHex(t, vec.PrivateKeyHex)
			id, err := identity.FromBytes(privKeyBytes)
			if err != nil {
				t.Fatalf("FromBytes failed: %v", err)
			}

			message := mustHex(t, vec.SignMessageHex)
			expectedSig := mustHex(t, vec.SignatureHex)

			sig, err := id.Sign(message)
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if !bytes.Equal(sig, expectedSig) {
				t.Errorf("Signature mismatch:\n  got:  %x\n  want: %x", sig, expectedSig)
			}
		})
	}
}

func TestIdentityVerifyCrossImpl(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.Identity {
		t.Run(fmt.Sprintf("verify_%d", i), func(t *testing.T) {
			pubKeyBytes := mustHex(t, vec.PublicKeyHex)
			id := identity.FromPublicKey(pubKeyBytes)
			if id == nil {
				t.Fatal("FromPublicKey returned nil")
			}

			message := mustHex(t, vec.SignMessageHex)
			signature := mustHex(t, vec.SignatureHex)

			if !id.Verify(message, signature) {
				t.Error("Failed to verify Python-generated signature with Go identity")
			}
		})
	}
}

func TestDestinationHash(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.DestinationHash {
		t.Run(fmt.Sprintf("dest_%d_%s", i, vec.AppName), func(t *testing.T) {
			// Reconstruct the expand name: app_name[.aspect1[.aspect2...]]
			var expandName strings.Builder
			expandName.WriteString(vec.AppName)
			for _, aspect := range vec.Aspects {
				expandName.WriteString("." + aspect)
			}

			if expandName.String() != vec.ExpandName {
				t.Errorf("Expand name mismatch:\n  got:  %s\n  want: %s", expandName.String(), vec.ExpandName)
			}

			// Compute name_hash_10
			nameHashFull := sha256.Sum256([]byte(expandName.String()))
			nameHash10 := nameHashFull[:10]
			if hex.EncodeToString(nameHash10) != vec.NameHash10Hex {
				t.Errorf("Name hash (10 bytes) mismatch:\n  got:  %x\n  want: %s", nameHash10, vec.NameHash10Hex)
			}

			identityHash := mustHex(t, vec.IdentityHashHex)

			// Single destination hash = SHA256(nameHash10 + identityHash)[:16]
			combined := append(nameHash10, identityHash...)
			singleFull := sha256.Sum256(combined)
			singleHash := singleFull[:16]
			if hex.EncodeToString(singleHash) != vec.SingleDestHashHex {
				t.Errorf("Single dest hash mismatch:\n  got:  %x\n  want: %s", singleHash, vec.SingleDestHashHex)
			}

			// Plain destination hash = SHA256(nameHash10)[:16]
			plainFull := sha256.Sum256(nameHash10)
			plainHash := plainFull[:16]
			if hex.EncodeToString(plainHash) != vec.PlainDestHashHex {
				t.Errorf("Plain dest hash mismatch:\n  got:  %x\n  want: %s", plainHash, vec.PlainDestHashHex)
			}
		})
	}
}

func TestHKDF(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.HKDF {
		t.Run(fmt.Sprintf("hkdf_%d", i), func(t *testing.T) {
			secret := mustHex(t, vec.SecretHex)
			salt := mustHex(t, vec.SaltHex)

			var ctx []byte
			if vec.ContextHex != "" {
				ctx = mustHex(t, vec.ContextHex)
			}

			expected := mustHex(t, vec.DerivedHex)

			derived, err := cryptography.DeriveKey(secret, salt, ctx, vec.Length)
			if err != nil {
				t.Fatalf("DeriveKey failed: %v", err)
			}

			if !bytes.Equal(derived, expected) {
				t.Errorf("HKDF output mismatch:\n  got:  %x\n  want: %x", derived, expected)
			}
		})
	}
}

func TestHMAC(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.HMAC {
		t.Run(fmt.Sprintf("hmac_%d", i), func(t *testing.T) {
			key := mustHex(t, vec.KeyHex)
			message := mustHex(t, vec.MessageHex)
			expected := mustHex(t, vec.HMACHex)

			got := cryptography.ComputeHMAC(key, message)
			if !bytes.Equal(got, expected) {
				t.Errorf("HMAC mismatch:\n  got:  %x\n  want: %x", got, expected)
			}
		})
	}
}

func TestHash(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.Hash {
		t.Run(fmt.Sprintf("hash_%d", i), func(t *testing.T) {
			input := mustHex(t, vec.InputHex)
			expectedFull := mustHex(t, vec.FullHashHex)
			expectedTrunc := mustHex(t, vec.TruncatedHashHex)

			gotFull := cryptography.Hash(input)
			if !bytes.Equal(gotFull, expectedFull) {
				t.Errorf("Full hash mismatch:\n  got:  %x\n  want: %x", gotFull, expectedFull)
			}

			gotTrunc := identity.TruncatedHash(input)
			if !bytes.Equal(gotTrunc, expectedTrunc) {
				t.Errorf("Truncated hash mismatch:\n  got:  %x\n  want: %x", gotTrunc, expectedTrunc)
			}
		})
	}
}

func TestTokenStructure(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.Token {
		t.Run(fmt.Sprintf("token_%d", i), func(t *testing.T) {
			token := mustHex(t, vec.TokenHex)

			if len(token) < vec.TokenOverhead {
				t.Fatalf("Token too short: %d bytes, expected at least %d", len(token), vec.TokenOverhead)
			}

			iv := token[:16]
			ct := token[16 : len(token)-32]
			mac := token[len(token)-32:]

			if hex.EncodeToString(iv) != vec.IVHex {
				t.Errorf("IV mismatch:\n  got:  %x\n  want: %s", iv, vec.IVHex)
			}
			if hex.EncodeToString(ct) != vec.CiphertextHex {
				t.Errorf("Ciphertext mismatch:\n  got:  %x\n  want: %s", ct, vec.CiphertextHex)
			}
			if hex.EncodeToString(mac) != vec.HMACHex {
				t.Errorf("HMAC mismatch:\n  got:  %x\n  want: %s", mac, vec.HMACHex)
			}

			// Verify token can be decrypted in Go using the key
			key := mustHex(t, vec.KeyHex)
			signingKey := key[:32]
			encryptionKey := key[32:64]

			if !cryptography.ValidateHMAC(signingKey, token[:len(token)-32], mac) {
				t.Error("HMAC validation failed on Python-generated token")
			}

			plaintext, err := cryptography.DecryptAES256CBC(encryptionKey, token[0:len(token)-32])
			if err != nil {
				t.Fatalf("AES decryption failed: %v", err)
			}

			expectedPlaintext := mustHex(t, vec.PlaintextHex)
			if !bytes.Equal(plaintext, expectedPlaintext) {
				t.Errorf("Token decryption mismatch:\n  got:  %x\n  want: %x", plaintext, expectedPlaintext)
			}
		})
	}
}

func TestPacketHeaderFlags(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.PacketHeader {
		t.Run(fmt.Sprintf("header_%d", i), func(t *testing.T) {
			// Encode flags the Go way
			flags := byte(0)
			flags |= byte(vec.HeaderType<<6) & packet.HeaderMaskHeaderType
			flags |= byte(vec.ContextFlag<<5) & packet.HeaderMaskContextFlag
			flags |= byte(vec.TransportType<<4) & packet.HeaderMaskTransportType
			flags |= byte(vec.DestinationType<<2) & packet.HeaderMaskDestinationType
			flags |= byte(vec.PacketType) & packet.HeaderMaskPacketType

			if int(flags) != vec.FlagsByte {
				t.Errorf("Flags byte mismatch: got 0x%02x (%08b), want 0x%02x (%08b)",
					flags, flags, vec.FlagsByte, vec.FlagsByte)
			}

			// Decode flags back
			gotHT := int((flags & packet.HeaderMaskHeaderType) >> 6)
			gotCF := int((flags & packet.HeaderMaskContextFlag) >> 5)
			gotTT := int((flags & packet.HeaderMaskTransportType) >> 4)
			gotDT := int((flags & packet.HeaderMaskDestinationType) >> 2)
			gotPT := int(flags & packet.HeaderMaskPacketType)

			if gotHT != vec.HeaderType {
				t.Errorf("Decoded header_type: got %d, want %d", gotHT, vec.HeaderType)
			}
			if gotCF != vec.ContextFlag {
				t.Errorf("Decoded context_flag: got %d, want %d", gotCF, vec.ContextFlag)
			}
			if gotTT != vec.TransportType {
				t.Errorf("Decoded transport_type: got %d, want %d", gotTT, vec.TransportType)
			}
			if gotDT != vec.DestinationType {
				t.Errorf("Decoded destination_type: got %d, want %d", gotDT, vec.DestinationType)
			}
			if gotPT != vec.PacketType {
				t.Errorf("Decoded packet_type: got %d, want %d", gotPT, vec.PacketType)
			}
		})
	}
}

func TestAnnounceSignatureVerification(t *testing.T) {
	v := loadVectors(t)

	privKeyBytes := mustHex(t, v.Identity[0].PrivateKeyHex)
	id, err := identity.FromBytes(privKeyBytes)
	if err != nil {
		t.Fatalf("FromBytes failed: %v", err)
	}

	pubID := identity.FromPublicKey(id.GetPublicKey())

	for i, vec := range v.Announce {
		t.Run(fmt.Sprintf("announce_%d_ratchet_%v", i, vec.HasRatchet), func(t *testing.T) {
			signedData := mustHex(t, vec.SignedDataHex)
			signature := mustHex(t, vec.SignatureHex)

			if !pubID.Verify(signedData, signature) {
				t.Error("Failed to verify Python-generated announce signature")
			}

			// Verify signed_data structure
			destHash := mustHex(t, vec.DestHashHex)
			pubKey := mustHex(t, vec.PublicKeyHex)
			nameHash := mustHex(t, vec.NameHashHex)
			randomHash := mustHex(t, vec.RandomHashHex)
			appData := mustHex(t, vec.AppDataHex)

			var reconstructed []byte
			reconstructed = append(reconstructed, destHash...)
			reconstructed = append(reconstructed, pubKey...)
			reconstructed = append(reconstructed, nameHash...)
			reconstructed = append(reconstructed, randomHash...)
			if vec.HasRatchet {
				ratchet := mustHex(t, vec.RatchetHex)
				reconstructed = append(reconstructed, ratchet...)
			}
			reconstructed = append(reconstructed, appData...)

			if !bytes.Equal(reconstructed, signedData) {
				t.Errorf("Signed data reconstruction mismatch:\n  got:  %x\n  want: %x", reconstructed, signedData)
			}

			// Verify announce payload layout
			payload := mustHex(t, vec.PayloadHex)
			offset := 0

			payloadPubKey := payload[offset : offset+64]
			offset += 64
			if !bytes.Equal(payloadPubKey, pubKey) {
				t.Error("Payload public key mismatch")
			}

			payloadNameHash := payload[offset : offset+10]
			offset += 10
			if !bytes.Equal(payloadNameHash, nameHash) {
				t.Error("Payload name hash mismatch")
			}

			payloadRandomHash := payload[offset : offset+10]
			offset += 10
			if !bytes.Equal(payloadRandomHash, randomHash) {
				t.Error("Payload random hash mismatch")
			}

			if vec.HasRatchet {
				ratchet := mustHex(t, vec.RatchetHex)
				payloadRatchet := payload[offset : offset+32]
				offset += 32
				if !bytes.Equal(payloadRatchet, ratchet) {
					t.Error("Payload ratchet mismatch")
				}
			}

			payloadSig := payload[offset : offset+64]
			offset += 64
			if !bytes.Equal(payloadSig, signature) {
				t.Error("Payload signature mismatch")
			}

			payloadAppData := payload[offset:]
			if !bytes.Equal(payloadAppData, appData) {
				t.Error("Payload app data mismatch")
			}
		})
	}
}

func TestEncryptionCrossImpl(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.Encryption {
		t.Run(fmt.Sprintf("decrypt_%d", i), func(t *testing.T) {
			privKeyBytes := mustHex(t, vec.PrivateKeyHex)
			id, err := identity.FromBytes(privKeyBytes)
			if err != nil {
				t.Fatalf("FromBytes failed: %v", err)
			}

			ciphertext := mustHex(t, vec.CiphertextHex)
			expectedPlaintext := mustHex(t, vec.PlaintextHex)

			plaintext, err := id.Decrypt(ciphertext, nil, false, nil)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			if !bytes.Equal(plaintext, expectedPlaintext) {
				t.Errorf("Decryption mismatch:\n  got len: %d\n  want len: %d", len(plaintext), len(expectedPlaintext))
			}
		})
	}
}

func TestPacketPackUnpackRoundtrip(t *testing.T) {
	destHash := make([]byte, 16)
	for i := range destHash {
		destHash[i] = byte(i)
	}
	payload := []byte("cross-implementation test payload")

	p := packet.NewPacket(
		packet.DestinationSingle,
		payload,
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		false,
		packet.FlagUnset,
	)
	p.DestinationHash = destHash

	if err := p.Pack(); err != nil {
		t.Fatalf("Pack failed: %v", err)
	}

	p2 := &packet.Packet{Raw: make([]byte, len(p.Raw))}
	copy(p2.Raw, p.Raw)

	if err := p2.Unpack(); err != nil {
		t.Fatalf("Unpack failed: %v", err)
	}

	if p2.HeaderType != packet.HeaderType1 {
		t.Errorf("HeaderType: got %d, want %d", p2.HeaderType, packet.HeaderType1)
	}
	if p2.PacketType != packet.PacketTypeData {
		t.Errorf("PacketType: got %d, want %d", p2.PacketType, packet.PacketTypeData)
	}
	if p2.DestinationType != packet.DestinationSingle {
		t.Errorf("DestinationType: got %d, want %d", p2.DestinationType, packet.DestinationSingle)
	}
	if !bytes.Equal(p2.DestinationHash, destHash) {
		t.Errorf("DestinationHash mismatch:\n  got:  %x\n  want: %x", p2.DestinationHash, destHash)
	}
	if !bytes.Equal(p2.Data, payload) {
		t.Errorf("Payload mismatch:\n  got:  %q\n  want: %q", p2.Data, payload)
	}
}

func TestECDH(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.ECDH {
		t.Run(fmt.Sprintf("ecdh_%d", i), func(t *testing.T) {
			privA := mustHex(t, vec.PrivateAHex)
			pubB := mustHex(t, vec.PublicBHex)
			privB := mustHex(t, vec.PrivateBHex)
			pubA := mustHex(t, vec.PublicAHex)
			expected := mustHex(t, vec.SharedSecretHex)

			sharedAB, err := cryptography.DeriveSharedSecret(privA, pubB)
			if err != nil {
				t.Fatalf("DeriveSharedSecret(A,B) failed: %v", err)
			}
			if !bytes.Equal(sharedAB, expected) {
				t.Errorf("Shared secret A->B mismatch:\n  got:  %x\n  want: %x", sharedAB, expected)
			}

			sharedBA, err := cryptography.DeriveSharedSecret(privB, pubA)
			if err != nil {
				t.Fatalf("DeriveSharedSecret(B,A) failed: %v", err)
			}
			if !bytes.Equal(sharedBA, expected) {
				t.Errorf("Shared secret B->A mismatch:\n  got:  %x\n  want: %x", sharedBA, expected)
			}
		})
	}
}

func TestRatchetID(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.RatchetID {
		t.Run(fmt.Sprintf("ratchet_id_%d", i), func(t *testing.T) {
			ratchetPub := mustHex(t, vec.RatchetPublicHex)
			expectedID := mustHex(t, vec.RatchetIDHex)

			h := sha256.Sum256(ratchetPub)
			gotID := h[:10]

			if !bytes.Equal(gotID, expectedID) {
				t.Errorf("Ratchet ID mismatch:\n  got:  %x\n  want: %x", gotID, expectedID)
			}

			// Also verify that private key derives to the expected public key
			ratchetPriv := mustHex(t, vec.RatchetPrivateHex)
			derivedPub, err := cryptography.DeriveSharedSecret(ratchetPriv, cryptography.GetBasepoint())
			if err != nil {
				t.Fatalf("Failed to derive public key: %v", err)
			}
			if !bytes.Equal(derivedPub, ratchetPub) {
				t.Errorf("Ratchet public key mismatch:\n  got:  %x\n  want: %x", derivedPub, ratchetPub)
			}
		})
	}
}

func TestPacketWireFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.PacketWire {
		t.Run(fmt.Sprintf("wire_%d", i), func(t *testing.T) {
			raw := mustHex(t, vec.RawHex)

			p := &packet.Packet{Raw: make([]byte, len(raw))}
			copy(p.Raw, raw)

			if err := p.Unpack(); err != nil {
				t.Fatalf("Unpack failed: %v", err)
			}

			if int(p.HeaderType) != vec.HeaderType {
				t.Errorf("HeaderType: got %d, want %d", p.HeaderType, vec.HeaderType)
			}
			if int(p.PacketType) != vec.PacketType {
				t.Errorf("PacketType: got %d, want %d", p.PacketType, vec.PacketType)
			}
			if int(p.TransportType) != vec.TransportType {
				t.Errorf("TransportType: got %d, want %d", p.TransportType, vec.TransportType)
			}
			if int(p.DestinationType) != vec.DestinationType {
				t.Errorf("DestinationType: got %d, want %d", p.DestinationType, vec.DestinationType)
			}
			if int(p.ContextFlag) != vec.ContextFlag {
				t.Errorf("ContextFlag: got %d, want %d", p.ContextFlag, vec.ContextFlag)
			}
			if int(p.Context) != vec.Context {
				t.Errorf("Context: got 0x%02x, want 0x%02x", p.Context, vec.Context)
			}
			if int(p.Hops) != vec.Hops {
				t.Errorf("Hops: got %d, want %d", p.Hops, vec.Hops)
			}

			expectedDest := mustHex(t, vec.DestHashHex)
			if !bytes.Equal(p.DestinationHash, expectedDest) {
				t.Errorf("DestinationHash:\n  got:  %x\n  want: %x", p.DestinationHash, expectedDest)
			}

			if vec.TransportIDHex != "" {
				expectedTID := mustHex(t, vec.TransportIDHex)
				if !bytes.Equal(p.TransportID, expectedTID) {
					t.Errorf("TransportID:\n  got:  %x\n  want: %x", p.TransportID, expectedTID)
				}
			}

			expectedData := mustHex(t, vec.DataHex)
			if !bytes.Equal(p.Data, expectedData) {
				t.Errorf("Data:\n  got:  %x\n  want: %x", p.Data, expectedData)
			}
		})
	}
}

func TestPacketHashCrossImpl(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.PacketHash {
		t.Run(fmt.Sprintf("pkt_hash_%d", i), func(t *testing.T) {
			raw := mustHex(t, vec.RawHex)

			p := &packet.Packet{Raw: make([]byte, len(raw))}
			copy(p.Raw, raw)

			if err := p.Unpack(); err != nil {
				t.Fatalf("Unpack failed: %v", err)
			}

			expectedHash := mustHex(t, vec.PacketHashHex)
			gotHash := p.GetHash()
			if !bytes.Equal(gotHash, expectedHash) {
				t.Errorf("Packet hash mismatch:\n  got:  %x\n  want: %x", gotHash, expectedHash)
			}

			expectedTrunc := mustHex(t, vec.TruncatedHashHex)
			gotTrunc := p.TruncatedHash()
			if !bytes.Equal(gotTrunc, expectedTrunc) {
				t.Errorf("Truncated packet hash mismatch:\n  got:  %x\n  want: %x", gotTrunc, expectedTrunc)
			}
		})
	}
}

func TestAESCrossImpl(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.AES {
		t.Run(fmt.Sprintf("aes_%d", i), func(t *testing.T) {
			key := mustHex(t, vec.KeyHex)
			iv := mustHex(t, vec.IVHex)
			expectedCT := mustHex(t, vec.CiphertextHex)
			expectedPlaintext := mustHex(t, vec.PlaintextHex)

			ivPlusCT := append(iv, expectedCT...)
			plaintext, err := cryptography.DecryptAES256CBC(key, ivPlusCT)
			if err != nil {
				t.Fatalf("DecryptAES256CBC failed: %v", err)
			}

			if !bytes.Equal(plaintext, expectedPlaintext) {
				t.Errorf("AES decryption mismatch:\n  got:  %x\n  want: %x", plaintext, expectedPlaintext)
			}
		})
	}
}

func TestCrossSign(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.CrossSign {
		t.Run(fmt.Sprintf("cross_sign_%d", i), func(t *testing.T) {
			pubKeyBytes := mustHex(t, vec.SignerPublicKeyHex)
			id := identity.FromPublicKey(pubKeyBytes)
			if id == nil {
				t.Fatal("FromPublicKey returned nil")
			}

			data := mustHex(t, vec.DataHex)
			signature := mustHex(t, vec.SignatureHex)

			if !id.Verify(data, signature) {
				t.Errorf("Failed to verify signature from identity %d on data of length %d",
					vec.SignerIndex, len(data))
			}

			// Verify wrong data does not pass
			if len(data) > 0 {
				tampered := make([]byte, len(data))
				copy(tampered, data)
				tampered[0] ^= 0xFF
				if id.Verify(tampered, signature) {
					t.Error("Tampered data should not verify")
				}
			}
		})
	}
}

func TestIdentityFileFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.IdentityFile {
		t.Run(fmt.Sprintf("identity_file_%d", i), func(t *testing.T) {
			fileBytes := mustHex(t, vec.FileBytesHex)

			if len(fileBytes) != 64 {
				t.Fatalf("Identity file should be 64 bytes, got %d", len(fileBytes))
			}

			id, err := identity.FromBytes(fileBytes)
			if err != nil {
				t.Fatalf("FromBytes failed: %v", err)
			}

			gotPub := hex.EncodeToString(id.GetPublicKey())
			if gotPub != vec.PublicKeyHex {
				t.Errorf("Public key mismatch:\n  got:  %s\n  want: %s", gotPub, vec.PublicKeyHex)
			}

			gotHash := hex.EncodeToString(id.Hash())
			if gotHash != vec.HashHex {
				t.Errorf("Hash mismatch:\n  got:  %s\n  want: %s", gotHash, vec.HashHex)
			}

			gotPriv, err := id.GetPrivateKey()
			if err != nil {
				t.Fatalf("GetPrivateKey: %v", err)
			}
			if !bytes.Equal(gotPriv, fileBytes) {
				t.Errorf("Private key roundtrip mismatch:\n  got:  %x\n  want: %x", gotPriv, fileBytes)
			}

			// Write to temp file, read back, verify same identity
			tmpFile := filepath.Join(t.TempDir(), "test_identity")
			if err := id.ToFile(tmpFile); err != nil {
				t.Fatalf("ToFile failed: %v", err)
			}

			id2, err := identity.FromFile(tmpFile)
			if err != nil {
				t.Fatalf("FromFile failed: %v", err)
			}

			if !bytes.Equal(id2.GetPublicKey(), id.GetPublicKey()) {
				t.Error("Public key mismatch after file roundtrip")
			}
			if !bytes.Equal(id2.Hash(), id.Hash()) {
				t.Error("Hash mismatch after file roundtrip")
			}
		})
	}
}

func TestGoEncryptPythonStructure(t *testing.T) {
	v := loadVectors(t)

	if len(v.Identity) == 0 {
		t.Skip("No identity vectors")
	}

	privKeyBytes := mustHex(t, v.Identity[0].PrivateKeyHex)
	id, err := identity.FromBytes(privKeyBytes)
	if err != nil {
		t.Fatalf("FromBytes failed: %v", err)
	}

	testData := [][]byte{
		[]byte("hello"),
		[]byte(""),
		bytes.Repeat([]byte{0x42}, 200),
		[]byte("cross-implementation roundtrip test"),
	}

	for i, data := range testData {
		t.Run(fmt.Sprintf("go_encrypt_%d", i), func(t *testing.T) {
			ciphertext, err := id.Encrypt(data, nil)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			// Token structure: [ephemeral_pub:32][iv:16][aes_ciphertext][hmac:32]
			if len(ciphertext) < 32+16+16+32 {
				t.Fatalf("Ciphertext too short: %d bytes", len(ciphertext))
			}

			ephemeralPub := ciphertext[:32]
			if len(ephemeralPub) != 32 {
				t.Error("Ephemeral public key should be 32 bytes")
			}

			hmacPart := ciphertext[len(ciphertext)-32:]
			if len(hmacPart) != 32 {
				t.Error("HMAC should be 32 bytes")
			}

			// Self-decrypt should work
			plaintext, err := id.Decrypt(ciphertext, nil, false, nil)
			if err != nil {
				t.Fatalf("Self-decrypt failed: %v", err)
			}

			if !bytes.Equal(plaintext, data) {
				t.Errorf("Self-decrypt mismatch:\n  got len: %d\n  want len: %d", len(plaintext), len(data))
			}
		})
	}
}

func TestLinkSignalling(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.LinkSignalling {
		t.Run(fmt.Sprintf("signalling_%d", i), func(t *testing.T) {
			expected := mustHex(t, vec.SignallingHex)

			// Encode signalling bytes the Go way
			mtu := vec.MTU
			mode := byte(vec.Mode)
			sig := make([]byte, 3)
			sig[0] = byte((mtu >> 16) & 0xFF)
			sig[1] = byte((mtu >> 8) & 0xFF)
			sig[2] = byte(mtu & 0xFF)
			sig[0] |= mode << 5

			if !bytes.Equal(sig, expected) {
				t.Errorf("Signalling bytes mismatch:\n  got:  %x\n  want: %x", sig, expected)
			}

			// Decode back
			decodedMTU := ((int(sig[0]) << 16) + (int(sig[1]) << 8) + int(sig[2])) & 0x1FFFFF
			decodedMode := int((sig[0] & 0xE0) >> 5)

			if decodedMTU != vec.DecodedMTU {
				t.Errorf("Decoded MTU: got %d, want %d", decodedMTU, vec.DecodedMTU)
			}
			if decodedMode != vec.DecodedMode {
				t.Errorf("Decoded mode: got %d, want %d", decodedMode, vec.DecodedMode)
			}
		})
	}
}

func TestLinkKeyDerivation(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.LinkKeyDerivation {
		t.Run(fmt.Sprintf("link_kdf_%d", i), func(t *testing.T) {
			initiatorPriv := mustHex(t, vec.InitiatorPrvHex)
			responderPub := mustHex(t, vec.ResponderPubHex)
			expectedShared := mustHex(t, vec.SharedKeyHex)
			linkID := mustHex(t, vec.LinkIDHex)
			expectedDerived := mustHex(t, vec.DerivedKeyHex)
			expectedHMAC := mustHex(t, vec.HMACKeyHex)
			expectedSession := mustHex(t, vec.SessionKeyHex)

			sharedKey, err := cryptography.DeriveSharedSecret(initiatorPriv, responderPub)
			if err != nil {
				t.Fatalf("DeriveSharedSecret failed: %v", err)
			}
			if !bytes.Equal(sharedKey, expectedShared) {
				t.Errorf("Shared key mismatch:\n  got:  %x\n  want: %x", sharedKey, expectedShared)
			}

			derivedKeyLen := len(expectedDerived)
			derivedKey, err := cryptography.DeriveKey(sharedKey, linkID, nil, derivedKeyLen)
			if err != nil {
				t.Fatalf("DeriveKey failed: %v", err)
			}
			if !bytes.Equal(derivedKey, expectedDerived) {
				t.Errorf("Derived key mismatch:\n  got:  %x\n  want: %x", derivedKey, expectedDerived)
			}

			var hmacKey, sessionKey []byte
			if derivedKeyLen == 64 {
				hmacKey = derivedKey[:32]
				sessionKey = derivedKey[32:64]
			} else {
				hmacKey = derivedKey[:16]
				sessionKey = derivedKey[16:32]
			}

			if !bytes.Equal(hmacKey, expectedHMAC) {
				t.Errorf("HMAC key mismatch:\n  got:  %x\n  want: %x", hmacKey, expectedHMAC)
			}
			if !bytes.Equal(sessionKey, expectedSession) {
				t.Errorf("Session key mismatch:\n  got:  %x\n  want: %x", sessionKey, expectedSession)
			}
		})
	}
}

func TestLinkRequestFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.LinkRequest {
		t.Run(fmt.Sprintf("link_req_%d", i), func(t *testing.T) {
			payload := mustHex(t, vec.PayloadHex)

			if len(payload) != vec.PayloadLen {
				t.Fatalf("Payload length: got %d, want %d", len(payload), vec.PayloadLen)
			}

			x25519Pub := payload[:32]
			ed25519Pub := payload[32:64]
			signalling := payload[64:67]

			expectedX := mustHex(t, vec.X25519PubHex)
			expectedEd := mustHex(t, vec.Ed25519PubHex)
			expectedSig := mustHex(t, vec.SignallingHex)

			if !bytes.Equal(x25519Pub, expectedX) {
				t.Error("X25519 public key mismatch in link request payload")
			}
			if !bytes.Equal(ed25519Pub, expectedEd) {
				t.Error("Ed25519 public key mismatch in link request payload")
			}
			if !bytes.Equal(signalling, expectedSig) {
				t.Error("Signalling bytes mismatch in link request payload")
			}

			if vec.ECPubSize != 64 {
				t.Errorf("ECPubSize: got %d, want 64", vec.ECPubSize)
			}
		})
	}
}

func TestLinkProofFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.LinkProof {
		t.Run(fmt.Sprintf("link_proof_%d", i), func(t *testing.T) {
			signerPubKey := mustHex(t, vec.SignerPublicKeyHex)
			signer := identity.FromPublicKey(signerPubKey)
			if signer == nil {
				t.Fatal("FromPublicKey returned nil")
			}

			signedData := mustHex(t, vec.SignedDataHex)
			signature := mustHex(t, vec.SignatureHex)

			if !signer.Verify(signedData, signature) {
				t.Error("Failed to verify Python-generated link proof signature")
			}

			// Verify signed data structure: linkID + pub + sigPub + signalling
			linkID := mustHex(t, vec.LinkIDHex)
			x25519Pub := mustHex(t, vec.X25519PubHex)
			ed25519Pub := mustHex(t, vec.Ed25519PubHex)
			signalling := mustHex(t, vec.SignallingHex)

			var reconstructed []byte
			reconstructed = append(reconstructed, linkID...)
			reconstructed = append(reconstructed, x25519Pub...)
			reconstructed = append(reconstructed, ed25519Pub...)
			reconstructed = append(reconstructed, signalling...)

			if !bytes.Equal(reconstructed, signedData) {
				t.Error("Link proof signed data reconstruction mismatch")
			}

			// Verify proof payload structure: signature(64) + pub(32) + signalling(3)
			proofPayload := mustHex(t, vec.ProofPayloadHex)
			if len(proofPayload) != 64+32+3 {
				t.Fatalf("Proof payload length: got %d, want 99", len(proofPayload))
			}

			proofSig := proofPayload[:64]
			proofPub := proofPayload[64:96]
			proofSignalling := proofPayload[96:99]

			if !bytes.Equal(proofSig, signature) {
				t.Error("Proof payload signature mismatch")
			}
			if !bytes.Equal(proofPub, x25519Pub) {
				t.Error("Proof payload public key mismatch")
			}
			if !bytes.Equal(proofSignalling, signalling) {
				t.Error("Proof payload signalling mismatch")
			}
		})
	}
}

func TestResourceAdvertisementCrossImpl(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.ResourceAdvertisement {
		t.Run(fmt.Sprintf("resource_adv_%d", i), func(t *testing.T) {
			flags := byte(vec.Flags)
			encrypted := (flags & 0x01) == 0x01
			compressed := ((flags >> 1) & 0x01) == 0x01
			split := ((flags >> 2) & 0x01) == 0x01
			isRequest := ((flags >> 3) & 0x01) == 0x01
			isResponse := ((flags >> 4) & 0x01) == 0x01
			hasMetadata := ((flags >> 5) & 0x01) == 0x01

			if encrypted != vec.Encrypted {
				t.Errorf("Encrypted flag: got %v, want %v", encrypted, vec.Encrypted)
			}
			if compressed != vec.Compressed {
				t.Errorf("Compressed flag: got %v, want %v", compressed, vec.Compressed)
			}
			if split != vec.Split {
				t.Errorf("Split flag: got %v, want %v", split, vec.Split)
			}
			if isRequest != vec.IsRequest {
				t.Errorf("IsRequest flag: got %v, want %v", isRequest, vec.IsRequest)
			}
			if isResponse != vec.IsResponse {
				t.Errorf("IsResponse flag: got %v, want %v", isResponse, vec.IsResponse)
			}
			if hasMetadata != vec.HasMetadata {
				t.Errorf("HasMetadata flag: got %v, want %v", hasMetadata, vec.HasMetadata)
			}

			adv := &resource.ResourceAdvertisement{
				TransferSize:  vec.TransferSize,
				DataSize:      vec.DataSize,
				Parts:         vec.Parts,
				Hash:          mustHex(t, vec.HashHex),
				RandomHash:    mustHex(t, vec.RandomHashHex),
				OriginalHash:  mustHex(t, vec.OriginalHashHex),
				SegmentIndex:  uint16(vec.SegmentIndex),
				TotalSegments: uint16(vec.TotalSegments),
				RequestID:     mustHex(t, vec.RequestIDHex),
				Flags:         flags,
				Hashmap:       mustHex(t, vec.HashmapHex),
				Encrypted:     vec.Encrypted,
				Compressed:    vec.Compressed,
				Split:         vec.Split,
				IsRequest:     vec.IsRequest,
				IsResponse:    vec.IsResponse,
				HasMetadata:   vec.HasMetadata,
			}

			packed, err := adv.Pack(0, 384)
			if err != nil {
				t.Fatalf("Pack failed: %v", err)
			}

			unpacked, err := resource.UnpackResourceAdvertisement(packed)
			if err != nil {
				t.Fatalf("Unpack roundtrip failed: %v", err)
			}

			if unpacked.TransferSize != vec.TransferSize {
				t.Errorf("Roundtrip TransferSize: got %d, want %d", unpacked.TransferSize, vec.TransferSize)
			}
			if unpacked.DataSize != vec.DataSize {
				t.Errorf("Roundtrip DataSize: got %d, want %d", unpacked.DataSize, vec.DataSize)
			}
			if unpacked.Parts != vec.Parts {
				t.Errorf("Roundtrip Parts: got %d, want %d", unpacked.Parts, vec.Parts)
			}
			if int(unpacked.Flags) != vec.Flags {
				t.Errorf("Roundtrip Flags: got 0x%02x, want 0x%02x", unpacked.Flags, vec.Flags)
			}
			if unpacked.Encrypted != vec.Encrypted {
				t.Errorf("Roundtrip Encrypted: got %v, want %v", unpacked.Encrypted, vec.Encrypted)
			}
			if unpacked.Compressed != vec.Compressed {
				t.Errorf("Roundtrip Compressed: got %v, want %v", unpacked.Compressed, vec.Compressed)
			}
			if unpacked.Split != vec.Split {
				t.Errorf("Roundtrip Split: got %v, want %v", unpacked.Split, vec.Split)
			}
			if unpacked.IsRequest != vec.IsRequest {
				t.Errorf("Roundtrip IsRequest: got %v, want %v", unpacked.IsRequest, vec.IsRequest)
			}
			if unpacked.IsResponse != vec.IsResponse {
				t.Errorf("Roundtrip IsResponse: got %v, want %v", unpacked.IsResponse, vec.IsResponse)
			}
			if unpacked.HasMetadata != vec.HasMetadata {
				t.Errorf("Roundtrip HasMetadata: got %v, want %v", unpacked.HasMetadata, vec.HasMetadata)
			}
			if !bytes.Equal(unpacked.Hash, adv.Hash) {
				t.Error("Roundtrip Hash mismatch")
			}
			if !bytes.Equal(unpacked.RandomHash, adv.RandomHash) {
				t.Error("Roundtrip RandomHash mismatch")
			}
		})
	}
}

func TestChannelEnvelopeCrossImpl(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.ChannelEnvelope {
		t.Run(fmt.Sprintf("channel_%d", i), func(t *testing.T) {
			envelope := mustHex(t, vec.EnvelopeHex)

			if len(envelope) < channel.ChannelHeaderSize {
				t.Fatalf("Envelope too short: %d", len(envelope))
			}

			msgType := uint16(envelope[0])<<uint16(channel.ChannelHeaderBits) | uint16(envelope[1])
			sequence := uint16(envelope[2])<<uint16(channel.ChannelHeaderBits) | uint16(envelope[3])
			length := uint16(envelope[4])<<uint16(channel.ChannelHeaderBits) | uint16(envelope[5])

			if int(msgType) != vec.MsgType {
				t.Errorf("MsgType: got 0x%04x, want 0x%04x", msgType, vec.MsgType)
			}
			if int(sequence) != vec.Sequence {
				t.Errorf("Sequence: got %d, want %d", sequence, vec.Sequence)
			}
			if int(length) != vec.Length {
				t.Errorf("Length: got %d, want %d", length, vec.Length)
			}

			data := envelope[channel.ChannelHeaderSize:]
			expectedData := mustHex(t, vec.DataHex)
			if !bytes.Equal(data, expectedData) {
				t.Errorf("Data mismatch:\n  got:  %x\n  want: %x", data, expectedData)
			}

			if channel.SeqMax != 0xFFFF {
				t.Errorf("SeqMax: got 0x%04x, want 0xFFFF", channel.SeqMax)
			}
		})
	}
}

func TestBufferStreamCrossImpl(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.BufferStream {
		t.Run(fmt.Sprintf("buffer_%d", i), func(t *testing.T) {
			packed := mustHex(t, vec.PackedHex)

			msg := &buffer.StreamDataMessage{}
			if err := msg.Unpack(packed); err != nil {
				t.Fatalf("Unpack failed: %v", err)
			}

			if int(msg.StreamID) != vec.StreamID {
				t.Errorf("StreamID: got %d, want %d", msg.StreamID, vec.StreamID)
			}
			if msg.EOF != vec.EOF {
				t.Errorf("EOF: got %v, want %v", msg.EOF, vec.EOF)
			}
			if msg.Compressed != vec.Compressed {
				t.Errorf("Compressed: got %v, want %v", msg.Compressed, vec.Compressed)
			}

			expectedData := mustHex(t, vec.DataHex)
			if !bytes.Equal(msg.Data, expectedData) {
				t.Errorf("Data mismatch:\n  got:  %x\n  want: %x", msg.Data, expectedData)
			}

			// Roundtrip: Pack and compare
			repacked, err := msg.Pack()
			if err != nil {
				t.Fatalf("Pack failed: %v", err)
			}
			if !bytes.Equal(repacked, packed) {
				t.Errorf("Pack roundtrip mismatch:\n  got:  %x\n  want: %x", repacked, packed)
			}
		})
	}
}

func TestResourceHashCrossImpl(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.ResourceHash {
		t.Run(fmt.Sprintf("resource_hash_%d", i), func(t *testing.T) {
			data := mustHex(t, vec.DataHex)
			randomHash := mustHex(t, vec.RandomHashHex)

			// resource_hash = SHA256(data + random_hash)
			h := sha256.New()
			h.Write(data)
			h.Write(randomHash)
			resourceHash := h.Sum(nil)

			expectedHash := mustHex(t, vec.ResourceHashHex)
			if !bytes.Equal(resourceHash, expectedHash) {
				t.Errorf("Resource hash mismatch:\n  got:  %x\n  want: %x", resourceHash, expectedHash)
			}

			// truncated_hash = resource_hash[:16]
			truncatedHash := resourceHash[:16]
			expectedTrunc := mustHex(t, vec.TruncatedHashHex)
			if !bytes.Equal(truncatedHash, expectedTrunc) {
				t.Errorf("Truncated hash mismatch:\n  got:  %x\n  want: %x", truncatedHash, expectedTrunc)
			}

			// proof_hash = SHA256(data + resource_hash)
			hp := sha256.New()
			hp.Write(data)
			hp.Write(resourceHash)
			proofHash := hp.Sum(nil)

			expectedProof := mustHex(t, vec.ProofHashHex)
			if !bytes.Equal(proofHash, expectedProof) {
				t.Errorf("Proof hash mismatch:\n  got:  %x\n  want: %x", proofHash, expectedProof)
			}

			// map_hash = SHA256(data[:384] + random_hash)[:4]
			mapData := data
			if len(mapData) > 384 {
				mapData = mapData[:384]
			}
			hm := sha256.New()
			hm.Write(mapData)
			hm.Write(randomHash)
			mapHash := hm.Sum(nil)[:vec.MapHashLen]

			expectedMap := mustHex(t, vec.MapHashHex)
			if !bytes.Equal(mapHash, expectedMap) {
				t.Errorf("Map hash mismatch:\n  got:  %x\n  want: %x", mapHash, expectedMap)
			}
		})
	}
}

func TestLinkEncryptionCrossImpl(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.LinkEncryption {
		t.Run(fmt.Sprintf("link_enc_%d", i), func(t *testing.T) {
			derivedKey := mustHex(t, vec.DerivedKeyHex)
			signingKey := derivedKey[:32]
			encryptionKey := derivedKey[32:64]

			token := mustHex(t, vec.TokenHex)
			expectedPlaintext := mustHex(t, vec.PlaintextHex)

			// Token structure: [iv:16][ciphertext][hmac:32]
			if len(token) < 16+16+32 {
				t.Fatalf("Token too short: %d bytes", len(token))
			}

			tokenPayload := token[:len(token)-32]
			mac := token[len(token)-32:]

			// Verify HMAC
			if !cryptography.ValidateHMAC(signingKey, tokenPayload, mac) {
				t.Error("HMAC validation failed on link encryption token")
			}

			// Decrypt
			plaintext, err := cryptography.DecryptAES256CBC(encryptionKey, tokenPayload)
			if err != nil {
				t.Fatalf("AES decryption failed: %v", err)
			}

			if !bytes.Equal(plaintext, expectedPlaintext) {
				t.Errorf("Link decryption mismatch:\n  got:  %x\n  want: %x", plaintext, expectedPlaintext)
			}
		})
	}
}

func TestLinkConstants(t *testing.T) {
	if channel.WindowInitial != 2 {
		t.Errorf("Channel WindowInitial: got %d, want 2", channel.WindowInitial)
	}
	if channel.WindowMaxSlow != 5 {
		t.Errorf("Channel WindowMaxSlow: got %d, want 5", channel.WindowMaxSlow)
	}
	if channel.WindowMaxMedium != 12 {
		t.Errorf("Channel WindowMaxMedium: got %d, want 12", channel.WindowMaxMedium)
	}
	if channel.WindowMaxFast != 48 {
		t.Errorf("Channel WindowMaxFast: got %d, want 48", channel.WindowMaxFast)
	}
	if channel.SeqMax != 0xFFFF {
		t.Errorf("Channel SeqMax: got 0x%04x, want 0xFFFF", channel.SeqMax)
	}
	if channel.ChannelHeaderSize != 6 {
		t.Errorf("Channel ChannelHeaderSize: got %d, want 6", channel.ChannelHeaderSize)
	}
	if buffer.StreamIDMax != 0x3FFF {
		t.Errorf("Buffer StreamIDMax: got 0x%04x, want 0x3FFF", buffer.StreamIDMax)
	}
	if buffer.StreamHeaderEOF != 0x8000 {
		t.Errorf("Buffer StreamHeaderEOF: got 0x%04x, want 0x8000", buffer.StreamHeaderEOF)
	}
	if buffer.StreamHeaderCompressed != 0x4000 {
		t.Errorf("Buffer StreamHeaderCompressed: got 0x%04x, want 0x4000", buffer.StreamHeaderCompressed)
	}
	if resource.MapHashLen != 4 {
		t.Errorf("Resource MapHashLen: got %d, want 4", resource.MapHashLen)
	}
	if resource.RandomHashSize != 4 {
		t.Errorf("Resource RandomHashSize: got %d, want 4", resource.RandomHashSize)
	}
}

func TestPythonPackedResourceAdvertisementUnpack(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.ResourceAdvertisement {
		t.Run(fmt.Sprintf("python_packed_%d", i), func(t *testing.T) {
			packed := mustHex(t, vec.PackedHex)
			adv, err := resource.UnpackResourceAdvertisement(packed)
			if err != nil {
				t.Fatalf("UnpackResourceAdvertisement(Python packed) failed: %v", err)
			}
			if adv.TransferSize != vec.TransferSize {
				t.Errorf("TransferSize: got %d, want %d", adv.TransferSize, vec.TransferSize)
			}
			if adv.DataSize != vec.DataSize {
				t.Errorf("DataSize: got %d, want %d", adv.DataSize, vec.DataSize)
			}
			if adv.Parts != vec.Parts {
				t.Errorf("Parts: got %d, want %d", adv.Parts, vec.Parts)
			}
			if int(adv.Flags) != vec.Flags {
				t.Errorf("Flags: got 0x%02x, want 0x%02x", adv.Flags, vec.Flags)
			}
		})
	}
}

func TestPathRequestFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.PathRequest {
		t.Run(fmt.Sprintf("path_req_%d", i), func(t *testing.T) {
			data := mustHex(t, vec.DataHex)
			expectedDest := mustHex(t, vec.DestHashHex)

			if len(data) < 16 {
				t.Fatalf("Path request data too short: %d", len(data))
			}
			destHash := data[:16]
			if !bytes.Equal(destHash, expectedDest) {
				t.Errorf("Dest hash mismatch:\n  got:  %x\n  want: %x", destHash, expectedDest)
			}

			var requestorID, tag []byte
			if vec.RequestorIDHex != "" {
				requestorID = mustHex(t, vec.RequestorIDHex)
				if len(data) >= 32 {
					gotReq := data[16:32]
					if !bytes.Equal(gotReq, requestorID) {
						t.Error("Requestor ID mismatch")
					}
				}
			}
			if vec.TagHex != "" {
				tag = mustHex(t, vec.TagHex)
				if len(data) > 32 {
					gotTag := data[32:]
					if !bytes.Equal(gotTag, tag) {
						t.Error("Tag mismatch")
					}
				} else if len(data) > 16 {
					gotTag := data[16:]
					if !bytes.Equal(gotTag, tag) {
						t.Error("Tag mismatch (no requestor)")
					}
				}
			}
		})
	}
}

func TestReceiptProofFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.ReceiptProof {
		t.Run(fmt.Sprintf("receipt_proof_%d", i), func(t *testing.T) {
			proof := mustHex(t, vec.ProofHex)
			expectedHash := mustHex(t, vec.PacketHashHex)
			expectedSig := mustHex(t, vec.SignatureHex)
			pubKey := mustHex(t, vec.PublicKeyHex)

			if len(proof) != vec.ExplLength {
				t.Errorf("Proof length: got %d, want %d", len(proof), vec.ExplLength)
			}
			proofHash := proof[:32]
			signature := proof[32:96]
			if !bytes.Equal(proofHash, expectedHash) {
				t.Error("Proof hash mismatch")
			}
			if !bytes.Equal(signature, expectedSig) {
				t.Error("Signature mismatch")
			}

			id := identity.FromPublicKey(pubKey)
			if id == nil {
				t.Fatal("FromPublicKey failed")
			}
			if !id.Verify(expectedHash, signature) {
				t.Error("Go failed to verify Python-generated receipt proof signature")
			}
		})
	}
}

func TestLRProofPacketFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.LRProofPacket {
		t.Run(fmt.Sprintf("lrproof_%d", i), func(t *testing.T) {
			raw := mustHex(t, vec.RawHex)
			pkt := &packet.Packet{Raw: raw}
			if err := pkt.Unpack(); err != nil {
				t.Fatalf("Unpack failed: %v", err)
			}

			if int(pkt.HeaderType) != vec.HeaderType {
				t.Errorf("HeaderType: got %d, want %d", pkt.HeaderType, vec.HeaderType)
			}
			if int(pkt.PacketType) != vec.PacketType {
				t.Errorf("PacketType: got %d, want %d", pkt.PacketType, vec.PacketType)
			}
			if int(pkt.DestinationType) != vec.DestinationType {
				t.Errorf("DestinationType: got %d, want %d", pkt.DestinationType, vec.DestinationType)
			}
			if int(pkt.Context) != vec.Context {
				t.Errorf("Context: got 0x%02x, want 0x%02x", pkt.Context, vec.Context)
			}

			expectedLinkID := mustHex(t, vec.LinkIDHex)
			if !bytes.Equal(pkt.DestinationHash, expectedLinkID) {
				t.Errorf("Link ID (dest hash) mismatch:\n  got:  %x\n  want: %x", pkt.DestinationHash, expectedLinkID)
			}

			expectedPayload := mustHex(t, vec.ProofPayloadHex)
			if !bytes.Equal(pkt.Data, expectedPayload) {
				t.Errorf("Proof payload mismatch:\n  got:  %x\n  want: %x", pkt.Data, expectedPayload)
			}
		})
	}
}

func TestPythonPacketUnpack(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.PythonPacket {
		t.Run(fmt.Sprintf("python_pkt_%d", i), func(t *testing.T) {
			raw := mustHex(t, vec.RawHex)
			pkt := &packet.Packet{Raw: raw}
			if err := pkt.Unpack(); err != nil {
				t.Fatalf("Unpack(Python packet) failed: %v", err)
			}

			if int(pkt.PacketType) != vec.PacketType {
				t.Errorf("PacketType: got %d, want %d", pkt.PacketType, vec.PacketType)
			}
			if int(pkt.HeaderType) != vec.HeaderType {
				t.Errorf("HeaderType: got %d, want %d", pkt.HeaderType, vec.HeaderType)
			}

			expectedDest := mustHex(t, vec.DestHashHex)
			if !bytes.Equal(pkt.DestinationHash, expectedDest) {
				t.Errorf("Dest hash mismatch:\n  got:  %x\n  want: %x", pkt.DestinationHash, expectedDest)
			}

			if vec.DataHex != "" {
				expectedData := mustHex(t, vec.DataHex)
				if !bytes.Equal(pkt.Data, expectedData) {
					t.Errorf("Data mismatch:\n  got:  %x\n  want: %x", pkt.Data, expectedData)
				}
			}
			if vec.DataLen > 0 && len(pkt.Data) != vec.DataLen {
				t.Errorf("Data length: got %d, want %d", len(pkt.Data), vec.DataLen)
			}
		})
	}
}

func TestResourceContextConstants(t *testing.T) {
	v := loadVectors(t)

	expected := map[string]int{
		"RESOURCE":     int(packet.ContextResource),
		"RESOURCE_ADV": int(packet.ContextResourceAdv),
		"RESOURCE_REQ": int(packet.ContextResourceReq),
		"RESOURCE_HMU": int(packet.ContextResourceHMU),
		"RESOURCE_PRF": int(packet.ContextResourcePRF),
		"RESOURCE_ICL": int(packet.ContextResourceICL),
		"RESOURCE_RCL": int(packet.ContextResourceRCL),
		"LRPROOF":      int(packet.ContextLRProof),
	}

	for _, vec := range v.ResourceContext {
		want, ok := expected[vec.Name]
		if !ok {
			t.Errorf("Unknown context name: %s", vec.Name)
			continue
		}
		if vec.Value != want {
			t.Errorf("%s: got 0x%02x, want 0x%02x", vec.Name, vec.Value, want)
		}
	}
}

func TestResourceMetadataPrefix(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.ResourceMetadataPrefix {
		t.Run(fmt.Sprintf("metadata_%d", i), func(t *testing.T) {
			full := mustHex(t, vec.FullHex)
			prefix := mustHex(t, vec.PrefixHex)

			if len(prefix) != 3 {
				t.Errorf("Prefix length: got %d, want 3", len(prefix))
			}
			metadataSize := int(prefix[0])<<16 | int(prefix[1])<<8 | int(prefix[2])
			if metadataSize != vec.MetadataSize {
				t.Errorf("Metadata size from prefix: got %d, want %d", metadataSize, vec.MetadataSize)
			}

			expectedMetadata := mustHex(t, vec.MetadataHex)
			metadata := full[3:]
			if !bytes.Equal(metadata, expectedMetadata) {
				t.Errorf("Metadata mismatch:\n  got:  %x\n  want: %x", metadata, expectedMetadata)
			}
		})
	}
}

func TestBufferCompressed(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.BufferCompressed {
		t.Run(fmt.Sprintf("compressed_%d", i), func(t *testing.T) {
			packed := mustHex(t, vec.PackedHex)

			msg := &buffer.StreamDataMessage{}
			if err := msg.Unpack(packed); err != nil {
				t.Fatalf("Unpack failed: %v", err)
			}

			if int(msg.StreamID) != vec.StreamID {
				t.Errorf("StreamID: got %d, want %d", msg.StreamID, vec.StreamID)
			}
			if msg.Compressed != vec.Compressed {
				t.Errorf("Compressed: got %v, want %v", msg.Compressed, vec.Compressed)
			}
			if msg.EOF != vec.EOF {
				t.Errorf("EOF: got %v, want %v", msg.EOF, vec.EOF)
			}

			expectedCompressed := mustHex(t, vec.CompressedHex)
			if !bytes.Equal(msg.Data, expectedCompressed) {
				t.Errorf("Compressed data mismatch:\n  got:  %x\n  want: %x", msg.Data, expectedCompressed)
			}

			// Decompress and verify
			rd := bzip2.NewReader(bytes.NewReader(msg.Data))
			decompressed, err := io.ReadAll(rd)
			if err != nil {
				t.Fatalf("Decompress failed: %v", err)
			}
			expectedOriginal := mustHex(t, vec.OriginalDataHex)
			if !bytes.Equal(decompressed, expectedOriginal) {
				t.Errorf("Decompressed data mismatch:\n  got:  %x\n  want: %x", decompressed, expectedOriginal)
			}
		})
	}
}

func TestProtocolConstants(t *testing.T) {
	if packet.MTU != 500 {
		t.Errorf("MTU: got %d, want 500", packet.MTU)
	}
	if packet.TruncatedHashLength != 16 {
		t.Errorf("TruncatedHashLength: got %d, want 16", packet.TruncatedHashLength)
	}
	if packet.PlainMDU != 464 {
		t.Errorf("PlainMDU: got %d, want 464", packet.PlainMDU)
	}
	if packet.EncryptedMDU != 383 {
		t.Errorf("EncryptedMDU: got %d, want 383", packet.EncryptedMDU)
	}
	if packet.PacketTypeData != 0x00 {
		t.Errorf("PacketTypeData: got 0x%02x, want 0x00", packet.PacketTypeData)
	}
	if packet.PacketTypeAnnounce != 0x01 {
		t.Errorf("PacketTypeAnnounce: got 0x%02x, want 0x01", packet.PacketTypeAnnounce)
	}
	if packet.PacketTypeLinkReq != 0x02 {
		t.Errorf("PacketTypeLinkReq: got 0x%02x, want 0x02", packet.PacketTypeLinkReq)
	}
	if packet.PacketTypeProof != 0x03 {
		t.Errorf("PacketTypeProof: got 0x%02x, want 0x03", packet.PacketTypeProof)
	}
	if packet.ContextNone != 0x00 {
		t.Errorf("ContextNone: got 0x%02x, want 0x00", packet.ContextNone)
	}
	if packet.ContextPathResponse != 0x0B {
		t.Errorf("ContextPathResponse: got 0x%02x, want 0x0B", packet.ContextPathResponse)
	}
	if packet.ContextChannel != 0x0E {
		t.Errorf("ContextChannel: got 0x%02x, want 0x0E", packet.ContextChannel)
	}
	if packet.ContextLinkIdentify != 0xFB {
		t.Errorf("ContextLinkIdentify: got 0x%02x, want 0xFB", packet.ContextLinkIdentify)
	}
	if packet.ContextLinkClose != 0xFC {
		t.Errorf("ContextLinkClose: got 0x%02x, want 0xFC", packet.ContextLinkClose)
	}
	if packet.ContextLinkProof != 0xFD {
		t.Errorf("ContextLinkProof: got 0x%02x, want 0xFD", packet.ContextLinkProof)
	}
	if packet.ContextLRRTT != 0xFE {
		t.Errorf("ContextLRRTT: got 0x%02x, want 0xFE", packet.ContextLRRTT)
	}
	if packet.ContextLRProof != 0xFF {
		t.Errorf("ContextLRProof: got 0x%02x, want 0xFF", packet.ContextLRProof)
	}
}

func TestResourceReqFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.ResourceReq {
		t.Run(fmt.Sprintf("resource_req_%d", i), func(t *testing.T) {
			data := mustHex(t, vec.DataHex)
			expectedHash := mustHex(t, vec.ResourceHashHex)

			if len(data) < 1+32 {
				t.Fatalf("Resource REQ data too short: %d", len(data))
			}
			hmuPart := data[0]
			if vec.HashmapExhausted && hmuPart != 0xFF {
				t.Errorf("HMU part: got 0x%02x, want 0xFF (HASHMAP_IS_EXHAUSTED)", hmuPart)
			}
			if !vec.HashmapExhausted && hmuPart != 0x00 {
				t.Errorf("HMU part: got 0x%02x, want 0x00", hmuPart)
			}

			var hashStart, hashEnd int
			if vec.HashmapExhausted {
				if len(data) < 1+4+32 {
					t.Fatalf("Exhausted REQ too short: %d", len(data))
				}
				hashStart = 1 + 4
				hashEnd = hashStart + 32
			} else {
				hashStart = 1
				hashEnd = 1 + 32
			}
			resourceHash := data[hashStart:hashEnd]
			if !bytes.Equal(resourceHash, expectedHash) {
				t.Errorf("Resource hash mismatch:\n  got:  %x\n  want: %x", resourceHash, expectedHash)
			}
		})
	}
}

func TestResourceHMUFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.ResourceHMU {
		t.Run(fmt.Sprintf("resource_hmu_%d", i), func(t *testing.T) {
			data := mustHex(t, vec.DataHex)
			expectedHash := mustHex(t, vec.ResourceHashHex)
			expectedHashmap := mustHex(t, vec.HashmapHex)

			if len(data) < 32 {
				t.Fatalf("Resource HMU data too short: %d", len(data))
			}
			resourceHash := data[:32]
			msgpackPart := data[32:]

			if !bytes.Equal(resourceHash, expectedHash) {
				t.Errorf("Resource hash mismatch:\n  got:  %x\n  want: %x", resourceHash, expectedHash)
			}

			var unpacked []any
			if err := msgpack.Unmarshal(msgpackPart, &unpacked); err != nil {
				t.Fatalf("msgpack unpack failed: %v", err)
			}
			if len(unpacked) != 2 {
				t.Fatalf("HMU array length: got %d, want 2", len(unpacked))
			}
			seg := toInt64(unpacked[0])
			if int(seg) != vec.Segment {
				t.Errorf("Segment: got %d, want %d", seg, vec.Segment)
			}
			hashmap, ok := unpacked[1].([]byte)
			if !ok {
				t.Fatalf("Hashmap type: %T", unpacked[1])
			}
			if !bytes.Equal(hashmap, expectedHashmap) {
				t.Errorf("Hashmap mismatch:\n  got:  %x\n  want: %x", hashmap, expectedHashmap)
			}
		})
	}
}

func TestResourcePRFFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.ResourcePRF {
		t.Run(fmt.Sprintf("resource_prf_%d", i), func(t *testing.T) {
			data := mustHex(t, vec.DataHex)
			expectedHash := mustHex(t, vec.ResourceHashHex)
			expectedProof := mustHex(t, vec.ProofHex)

			h := sha256.New()
			h.Write(data)
			h.Write(expectedHash)
			computedProof := h.Sum(nil)

			if !bytes.Equal(computedProof, expectedProof) {
				t.Errorf("Proof computation mismatch:\n  got:  %x\n  want: %x", computedProof, expectedProof)
			}

			proofData := mustHex(t, vec.ProofDataHex)
			if len(proofData) != 64 {
				t.Fatalf("Proof data length: got %d, want 64", len(proofData))
			}
			proofHash := proofData[:32]
			proof := proofData[32:64]
			if !bytes.Equal(proofHash, expectedHash) {
				t.Error("Proof data hash mismatch")
			}
			if !bytes.Equal(proof, expectedProof) {
				t.Error("Proof data proof mismatch")
			}
		})
	}
}

func TestResourceICLRCLFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.ResourceICLRCL {
		t.Run(fmt.Sprintf("resource_icl_rcl_%d", i), func(t *testing.T) {
			payload := mustHex(t, vec.PayloadHex)
			expectedHash := mustHex(t, vec.ResourceHashHex)

			if len(payload) != 32 {
				t.Errorf("ICL/RCL payload length: got %d, want 32", len(payload))
			}
			if !bytes.Equal(payload, expectedHash) {
				t.Errorf("Payload mismatch:\n  got:  %x\n  want: %x", payload, expectedHash)
			}
		})
	}
}

func TestLRRTTFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.LRRTT {
		t.Run(fmt.Sprintf("lrrtt_%d", i), func(t *testing.T) {
			payload := mustHex(t, vec.PayloadHex)

			var rtt float64
			if err := msgpack.Unmarshal(payload, &rtt); err != nil {
				t.Fatalf("msgpack unpack failed: %v", err)
			}
			if rtt != vec.RTT {
				t.Errorf("RTT: got %v, want %v", rtt, vec.RTT)
			}
		})
	}
}

func TestDestinationTypeConstants(t *testing.T) {
	v := loadVectors(t)

	expected := map[string]int{
		"SINGLE": int(packet.DestinationSingle),
		"GROUP":  int(packet.DestinationGroup),
		"PLAIN":  int(packet.DestinationPlain),
		"LINK":   int(packet.DestinationLink),
	}

	for _, vec := range v.DestinationType {
		want, ok := expected[vec.Name]
		if !ok {
			t.Errorf("Unknown destination type: %s", vec.Name)
			continue
		}
		if vec.Value != want {
			t.Errorf("%s: got %d, want %d", vec.Name, vec.Value, want)
		}
	}
}

func TestCacheRequestFormat(t *testing.T) {
	v := loadVectors(t)

	for i, vec := range v.CacheRequest {
		t.Run(fmt.Sprintf("cache_request_%d", i), func(t *testing.T) {
			payload := mustHex(t, vec.PayloadHex)
			expectedHash := mustHex(t, vec.PacketHashHex)

			if len(payload) != 32 {
				t.Errorf("Cache request payload length: got %d, want 32", len(payload))
			}
			if !bytes.Equal(payload, expectedHash) {
				t.Errorf("Payload mismatch:\n  got:  %x\n  want: %x", payload, expectedHash)
			}
			if vec.Context != int(packet.ContextCacheReq) {
				t.Errorf("Context: got 0x%02x, want 0x%02x", vec.Context, packet.ContextCacheReq)
			}
		})
	}
}
