// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

import (
	"fmt"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

type envelopeRecord struct {
	env *rrc.Envelope
}

func EnvelopeCreate(msgType uint64, sender []byte) (uint64, int) {
	if len(sender) != rrc.IdentityLength {
		return 0, setLastError(fmt.Errorf("%w: sender hash", errInvalidArg))
	}
	env, err := rrc.NewEnvelope(msgType, sender)
	if err != nil {
		return 0, setLastError(err)
	}
	runtimeMu.Lock()
	id := handles.insert(kindEnvelope, &envelopeRecord{env: env})
	runtimeMu.Unlock()
	return id, OK
}

func EnvelopeSetRoom(handle uint64, room string) int {
	rec, err := envelopeByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	rec.env.Room = rrc.NormalizeRoom(room)
	rec.env.HasRoom = true
	return OK
}

func EnvelopeSetNick(handle uint64, nick string) int {
	rec, err := envelopeByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	rec.env.Nick = rrc.SanitizeNick(nick)
	rec.env.HasNick = true
	return OK
}

func EnvelopeSetBodyText(handle uint64, text string) int {
	rec, err := envelopeByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	rec.env.Body = text
	rec.env.HasBody = true
	return OK
}

func EnvelopeSetDestination(handle uint64, dest []byte) int {
	rec, err := envelopeByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	if len(dest) != rrc.IdentityLength {
		return setLastError(fmt.Errorf("%w: destination hash", errInvalidArg))
	}
	rec.env.Destination = append([]byte(nil), dest...)
	rec.env.HasDestination = true
	return OK
}

func EnvelopeGetType(handle uint64, out *uint64) int {
	rec, err := envelopeByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	if out == nil {
		return setLastError(errInvalidArg)
	}
	*out = rec.env.Type
	return OK
}

func EnvelopeGetSender(handle uint64) ([]byte, int) {
	rec, err := envelopeByHandle(handle)
	if err != nil {
		return nil, setLastError(err)
	}
	return append([]byte(nil), rec.env.Sender...), OK
}

func EnvelopeGetRoom(handle uint64) (string, int) {
	rec, err := envelopeByHandle(handle)
	if err != nil {
		return "", setLastError(err)
	}
	return rec.env.Room, OK
}

func EnvelopeGetNick(handle uint64) (string, int) {
	rec, err := envelopeByHandle(handle)
	if err != nil {
		return "", setLastError(err)
	}
	return rec.env.Nick, OK
}

func EnvelopeGetBodyText(handle uint64) (string, int) {
	rec, err := envelopeByHandle(handle)
	if err != nil {
		return "", setLastError(err)
	}
	if rec.env.Body == nil {
		return "", OK
	}
	text, ok := rrc.BodyAsString(rec.env.Body)
	if !ok {
		return "", OK
	}
	return text, OK
}

func EnvelopeMarshal(handle uint64) ([]byte, int) {
	rec, err := envelopeByHandle(handle)
	if err != nil {
		return nil, setLastError(err)
	}
	data, err := rec.env.Marshal()
	if err != nil {
		return nil, setLastError(err)
	}
	return data, OK
}

func EnvelopeUnmarshal(data []byte) (uint64, int) {
	env, err := rrc.UnmarshalEnvelope(data)
	if err != nil {
		return 0, setLastError(err)
	}
	runtimeMu.Lock()
	id := handles.insert(kindEnvelope, &envelopeRecord{env: env})
	runtimeMu.Unlock()
	return id, OK
}

func EnvelopeDestroy(handle uint64) int {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if !handles.delete(handle) {
		return setLastError(errInvalidHandle)
	}
	return OK
}

func NormalizeRoom(in string) (string, int) {
	return rrc.NormalizeRoom(in), OK
}

func SanitizeNick(in string) (string, int) {
	return rrc.SanitizeNick(in), OK
}

func envelopeByHandle(id uint64) (*envelopeRecord, error) {
	ref, err := handles.get(id, kindEnvelope)
	if err != nil {
		return nil, err
	}
	return ref.(*envelopeRecord), nil
}
