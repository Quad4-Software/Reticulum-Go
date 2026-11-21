// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

type ID [rrc.IdentityLength]byte

func idFrom(b []byte) (ID, bool) {
	var id ID
	if len(b) != rrc.IdentityLength {
		return id, false
	}
	copy(id[:], b)
	return id, true
}

func (id ID) Bytes() []byte {
	out := make([]byte, rrc.IdentityLength)
	copy(out, id[:])
	return out
}

func (id ID) Hex() string {
	return hex.EncodeToString(id[:])
}

func (id ID) Prefix(n int) string {
	s := id.Hex()
	if n <= 0 || n >= len(s) {
		return s
	}
	return s[:n]
}

func parseIdentityHash(text string) ([]byte, error) {
	s := strings.TrimSpace(strings.ToLower(text))
	s = strings.TrimPrefix(s, "0x")
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid identity hash %q", text)
	}
	if len(b) < 4 {
		return nil, fmt.Errorf("identity hash too short: %q", text)
	}
	return b, nil
}

func parseFullID(text string) (ID, error) {
	b, err := parseIdentityHash(text)
	if err != nil {
		return ID{}, err
	}
	id, ok := idFrom(b)
	if !ok {
		return ID{}, fmt.Errorf("identity hash must be %d bytes", rrc.IdentityLength)
	}
	return id, nil
}

func hasHexPrefix(token string) (prefix []byte, ok bool) {
	s := strings.TrimSpace(strings.ToLower(token))
	s = strings.TrimPrefix(s, "0x")
	if len(s) < 6 {
		return nil, false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return nil, false
		}
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return b, true
}
