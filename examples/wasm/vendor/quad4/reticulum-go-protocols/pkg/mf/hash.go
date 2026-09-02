// SPDX-License-Identifier: 0BSD
package mf

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const hashGroup = 4

func ParseHash(s string) ([]byte, error) {
	raw, err := decodeHashHex(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidHash, err)
	}
	if len(raw) != SenderHashLength {
		return nil, fmt.Errorf("%w: length %d", ErrInvalidHash, len(raw))
	}
	return raw, nil
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
		return formatGroupedBytes(tmp, hashGroup)
	}
	hex.Encode(hexBuf[:n], hash)
	return formatGroupedBytes(hexBuf[:n], hashGroup)
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
