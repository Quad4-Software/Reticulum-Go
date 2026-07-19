// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/securemem"
)

// PacketCallback handles plaintext delivered to a destination.
type PacketCallback = common.PacketCallback

// ProofRequestedCallback is invoked when the stack requests an app-level proof.
type ProofRequestedCallback = common.ProofRequestedCallback

// LinkEstablishedCallback is invoked when an inbound or outbound link is ready.
type LinkEstablishedCallback = common.LinkEstablishedCallback

// ResponseGeneratorFunc builds a request-handler response payload.
type ResponseGeneratorFunc func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any

// RequestHandler binds a path to a response generator and allow policy.
type RequestHandler struct {
	Path              string
	ResponseGenerator ResponseGeneratorFunc
	AllowMode         byte
	AllowedList       [][]byte
	AutoCompress      bool
}

// Transport is the subset of transport.Transport needed by Destination.
type Transport interface {
	GetConfig() *common.ReticulumConfig
	GetInterfaces() map[string]common.NetworkInterface
	RegisterDestination(hash []byte, dest any)
}

// IncomingLinkHandler builds a link from an inbound link-request packet.
type IncomingLinkHandler func(pkt *packet.Packet, dest *Destination, transport any, networkIface common.NetworkInterface) (any, error)

var incomingLinkHandler IncomingLinkHandler

// RegisterIncomingLinkHandler installs the package-level inbound link handler.
// Pass nil to clear it (tests and destinations without link support).
func RegisterIncomingLinkHandler(handler IncomingLinkHandler) {
	incomingLinkHandler = handler
}

// Destination is a named Reticulum endpoint bound to an identity.
type Destination struct {
	identity  *identity.Identity
	direction byte
	destType  byte
	appName   string
	aspects   []string
	hashValue []byte
	transport Transport

	acceptsLinks  bool
	proofStrategy byte

	packetCallback PacketCallback
	proofCallback  ProofRequestedCallback
	linkCallback   LinkEstablishedCallback

	ratchetsEnabled   bool
	ratchetPath       string
	ratchetCount      int
	ratchetInterval   int
	enforceRatchets   bool
	latestRatchetTime time.Time
	latestRatchetID   []byte
	ratchets          []*securemem.Buf
	ratchetFileLock   sync.Mutex

	defaultAppData []byte
	mutex          sync.RWMutex

	requestHandlers map[string]*RequestHandler
}

// New creates a Destination for appName and optional aspects.
// Direction In requires a non-nil transport so inbound packets can register.
func New(id *identity.Identity, direction byte, destType byte, appName string, transport Transport, aspects ...string) (*Destination, error) {
	debug.Log(debug.DebugInfo, "Creating new destination", "app", appName, "type", destType, "direction", direction)

	if id == nil && destType != Plain {
		debug.Log(debug.DebugError, "Cannot create destination: identity is nil for non-Plain destination")
		return nil, errors.New("identity cannot be nil for non-Plain destination")
	}
	if err := ValidateNameParts(appName, aspects...); err != nil {
		return nil, err
	}
	if (direction&In) != 0 && transport == nil {
		return nil, common.ErrDestTransportRequiredForIn
	}

	d := &Destination{
		identity:        id,
		direction:       direction,
		destType:        destType,
		appName:         appName,
		aspects:         aspects,
		transport:       transport,
		acceptsLinks:    false,
		proofStrategy:   ProveNone,
		ratchetCount:    RatchetCount,
		ratchetInterval: RatchetInterval,
		requestHandlers: make(map[string]*RequestHandler),
	}

	// Generate destination hash
	d.hashValue = d.calculateHash()
	debug.Log(debug.DebugVerbose, "Created destination with hash", "hash", fmt.Sprintf("%x", d.hashValue))

	// Auto-register with transport if direction is In
	if (direction & In) != 0 {
		transport.RegisterDestination(d.hashValue, d)
		debug.Log(debug.DebugInfo, "Destination auto-registered with transport", "hash", fmt.Sprintf("%x", d.hashValue))
	}

	return d, nil
}

// FromHash creates a destination from a known hash (e.g., from an announce).
// This is used by clients to create destination objects for servers they've discovered.
func FromHash(hash []byte, id *identity.Identity, destType byte, transport Transport) (*Destination, error) {
	debug.Log(debug.DebugInfo, "Creating destination from hash", "hash", fmt.Sprintf("%x", hash))

	if id == nil && destType != Plain {
		debug.Log(debug.DebugError, "Cannot create destination: identity is nil for non-Plain destination")
		return nil, errors.New("identity cannot be nil for non-Plain destination")
	}

	d := &Destination{
		identity:        id,
		direction:       Out,
		destType:        destType,
		hashValue:       hash,
		transport:       transport,
		acceptsLinks:    false,
		proofStrategy:   ProveNone,
		ratchetCount:    RatchetCount,
		ratchetInterval: RatchetInterval,
		requestHandlers: make(map[string]*RequestHandler),
	}

	debug.Log(debug.DebugVerbose, "Created destination from hash", "hash", fmt.Sprintf("%x", hash))
	return d, nil
}

func (d *Destination) calculateHash() []byte {
	return Hash(d.identity, d.appName, d.aspects...)
}

// ExpandAppName builds the dotted destination name used in hashing.
func ExpandAppName(appName string, aspects ...string) string {
	if len(aspects) == 0 {
		return appName
	}
	var name strings.Builder
	name.Grow(len(appName) + len(aspects)*8)
	name.WriteString(appName)
	for _, aspect := range aspects {
		name.WriteByte('.')
		name.WriteString(aspect)
	}
	return name.String()
}

// ValidateNameParts rejects dots in app names and aspects, matching Python
// RNS.Destination. Dots would otherwise make expand_name / ParseName ambiguous.
func ValidateNameParts(appName string, aspects ...string) error {
	if appName == "" {
		return errors.New("app name required")
	}
	if strings.Contains(appName, ".") {
		return errors.New("dots can't be used in app names")
	}
	for _, aspect := range aspects {
		if strings.Contains(aspect, ".") {
			return errors.New("dots can't be used in aspects")
		}
	}
	return nil
}

// ParseName splits a dotted destination name into app name and aspects.
func ParseName(full string) (appName string, aspects []string, err error) {
	full = strings.TrimSpace(full)
	if full == "" {
		return "", nil, errors.New("empty destination name")
	}
	parts := strings.Split(full, ".")
	if len(parts) < 1 || parts[0] == "" {
		return "", nil, errors.New("invalid destination name")
	}
	if len(parts) == 1 {
		return parts[0], nil, nil
	}
	return parts[0], parts[1:], nil
}

// Hash computes a 16-byte destination hash from identity and app name aspects.
func Hash(id *identity.Identity, appName string, aspects ...string) []byte {
	nameHashFull := sha256.Sum256([]byte(ExpandAppName(appName, aspects...)))
	nameHash10 := nameHashFull[:10]

	var combined [26]byte
	n := copy(combined[:], nameHash10)
	if id != nil {
		identityHash := identity.TruncatedHash(id.GetPublicKey())
		n += copy(combined[n:], identityHash)
	}
	finalHashFull := sha256.Sum256(combined[:n])
	out := make([]byte, 16)
	copy(out, finalHashFull[:16])
	return out
}

// ExpandName returns appName joined with aspects using dots.
func (d *Destination) ExpandName() string {
	return ExpandAppName(d.appName, d.aspects...)
}

// Announce builds and sends an announce packet on registered interfaces.
// Returns ErrDestTransportNotSet, ErrDestAnnounceNoInterfaces, or
// ErrDestAnnounceNoWritable when no usable outbound path exists.
func (d *Destination) Announce(pathResponse bool, tag []byte, attachedInterface common.NetworkInterface) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	debug.Log(debug.DebugVerbose, "Announcing destination", "name", d.ExpandName(), "path_response", pathResponse)

	if d.transport == nil {
		return common.ErrDestTransportNotSet
	}

	appData := d.defaultAppData

	// Create announce packet using announce package
	announceObj, err := announce.New(d.identity, d.hashValue, d.ExpandName(), appData, pathResponse, d.transport.GetConfig())
	if err != nil {
		return fmt.Errorf("failed to create announce: %w", err)
	}

	packet, err := announceObj.GetPacket()
	if err != nil {
		return fmt.Errorf("failed to create announce packet: %w", err)
	}

	if pathResponse && tag != nil {
		debug.Log(debug.DebugInfo, "Sending path response announce", "tag", fmt.Sprintf("%x", tag))
	}

	var lastErr error
	sent := 0
	if attachedInterface != nil {
		if attachedInterface.IsEnabled() && attachedInterface.IsOnline() {
			if !common.InterfaceAllowsOutgoing(attachedInterface) {
				debug.Log(debug.DebugVerbose, "Skipping announce on receive-only attached interface", "name", attachedInterface.GetName())
			} else {
				debug.Log(debug.DebugVerbose, "Sending announce to attached interface", "name", attachedInterface.GetName())
				if err := attachedInterface.Send(packet, ""); err != nil {
					debug.Log(debug.DebugError, "Failed to send announce on attached interface", "error", err)
					lastErr = err
				} else {
					sent++
				}
			}
		}
	} else {
		interfaces := d.transport.GetInterfaces()
		if len(interfaces) == 0 {
			return common.ErrDestAnnounceNoInterfaces
		}
		for name, iface := range interfaces {
			if !iface.IsEnabled() || !iface.IsOnline() {
				continue
			}
			if !common.InterfaceAllowsOutgoing(iface) {
				debug.Log(debug.DebugVerbose, "Skipping announce on receive-only interface", "name", name)
				continue
			}
			debug.Log(debug.DebugVerbose, "Sending announce to interface", "name", name)
			if err := iface.Send(packet, ""); err != nil {
				debug.Log(debug.DebugError, "Failed to send announce on interface", "name", name, "error", err)
				lastErr = err
				continue
			}
			sent++
		}
	}

	if lastErr != nil {
		return lastErr
	}
	if sent == 0 {
		return common.ErrDestAnnounceNoWritable
	}
	return nil
}

// AcceptsLinks marks whether this destination should accept incoming links.
// AcceptsLinks(true) registers the destination with transport if one is set.
// Direction In already auto-registers in New. AcceptsLinks(false) only clears
// the flag and does not unregister from transport.
func (d *Destination) AcceptsLinks(accepts bool) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.acceptsLinks = accepts

	if accepts && d.transport != nil {
		d.transport.RegisterDestination(d.hashValue, d)
		debug.Log(debug.DebugVerbose, "Destination registered with transport for link requests", "hash", fmt.Sprintf("%x", d.hashValue))
		return
	}
	if !accepts {
		debug.Log(debug.DebugInfo, common.MsgDestAcceptsLinksFalseOnly)
	}
}

func (d *Destination) SetLinkEstablishedCallback(callback common.LinkEstablishedCallback) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.linkCallback = callback
}

func (d *Destination) GetLinkCallback() common.LinkEstablishedCallback {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.linkCallback
}

func (d *Destination) HandleIncomingLinkRequest(pkt any, transport any, networkIface common.NetworkInterface) error {
	debug.Log(debug.DebugInfo, "Handling incoming link request for destination", "hash", fmt.Sprintf("%x", d.GetHash()))

	pktObj, ok := pkt.(*packet.Packet)
	if !ok {
		return errors.New("invalid packet type")
	}

	if incomingLinkHandler == nil {
		return common.ErrDestNoIncomingLinkHandler
	}

	_, err := incomingLinkHandler(pktObj, d, transport, networkIface)
	if err != nil {
		return fmt.Errorf("failed to handle link request: %w", err)
	}

	// Note: For responders, the link established callback is now handled
	// within the Link lifecycle (in handleRTTPacket) to ensure the link
	// is actually active before the application is notified.

	return nil
}

func (d *Destination) SetPacketCallback(callback common.PacketCallback) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.packetCallback = callback
}

func (d *Destination) Receive(pkt *packet.Packet, iface common.NetworkInterface) {
	if pkt != nil && pkt.PacketType == packet.PacketTypeLinkReq {
		debug.Log(debug.DebugInfo, "Received link request for destination")
		if err := d.HandleIncomingLinkRequest(pkt, d.transport, iface); err != nil {
			debug.Log(debug.DebugError, "Failed to handle incoming link request", "error", err)
		}
		return
	}

	d.mutex.RLock()
	callback := d.packetCallback
	d.mutex.RUnlock()

	if callback == nil {
		debug.Log(debug.DebugInfo, common.MsgDestNoPacketCallback, "hash", fmt.Sprintf("%x", d.GetHash()))
		return
	}

	plaintext, err := d.Decrypt(pkt.Data)
	if err != nil {
		debug.Log(debug.DebugInfo, "Failed to decrypt packet data", "error", err)
		return
	}

	debug.Log(debug.DebugInfo, "Destination received packet", "bytes", len(plaintext))

	callback(plaintext, iface)
}

func (d *Destination) SetProofRequestedCallback(callback common.ProofRequestedCallback) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.proofCallback = callback
}

func (d *Destination) SetProofStrategy(strategy byte) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.proofStrategy = strategy
}

// ProofStrategy returns the configured proof strategy.
func (d *Destination) ProofStrategy() byte {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.proofStrategy
}

// ProofRequestedCallback returns the app proof callback, if any.
func (d *Destination) ProofRequestedCallback() common.ProofRequestedCallback {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.proofCallback
}

func (d *Destination) EnableRatchets(path string) bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if path == "" {
		debug.Log(debug.DebugError, "No ratchet file path specified")
		return false
	}

	d.ratchetsEnabled = true
	d.ratchetPath = path
	d.latestRatchetTime = time.Time{} // Zero time to force rotation

	// Load or initialize ratchets
	if err := d.reloadRatchets(); err != nil {
		debug.Log(debug.DebugError, "Failed to load ratchets", "error", err)
		// Initialize empty ratchet list
		d.ratchets = make([]*securemem.Buf, 0)
		if err := d.persistRatchets(); err != nil {
			debug.Log(debug.DebugError, "Failed to create initial ratchet file", "error", err)
			return false
		}
	}

	debug.Log(debug.DebugInfo, "Ratchets enabled", "path", path)
	return true
}

// EnableRatchetsInMemory enables forward-secrecy ratchets without writing to
// disk. Suitable for InMemoryStorage and ephemeral embedders.
func (d *Destination) EnableRatchetsInMemory() bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.ratchetsEnabled = true
	d.ratchetPath = ""
	d.latestRatchetTime = time.Time{}
	d.ratchets = make([]*securemem.Buf, 0)

	debug.Log(debug.DebugInfo, "Ratchets enabled in memory")
	return true
}

func (d *Destination) EnforceRatchets() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.enforceRatchets = true
}

func (d *Destination) SetRetainedRatchets(count int) bool {
	if count < 1 {
		return false
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.ratchetCount = count
	return true
}

func (d *Destination) SetRatchetInterval(interval int) bool {
	if interval < 1 {
		return false
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.ratchetInterval = interval
	return true
}

func (d *Destination) SetDefaultAppData(data []byte) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.defaultAppData = data
}

func (d *Destination) ClearDefaultAppData() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.defaultAppData = nil
}

func (d *Destination) RegisterRequestHandler(path string, responseGen func(string, []byte, []byte, []byte, *identity.Identity, int64) []byte, allow byte, allowedList [][]byte) error {
	wrapped := func(p string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any {
		out := responseGen(p, data, requestID, linkID, remoteIdentity, requestedAt)
		if out == nil {
			return nil
		}
		return out
	}
	return d.RegisterRequestHandlerAny(path, wrapped, allow, allowedList)
}

func (d *Destination) RegisterRequestHandlerAny(path string, responseGen ResponseGeneratorFunc, allow byte, allowedList [][]byte) error {
	if path == "" {
		return errors.New("path cannot be empty")
	}

	if allow != AllowNone && allow != AllowAll && allow != AllowList {
		return errors.New("invalid allow mode")
	}

	if allow == AllowList && len(allowedList) == 0 {
		return errors.New("allowed list required for AllowList mode")
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.requestHandlers[path] = &RequestHandler{
		Path:              path,
		ResponseGenerator: responseGen,
		AllowMode:         allow,
		AllowedList:       allowedList,
	}

	return nil
}

func (d *Destination) DeregisterRequestHandler(path string) bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if _, exists := d.requestHandlers[path]; exists {
		delete(d.requestHandlers, path)
		return true
	}
	return false
}

// HasRequestHandlers reports whether any request handlers are registered.
func (d *Destination) HasRequestHandlers() bool {
	if d == nil {
		return false
	}
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return len(d.requestHandlers) > 0
}

func (d *Destination) GetRequestHandler(pathHash []byte) func([]byte, []byte, []byte, []byte, *identity.Identity, time.Time) any {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	for _, handler := range d.requestHandlers {
		handlerPathHash := identity.TruncatedHash([]byte(handler.Path))
		if string(handlerPathHash) == string(pathHash) {
			return func(pathHash []byte, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt time.Time) any {
				allowed := false
				if handler.AllowMode == AllowAll {
					allowed = true
				} else if handler.AllowMode == AllowList && remoteIdentity != nil {
					remoteHash := remoteIdentity.Hash()
					for _, allowedHash := range handler.AllowedList {
						if string(remoteHash) == string(allowedHash) {
							allowed = true
							break
						}
					}
				}

				if !allowed {
					return nil
				}

				return handler.ResponseGenerator(handler.Path, data, requestID, linkID, remoteIdentity, requestedAt.Unix())
			}
		}
	}
	return nil
}

func (d *Destination) HandleRequest(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) []byte {
	d.mutex.RLock()
	handler, exists := d.requestHandlers[path]
	d.mutex.RUnlock()

	if !exists {
		debug.Log(debug.DebugInfo, common.MsgDestNoRequestHandler, "path", path)
		return []byte(">Not Found\n\nThe requested resource was not found.")
	}

	allowed := false
	if handler.AllowMode == AllowAll {
		allowed = true
	} else if handler.AllowMode == AllowList && remoteIdentity != nil {
		remoteHash := remoteIdentity.Hash()
		for _, allowedHash := range handler.AllowedList {
			if string(remoteHash) == string(allowedHash) {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		debug.Log(debug.DebugInfo, "Request denied by allow mode", "path", path)
		return []byte(">Not Found\n\nThe requested resource was not found.")
	}

	debug.Log(debug.DebugVerbose, "Calling request handler", "path", path)
	result := handler.ResponseGenerator(path, data, requestID, linkID, remoteIdentity, requestedAt)
	if result == nil {
		return []byte(">Not Found\n\nThe requested resource was not found.")
	}
	if b, ok := result.([]byte); ok {
		return b
	}
	encoded, err := msgpack.Marshal(result)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to msgpack-encode handler response", "path", path, "error", err)
		return []byte(">Not Found\n\nThe requested resource was not found.")
	}
	return encoded
}

func (d *Destination) Encrypt(plaintext []byte) ([]byte, error) {
	if d.destType == Plain {
		debug.Log(debug.DebugVerbose, "Using plaintext transmission for Plain destination")
		return plaintext, nil
	}

	if d.identity == nil {
		debug.Log(debug.DebugInfo, "Cannot encrypt: no identity available")
		return nil, errors.New("no identity available for encryption")
	}

	debug.Log(debug.DebugVerbose, "Encrypting bytes for destination", "bytes", len(plaintext), "destType", d.destType)

	switch d.destType {
	case Single:
		recipientKey := d.identity.GetEncryptionKey()
		debug.Log(debug.DebugVerbose, "Encrypting for single recipient", "key", fmt.Sprintf("%x", recipientKey[:8]))
		return d.identity.Encrypt(plaintext, recipientKey)
	case Group:
		key := d.identity.GetCurrentRatchetKey()
		if key == nil {
			debug.Log(debug.DebugInfo, "Cannot encrypt: no ratchet key available")
			return nil, errors.New("no ratchet key available")
		}
		debug.Log(debug.DebugVerbose, "Encrypting for group destination")
		return d.identity.EncryptWithHMAC(plaintext, key)
	default:
		debug.Log(debug.DebugInfo, "Unsupported destination type for encryption", "destType", d.destType)
		return nil, errors.New("unsupported destination type for encryption")
	}
}

func (d *Destination) Decrypt(ciphertext []byte) ([]byte, error) {
	if d.destType == Plain {
		return ciphertext, nil
	}

	if d.identity == nil {
		return nil, errors.New("no identity available for decryption")
	}

	// Create empty ratchet receiver to get latest ratchet ID if available
	ratchetReceiver := &common.RatchetIDReceiver{}

	// Call Decrypt with full parameter list:
	// - ciphertext: the encrypted data
	// - ratchets: nil since we're not providing specific ratchets
	// - enforceRatchets: false to allow fallback to normal decryption
	// - ratchetIDReceiver: to receive the latest ratchet ID used
	return d.identity.Decrypt(ciphertext, nil, false, ratchetReceiver)
}

func (d *Destination) Sign(data []byte) ([]byte, error) {
	if d.identity == nil {
		return nil, errors.New("no identity available")
	}
	return d.identity.Sign(data)
}

func (d *Destination) GetPublicKey() []byte {
	if d.identity == nil {
		return nil
	}
	return d.identity.GetPublicKey()
}

func (d *Destination) GetIdentity() *identity.Identity {
	return d.identity
}

func (d *Destination) GetType() byte {
	return d.destType
}

func (d *Destination) GetHash() []byte {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	if d.hashValue == nil {
		d.mutex.RUnlock()
		d.mutex.Lock()
		defer d.mutex.Unlock()
		if d.hashValue == nil {
			d.hashValue = d.calculateHash()
		}
	}
	return d.hashValue
}

func (d *Destination) persistRatchets() error {
	d.ratchetFileLock.Lock()
	defer d.ratchetFileLock.Unlock()

	if !d.ratchetsEnabled {
		return errors.New("ratchets not enabled")
	}
	if d.ratchetPath == "" {
		return nil
	}

	debug.Log(debug.DebugPackets, "Persisting ratchets", "count", len(d.ratchets), "path", d.ratchetPath)

	raw := make([][]byte, 0, len(d.ratchets))
	for _, buf := range d.ratchets {
		if buf == nil {
			continue
		}
		raw = append(raw, buf.CopyOut())
	}
	defer func() {
		for _, b := range raw {
			securemem.WipeBytes(b)
		}
	}()

	packedRatchets, err := msgpack.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to pack ratchets: %w", err)
	}

	// Sign the packed ratchets
	signature, err := d.Sign(packedRatchets)
	if err != nil {
		return fmt.Errorf("failed to sign ratchets: %w", err)
	}

	// Create structure
	persistedData := map[string][]byte{
		"signature": signature,
		"ratchets":  packedRatchets,
	}

	// Pack the entire structure
	finalData, err := msgpack.Marshal(persistedData)
	if err != nil {
		return fmt.Errorf("failed to pack ratchet data: %w", err)
	}

	// Write to temporary file first, then rename (atomic operation)
	tempPath := d.ratchetPath + ".tmp"
	file, err := os.Create(tempPath) // #nosec G304
	if err != nil {
		return fmt.Errorf("failed to create temp ratchet file: %w", err)
	}

	if _, err := file.Write(finalData); err != nil {
		// #nosec G104 - Error already being handled, cleanup errors are non-critical
		file.Close()
		// #nosec G104 - Error already being handled, cleanup errors are non-critical
		os.Remove(tempPath)
		return fmt.Errorf("failed to write ratchet data: %w", err)
	}
	// #nosec G104 - File is being closed after successful write, error is non-critical
	file.Close()

	// Remove old file if exists
	if _, err := os.Stat(d.ratchetPath); err == nil {
		// #nosec G104 - Removing old file, error is non-critical if it doesn't exist
		os.Remove(d.ratchetPath)
	}

	// Atomic rename
	if err := os.Rename(tempPath, d.ratchetPath); err != nil {
		// #nosec G104 - Error already being handled, cleanup errors are non-critical
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename ratchet file: %w", err)
	}

	debug.Log(debug.DebugPackets, "Ratchets persisted successfully")
	return nil
}

func (d *Destination) reloadRatchets() error {
	d.ratchetFileLock.Lock()
	defer d.ratchetFileLock.Unlock()

	if d.ratchetPath == "" {
		if d.ratchets == nil {
			d.ratchets = make([]*securemem.Buf, 0)
		}
		return nil
	}

	if _, err := os.Stat(d.ratchetPath); os.IsNotExist(err) {
		debug.Log(debug.DebugInfo, "No existing ratchet data found, initializing new ratchet file")
		d.ratchets = make([]*securemem.Buf, 0)
		return nil
	}

	file, err := os.Open(d.ratchetPath) // #nosec G304
	if err != nil {
		return fmt.Errorf("failed to open ratchet file: %w", err)
	}
	defer file.Close()

	// Read all data
	fileData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read ratchet file: %w", err)
	}

	// Unpack outer structure
	var persistedData map[string][]byte
	if err := msgpack.Unmarshal(fileData, &persistedData); err != nil {
		return fmt.Errorf("failed to unpack ratchet data: %w", err)
	}

	signature, hasSignature := persistedData["signature"]
	packedRatchets, hasRatchets := persistedData["ratchets"]

	if !hasSignature || !hasRatchets {
		return fmt.Errorf("invalid ratchet file format")
	}

	// Verify signature
	if !d.identity.Verify(packedRatchets, signature) {
		return fmt.Errorf("invalid ratchet file signature")
	}

	// Unpack ratchet list into locked buffers
	var raw [][]byte
	if err := msgpack.Unmarshal(packedRatchets, &raw); err != nil {
		return fmt.Errorf("failed to unpack ratchet list: %w", err)
	}
	for _, old := range d.ratchets {
		if old != nil {
			_ = old.Close()
		}
	}
	d.ratchets = make([]*securemem.Buf, 0, len(raw))
	for _, r := range raw {
		buf, err := securemem.New(len(r))
		if err != nil {
			securemem.WipeBytes(r)
			return err
		}
		if err := buf.CopyFrom(r); err != nil {
			_ = buf.Close()
			securemem.WipeBytes(r)
			return err
		}
		securemem.WipeBytes(r)
		d.ratchets = append(d.ratchets, buf)
	}

	debug.Log(debug.DebugInfo, "Ratchets reloaded successfully", "count", len(d.ratchets))
	return nil
}

func (d *Destination) RotateRatchets() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if !d.ratchetsEnabled {
		return errors.New("ratchets not enabled")
	}

	now := time.Now()
	if !d.latestRatchetTime.IsZero() && now.Before(d.latestRatchetTime.Add(time.Duration(d.ratchetInterval)*time.Second)) {
		debug.Log(debug.DebugTrace, "Ratchet rotation interval not reached")
		return nil
	}

	debug.Log(debug.DebugInfo, "Rotating ratchets", "destination", d.ExpandName())

	newRatchet := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, newRatchet); err != nil {
		return fmt.Errorf("failed to generate new ratchet: %w", err)
	}
	buf, err := securemem.New(len(newRatchet))
	if err != nil {
		securemem.WipeBytes(newRatchet)
		return err
	}
	if err := buf.CopyFrom(newRatchet); err != nil {
		_ = buf.Close()
		securemem.WipeBytes(newRatchet)
		return err
	}

	d.ratchets = append([]*securemem.Buf{buf}, d.ratchets...)
	d.latestRatchetTime = now

	ratchetPub, err := cryptography.PublicKeyFromPrivate(newRatchet)
	securemem.WipeBytes(newRatchet)
	if err == nil {
		d.latestRatchetID = identity.TruncatedHash(ratchetPub)[:identity.NameHashLength/8]
	}

	d.cleanRatchets()

	if err := d.persistRatchets(); err != nil {
		debug.Log(debug.DebugError, "Failed to persist ratchets after rotation", "error", err)
		return err
	}

	debug.Log(debug.DebugInfo, "Ratchet rotation completed", "total_ratchets", len(d.ratchets))
	return nil
}

func (d *Destination) cleanRatchets() {
	if len(d.ratchets) > d.ratchetCount {
		debug.Log(debug.DebugTrace, "Cleaning old ratchets", "before", len(d.ratchets), "keeping", d.ratchetCount)
		for _, old := range d.ratchets[d.ratchetCount:] {
			if old != nil {
				_ = old.Close()
			}
		}
		d.ratchets = d.ratchets[:d.ratchetCount]
	}
}

func (d *Destination) GetRatchets() [][]byte {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	if !d.ratchetsEnabled {
		return nil
	}

	ratchetsCopy := make([][]byte, 0, len(d.ratchets))
	for _, buf := range d.ratchets {
		if buf != nil {
			ratchetsCopy = append(ratchetsCopy, buf.CopyOut())
		}
	}
	return ratchetsCopy
}
