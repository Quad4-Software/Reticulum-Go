// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package server

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m 30s"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h 30m"},
		{6 * time.Hour, "6h"},
		{360 * time.Minute, "6h"},
	}
	for _, tc := range cases {
		if got := FormatDuration(tc.d); got != tc.want {
			t.Errorf("FormatDuration(%v) = %q want %q", tc.d, got, tc.want)
		}
	}
}
