// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

// MessagingInteropFields returns a representative map covering native LXMF field types.
// innerPacked may be nil to omit FieldEmbeddedLXMs.
func MessagingInteropFields(innerPacked []byte) map[byte]any {
	msgHash := make([]byte, 32)
	_, _ = rand.Read(msgHash)

	fields := map[byte]any{
		FieldThread:       append([]byte(nil), msgHash...),
		FieldReplyTo:      append([]byte(nil), msgHash...),
		FieldReplyQuote:   []byte("quoted interop text"),
		FieldRenderer:     []byte{RendererMarkdown},
		FieldComment:      map[byte]any{CommentFor: append([]byte(nil), msgHash...)},
		FieldContinuation: map[byte]any{ContinuationOf: append([]byte(nil), msgHash...)},
		FieldReaction: map[byte]any{
			ReactionTo:      append([]byte(nil), msgHash...),
			ReactionContent: []byte("ok"),
		},
		FieldCustomType: []byte("demo"),
		FieldCustomData: []byte{0xde, 0xad, 0xbe, 0xef},
		FieldCustomMeta: map[byte]any{0x00: []byte("meta")},
		FieldTicket:     append([]byte(nil), interopBytesRepeat(0x33, TicketLength)...),
		FieldAudio:      []any{int(AudioOpusPTT), []byte{0x01, 0x02, 0x03}},
		FieldImage:      []any{[]byte("png"), []byte{0x89, 0x50, 0x4e, 0x47}},
		FieldIconAppearance: map[byte]any{
			0x00: []byte("emoji"),
			0x01: []byte{0x89, 0x50, 0x4e, 0x47},
		},
		FieldFileAttachments: []any{
			map[byte]any{
				0x00: []byte("report.pdf"),
				0x01: []byte("attachment-bytes"),
			},
		},
		FieldTelemetry: map[byte]any{
			0x00: 42.5,
			0x01: []byte{0x01, 0x02},
		},
		FieldTelemetryStream: []any{[]byte("stream-chunk")},
		FieldCommands:        []any{[]byte("ping"), []any{[]byte("arg1")}},
		FieldResults:         []any{[]byte("pong")},
		FieldGroup:           []byte("group-1"),
		FieldEvent: map[byte]any{
			0x00: []byte("meeting"),
			0x01: 1700000100.0,
		},
		FieldRNRRefs:     []any{[]byte{0xaa, 0xbb}},
		FieldNonSpecific: []byte("non-specific"),
		FieldDebug:       []byte("debug"),
	}
	if len(innerPacked) > 0 {
		fields[FieldEmbeddedLXMs] = [][]byte{append([]byte(nil), innerPacked...)}
	}
	return fields
}

// MessagingInteropFieldsPython returns harness-style field JSON for cmd pack.
func MessagingInteropFieldsPython(innerPackedHex string) map[string]any {
	msgHash := interopBytesRepeat(0xAB, 32)
	fields := map[string]any{
		"0x08": "hex:" + hex.EncodeToString(msgHash),
		"0x30": "hex:" + hex.EncodeToString(msgHash),
		"0x31": "quoted interop text",
		"0x0f": []int{int(RendererMarkdown)},
		"0x41": map[string]any{"0x00": "hex:" + hex.EncodeToString(msgHash)},
		"0x42": map[string]any{"0x00": "hex:" + hex.EncodeToString(msgHash)},
		"0x40": map[string]any{
			"0x00": "hex:" + hex.EncodeToString(msgHash),
			"0x01": "ok",
		},
		"0xfb": "hex:" + hex.EncodeToString([]byte("demo")),
		"0xfc": "hex:deadbeef",
		"0xfd": map[string]any{"0x00": "hex:" + hex.EncodeToString([]byte("meta"))},
		"0x0c": "hex:" + hex.EncodeToString(interopBytesRepeat(0x33, TicketLength)),
		"0x07": []any{int(AudioOpusPTT), "hex:010203"},
		"0x06": []any{"hex:" + hex.EncodeToString([]byte("png")), "hex:89504e47"},
		"0x04": map[string]any{
			"0x00": "hex:" + hex.EncodeToString([]byte("emoji")),
			"0x01": "hex:89504e47",
		},
		"0x05": []any{
			map[string]any{
				"0x00": "hex:" + hex.EncodeToString([]byte("report.pdf")),
				"0x01": "hex:" + hex.EncodeToString([]byte("attachment-bytes")),
			},
		},
		"0x02": map[string]any{
			"0x00": 42.5,
			"0x01": "hex:0102",
		},
		"0x03": []any{"hex:" + hex.EncodeToString([]byte("stream-chunk"))},
		"0x09": []any{"hex:" + hex.EncodeToString([]byte("ping")), []any{"hex:" + hex.EncodeToString([]byte("arg1"))}},
		"0x0a": []any{"hex:" + hex.EncodeToString([]byte("pong"))},
		"0x0b": "hex:" + hex.EncodeToString([]byte("group-1")),
		"0x0d": map[string]any{
			"0x00": "hex:" + hex.EncodeToString([]byte("meeting")),
			"0x01": 1700000100.0,
		},
		"0x0e": []any{"hex:aabb"},
		"0xfe": "hex:" + hex.EncodeToString([]byte("non-specific")),
		"0xff": "hex:" + hex.EncodeToString([]byte("debug")),
	}
	if innerPackedHex != "" {
		fields["0x01"] = []any{"hex:" + innerPackedHex}
	}
	return fields
}

// MessagingInteropFieldKeys lists every field ID used by MessagingInteropFields.
func MessagingInteropFieldKeys(includeEmbedded bool) []byte {
	keys := []byte{
		FieldThread, FieldReplyTo, FieldReplyQuote, FieldRenderer,
		FieldComment, FieldContinuation, FieldReaction,
		FieldCustomType, FieldCustomData, FieldCustomMeta, FieldTicket,
		FieldAudio, FieldImage, FieldIconAppearance, FieldFileAttachments,
		FieldTelemetry, FieldTelemetryStream, FieldCommands, FieldResults,
		FieldGroup, FieldEvent, FieldRNRRefs, FieldNonSpecific, FieldDebug,
	}
	if includeEmbedded {
		keys = append([]byte{FieldEmbeddedLXMs}, keys...)
	}
	return keys
}

func interopBytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

// DeliveryHash returns the lxmf delivery destination hash for an identity.
func DeliveryHash(id *identity.Identity) ([]byte, error) {
	if id == nil {
		return nil, fmt.Errorf("identity nil")
	}
	dest, err := destination.New(id, destination.Out, destination.Single, AppName, nil, "delivery")
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), dest.GetHash()...), nil
}

// PackMessagingInterop builds a signed message carrying MessagingInteropFields.
func PackMessagingInterop(src, dst *identity.Identity, includeEmbedded bool) ([]byte, error) {
	srcHash, err := DeliveryHash(src)
	if err != nil {
		return nil, err
	}
	dstHash, err := DeliveryHash(dst)
	if err != nil {
		return nil, err
	}
	var innerPacked []byte
	if includeEmbedded {
		inner, err := NewMessage(dstHash, srcHash, []byte("inner"), []byte("embedded"), nil)
		if err != nil {
			return nil, err
		}
		inner.Timestamp = 1700000098.0
		innerPacked, err = inner.Pack(src)
		if err != nil {
			return nil, err
		}
	}
	fields := MessagingInteropFields(innerPacked)
	msg, err := NewMessage(dstHash, srcHash, []byte("interop"), []byte("messaging fields"), fields)
	if err != nil {
		return nil, err
	}
	msg.Timestamp = 1700000100.0
	return msg.Pack(src)
}
