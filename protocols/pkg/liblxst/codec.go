// SPDX-License-Identifier: LicenseRef-Reticulum
package liblxst

import (
	"errors"
	"fmt"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func PackSignalling(signals []int) ([]byte, int) {
	if len(signals) == 0 {
		return nil, setLastError(fmt.Errorf("%w: empty signals", errInvalidArg))
	}
	data, err := proto.PackSignalling(signals)
	if err != nil {
		return nil, setLastError(mapErr(err))
	}
	return data, OK
}

func PackFrame(codec byte, payload []byte) ([]byte, int) {
	data, err := proto.PackFrame(codec, payload)
	if err != nil {
		return nil, setLastError(mapErr(err))
	}
	return data, OK
}

func Unpack(data []byte) (uint64, int) {
	pkt, err := proto.Unpack(data)
	if err != nil {
		return 0, setLastError(mapErr(err))
	}
	return handles.insert(kindPacket, pkt), OK
}

func PacketDestroy(handle uint64) int {
	if !handles.delete(handle) {
		return setLastError(errInvalidHandle)
	}
	return OK
}

func packetAt(handle uint64) (proto.Packet, int) {
	ref, err := handles.get(handle, kindPacket)
	if err != nil {
		return proto.Packet{}, setLastError(err)
	}
	pkt, ok := ref.(proto.Packet)
	if !ok {
		return proto.Packet{}, setLastError(errInvalidHandle)
	}
	return pkt, OK
}

func PacketSignalCount(handle uint64) (int, int) {
	pkt, code := packetAt(handle)
	if code != OK {
		return 0, code
	}
	return len(pkt.Signals), OK
}

func PacketSignalAt(handle uint64, index int) (int, int) {
	pkt, code := packetAt(handle)
	if code != OK {
		return 0, code
	}
	if index < 0 || index >= len(pkt.Signals) {
		return 0, setLastError(fmt.Errorf("%w: signal index", errInvalidArg))
	}
	return pkt.Signals[index], OK
}

func PacketFrameCount(handle uint64) (int, int) {
	pkt, code := packetAt(handle)
	if code != OK {
		return 0, code
	}
	return len(pkt.Frames), OK
}

func PacketFrameAt(handle uint64, index int) ([]byte, int) {
	pkt, code := packetAt(handle)
	if code != OK {
		return nil, code
	}
	if index < 0 || index >= len(pkt.Frames) {
		return nil, setLastError(fmt.Errorf("%w: frame index", errInvalidArg))
	}
	return append([]byte(nil), pkt.Frames[index]...), OK
}

func SplitFrame(frame []byte) (byte, []byte, int) {
	codec, payload, err := proto.SplitFrame(frame)
	if err != nil {
		return 0, nil, setLastError(mapErr(err))
	}
	return codec, payload, OK
}

func TelephonyHash(identityHash []byte) ([]byte, int) {
	if len(identityHash) != proto.IdentityHashLen {
		return nil, setLastError(fmt.Errorf("%w: identity hash length", errInvalidArg))
	}
	return append([]byte(nil), proto.TelephonyHash(identityHash)...), OK
}

func DestHash(identityHash []byte, appName, aspect string) ([]byte, int) {
	if len(identityHash) != proto.IdentityHashLen {
		return nil, setLastError(fmt.Errorf("%w: identity hash length", errInvalidArg))
	}
	if appName == "" {
		return nil, setLastError(fmt.Errorf("%w: app name", errInvalidArg))
	}
	return append([]byte(nil), proto.DestHash(identityHash, appName, aspect)...), OK
}

func SignalPreferredMode(mode int) int {
	return proto.SignalPreferredMode(mode)
}

func SignalPreferredProfile(profile int) int {
	return proto.SignalPreferredProfile(profile)
}

func ProfileFromName(name string) int {
	return proto.ProfileFromName(name)
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, proto.ErrEmptyPacket),
		errors.Is(err, proto.ErrMissingFields),
		errors.Is(err, proto.ErrPacketTooLarge):
		return fmt.Errorf("%w: %v", errInvalidArg, err)
	default:
		return err
	}
}
