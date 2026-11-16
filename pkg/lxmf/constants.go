// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

// AppName is the LXMF destination application identifier.
const AppName = "lxmf"

// Version is the LXMF package version aligned with upstream LXMF.
const Version = "1.3.0"

// Wire layout sizes for packed LXMF messages.
const (
	// DestinationLength is the truncated identity hash size for destination and source (16 bytes).
	DestinationLength = 16
	// SignatureLength is the Ed25519 signature size appended to each packed message (64 bytes).
	SignatureLength = 64
	// TicketLength is the outbound ticket hash size (16 bytes).
	TicketLength = 16
	// TimestampSize is the msgpack width of the payload timestamp (float64).
	TimestampSize = 8
	// StructOverhead is msgpack structural overhead used in content limit calculations.
	StructOverhead = 8
	// Overhead is per-message fixed overhead: destination, source, signature, timestamp, and struct overhead.
	Overhead = 2*DestinationLength + SignatureLength + TimestampSize + StructOverhead
)

// Delivery method values.
const (
	MethodUnknown       byte = 0x00
	MethodOpportunistic byte = 0x01
	MethodDirect        byte = 0x02
	MethodPropagated    byte = 0x03
	MethodPaper         byte = 0x05
)

// LXMessage state values.
const (
	StateGenerating byte = 0x00
	StateOutbound   byte = 0x01
	StateSending    byte = 0x02
	StateSent       byte = 0x04
	StateDelivered  byte = 0x08
	StateRejected   byte = 0xFD
	StateCancelled  byte = 0xFE
	StateFailed     byte = 0xFF
)

// Representation describes how the message body is carried on the wire.
const (
	RepresentationUnknown  byte = 0x00
	RepresentationPacket   byte = 0x01
	RepresentationResource byte = 0x02
)

// UnverifiedReason values when signature validation fails or is skipped.
const (
	UnverifiedSourceUnknown    byte = 0x01
	UnverifiedSignatureInvalid byte = 0x02
)

// Field identifiers for the payload fields map.
const (
	FieldEmbeddedLXMs    byte = 0x01
	FieldTelemetry       byte = 0x02
	FieldTelemetryStream byte = 0x03
	FieldIconAppearance  byte = 0x04
	FieldFileAttachments byte = 0x05
	FieldImage           byte = 0x06
	FieldAudio           byte = 0x07
	FieldThread          byte = 0x08
	FieldCommands        byte = 0x09
	FieldResults         byte = 0x0A
	FieldGroup           byte = 0x0B
	FieldTicket          byte = 0x0C
	FieldEvent           byte = 0x0D
	FieldRNRRefs         byte = 0x0E
	FieldRenderer        byte = 0x0F

	FieldReplyTo      byte = 0x30
	FieldReplyQuote   byte = 0x31
	FieldReaction     byte = 0x40
	FieldComment      byte = 0x41
	FieldContinuation byte = 0x42

	FieldCustomType byte = 0xFB
	FieldCustomData byte = 0xFC
	FieldCustomMeta byte = 0xFD

	FieldNonSpecific byte = 0xFE
	FieldDebug       byte = 0xFF
)

// Renderer values for FieldRenderer.
const (
	RendererPlain    byte = 0x00
	RendererMicron   byte = 0x01
	RendererMarkdown byte = 0x02
	RendererBBCode   byte = 0x03
)

// Reaction dict keys for FieldReaction.
const (
	ReactionTo      byte = 0x00
	ReactionContent byte = 0x01
)

// Comment dict keys for FieldComment.
const (
	CommentFor byte = 0x00
)

// Continuation dict keys for FieldContinuation.
const (
	ContinuationOf byte = 0x00
)

// Audio codec modes for FieldAudio.
const (
	AudioCodec2_450PWB byte = 0x01
	AudioCodec2_450    byte = 0x02
	AudioCodec2_700C   byte = 0x03
	AudioCodec2_1200   byte = 0x04
	AudioCodec2_1300   byte = 0x05
	AudioCodec2_1400   byte = 0x06
	AudioCodec2_1600   byte = 0x07
	AudioCodec2_2400   byte = 0x08
	AudioCodec2_3200   byte = 0x09

	AudioOpusOgg       byte = 0x10
	AudioOpusLBW       byte = 0x11
	AudioOpusMBW       byte = 0x12
	AudioOpusPTT       byte = 0x13
	AudioOpusRTHDX     byte = 0x14
	AudioOpusRTFDX     byte = 0x15
	AudioOpusStandard  byte = 0x16
	AudioOpusHQ        byte = 0x17
	AudioOpusBroadcast byte = 0x18
	AudioOpusLossless  byte = 0x19

	AudioCustom byte = 0xFF
)

// Propagation node metadata map keys.
const (
	PNMetaVersion      byte = 0x00
	PNMetaName         byte = 0x01
	PNMetaSyncStratum  byte = 0x02
	PNMetaSyncThrottle byte = 0x03
	PNMetaAuthBand     byte = 0x04
	PNMetaUtilPressure byte = 0x05
	PNMetaImplName     byte = 0xFE
	PNMetaCustom       byte = 0xFF
)

// Announce functionality codes (third element of v0.5.0+ app data). Omitted lists imply all SF flags for older peers.
const (
	// SFCompression means the peer accepts bz2-compressed Resource transfers. Omit on stacks without bz2.
	SFCompression byte = 0x00
)

// Transport encryption labels for packed_container metadata. Upstream LXMF 1.3.0 labels AES group encryption AES-256, matching the RNS 1.6.0 Token default.
const (
	EncryptionDescriptionAES         = "AES-256"
	EncryptionDescriptionAES256      = EncryptionDescriptionAES
	EncryptionDescriptionEC          = "Curve25519"
	EncryptionDescriptionUnencrypted = "Unencrypted"
)

// Destination type bytes; same values as Reticulum-Go destination types (duplicated to avoid importing destination where only constants are needed).
const (
	DestinationTypeSingle byte = 0x00
	DestinationTypeGroup  byte = 0x01
	DestinationTypePlain  byte = 0x02
	DestinationTypeLink   byte = 0x03
)

// URISchema is the paper-message URI scheme prefix (without "://").
const URISchema = "lxm"

// Packet and link MDU limits aligned with Reticulum-Go defaults. Upstream
// LXMF 1.3.0 uses RNS.Packet.ENCRYPTED_MDU directly without adding
// TIMESTAMP_SIZE, so a single encrypted packet carries at most 383 bytes.
const (
	EncryptedPacketMDU = 383
	PlainPacketMDU     = 464
	LinkPacketMDU      = 431

	EncryptedPacketMaxContent = EncryptedPacketMDU - Overhead + DestinationLength
	LinkPacketMaxContent      = LinkPacketMDU - Overhead
	PlainPacketMaxContent     = PlainPacketMDU - Overhead + DestinationLength
)

// Paper and QR parameters. PaperMDU is usable payload bytes in an lxm:// URI at the configured error correction.
const (
	QRMaxStorage    = 2953
	uriSchemaPrefix = URISchema + "://"
	uriPrefixLen    = len(uriSchemaPrefix)
	PaperMDU        = ((QRMaxStorage - uriPrefixLen) * 6) / 8
	QRErrorCorrectL = "ERROR_CORRECT_L"
)

// Ticket timing defaults (seconds) and stamp ticket sentinel.
const (
	TicketExpirySeconds   = 21 * 24 * 60 * 60
	TicketGraceSeconds    = 5 * 24 * 60 * 60
	TicketRenewSeconds    = 14 * 24 * 60 * 60
	TicketIntervalSeconds = 1 * 24 * 60 * 60
	StampValueTicket      = 0x100
)
