// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package channel

// Window, sequence, timeout and channel sizing constants.
const (
	InitialHandlerCapacity = 0

	WindowInitial     = 2
	WindowMin         = 2
	WindowMinSlow     = 2
	WindowMinMedium   = 5
	WindowMinFast     = 16
	WindowMaxSlow     = 5
	WindowMaxMedium   = 12
	WindowMaxFast     = 48
	WindowMax         = WindowMaxFast
	WindowFlexibility = 4

	RTTFast   = 0.18
	RTTMedium = 0.75
	RTTSlow   = 1.45

	SeqMax     uint16 = 0xFFFF
	SeqModulus uint16 = SeqMax

	FastRateThreshold = 10

	RTTMinThreshold       = 0.025
	TimeoutBaseMultiplier = 1.5
	TimeoutRingMultiplier = 2.5
	TimeoutRingOffset     = 2

	ChannelHeaderSize = 6
	ChannelHeaderBits = 8

	DefaultMaxTries = 3
)

// MessageState represents the delivery state of a channel message.
type MessageState int

const (
	// MsgStateNew is the initial state before send.
	MsgStateNew MessageState = iota
	MsgStateSent
	MsgStateDelivered
	MsgStateFailed
)
