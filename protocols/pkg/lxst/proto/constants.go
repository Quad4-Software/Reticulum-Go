// SPDX-License-Identifier: LicenseRef-Reticulum
package proto

const (
	AppName    = "lxst"
	AspectName = "telephony"

	FieldSignalling = 0x00
	FieldFrames     = 0x01

	StatusBusy        = 0x00
	StatusRejected    = 0x01
	StatusCalling     = 0x02
	StatusAvailable   = 0x03
	StatusRinging     = 0x04
	StatusConnecting  = 0x05
	StatusEstablished = 0x06

	PreferredMode    = 0xF0
	PreferredProfile = 0xFF

	CodecRaw    byte = 0x00
	CodecOpus   byte = 0x01
	CodecCodec2 byte = 0x02
	CodecNull   byte = 0xFF

	ModeFullDuplex = 0x01
	ModeHalfDuplex = 0x02
	DefaultMode    = ModeFullDuplex

	ProfileBandwidthUltraLow = 0x10
	ProfileBandwidthVeryLow  = 0x20
	ProfileBandwidthLow      = 0x30
	ProfileQualityMedium     = 0x40
	ProfileQualityHigh       = 0x50
	ProfileQualityMax        = 0x60
	ProfileLatencyUltraLow   = 0x70
	ProfileLatencyLow        = 0x80
	DefaultProfile           = ProfileQualityMedium

	OpusVoiceLow    = 0x00
	OpusVoiceMedium = 0x01
	OpusVoiceHigh   = 0x02
	OpusVoiceMax    = 0x03
	OpusAudioMin    = 0x04
	OpusAudioLow    = 0x05
	OpusAudioMedium = 0x06
	OpusAudioHigh   = 0x07
	OpusAudioMax    = 0x08

	MinBitrate = 6000
	MaxBitrate = 128000

	PlaybackSampleRate = 48000
	PlaybackChannels   = 1

	IdentityHashLen  = 16
	DestHashLen      = 16
	AppHashPrefixLen = 10

	MaxSignals     = 32
	MaxFrames      = 8
	MaxFrameBytes  = 2048
	MaxUnpackBytes = 4096

	Codec2Header700  byte = 0x00
	Codec2Header1200 byte = 0x01
	Codec2Header1300 byte = 0x02
	Codec2Header1400 byte = 0x03
	Codec2Header1600 byte = 0x04
	Codec2Header2400 byte = 0x05
	Codec2Header3200 byte = 0x06
)

func IsAutoStatus(signal int) bool {
	switch signal {
	case StatusCalling, StatusAvailable, StatusRinging, StatusConnecting, StatusEstablished:
		return true
	default:
		return false
	}
}

func IsPreferredProfile(signal int) bool {
	return signal >= PreferredProfile
}

func IsPreferredMode(signal int) bool {
	return signal >= PreferredMode && signal < PreferredProfile
}

func ProfileFromSignal(signal int) int {
	return signal - PreferredProfile
}

func ModeFromSignal(signal int) int {
	return signal - PreferredMode
}

func SignalPreferredProfile(profile int) int {
	if profile == 0 {
		profile = DefaultProfile
	}
	return PreferredProfile + profile
}

func SignalPreferredMode(mode int) int {
	if mode == 0 {
		mode = DefaultMode
	}
	return PreferredMode + mode
}
