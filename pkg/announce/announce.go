// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package announce

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/cryptography"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
)

type Announce struct {
	mutex           *sync.RWMutex
	destinationHash []byte
	destinationName string
	identity        *identity.Identity
	appData         []byte
	config          *common.ReticulumConfig
	hops            uint8
	timestamp       int64
	signature       []byte
	pathResponse    bool
	retries         int
	handlers        []Handler
	ratchetID       []byte
	packet          []byte
	hash            []byte
}

func New(dest *identity.Identity, destinationHash []byte, destinationName string, appData []byte, pathResponse bool, config *common.ReticulumConfig) (*Announce, error) {
	if dest == nil {
		return nil, errors.New("destination identity required")
	}

	if len(destinationHash) == 0 {
		return nil, errors.New("destination hash required")
	}

	if destinationName == "" {
		return nil, errors.New("destination name required")
	}

	a := &Announce{
		mutex:           &sync.RWMutex{},
		identity:        dest,
		destinationHash: destinationHash,
		destinationName: destinationName,
		appData:         appData,
		config:          config,
		hops:            0,
		timestamp:       time.Now().Unix(),
		pathResponse:    pathResponse,
		retries:         0,
		handlers:        make([]Handler, 0),
	}

	// Get current ratchet ID if enabled
	currentRatchet := dest.GetCurrentRatchetKey()
	if currentRatchet != nil {
		ratchetPub, err := cryptography.PublicKeyFromPrivate(currentRatchet)
		if err == nil {
			a.ratchetID = dest.GetRatchetID(ratchetPub)
		}
	}

	signData := append(a.destinationHash, a.appData...)
	if a.ratchetID != nil {
		signData = append(signData, a.ratchetID...)
	}
	sig, err := dest.Sign(signData)
	if err != nil {
		return nil, fmt.Errorf("sign announce: %w", err)
	}
	a.signature = sig

	return a, nil
}

func (a *Announce) Propagate(interfaces []common.NetworkInterface) error {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	debug.Log(debug.DebugTrace, "Propagating announce across interfaces", "count", len(interfaces))

	var packet []byte
	if a.packet != nil {
		debug.Log(debug.DebugTrace, "Using cached packet", "bytes", len(a.packet))
		packet = a.packet
	} else {
		debug.Log(debug.DebugTrace, "Creating new packet")
		var err error
		packet, err = a.CreatePacket()
		if err != nil {
			return err
		}
		a.packet = packet
	}

	for _, iface := range interfaces {
		if !iface.IsEnabled() {
			debug.Log(debug.DebugTrace, "Skipping disabled interface", "name", iface.GetName())
			continue
		}
		if !iface.GetBandwidthAvailable() {
			debug.Log(debug.DebugTrace, "Skipping interface with insufficient bandwidth", "name", iface.GetName())
			continue
		}

		debug.Log(debug.DebugTrace, "Sending announce on interface", "name", iface.GetName())
		if err := iface.Send(packet, ""); err != nil {
			debug.Log(debug.DebugTrace, "Failed to send on interface", "name", iface.GetName(), "error", err)
			return fmt.Errorf("failed to propagate on interface %s: %w", iface.GetName(), err)
		}
		debug.Log(debug.DebugTrace, "Successfully sent announce on interface", "name", iface.GetName())
	}

	return nil
}

func (a *Announce) RegisterHandler(handler Handler) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.handlers = append(a.handlers, handler)
}

func (a *Announce) DeregisterHandler(handler Handler) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	for i, h := range a.handlers {
		if h == handler {
			a.handlers = append(a.handlers[:i], a.handlers[i+1:]...)
			break
		}
	}
}

func (a *Announce) HandleAnnounce(data []byte) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	debug.Log(debug.DebugTrace, "Handling announce packet", "bytes", len(data))

	// Minimum packet size validation:
	// header + desthash + context + enckey + signkey + namehash +
	// randomhash + signature + 3 bytes of app data.
	if len(data) < MinAnnouncePacketSize {
		debug.Log(debug.DebugTrace, "Invalid announce data length", "bytes", len(data), "minimum", MinAnnouncePacketSize)
		return errors.New("invalid announce data length")
	}

	// Extract header and check packet type
	header := data[:HeaderSize]
	if header[0]&HeaderPacketTypeMask != PacketTypeAnnounce {
		return errors.New("not an announce packet")
	}

	// Get hop count
	hopCount := header[1]
	if hopCount > MaxHops {
		debug.Log(debug.DebugTrace, "Announce exceeded max hops", "hops", hopCount)
		return errors.New("announce exceeded maximum hop count")
	}

	// Parse the packet based on header type
	headerType := (header[0] & HeaderTypeMask) >> HeaderTypeShift
	var contextByte byte
	var packetData []byte

	const (
		destHashStart  = HeaderSize
		destHashEnd    = HeaderSize + AddrHashSize  // 18
		transportIDEnd = destHashEnd + AddrHashSize // 34
	)

	if headerType == HeaderType2 {
		// Header type 2 format: header + desthash + transportid + context + data
		if len(data) < MinHeaderType2Size {
			return errors.New("header type 2 packet too short")
		}
		destHash := data[destHashStart:destHashEnd]
		transportID := data[destHashEnd:transportIDEnd]
		contextByte = data[transportIDEnd]
		packetData = data[HeaderType2Offset:]

		debug.Log(debug.DebugTrace, "Header type 2 announce", "destHash", fmt.Sprintf("%x", destHash), "transportID", fmt.Sprintf("%x", transportID), "context", contextByte)
	} else {
		// Header type 1 format: header + desthash + context + data
		if len(data) < MinHeaderType1Size {
			return errors.New("header type 1 packet too short")
		}
		destHash := data[destHashStart:destHashEnd]
		contextByte = data[destHashEnd]
		packetData = data[HeaderType1Offset:]

		debug.Log(debug.DebugTrace, "Header type 1 announce", "destHash", fmt.Sprintf("%x", destHash), "context", contextByte)
	}

	// Now parse the data portion according to the spec:
	// Public Key + Signing Key + Name Hash + Random Hash + Ratchet + Signature + App Data
	if len(packetData) < MinAnnounceDataSize {
		return errors.New("announce data too short")
	}

	// Extract the components
	encKey := packetData[AnnounceEncKeyOffset:AnnounceSignKeyOffset]
	signKey := packetData[AnnounceSignKeyOffset:AnnounceNameHashOffset]
	nameHash := packetData[AnnounceNameHashOffset:AnnounceRandomOffset]
	randomHash := packetData[AnnounceRandomOffset:AnnounceRatchetOffset]
	ratchetData := packetData[AnnounceRatchetOffset:AnnounceSignatureOffset]
	signature := packetData[AnnounceSignatureOffset:AnnounceAppDataOffset]
	appData := packetData[AnnounceAppDataOffset:]

	debug.Log(debug.DebugTrace, "Announce fields", "encKey", fmt.Sprintf("%x", encKey), "signKey", fmt.Sprintf("%x", signKey))
	debug.Log(debug.DebugTrace, "Name and random hash", "nameHash", fmt.Sprintf("%x", nameHash), "randomHash", fmt.Sprintf("%x", randomHash))
	debug.Log(debug.DebugTrace, "Ratchet data", "ratchet", fmt.Sprintf("%x", ratchetData[:8]))
	debug.Log(debug.DebugTrace, "Signature and app data", "signature", fmt.Sprintf("%x", signature[:8]), "appDataLen", len(appData))

	// Destination hash sits in the same position for both header types.
	destHash := data[destHashStart:destHashEnd]

	// Combine public keys
	pubKey := append(encKey, signKey...)

	// Create announced identity from public keys
	announcedIdentity := identity.FromPublicKey(pubKey)
	if announcedIdentity == nil {
		return errors.New("invalid identity public key")
	}

	// Verify signature
	signedData := make([]byte, 0)
	signedData = append(signedData, destHash...)
	signedData = append(signedData, encKey...)
	signedData = append(signedData, signKey...)
	signedData = append(signedData, nameHash...)
	signedData = append(signedData, randomHash...)
	signedData = append(signedData, ratchetData...)
	signedData = append(signedData, appData...)

	if !announcedIdentity.Verify(signedData, signature) {
		return errors.New("invalid announce signature")
	}

	// Process with handlers
	for _, handler := range a.handlers {
		if handler.ReceivePathResponses() || !a.pathResponse {
			if err := handler.ReceivedAnnounce(destHash, announcedIdentity, appData, hopCount); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *Announce) RequestPath(destHash []byte, onInterface common.NetworkInterface) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	// Create path request packet
	packet := make([]byte, 0)
	packet = append(packet, destHash...)
	packet = append(packet, byte(0)) // Initial hop count

	// Send path request
	return onInterface.Send(packet, "")
}

// CreateHeader creates a Reticulum packet header according to spec
func CreateHeader(ifacFlag byte, headerType byte, contextFlag byte, propType byte, destType byte, packetType byte, hops byte) []byte {
	header := make([]byte, 2)

	// First byte: [IFAC Flag], [Header Type], [Context Flag], [Propagation Type], [Destination Type] and [Packet Type]
	header[0] = ifacFlag | (headerType << 6) | (contextFlag << 5) |
		(propType << 4) | (destType << 2) | packetType

	// Second byte: Number of hops
	header[1] = hops

	return header
}

func (a *Announce) CreatePacket() ([]byte, error) {
	// This function creates the complete announce packet according to the Reticulum specification.
	// Announce Packet Structure:
	// [Header (2 bytes)][Dest Hash (16 bytes)][Context (1 byte)][Announce Data]
	// Announce Data Structure:
	// [Public Key (64 bytes)][Name Hash (10 bytes)][Random Hash (10 bytes)][Ratchet (32 bytes optional)][Signature (64 bytes)][App Data]

	// 2. Destination Hash
	destHash := a.destinationHash
	if len(destHash) > 16 {
		destHash = destHash[:16]
	}

	// 3. Announce Data
	// 3.1 Public Key (full 64 bytes - not split into enc/sign keys in packet)
	pubKey := a.identity.GetPublicKey()
	if len(pubKey) != 64 {
		debug.Log(debug.DebugTrace, "Invalid public key length", "expected", 64, "got", len(pubKey))
	}

	// 3.2 Name Hash
	nameHash := sha256.Sum256([]byte(a.destinationName))
	nameHash10 := nameHash[:10]

	randomHash := make([]byte, 10)
	if _, err := rand.Read(randomHash[:5]); err != nil {
		debug.Log(debug.DebugError, "Failed to read random bytes for announce", "error", err)
	}
	timeBytes := make([]byte, 8)
	// #nosec G115 - Unix timestamp is always positive, no overflow risk
	binary.BigEndian.PutUint64(timeBytes, uint64(time.Now().Unix()))
	copy(randomHash[5:], timeBytes[3:8])

	// 3.4 Ratchet (only include if exists)
	var ratchetData []byte
	currentRatchetKey := a.identity.GetCurrentRatchetKey()
	if currentRatchetKey != nil {
		ratchetPub, err := cryptography.PublicKeyFromPrivate(currentRatchetKey)
		if err == nil {
			ratchetData = make([]byte, 32)
			copy(ratchetData, ratchetPub)
		}
	}

	contextFlag := byte(0)
	if len(ratchetData) > 0 {
		contextFlag = 1
	}

	// 1. Create Header - Use HeaderType1
	header := CreateHeader(
		IFACNone,
		HeaderType1,
		contextFlag,
		PropTypeBroadcast,
		DestTypeSingle,
		PacketTypeAnnounce,
		a.hops,
	)

	contextByte := byte(0)
	if a.pathResponse {
		contextByte = 0x0B
	}

	// 3.5 Signature
	// The signature is calculated over: Dest Hash + Public Key (64 bytes) + Name Hash + Random Hash + Ratchet (if exists) + App Data
	validationData := make([]byte, 0)
	validationData = append(validationData, destHash...)
	validationData = append(validationData, pubKey...)
	validationData = append(validationData, nameHash10...)
	validationData = append(validationData, randomHash...)
	if len(ratchetData) > 0 {
		validationData = append(validationData, ratchetData...)
	}
	validationData = append(validationData, a.appData...)
	signature, err := a.identity.Sign(validationData)
	if err != nil {
		return nil, fmt.Errorf("sign announce packet: %w", err)
	}

	debug.Log(debug.DebugTrace, "Creating announce packet", "destHash", fmt.Sprintf("%x", destHash), "pubKeyLen", len(pubKey), "nameHash", fmt.Sprintf("%x", nameHash10), "randomHash", fmt.Sprintf("%x", randomHash), "ratchetLen", len(ratchetData), "sigLen", len(signature), "appDataLen", len(a.appData))

	// 5. Assemble the packet (HeaderType1 format)
	packet := make([]byte, 0)
	packet = append(packet, header...)
	packet = append(packet, destHash...)
	packet = append(packet, contextByte)
	packet = append(packet, pubKey...)
	packet = append(packet, nameHash10...)
	packet = append(packet, randomHash...)
	if len(ratchetData) > 0 {
		packet = append(packet, ratchetData...)
	}
	packet = append(packet, signature...)
	packet = append(packet, a.appData...)

	debug.Log(debug.DebugTrace, "Final announce packet", "totalBytes", len(packet), "ratchetLen", len(ratchetData), "appDataLen", len(a.appData))

	return packet, nil
}

type AnnouncePacket struct {
	Data []byte
}

func NewAnnouncePacket(pubKey []byte, appData []byte, announceID []byte) *AnnouncePacket {
	packet := &AnnouncePacket{}

	// Build packet data
	packet.Data = make([]byte, 0, len(pubKey)+len(appData)+len(announceID)+4)

	// Add header
	packet.Data = append(packet.Data, PacketTypeAnnounce)
	packet.Data = append(packet.Data, AnnounceIdentity)

	// Add public key
	packet.Data = append(packet.Data, pubKey...)

	// Add app data length and content
	appDataLen := make([]byte, 2)
	binary.BigEndian.PutUint16(appDataLen, uint16(len(appData))) // #nosec G115
	packet.Data = append(packet.Data, appDataLen...)
	packet.Data = append(packet.Data, appData...)

	// Add announce ID
	packet.Data = append(packet.Data, announceID...)

	return packet
}

// NewAnnounce creates a new announce packet for a destination
func NewAnnounce(identity *identity.Identity, destinationHash []byte, appData []byte, ratchetID []byte, pathResponse bool, config *common.ReticulumConfig) (*Announce, error) {
	debug.Log(debug.DebugTrace, "Creating new announce", "destHash", fmt.Sprintf("%x", destinationHash), "appDataLen", len(appData), "hasRatchet", ratchetID != nil, "pathResponse", pathResponse)

	if identity == nil {
		debug.Log(debug.DebugError, "Nil identity provided")
		return nil, errors.New("identity cannot be nil")
	}

	if config == nil {
		return nil, errors.New("config cannot be nil")
	}

	if len(destinationHash) == 0 {
		return nil, errors.New("destination hash cannot be empty")
	}

	destHash := destinationHash
	debug.Log(debug.DebugTrace, "Using provided destination hash", "destHash", fmt.Sprintf("%x", destHash))

	a := &Announce{
		identity:        identity,
		appData:         appData,
		ratchetID:       ratchetID,
		pathResponse:    pathResponse,
		destinationHash: destHash,
		hops:            0,
		mutex:           &sync.RWMutex{},
		handlers:        make([]Handler, 0),
		config:          config,
	}

	debug.Log(debug.DebugTrace, "Created announce object", "destHash", fmt.Sprintf("%x", a.destinationHash), "hops", a.hops)

	packet, err := a.CreatePacket()
	if err != nil {
		return nil, err
	}
	a.packet = packet

	hash := a.Hash()
	debug.Log(debug.DebugTrace, "Generated announce hash", "hash", fmt.Sprintf("%x", hash))

	return a, nil
}

func (a *Announce) Hash() []byte {
	if a.hash == nil {
		// Generate hash from announce data
		h := sha256.New()
		h.Write(a.destinationHash)
		h.Write(a.identity.GetPublicKey())
		h.Write([]byte{a.hops})
		h.Write(a.appData)
		if a.ratchetID != nil {
			h.Write(a.ratchetID)
		}
		a.hash = h.Sum(nil)
	}
	return a.hash
}

func (a *Announce) GetPacket() ([]byte, error) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.packet == nil {
		var err error
		a.packet, err = a.CreatePacket()
		if err != nil {
			return nil, err
		}
	}

	return a.packet, nil
}
