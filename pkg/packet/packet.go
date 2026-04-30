// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package packet

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
)

// Packet represents a Reticulum packet with header, destination, context and payload.
type Packet struct {
	HeaderType    byte
	PacketType    byte
	TransportType byte
	Context       byte
	ContextFlag   byte
	Hops          byte

	DestinationType byte
	DestinationHash []byte
	Destination     any
	TransportID     []byte
	Data            []byte

	Raw           []byte
	Packed        bool
	Sent          bool
	CreateReceipt bool
	FromPacked    bool

	SentAt     time.Time
	PacketHash []byte
	RatchetID  []byte

	RSSI *float64
	SNR  *float64
	Q    *float64

	Addresses []byte
	Link      any
}

// PacketConfig holds the parameters used to create a new packet.
type PacketConfig struct {
	DestType      byte
	Data          []byte
	PacketType    byte
	Context       byte
	TransportType byte
	HeaderType    byte
	TransportID   []byte
	CreateReceipt bool
	ContextFlag   byte
}

// NewPacket builds a new Packet from the given config fields.
func NewPacket(destType byte, data []byte, packetType byte, context byte,
	transportType byte, headerType byte, transportID []byte, createReceipt bool,
	contextFlag byte) *Packet {

	return &Packet{
		HeaderType:      headerType,
		PacketType:      packetType,
		TransportType:   transportType,
		Context:         context,
		ContextFlag:     contextFlag,
		Hops:            0,
		DestinationType: destType,
		Data:            data,
		TransportID:     transportID,
		CreateReceipt:   createReceipt,
		Packed:          false,
		Sent:            false,
		FromPacked:      false,
	}
}

func (p *Packet) Pack() error {
	if p.Packed {
		return nil
	}

	debug.Log(debug.DebugPackets, "Packing packet", "type", p.PacketType, "header", p.HeaderType)

	flags := byte(0)
	flags |= (p.HeaderType << 6) & HeaderMaskHeaderType
	flags |= (p.ContextFlag << 5) & HeaderMaskContextFlag
	flags |= (p.TransportType << 4) & HeaderMaskTransportType
	flags |= (p.DestinationType << 2) & HeaderMaskDestinationType
	flags |= p.PacketType & HeaderMaskPacketType

	header := []byte{flags, p.Hops}
	debug.Log(debug.DebugTrace, "Created packet header", "flags", fmt.Sprintf("%08b", flags), "hops", p.Hops)

	if p.HeaderType == HeaderType2 {
		if p.TransportID == nil {
			return errors.New("transport ID required for header type 2")
		}
		header = append(header, p.TransportID...)
		debug.Log(debug.DebugAll, "Added transport ID to header", "transport_id", fmt.Sprintf("%x", p.TransportID))
	}

	header = append(header, p.DestinationHash...)

	header = append(header, p.Context)
	debug.Log(debug.DebugPackets, "Final header length", "bytes", len(header))

	p.Raw = append(header, p.Data...)
	debug.Log(debug.DebugTrace, "Final packet size", "bytes", len(p.Raw))

	if len(p.Raw) > MTU {
		return errors.New("packet size exceeds MTU")
	}

	p.Packed = true
	p.updateHash()
	debug.Log(debug.DebugAll, "Packet hash", "hash", fmt.Sprintf("%x", p.PacketHash))
	return nil
}

func (p *Packet) Unpack() error {
	if len(p.Raw) < MinPacketSize {
		return errors.New("packet too short")
	}

	flags := p.Raw[0]
	p.Hops = p.Raw[1]

	p.HeaderType = (flags & HeaderMaskHeaderType) >> 6
	p.ContextFlag = (flags & HeaderMaskContextFlag) >> 5
	p.TransportType = (flags & HeaderMaskTransportType) >> 4
	p.DestinationType = (flags & HeaderMaskDestinationType) >> 2
	p.PacketType = flags & HeaderMaskPacketType

	dstLen := TruncatedHashLength

	if p.HeaderType == HeaderType2 {
		if len(p.Raw) < 2*dstLen+MinPacketSize {
			return errors.New("packet too short for header type 2")
		}
		p.TransportID = p.Raw[2 : dstLen+2]
		p.DestinationHash = p.Raw[dstLen+2 : 2*dstLen+2]
		p.Context = p.Raw[2*dstLen+2]
		p.Data = p.Raw[2*dstLen+3:]
	} else {
		if len(p.Raw) < dstLen+MinPacketSize {
			return errors.New("packet too short for header type 1")
		}
		p.TransportID = nil
		p.DestinationHash = p.Raw[2 : dstLen+2]
		p.Context = p.Raw[dstLen+2]
		p.Data = p.Raw[dstLen+3:]
	}

	p.Packed = false
	p.updateHash()
	return nil
}

func (p *Packet) GetHash() []byte {
	hashable := p.getHashablePart()
	hash := sha256.Sum256(hashable)
	return hash[:]
}

func (p *Packet) getHashablePart() []byte {
	hashable := []byte{p.Raw[0] & HashableFlagsMask}
	if p.HeaderType == HeaderType2 {
		startIndex := TruncatedHashLength + 2
		if len(p.Raw) > startIndex {
			hashable = append(hashable, p.Raw[startIndex:]...)
		}
	} else {
		if len(p.Raw) > 2 {
			hashable = append(hashable, p.Raw[2:]...)
		}
	}
	return hashable
}

func (p *Packet) updateHash() {
	p.PacketHash = p.GetHash()
}

func (p *Packet) Hash() []byte {
	return p.GetHash()
}

func (p *Packet) TruncatedHash() []byte {
	hash := p.GetHash()
	if len(hash) >= TruncatedHashLength {
		return hash[:TruncatedHashLength]
	}
	return hash
}

func (p *Packet) Serialize() ([]byte, error) {
	if !p.Packed {
		if err := p.Pack(); err != nil {
			return nil, fmt.Errorf("failed to pack packet: %w", err)
		}
	}

	p.Addresses = p.DestinationHash

	return p.Raw, nil
}

func NewAnnouncePacket(destHash []byte, identity *identity.Identity, appData []byte, transportID []byte) (*Packet, error) {
	debug.Log(debug.DebugAll, "Creating new announce packet", "dest_hash", fmt.Sprintf("%x", destHash), "app_data", fmt.Sprintf("%x", appData))

	// Get public key separated into encryption and signing keys
	pubKey := identity.GetPublicKey()
	encKey := pubKey[:32]
	signKey := pubKey[32:]
	debug.Log(debug.DebugPackets, "Using public keys", "enc_key", fmt.Sprintf("%x", encKey), "sign_key", fmt.Sprintf("%x", signKey))

	// Parse app name from first msgpack element if possible
	// For nodes, we'll use "reticulum.node" as the name hash
	var appName string
	if len(appData) > 2 && appData[0] == 0x93 {
		// This is a node announce, use standard node name
		appName = "reticulum.node"
	} else if len(appData) > 3 && appData[0] == 0x92 && appData[1] == 0xc4 {
		// Try to extract name from peer announce appData
		nameLen := int(appData[2])
		if 3+nameLen <= len(appData) {
			appName = string(appData[3 : 3+nameLen])
		} else {
			// Default fallback
			appName = "reticulum-go.node"
		}
	} else {
		// Default fallback
		appName = "reticulum-go.node"
	}

	// Create name hash (10 bytes)
	nameHash := sha256.Sum256([]byte(appName))
	nameHash10 := nameHash[:10]
	debug.Log(debug.DebugPackets, "Using name hash", "name", appName, "hash", fmt.Sprintf("%x", nameHash10))

	// Create random hash (10 bytes) - 5 bytes random + 5 bytes time
	randomHash := make([]byte, 10)
	_, err := rand.Read(randomHash[:5]) // #nosec G104
	if err != nil {
		debug.Log(debug.DebugPackets, "Failed to read random bytes for hash", "error", err)
		return nil, err // Or handle the error appropriately
	}
	timeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timeBytes, uint64(time.Now().Unix())) // #nosec G115
	copy(randomHash[5:], timeBytes[3:8])
	debug.Log(debug.DebugPackets, "Generated random hash", "hash", fmt.Sprintf("%x", randomHash))

	// Prepare ratchet ID if available (not yet implemented)
	var ratchetID []byte

	// Prepare data for signature
	// Signature consists of destination hash, public keys, name hash, random hash, and app data
	signedData := make([]byte, 0, len(destHash)+len(encKey)+len(signKey)+len(nameHash10)+len(randomHash)+len(appData))
	signedData = append(signedData, destHash...)
	signedData = append(signedData, encKey...)
	signedData = append(signedData, signKey...)
	signedData = append(signedData, nameHash10...)
	signedData = append(signedData, randomHash...)
	signedData = append(signedData, appData...)
	debug.Log(debug.DebugTrace, "Created signed data", "bytes", len(signedData))

	signature, err := identity.Sign(signedData)
	if err != nil {
		return nil, fmt.Errorf("sign announce: %w", err)
	}
	debug.Log(debug.DebugPackets, "Generated signature", "signature", fmt.Sprintf("%x", signature))

	// Combine all fields according to spec
	// Data structure: Public Key (32) + Signing Key (32) + Name Hash (10) + Random Hash (10) + Ratchet (optional) + Signature (64) + App Data
	data := make([]byte, 0, 32+32+10+10+64+len(appData))
	data = append(data, encKey...)     // Encryption key (32 bytes)
	data = append(data, signKey...)    // Signing key (32 bytes)
	data = append(data, nameHash10...) // Name hash (10 bytes)
	data = append(data, randomHash...) // Random hash (10 bytes)
	if ratchetID != nil {
		data = append(data, ratchetID...) // Ratchet ID (32 bytes if present)
	}
	data = append(data, signature...) // Signature (64 bytes)
	data = append(data, appData...)   // Application data (variable)

	debug.Log(debug.DebugTrace, "Combined packet data", "bytes", len(data))

	// Create the packet with header type 2 (two address fields)
	p := &Packet{
		HeaderType:      HeaderType2,
		PacketType:      PacketTypeAnnounce,
		TransportID:     transportID,
		DestinationHash: destHash,
		Data:            data,
	}

	debug.Log(debug.DebugVerbose, "Created announce packet", "type", p.PacketType, "header", p.HeaderType)
	return p, nil
}
