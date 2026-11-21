// SPDX-License-Identifier: LicenseRef-Reticulum

package session

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func ParseHash(s string) ([]byte, error) {
	raw, err := decodeHashHex(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidHash, err)
	}
	if len(raw) != proto.IdentityHashLen && len(raw) != proto.DestHashLen {
		return nil, fmt.Errorf("%w: length %d", ErrInvalidHash, len(raw))
	}
	return raw, nil
}

func decodeHashHex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '<' && s[len(s)-1] == '>' {
		s = s[1 : len(s)-1]
	}
	var buf [128]byte
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ':' || c == ' ' || c == '-' {
			continue
		}
		if n >= len(buf) {
			return nil, fmt.Errorf("too long")
		}
		buf[n] = c
		n++
	}
	if n == 0 {
		return nil, fmt.Errorf("empty")
	}
	out := make([]byte, hex.DecodedLen(n))
	if _, err := hex.Decode(out, buf[:n]); err != nil {
		return nil, err
	}
	return out, nil
}
