// SPDX-License-Identifier: LicenseRef-Reticulum
package liblxmf

import (
	"context"
	"fmt"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

// StampCostFromAppData wraps lxmf.StampCostFromAppData.
// Returns ErrInvalidArg when the cost is absent (ok=false). cost is 0 when absent.
func StampCostFromAppData(appData []byte) (int64, int) {
	cost, ok, err := lxmf.StampCostFromAppData(appData)
	if err != nil {
		return 0, setLastError(err)
	}
	if !ok {
		return 0, setLastError(fmt.Errorf("%w: stamp cost absent", errInvalidArg))
	}
	return cost, OK
}

// PNStampCostFromAppData wraps lxmf.PNStampCostFromAppData.
// Returns ErrInvalidArg when the cost is absent (ok=false). cost is 0 when absent.
func PNStampCostFromAppData(appData []byte) (int64, int) {
	cost, ok, err := lxmf.PNStampCostFromAppData(appData)
	if err != nil {
		return 0, setLastError(err)
	}
	if !ok {
		return 0, setLastError(fmt.Errorf("%w: pn stamp cost absent", errInvalidArg))
	}
	return cost, OK
}

// MessageApplyStamp pre-packs, generates a PoW stamp, then re-packs with the stamp embedded.
// stampCost <= 0 is a no-op that returns OK. timeoutMs <= 0 defaults to 60000.
func MessageApplyStamp(messageHandle, identityHandle uint64, stampCost, timeoutMs int) int {
	if stampCost <= 0 {
		return OK
	}
	rec, err := messageByHandle(messageHandle)
	if err != nil {
		return setLastError(err)
	}
	id, code := identityFromHandle(identityHandle)
	if code != OK {
		return code
	}
	if _, err := rec.msg.Pack(id); err != nil {
		return setLastError(fmt.Errorf("pre-pack: %w", err))
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeoutMs <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stamp, value, err := lxmf.GenerateStamp(ctx, rec.msg.Hash, stampCost, lxmf.WorkblockExpandRounds)
	if err != nil {
		return setLastError(fmt.Errorf("stamp generation: %w", err))
	}
	rec.msg.Stamp = stamp
	rec.msg.StampValue = value
	rec.msg.StampValid = true
	if _, err := rec.msg.Pack(id); err != nil {
		return setLastError(fmt.Errorf("re-pack: %w", err))
	}
	return OK
}

// EncodeAnnounceAppData builds v0.5 announce app data, optionally with icon appearance.
func EncodeAnnounceAppData(displayName string, stampCost int64, iconName string, fgRGB, bgRGB []byte) ([]byte, int) {
	data, err := lxmf.EncodeAnnounceAppDataWithIcon(displayName, stampCost, iconName, fgRGB, bgRGB)
	if err != nil {
		return nil, setLastError(err)
	}
	return data, OK
}
