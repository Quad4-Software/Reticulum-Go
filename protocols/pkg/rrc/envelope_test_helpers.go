// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import "github.com/fxamacker/cbor/v2"

func mustMarshalMap(m map[uint64]any) ([]byte, error) {
	return cbor.Marshal(m)
}

func mustMarshalWithExtra(env Envelope, key uint64, val any) ([]byte, error) {
	m := map[uint64]any{
		KeyVersion:   env.Version,
		KeyType:      env.Type,
		KeyMsgID:     env.MsgID,
		KeyTimestamp: env.Timestamp,
		KeySender:    env.Sender,
		key:          val,
	}
	if env.HasRoom {
		m[KeyRoom] = env.Room
	}
	if env.HasBody {
		m[KeyBody] = env.Body
	}
	if env.HasNick {
		m[KeyNick] = env.Nick
	}
	return cbor.Marshal(m)
}

func mustMarshalWithVersion(env *Envelope, version uint64) ([]byte, error) {
	m := map[uint64]any{
		KeyVersion:   version,
		KeyType:      env.Type,
		KeyMsgID:     env.MsgID,
		KeyTimestamp: env.Timestamp,
		KeySender:    env.Sender,
	}
	if env.HasRoom {
		m[KeyRoom] = env.Room
	}
	if env.HasBody {
		m[KeyBody] = env.Body
	}
	if env.HasNick {
		m[KeyNick] = env.Nick
	}
	return cbor.Marshal(m)
}
