// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !unix || wasm || haiku

package securemem

func allocLocked(n int) (data []byte, locked bool, native bool, err error) {
	return make([]byte, n), false, false, nil
}

func freeLocked(data []byte, locked bool, native bool) error {
	WipeBytes(data)
	return nil
}
