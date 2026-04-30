// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package discovery

import (
	"errors"
	"fmt"
)

// BuildAppData composes the full app_data payload that should be carried in
// a discovery announce. Equivalent to Discovery.get_interface_announce_data
// without the optional encryption step (callers handling FlagEncrypted must
// encrypt the (info||stamp) bytes with their network identity before passing
// the result to BuildAppDataPrebuilt).
//
// info is encoded with EncodeInfo, the stamp is generated with the supplied
// stampCost and expandRounds (use WorkblockExpandRounds for full Discovery
// compatibility), and the resulting bytes are flag||info||stamp.
func BuildAppData(info Info, stampCost int, expandRounds int) ([]byte, error) {
	packed, err := EncodeInfo(info)
	if err != nil {
		return nil, err
	}
	stamp, _, err := GenerateStamp(InfoHash(packed), stampCost, expandRounds)
	if err != nil {
		return nil, err
	}
	return EncodeAppData(0x00, packed, stamp)
}

// ReceivedAnnounceInfo is the value yielded by an InterfaceAnnounceHandler
// after successful validation, matching the dict produced by
// InterfaceAnnounceHandler.received_announce.
type ReceivedAnnounceInfo struct {
	Flags          byte
	Info           Info
	Stamp          []byte
	StampValue     int
	RequiredValue  int
	RemoteIdentity []byte
}

// ValidateAndDecode parses an inbound discovery announce app_data buffer.
// It returns the decoded Info and stamp value, validating that the stamp
// meets requiredValue. The encrypted flag must be cleared by the caller
// before this function is invoked when FlagEncrypted is set; pass the
// already-decrypted payload in that case.
func ValidateAndDecode(appData []byte, requiredValue int, expandRounds int) (*ReceivedAnnounceInfo, error) {
	flags, packed, stamp, err := DecodeAppData(appData)
	if err != nil {
		return nil, err
	}
	if flags&FlagEncrypted != 0 {
		return nil, errors.New("discovery: encrypted announce; decrypt before calling ValidateAndDecode")
	}
	infoHash := InfoHash(packed)
	wb, err := StampWorkblock(infoHash, expandRounds)
	if err != nil {
		return nil, err
	}
	if !StampValid(stamp, requiredValue, wb) {
		return nil, fmt.Errorf("discovery: stamp value below required cost %d", requiredValue)
	}
	value := StampValue(wb, stamp)
	info, err := DecodeInfo(packed)
	if err != nil {
		return nil, err
	}
	return &ReceivedAnnounceInfo{
		Flags:         flags,
		Info:          info,
		Stamp:         stamp,
		StampValue:    value,
		RequiredValue: requiredValue,
	}, nil
}
