// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"errors"
	"fmt"

	"quad4/reticulum-go/pkg/identity"
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

// BuildEncryptedAppData builds discovery app_data encrypted with networkID.
// Wire layout matches Python: flags || encrypt(packed||stamp).
func BuildEncryptedAppData(info Info, stampCost int, expandRounds int, networkID *identity.Identity) ([]byte, error) {
	if networkID == nil {
		return nil, errors.New("discovery: network identity required for encrypted announce")
	}
	packed, err := EncodeInfo(info)
	if err != nil {
		return nil, err
	}
	stamp, _, err := GenerateStamp(InfoHash(packed), stampCost, expandRounds)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, 0, len(packed)+len(stamp))
	plain = append(plain, packed...)
	plain = append(plain, stamp...)
	cipher, err := networkID.Encrypt(plain, nil)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(cipher))
	out = append(out, FlagEncrypted)
	out = append(out, cipher...)
	return out, nil
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
// Encrypted announces require ValidateAndDecodeWithIdentity.
func ValidateAndDecode(appData []byte, requiredValue int, expandRounds int) (*ReceivedAnnounceInfo, error) {
	return ValidateAndDecodeWithIdentity(appData, requiredValue, expandRounds, nil)
}

// ValidateAndDecodeWithIdentity decrypts FlagEncrypted announces with
// networkID before stamp validation.
func ValidateAndDecodeWithIdentity(appData []byte, requiredValue int, expandRounds int, networkID *identity.Identity) (*ReceivedAnnounceInfo, error) {
	if len(appData) <= 1+StampSize {
		return nil, fmt.Errorf("discovery: app_data too short (%d bytes)", len(appData))
	}
	flags := appData[0]
	rest := appData[1:]

	var packed, stamp []byte
	if flags&FlagEncrypted != 0 {
		if networkID == nil {
			return nil, errors.New("discovery: encrypted announce requires network identity")
		}
		plain, err := networkID.Decrypt(rest, nil, false, nil)
		if err != nil {
			return nil, fmt.Errorf("discovery: decrypt announce: %w", err)
		}
		if len(plain) < StampSize {
			return nil, errors.New("discovery: decrypted announce too short")
		}
		stamp = plain[len(plain)-StampSize:]
		packed = plain[:len(plain)-StampSize]
		flags &^= FlagEncrypted
	} else {
		var err error
		flags, packed, stamp, err = DecodeAppData(appData)
		if err != nil {
			return nil, err
		}
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
