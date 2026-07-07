// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package resource

const (
	StatusPending   = 0x00
	StatusActive    = 0x01
	StatusComplete  = 0x02
	StatusFailed    = 0x03
	StatusCancelled = 0x04

	DefaultSegmentSize = 384
	MaxSegments        = 65535
	CleanupInterval    = 300

	Window            = 4
	WindowMin         = 2
	WindowMaxSlow     = 10
	WindowMaxVerySlow = 4
	WindowMaxFast     = 75
	WindowMax         = WindowMaxFast

	FastRateThreshold     = WindowMaxSlow - Window - 2
	VerySlowRateThreshold = 2

	RateFast     = (50 * 1000) / 8
	RateVerySlow = (2 * 1000) / 8

	WindowFlexibility = 4

	MapHashLen     = 4
	RandomHashSize = 4

	MaxEfficientSize    = 16*1024*1024 - 1
	AutoCompressMaxSize = MaxEfficientSize

	PartTimeoutFactor         = 4
	PartTimeoutFactorAfterRTT = 2
	ProofTimeoutFactor        = 3
	MaxRetries                = 16
	MaxAdvRetries             = 4
	SenderGraceTime           = 10.0
	ProcessingGrace           = 1.0
	RetryGraceTime            = 0.25
	PerRetryDelay             = 0.5
	ResponseMaxGraceTime      = 10.0
)

const (
	Overhead           = 134
	CollisionGuardSize = 2*WindowMax + 100
)

// ResourceAdvertisement flag bits packed into the wire `f` field.
// Bit positions and shifts are part of the wire format. Do not reorder.

const (
	AdvFlagEncrypted   byte = 0x01
	AdvFlagCompressed  byte = 0x02
	AdvFlagSplit       byte = 0x04
	AdvFlagIsRequest   byte = 0x08
	AdvFlagIsResponse  byte = 0x10
	AdvFlagHasMetadata byte = 0x20

	AdvFlagShiftCompressed  = 1
	AdvFlagShiftSplit       = 2
	AdvFlagShiftIsRequest   = 3
	AdvFlagShiftIsResponse  = 4
	AdvFlagShiftHasMetadata = 5
)

// Heuristic compression-ratio estimates used to size segment buffers
// before transmission. These are rough guesses by file class. Tune them

// as we collect real-world numbers.
const (
	// CompressionEntropyBase is the floor compression ratio applied to
	// any sampled entropy estimate, and CompressionEntropyRange scales
	// from the unique-byte ratio of the sample.
	CompressionEntropyBase  = 0.3
	CompressionEntropyRange = 0.7

	// CompressionRatio* are per-class compressibility estimates.
	CompressionRatioText          = 0.4  // .txt, .log, .json, .xml, .html
	CompressionRatioCSV           = 0.5  // structured but partially compressed
	CompressionRatioOfficeLegacy  = 0.8  // .doc
	CompressionRatioOfficeModern  = 0.95 // .docx, .pdf - already zipped
	CompressionRatioAlreadyPacked = 0.99 // images, audio, video, archives
	CompressionRatioUnknown       = 0.7  // fallback for unrecognised extensions
)
