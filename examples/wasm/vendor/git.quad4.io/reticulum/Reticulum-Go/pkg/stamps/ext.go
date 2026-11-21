// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package stamps

// Announce ext-payload constants (RNS.Identity EXT* fields, 1.6.0).
const (
	ExtPayloadMarker = 0xC1
	ExtHeaderLen     = 2
	ExtLRCostLen     = 2
	ExtPayloadLen    = ExtHeaderLen + StampLength + ExtLRCostLen // 18
)

// AnnounceExt builds the 18-byte announce extension trailer:
// [0xC1, flags=0x00] + stamp(14) + [lr_cost_frac, lr_cost_int].
// stamp must be StampLength bytes.
func AnnounceExt(stamp []byte, lrCost float64) []byte {
	cost := CostToBytes(lrCost)
	ext := make([]byte, 0, ExtPayloadLen)
	ext = append(ext, ExtPayloadMarker, 0x00)
	ext = append(ext, stamp[:StampLength]...)
	ext = append(ext, cost[1], cost[0])
	return ext
}

// ParseAnnounceExt inspects the trailing ext payload of announce app_data,
// matching Identity.validate_announce's stamp block (1.6.0).
//
// stamped reports the trailer had the known format (marker 0xC1, anchor MSB
// clear, lr_cost <= StampCostCap). When stamped, stamp carries ext[2:16] and
// value the stamp's computed value. valid reports the stamp satisfied
// requiredCost; only then is stripped set (app_data minus the trailer, nil
// when empty) and lrCost populated. When the format is unknown or the stamp
// is invalid, stripped is nil and callers must keep the original app_data.
// material = destHash || pubKey || nameHash with expand_rounds=0.
func ParseAnnounceExt(appData, destHash, pubKey, nameHash []byte, requiredCost float64) (stripped []byte, stamped, valid bool, stamp []byte, value float64, lrCost float64) {
	if len(appData) < ExtPayloadLen {
		return nil, false, false, nil, 0, 0
	}
	ext := appData[len(appData)-ExtPayloadLen:]
	anchor := ext[ExtPayloadLen-1]
	if anchor&0x80 != 0 {
		return nil, false, false, nil, 0, 0
	}
	lr := CostFromBytes([]byte{anchor & 0x7F, ext[ExtPayloadLen-2]})
	if ext[0] != ExtPayloadMarker || lr > StampCostCap {
		return nil, false, false, nil, 0, 0
	}

	stamp = append([]byte(nil), ext[ExtHeaderLen:ExtHeaderLen+StampLength]...)
	material := make([]byte, 0, len(destHash)+len(pubKey)+len(nameHash))
	material = append(material, destHash...)
	material = append(material, pubKey...)
	material = append(material, nameHash...)

	value = StampValue(material, stamp)
	if !StampValid(stamp, requiredCost, material) {
		return nil, true, false, stamp, value, 0
	}

	stripped = appData[:len(appData)-ExtPayloadLen]
	if len(stripped) == 0 {
		stripped = nil
	}
	return stripped, true, true, stamp, value, lr
}
