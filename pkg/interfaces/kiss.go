// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package interfaces

func escapeKISS(data []byte) []byte {
	escaped := make([]byte, 0, len(data)*2)
	for _, b := range data {
		if b == KISSFend {
			escaped = append(escaped, KISSFesc, KISSTFend)
		} else if b == KISSFesc {
			escaped = append(escaped, KISSFesc, KISSTFesc)
		} else {
			escaped = append(escaped, b)
		}
	}
	return escaped
}
