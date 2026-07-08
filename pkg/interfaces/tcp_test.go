// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"testing"
)

func TestEscapeHDLC(t *testing.T) {
	testCases := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"NoEscape", []byte{0x01, 0x02, 0x03}, []byte{0x01, 0x02, 0x03}},
		{"EscapeFlag", []byte{0x01, HDLCFlag, 0x03}, []byte{0x01, HDLCEsc, HDLCFlag ^ HDLCEscMask, 0x03}},
		{"EscapeEsc", []byte{0x01, HDLCEsc, 0x03}, []byte{0x01, HDLCEsc, HDLCEsc ^ HDLCEscMask, 0x03}},
		{"EscapeBoth", []byte{HDLCFlag, HDLCEsc}, []byte{HDLCEsc, HDLCFlag ^ HDLCEscMask, HDLCEsc, HDLCEsc ^ HDLCEscMask}},
		{"Empty", []byte{}, []byte{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := escapeHDLC(tc.input)
			if !bytes.Equal(result, tc.expected) {
				t.Errorf("escapeHDLC(%x) = %x; want %x", tc.input, result, tc.expected)
			}
		})
	}
}

func TestEscapeKISS(t *testing.T) {
	testCases := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{"NoEscape", []byte{0x01, 0x02, 0x03}, []byte{0x01, 0x02, 0x03}},
		{"EscapeFEND", []byte{0x01, KISSFend, 0x03}, []byte{0x01, KISSFesc, KISSTFend, 0x03}},
		{"EscapeFESC", []byte{0x01, KISSFesc, 0x03}, []byte{0x01, KISSFesc, KISSTFesc, 0x03}},
		{"EscapeBoth", []byte{KISSFend, KISSFesc}, []byte{KISSFesc, KISSTFend, KISSFesc, KISSTFesc}},
		{"Empty", []byte{}, []byte{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := escapeKISS(tc.input)
			if !bytes.Equal(result, tc.expected) {
				t.Errorf("escapeKISS(%x) = %x; want %x", tc.input, result, tc.expected)
			}
		})
	}
}
