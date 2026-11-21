// SPDX-License-Identifier: LicenseRef-Reticulum
package liblxmf

import (
	"fmt"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

type messageRecord struct {
	msg *lxmf.LXMessage
}

func MessageCreate(dest, source []byte, title, content string) (uint64, int) {
	if len(dest) != lxmf.DestinationLength || len(source) != lxmf.DestinationLength {
		return 0, setLastError(fmt.Errorf("%w: hash length", errInvalidArg))
	}
	msg, err := lxmf.NewMessage(dest, source, []byte(title), []byte(content), nil)
	if err != nil {
		return 0, setLastError(err)
	}
	return handles.insert(kindMessage, &messageRecord{msg: msg}), OK
}

func MessagePack(messageHandle, identityHandle uint64) ([]byte, int) {
	rec, err := messageByHandle(messageHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	id, code := identityFromHandle(identityHandle)
	if code != OK {
		return nil, code
	}
	data, err := rec.msg.Pack(id)
	if err != nil {
		return nil, setLastError(err)
	}
	return data, OK
}

func MessageEncryptedPayload(messageHandle uint64) ([]byte, int) {
	rec, err := messageByHandle(messageHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	data, err := rec.msg.EncryptedPayload()
	if err != nil {
		return nil, setLastError(err)
	}
	return data, OK
}

func MessageUnpack(data []byte) (uint64, int) {
	msg, err := lxmf.Unpack(data, nil)
	if err != nil {
		return 0, setLastError(err)
	}
	return handles.insert(kindMessage, &messageRecord{msg: msg}), OK
}

func MessageGetDest(handle uint64) ([]byte, int) {
	rec, err := messageByHandle(handle)
	if err != nil {
		return nil, setLastError(err)
	}
	return append([]byte(nil), rec.msg.DestinationHash...), OK
}

func MessageGetSource(handle uint64) ([]byte, int) {
	rec, err := messageByHandle(handle)
	if err != nil {
		return nil, setLastError(err)
	}
	return append([]byte(nil), rec.msg.SourceHash...), OK
}

func MessageGetTitle(handle uint64) (string, int) {
	rec, err := messageByHandle(handle)
	if err != nil {
		return "", setLastError(err)
	}
	return rec.msg.TitleString(), OK
}

func MessageGetContent(handle uint64) (string, int) {
	rec, err := messageByHandle(handle)
	if err != nil {
		return "", setLastError(err)
	}
	return rec.msg.ContentString(), OK
}

// MessageGetHash returns the 32-byte LXMF message id after Pack or Unpack.
func MessageGetHash(handle uint64) ([]byte, int) {
	rec, err := messageByHandle(handle)
	if err != nil {
		return nil, setLastError(err)
	}
	if len(rec.msg.Hash) == 0 {
		return nil, setLastError(fmt.Errorf("%w: message has no hash yet", errInvalidArg))
	}
	return append([]byte(nil), rec.msg.Hash...), OK
}

func MessageDestroy(handle uint64) int {
	if !handles.delete(handle) {
		return setLastError(errInvalidHandle)
	}
	return OK
}

func messageByHandle(id uint64) (*messageRecord, error) {
	ref, err := handles.get(id, kindMessage)
	if err != nil {
		return nil, err
	}
	return ref.(*messageRecord), nil
}
