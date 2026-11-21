// SPDX-License-Identifier: LicenseRef-Reticulum
package liblxmf

import (
	"fmt"
	"maps"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

func MessageSetFields(handle uint64, fields map[byte]any) int {
	rec, err := messageByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	rec.msg.Fields = copyMessageFields(fields)
	return OK
}

func MessageSetFieldsJSON(handle uint64, jsonData []byte) int {
	fields, err := lxmf.FieldsFromHarnessJSON(jsonData)
	if err != nil {
		return setLastError(err)
	}
	return MessageSetFields(handle, fields)
}

func MessageFieldsJSON(handle uint64) ([]byte, int) {
	rec, err := messageByHandle(handle)
	if err != nil {
		return nil, setLastError(err)
	}
	data, err := lxmf.FieldsToHarnessJSON(rec.msg.Fields)
	if err != nil {
		return nil, setLastError(err)
	}
	return data, OK
}

func MessageFieldCount(handle uint64) (int, int) {
	rec, err := messageByHandle(handle)
	if err != nil {
		return 0, setLastError(err)
	}
	if rec.msg.Fields == nil {
		return 0, OK
	}
	return len(rec.msg.Fields), OK
}

func MessageUnpackVerified(data []byte, identityHandle uint64) (uint64, int) {
	if identityHandle != 0 {
		if code := IdentityRegisterRecall(identityHandle); code != OK {
			return 0, code
		}
	}
	msg, err := lxmf.Unpack(data, lxmf.RecallSource)
	if err != nil {
		return 0, setLastError(err)
	}
	if !msg.SignatureValidated {
		return 0, setLastError(fmt.Errorf("signature not validated"))
	}
	return handles.insert(kindMessage, &messageRecord{msg: msg}), OK
}

func copyMessageFields(in map[byte]any) map[byte]any {
	if in == nil {
		return nil
	}
	out := make(map[byte]any, len(in))
	maps.Copy(out, in)
	return out
}
