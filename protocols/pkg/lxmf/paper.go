// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidURI means the URI is not a valid lxm:// paper URI or base64 payload.
var ErrInvalidURI = errors.New("lxmf: invalid paper URI")

// PaperURI returns an lxm:// URI with URL-safe unpadded base64, matching upstream LXMF and QR paper encoding.
// paperPacked is opaque ciphertext (e.g. from destination.Encrypt); this function only encodes bytes.
func PaperURI(paperPacked []byte) (string, error) {
	if len(paperPacked) == 0 {
		return "", errors.New("lxmf: empty paper payload")
	}
	if len(paperPacked) > PaperMDU {
		return "", fmt.Errorf("lxmf: paper payload of %d bytes exceeds PaperMDU (%d)", len(paperPacked), PaperMDU)
	}
	enc := base64.URLEncoding.EncodeToString(paperPacked)
	enc = strings.TrimRight(enc, "=")
	return uriSchemaPrefix + enc, nil
}

// DecodePaperURI parses an lxm:// URI and returns the decoded bytes (accepts canonical or padded base64).
func DecodePaperURI(uri string) ([]byte, error) {
	if !strings.HasPrefix(uri, uriSchemaPrefix) {
		return nil, fmt.Errorf("%w: missing %q prefix", ErrInvalidURI, uriSchemaPrefix)
	}
	encoded := uri[uriPrefixLen:]
	if encoded == "" {
		return nil, fmt.Errorf("%w: empty payload", ErrInvalidURI)
	}
	encoded = strings.TrimRight(encoded, "=")
	if pad := len(encoded) % 4; pad != 0 {
		encoded += strings.Repeat("=", 4-pad)
	}
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURI, err)
	}
	if len(decoded) > PaperMDU {
		return nil, fmt.Errorf("%w: paper payload of %d bytes exceeds PaperMDU (%d)", ErrInvalidURI, len(decoded), PaperMDU)
	}
	return decoded, nil
}
