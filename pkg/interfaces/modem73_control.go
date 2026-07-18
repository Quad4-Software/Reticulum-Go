// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const (
	modem73MaxControlJSON = 1 << 20
	modem73ControlHdrLen  = 4
)

// Modem73WriteControl sends one length-prefixed JSON control message.
func Modem73WriteControl(w io.Writer, msg any) error {
	return modem73WriteControl(w, msg)
}

// Modem73ReadControl reads one length-prefixed JSON control object.
func Modem73ReadControl(r io.Reader) (map[string]any, error) {
	return modem73ReadControl(r)
}

// modem73EncodeControl writes a length-prefixed JSON control message.
func modem73EncodeControl(msg any) ([]byte, error) {
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if len(body) > modem73MaxControlJSON {
		return nil, fmt.Errorf("modem73 control message too large: %d", len(body))
	}
	out := make([]byte, modem73ControlHdrLen+len(body))
	binary.BigEndian.PutUint32(out[:modem73ControlHdrLen], uint32(len(body))) // #nosec G115 -- body capped at modem73MaxControlJSON
	copy(out[modem73ControlHdrLen:], body)
	return out, nil
}

// modem73WriteControl sends one control message on w.
func modem73WriteControl(w io.Writer, msg any) error {
	frame, err := modem73EncodeControl(msg)
	if err != nil {
		return err
	}
	_, err = w.Write(frame)
	return err
}

// modem73ReadControl reads one length-prefixed JSON object from r.
func modem73ReadControl(r io.Reader) (map[string]any, error) {
	var hdr [modem73ControlHdrLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > modem73MaxControlJSON {
		return nil, fmt.Errorf("modem73 control length %d exceeds cap %d", n, modem73MaxControlJSON)
	}
	if n == 0 {
		return map[string]any{}, nil
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	var msg map[string]any
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// modem73ComputeMTU derives hardware MTU from modem payload size.
func modem73ComputeMTU(payloadSize, overhead, floor int) int {
	raw := payloadSize - overhead
	if raw < floor {
		return floor
	}
	return raw
}

// modem73NeedsFragmentation reports whether payloadSize cannot carry floor MTU.
func modem73NeedsFragmentation(payloadSize, overhead, floor int) bool {
	return (payloadSize - overhead) < floor
}
