// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import "strings"

// Mode selects IDS detect IPS prevent or smart auto behavior.
type Mode int

const (
	// ModeOff disables protect checks.
	ModeOff Mode = iota
	// ModeDetect records trips and warns on stdout without blocking.
	ModeDetect
	// ModePrevent records trips warns and blocks.
	ModePrevent
	// ModeAuto learns quietly then arms prevent and relearns on change.
	ModeAuto
)

// String returns the config value for m.
func (m Mode) String() string {
	switch m {
	case ModeDetect:
		return "detect"
	case ModePrevent:
		return "prevent"
	case ModeAuto:
		return "auto"
	default:
		return "off"
	}
}

// ParseMode maps a config string to Mode. Unknown values yield ModeOff and false.
func ParseMode(s string) (Mode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off", "no", "false", "0":
		return ModeOff, true
	case "detect", "ids":
		return ModeDetect, true
	case "prevent", "block", "ips":
		return ModePrevent, true
	case "auto", "smart":
		return ModeAuto, true
	default:
		return ModeOff, false
	}
}
