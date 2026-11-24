// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import (
	"bytes"
	"encoding/hex"
	"strings"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/phonebook"
)

const fingerprintGroup = 4

func (c *Call) callerAllowed(id *identity.Identity) bool {
	if id == nil {
		return false
	}
	hash := id.Hash()
	for _, b := range c.cfg.Blocked {
		if bytes.Equal(b, hash) {
			return false
		}
	}
	if c.cfg.AllowFunc != nil {
		return c.cfg.AllowFunc(hash)
	}
	if c.cfg.AllowPolicy == phonebook.AllowNone {
		return false
	}
	if c.cfg.AllowPolicy == phonebook.AllowAll {
		return true
	}
	if c.cfg.AllowPolicy == 0 && len(c.cfg.Allowed) == 0 {
		return true
	}
	for _, a := range c.cfg.Allowed {
		if bytes.Equal(a, hash) {
			return true
		}
	}
	return false
}

// Fingerprint groups a 16-byte identity hash for read-back at RINGING.
func Fingerprint(id *identity.Identity) string {
	if id == nil {
		return ""
	}
	return FormatHash(id.Hash())
}

func FormatHash(hash []byte) string {
	if len(hash) == 0 {
		return ""
	}
	n := hex.EncodedLen(len(hash))
	var hexBuf [72]byte
	if n > len(hexBuf) {
		tmp := make([]byte, n)
		hex.Encode(tmp, hash)
		return formatGroupedBytes(tmp, fingerprintGroup)
	}
	hex.Encode(hexBuf[:n], hash)
	return formatGroupedBytes(hexBuf[:n], fingerprintGroup)
}

func formatGroupedBytes(hexs []byte, group int) string {
	var b strings.Builder
	groups := (len(hexs) + group - 1) / group
	b.Grow(len(hexs) + max(groups-1, 0))
	for i := 0; i < len(hexs); i += group {
		if i > 0 {
			b.WriteByte(' ')
		}
		end := min(i+group, len(hexs))
		b.Write(hexs[i:end])
	}
	return b.String()
}
