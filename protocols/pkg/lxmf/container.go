// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// Container is msgpack packed_container metadata plus raw LXMF bytes (wire-compatible with upstream).
type Container struct {
	State               byte   `msgpack:"state"`
	LXMFBytes           []byte `msgpack:"lxmf_bytes"`
	TransportEncrypted  bool   `msgpack:"transport_encrypted"`
	TransportEncryption string `msgpack:"transport_encryption"`
	Method              byte   `msgpack:"method"`
}

// PackContainer msgpack-encodes msg's Packed bytes and delivery metadata. msg must already be packed.
func PackContainer(msg *LXMessage) ([]byte, error) {
	if msg == nil {
		return nil, errors.New("lxmf: nil message")
	}
	if len(msg.Packed) == 0 {
		return nil, errors.New("lxmf: message has not been packed")
	}
	c := MessageContainer(msg)
	return msgpack.Marshal(c)
}

// UnpackContainer decodes a container and unpacks lxmf_bytes like Unpack(resolver for signatures).
func UnpackContainer(data []byte, resolver SourceResolver) (Container, *LXMessage, error) {
	var c Container
	if err := msgpack.Unmarshal(data, &c); err != nil {
		return Container{}, nil, fmt.Errorf("lxmf: decode container: %w", err)
	}
	if len(c.LXMFBytes) == 0 {
		return c, nil, errors.New("lxmf: container missing lxmf_bytes")
	}
	msg, err := Unpack(c.LXMFBytes, resolver)
	if err != nil && msg == nil {
		return c, nil, err
	}
	msg.State = c.State
	msg.Method = c.Method
	if msg.Method != MethodUnknown && msg.Representation == RepresentationUnknown {
		msg.Representation = RepresentationPacket
	}
	return c, msg, err
}

// MessageContainer builds container metadata from msg without marshaling.
func MessageContainer(msg *LXMessage) Container {
	if msg == nil {
		return Container{}
	}
	encrypted, description := DetermineTransportEncryption(msg.Method, DestinationTypeSingle)
	return Container{
		State:               msg.State,
		LXMFBytes:           append([]byte(nil), msg.Packed...),
		TransportEncrypted:  encrypted,
		TransportEncryption: description,
		Method:              msg.Method,
	}
}

// WriteToDirectory writes PackContainer(msg) to dir as hex(hash) atomically. dir must exist.
func WriteToDirectory(msg *LXMessage, dir string) (string, error) {
	if msg == nil {
		return "", errors.New("lxmf: nil message")
	}
	if len(msg.Hash) == 0 {
		return "", errors.New("lxmf: message has no hash; pack it first")
	}
	data, err := PackContainer(msg)
	if err != nil {
		return "", err
	}
	name := hex.EncodeToString(msg.Hash)
	full := filepath.Join(dir, name)
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", fmt.Errorf("lxmf: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("lxmf: rename to %s: %w", full, err)
	}
	return full, nil
}

// ReadFromFile reads a container file and runs UnpackContainer.
func ReadFromFile(path string, resolver SourceResolver) (Container, *LXMessage, error) {
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- path is supplied by operator
	if err != nil {
		return Container{}, nil, err
	}
	return UnpackContainer(data, resolver)
}
