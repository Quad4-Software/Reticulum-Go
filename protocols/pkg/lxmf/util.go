// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// HexHash returns lower-case hex, or "<empty>" for nil/empty input.
func HexHash(b []byte) string {
	if len(b) == 0 {
		return "<empty>"
	}
	return hex.EncodeToString(b)
}

// PrettyHexRep returns the upstream "<hex>" log/filename form.
func PrettyHexRep(b []byte) string {
	return "<" + hex.EncodeToString(b) + ">"
}

// MessageStoreFilename builds the propagation store filename transientid_received_stamp.
func MessageStoreFilename(transientID []byte, receivedAt float64, stampValue int64) string {
	return fmt.Sprintf("%s_%s_%d", hex.EncodeToString(transientID), formatFloat(receivedAt), stampValue)
}

// ParseMessageStoreFilename parses MessageStoreFilename output.
func ParseMessageStoreFilename(name string) (transientID []byte, receivedAt float64, stampValue int64, err error) {
	parts := strings.Split(name, "_")
	if len(parts) != 3 {
		return nil, 0, 0, fmt.Errorf("lxmf: invalid message store filename %q", name)
	}
	transientID, err = hex.DecodeString(parts[0])
	if err != nil {
		return nil, 0, 0, fmt.Errorf("lxmf: invalid transient id in %q: %w", name, err)
	}
	receivedAt, err = parseFloat(parts[1])
	if err != nil {
		return nil, 0, 0, fmt.Errorf("lxmf: invalid receive timestamp in %q: %w", name, err)
	}
	stampValue, err = parseInt64(parts[2])
	if err != nil {
		return nil, 0, 0, fmt.Errorf("lxmf: invalid stamp value in %q: %w", name, err)
	}
	return transientID, receivedAt, stampValue, nil
}

func formatFloat(f float64) string {
	s := fmt.Sprintf("%.6f", f)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		s = "0"
	}
	return s
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	if err != nil {
		return 0, err
	}
	return f, nil
}

func parseInt64(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return 0, err
	}
	return v, nil
}
