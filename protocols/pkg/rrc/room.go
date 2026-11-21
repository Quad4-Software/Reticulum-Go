// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"strings"
	"unicode"
)

// NormalizeRoom returns a case-insensitive room key: trim space and lowercase.
func NormalizeRoom(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// SanitizeNick trims and strips NUL/newlines from an advisory nickname.
func SanitizeNick(nick string) string {
	nick = strings.TrimSpace(nick)
	if nick == "" {
		return ""
	}
	for _, r := range nick {
		if r == 0 || r == '\n' || r == '\r' || unicode.IsControl(r) {
			return stripNickControls(nick)
		}
	}
	return nick
}

func stripNickControls(nick string) string {
	var b strings.Builder
	b.Grow(len(nick))
	for _, r := range nick {
		if r == 0 || r == '\n' || r == '\r' {
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
