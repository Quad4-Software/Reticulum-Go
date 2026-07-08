// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package server

import (
	"fmt"
	"time"
)

// FormatDuration renders d as a short human string (e.g. 6h, 1h 30m, 45s).
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "0"
	}
	if d < time.Hour {
		m := int(d / time.Minute)
		s := int((d % time.Minute) / time.Second)
		if m == 0 {
			if s < 1 {
				s = 1
			}
			return fmt.Sprintf("%ds", s)
		}
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := d / time.Hour
	r := d % time.Hour
	if r < time.Minute {
		return fmt.Sprintf("%dh", h)
	}
	m := r / time.Minute
	s := (r % time.Minute) / time.Second
	if s == 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dh %dm %ds", h, m, s)
}
