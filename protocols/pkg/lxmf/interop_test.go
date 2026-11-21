// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"os"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

func TestDecodeHarnessOutputSkipsUVPreamble(t *testing.T) {
	raw := []byte("Using CPython 3.12.3 interpreter at: /usr/bin/python3\nCreating virtual environment at: .venv\n{\"ok\": true, \"lxmf_version\": \"1.3.0\"}\n")
	var resp interopResponse
	if err := decodeHarnessOutput(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.LXMFVersion != "1.3.0" {
		t.Fatalf("resp=%+v", resp)
	}
}

func TestInterop_Ping(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "ping"})
	if resp.LXMFVersion == "" {
		t.Fatal("expected lxmf_version in ping response")
	}
	if resp.LXMFVersion != "1.3.0" {
		t.Fatalf("expected lxmf 1.3.0 ref, got %q", resp.LXMFVersion)
	}
}

func TestInterop_GoPackPythonUnpack(t *testing.T) {
	requireInterop(t)

	src, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	dst, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}

	msg, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("interop"), []byte("hello from go"), nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	msg.Timestamp = 1700000000.5
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	resp := interopCall(t, map[string]any{
		"cmd":        "unpack",
		"packed":     hex.EncodeToString(packed),
		"identities": interopIdentities(t, src),
	})

	if !resp.Message.SignatureValidated {
		t.Fatalf("python did not validate signature: reason=%v", resp.Message.UnverifiedReason)
	}
	if resp.Message.Title != "interop" || resp.Message.Content != "hello from go" {
		t.Fatalf("content mismatch: %#v", resp.Message)
	}
	if resp.Message.Hash != hex.EncodeToString(msg.Hash) {
		t.Fatalf("hash mismatch: go=%s py=%s", hex.EncodeToString(msg.Hash), resp.Message.Hash)
	}
}

func TestInterop_GoPackPythonUnpack_Fields(t *testing.T) {
	requireInterop(t)

	src, _ := identity.NewIdentity()
	dst, _ := identity.NewIdentity()

	replyHash := make([]byte, 32)
	if _, err := rand.Read(replyHash); err != nil {
		t.Fatal(err)
	}
	fields := map[byte]any{
		FieldRenderer: []byte{RendererMarkdown},
		FieldReplyTo:  replyHash,
		FieldReaction: map[byte]any{
			ReactionTo:      replyHash,
			ReactionContent: []byte("ok"),
		},
	}
	msg, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("fields"), []byte("payload"), fields)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	msg.Timestamp = 1700000001.0
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	resp := interopCall(t, map[string]any{
		"cmd":        "unpack",
		"packed":     hex.EncodeToString(packed),
		"identities": interopIdentities(t, src),
	})
	if !resp.Message.SignatureValidated {
		t.Fatal("signature not validated")
	}
	if len(resp.Message.Fields) == 0 {
		t.Fatal("fields missing after python unpack")
	}
}

func TestInterop_GoPackPythonUnpack_Stamp(t *testing.T) {
	requireInterop(t)

	src, _ := identity.NewIdentity()
	dst, _ := identity.NewIdentity()
	msg, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("stamped"), []byte("pow"), nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	msg.Timestamp = 1700000002.0
	if _, err := msg.Pack(src); err != nil {
		t.Fatalf("pre-pack: %v", err)
	}
	stamp, _, err := GenerateStamp(context.Background(), msg.Hash, 12, WorkblockExpandRounds)
	if err != nil {
		t.Fatalf("GenerateStamp: %v", err)
	}
	msg.Stamp = stamp
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatalf("Pack with stamp: %v", err)
	}

	resp := interopCall(t, map[string]any{
		"cmd":        "unpack",
		"packed":     hex.EncodeToString(packed),
		"identities": interopIdentities(t, src),
	})
	if resp.Message.Stamp == "" {
		t.Fatal("stamp missing after python unpack")
	}
	if resp.Message.Stamp != hex.EncodeToString(stamp) {
		t.Fatalf("stamp mismatch: go=%x py=%s", stamp, resp.Message.Stamp)
	}
}

func TestInterop_PythonPackGoUnpack(t *testing.T) {
	requireInterop(t)

	resp := interopCall(t, map[string]any{
		"cmd":       "pack",
		"title":     "from python",
		"content":   "hello from lxmf-ref",
		"timestamp": 1700000010.25,
	})
	registerInteropSource(t, resp.SourceHash, resp.SourcePublicKey)

	packed, err := hex.DecodeString(resp.Packed)
	if err != nil {
		t.Fatalf("decode packed: %v", err)
	}
	got, err := Unpack(packed, RecallSource)
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if !got.SignatureValidated {
		t.Fatalf("go did not validate python signature: reason=%d", got.UnverifiedReason)
	}
	if got.TitleString() != "from python" || got.ContentString() != "hello from lxmf-ref" {
		t.Fatalf("content mismatch: %q / %q", got.TitleString(), got.ContentString())
	}
	if hex.EncodeToString(got.Hash) != resp.Hash {
		t.Fatalf("hash mismatch")
	}
}

func TestInterop_PythonPackGoUnpack_Fields(t *testing.T) {
	requireInterop(t)

	threadID := hex.EncodeToString(bytes.Repeat([]byte{0xAB}, 32))
	resp := interopCall(t, map[string]any{
		"cmd":       "pack",
		"title":     "thread",
		"content":   "with fields",
		"timestamp": 1700000011.0,
		"fields": map[string]any{
			"0x08": "hex:" + threadID,
			"0x0f": []int{int(RendererBBCode)},
		},
	})
	registerInteropSource(t, resp.SourceHash, resp.SourcePublicKey)

	packed, _ := hex.DecodeString(resp.Packed)
	got, err := Unpack(packed, RecallSource)
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if !got.SignatureValidated {
		t.Fatal("signature not validated")
	}
	if got.Fields == nil {
		t.Fatal("fields missing")
	}
}

func TestInterop_StampWorkblock(t *testing.T) {
	requireInterop(t)

	material := bytes.Repeat([]byte{0x42}, 32)
	wbGo, err := StampWorkblock(material, WorkblockExpandRounds)
	if err != nil {
		t.Fatalf("StampWorkblock: %v", err)
	}
	resp := interopCall(t, map[string]any{
		"cmd":           "stamp_workblock",
		"material":      hex.EncodeToString(material),
		"expand_rounds": WorkblockExpandRounds,
	})
	wbPy, err := hex.DecodeString(resp.Workblock)
	if err != nil {
		t.Fatalf("decode workblock: %v", err)
	}
	if !bytes.Equal(wbGo, wbPy) {
		t.Fatalf("workblock mismatch (len go=%d py=%d)", len(wbGo), len(wbPy))
	}
}

func TestInterop_StampValidCross(t *testing.T) {
	requireInterop(t)

	material := bytes.Repeat([]byte{0x11}, 32)
	wb, err := StampWorkblock(material, WorkblockExpandRounds)
	if err != nil {
		t.Fatal(err)
	}
	stamp, _, err := GenerateStamp(context.Background(), material, 14, WorkblockExpandRounds)
	if err != nil {
		t.Fatal(err)
	}
	if !StampValid(stamp, 14, wb) {
		t.Fatal("go stamp_valid failed locally")
	}
	resp := interopCall(t, map[string]any{
		"cmd":         "stamp_valid",
		"stamp":       hex.EncodeToString(stamp),
		"workblock":   hex.EncodeToString(wb),
		"target_cost": 14,
	})
	if !resp.Valid {
		t.Fatal("python rejected go stamp")
	}

	pyStamp := interopCall(t, map[string]any{
		"cmd":           "generate_stamp",
		"message_id":    hex.EncodeToString(material),
		"stamp_cost":    14,
		"expand_rounds": WorkblockExpandRounds,
	})
	st, err := hex.DecodeString(pyStamp.Stamp)
	if err != nil {
		t.Fatal(err)
	}
	if !StampValid(st, 14, wb) {
		t.Fatal("go rejected python stamp")
	}
}

func TestInterop_StampValueCross(t *testing.T) {
	requireInterop(t)

	material := bytes.Repeat([]byte{0x22}, 32)
	wb, err := StampWorkblock(material, 64)
	if err != nil {
		t.Fatal(err)
	}
	stamp, goVal, err := GenerateStamp(context.Background(), material, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	resp := interopCall(t, map[string]any{
		"cmd":       "stamp_value",
		"stamp":     hex.EncodeToString(stamp),
		"workblock": hex.EncodeToString(wb),
	})
	if int(resp.Value) != goVal {
		t.Fatalf("stamp value mismatch: go=%d py=%v", goVal, resp.Value)
	}
}

func TestInterop_TicketStamp(t *testing.T) {
	requireInterop(t)

	ticket := make([]byte, TicketLength)
	messageID := make([]byte, 32)
	if _, err := rand.Read(ticket); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(messageID); err != nil {
		t.Fatal(err)
	}

	resp := interopCall(t, map[string]any{
		"cmd":        "ticket_stamp",
		"ticket":     hex.EncodeToString(ticket),
		"message_id": hex.EncodeToString(messageID),
	})
	pyStamp, err := hex.DecodeString(resp.Stamp)
	if err != nil {
		t.Fatal(err)
	}
	goStamp := truncatedHash(ticket, messageID)
	if !bytes.Equal(pyStamp, goStamp) {
		t.Fatalf("ticket stamp mismatch")
	}

	msg := &LXMessage{Hash: messageID, Stamp: pyStamp}
	ok, err := msg.ValidateStamp(32, [][]byte{ticket})
	if err != nil {
		t.Fatalf("ValidateStamp: %v", err)
	}
	if !ok || msg.StampValue != StampValueTicket {
		t.Fatalf("ticket validation failed: ok=%v value=%d", ok, msg.StampValue)
	}
}

func TestInterop_ContainerRoundTrip(t *testing.T) {
	requireInterop(t)

	src, _ := identity.NewIdentity()
	dst, _ := identity.NewIdentity()
	msg, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("container"), []byte("data"), nil)
	if err != nil {
		t.Fatal(err)
	}
	msg.Timestamp = 1700000020.0
	msg.Method = MethodDirect
	msg.State = StateOutbound
	if _, err := msg.Pack(src); err != nil {
		t.Fatal(err)
	}

	container, err := PackContainer(msg)
	if err != nil {
		t.Fatalf("PackContainer: %v", err)
	}

	resp := interopCall(t, map[string]any{
		"cmd":        "container_unpack",
		"container":  hex.EncodeToString(container),
		"identities": interopIdentities(t, src),
	})
	if resp.Message.Title != "container" {
		t.Fatalf("title mismatch: %q", resp.Message.Title)
	}

	pyPack := interopCall(t, map[string]any{
		"cmd":       "pack",
		"title":     "py-container",
		"content":   "in box",
		"timestamp": 1700000021.0,
	})
	pyContainer := interopCall(t, map[string]any{
		"cmd":    "packed_container",
		"packed": pyPack.Packed,
		"state":  StateDelivered,
		"method": MethodDirect,
	})
	registerInteropSource(t, pyPack.SourceHash, pyPack.SourcePublicKey)
	cdata, err := hex.DecodeString(pyContainer.Container)
	if err != nil {
		t.Fatal(err)
	}
	c, got, err := UnpackContainer(cdata, RecallSource)
	if err != nil {
		t.Fatalf("UnpackContainer: %v", err)
	}
	if got.TitleString() != "py-container" {
		t.Fatalf("title mismatch: %q", got.TitleString())
	}
	if c.State != StateDelivered {
		t.Fatalf("state mismatch: %d", c.State)
	}
}

func TestInterop_AnnounceAppData(t *testing.T) {
	requireInterop(t)

	encoded, err := EncodeAnnounceAppDataV5WithFeatures("interop-node", 24, []byte{SFCompression})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	resp := interopCall(t, map[string]any{
		"cmd":      "announce_decode",
		"app_data": hex.EncodeToString(encoded),
	})
	if resp.DisplayName != "interop-node" {
		t.Fatalf("display name: %q", resp.DisplayName)
	}

	sanitizedBlob, err := marshalAnnounceAppData([]any{[]byte("  interop\x00node  "), int64(24), []any{int(SFCompression)}})
	if err != nil {
		t.Fatalf("marshal sanitized announce: %v", err)
	}
	sanitizedResp := interopCall(t, map[string]any{
		"cmd":      "announce_decode",
		"app_data": hex.EncodeToString(sanitizedBlob),
	})
	if sanitizedResp.DisplayName != "interopnode" {
		t.Fatalf("sanitized display name: %q", sanitizedResp.DisplayName)
	}
	cost, ok := resp.StampCost.(float64)
	if !ok || int(cost) != 24 {
		t.Fatalf("stamp cost: %#v", resp.StampCost)
	}
	if comp, ok := resp.CompressionSupport.(bool); !ok || !comp {
		t.Fatalf("compression support: %#v", resp.CompressionSupport)
	}

	pyEnc := interopCall(t, map[string]any{
		"cmd":          "announce_encode",
		"display_name": "from-py",
		"stamp_cost":   16,
		"features":     []string{"0x00"},
	})
	pyData, err := hex.DecodeString(pyEnc.AppData)
	if err != nil {
		t.Fatal(err)
	}
	name, err := DisplayNameFromAppData(pyData)
	if err != nil || name != "from-py" {
		t.Fatalf("display name from py: %q err=%v", name, err)
	}
	costGo, ok, err := StampCostFromAppData(pyData)
	if err != nil || !ok || costGo != 16 {
		t.Fatalf("stamp cost from py: %d ok=%v err=%v", costGo, ok, err)
	}
	support, ok, err := CompressionSupportFromAppData(pyData)
	if err != nil || !ok || !support {
		t.Fatalf("compression from py: support=%v ok=%v err=%v", support, ok, err)
	}
}

func TestInterop_PaperURI(t *testing.T) {
	requireInterop(t)

	payload := bytes.Repeat([]byte{0x5A}, 128)
	uri, err := PaperURI(payload)
	if err != nil {
		t.Fatalf("PaperURI: %v", err)
	}
	resp := interopCall(t, map[string]any{
		"cmd":          "paper_uri",
		"paper_packed": hex.EncodeToString(payload),
	})
	if resp.URI != uri {
		t.Fatalf("paper URI mismatch:\ngo %s\npy %s", uri, resp.URI)
	}
	decoded, err := DecodePaperURI(uri)
	if err != nil {
		t.Fatalf("DecodePaperURI: %v", err)
	}
	if !bytes.Equal(decoded, payload) {
		t.Fatal("paper URI round-trip failed")
	}
}

func TestInterop_E2E_MessengerPythonUnpack(t *testing.T) {
	requireInterop(t)

	src, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dst, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	srcDest := outDeliveryHash(t, src)
	dstDest := outDeliveryHash(t, dst)
	identity.Remember(nil, dstDest, dst.GetPublicKey(), nil)
	identity.Remember(nil, srcDest, src.GetPublicKey(), nil)

	msg, err := NewMessage(dstDest, srcDest, []byte("e2e"), []byte("messenger path"), map[byte]any{
		FieldRenderer: []byte{RendererPlain},
	})
	if err != nil {
		t.Fatal(err)
	}
	msg.Timestamp = float64(time.Now().UnixNano()) / 1e9
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatal(err)
	}

	inner, err := msg.EncryptedPayload()
	if err != nil {
		t.Fatal(err)
	}
	if len(inner) < DestinationLength+SignatureLength {
		t.Fatal("encrypted payload too short")
	}

	resp := interopCall(t, map[string]any{
		"cmd":        "unpack",
		"packed":     hex.EncodeToString(packed),
		"identities": interopIdentities(t, src),
	})
	if !resp.Message.SignatureValidated {
		t.Fatal("python failed to validate messenger-style message")
	}
	if resp.Message.Content != "messenger path" {
		t.Fatalf("content: %q", resp.Message.Content)
	}

	got, err := Unpack(packed, RecallSource)
	if err != nil {
		t.Fatalf("go unpack of same message: %v", err)
	}
	if got.ContentString() != resp.Message.Content {
		t.Fatalf("go/python content diverged")
	}
}

func TestInterop_AllNativeFields(t *testing.T) {
	requireInterop(t)
	src, _ := identity.NewIdentity()
	dst, _ := identity.NewIdentity()
	msgHash := bytes.Repeat([]byte{0x11}, 32)
	fields := map[byte]any{
		FieldThread:       bytes.Repeat([]byte{0x22}, 32),
		FieldReplyTo:      msgHash,
		FieldReplyQuote:   []byte("quoted"),
		FieldRenderer:     []byte{RendererMarkdown},
		FieldComment:      map[byte]any{CommentFor: msgHash},
		FieldContinuation: map[byte]any{ContinuationOf: msgHash},
		FieldReaction: map[byte]any{
			ReactionTo:      msgHash,
			ReactionContent: []byte("ok"),
		},
		FieldCustomType: []byte("demo"),
		FieldCustomData: []byte{0xde, 0xad},
		FieldTicket:     bytes.Repeat([]byte{0x33}, TicketLength),
		FieldAudio:      []any{int(AudioOpusPTT), []byte{0x01, 0x02}},
		FieldImage:      []any{[]byte("png"), []byte{0x89, 0x50}},
	}
	msg, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("all"), []byte("fields"), fields)
	if err != nil {
		t.Fatal(err)
	}
	msg.Timestamp = 1700000099.0
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatal(err)
	}
	resp := interopCall(t, map[string]any{
		"cmd":        "unpack",
		"packed":     hex.EncodeToString(packed),
		"identities": interopIdentities(t, src),
	})
	if !resp.Message.SignatureValidated {
		t.Fatal("python did not validate native fields")
	}
	if len(resp.Message.Fields) < 8 {
		t.Fatalf("fields=%#v", resp.Message.Fields)
	}
}

func TestInterop_InvalidSignatureNotValidated(t *testing.T) {
	requireInterop(t)
	src, _ := identity.NewIdentity()
	dst, _ := identity.NewIdentity()
	msg, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("t"), []byte("c"), nil)
	if err != nil {
		t.Fatal(err)
	}
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatal(err)
	}
	packed[2*DestinationLength] ^= 0xff
	resp := interopCall(t, map[string]any{
		"cmd":        "unpack",
		"packed":     hex.EncodeToString(packed),
		"identities": interopIdentities(t, src),
	})
	if resp.Message.SignatureValidated {
		t.Fatal("python validated tampered signature")
	}
	got, err := Unpack(packed, RecallSource)
	if err == nil && got != nil && got.SignatureValidated {
		t.Fatal("go validated tampered signature")
	}
}

func TestInterop_PaperDecodeCross(t *testing.T) {
	requireInterop(t)
	payload := bytes.Repeat([]byte{0x7e}, 64)
	uri, err := PaperURI(payload)
	if err != nil {
		t.Fatal(err)
	}
	resp := interopCall(t, map[string]any{
		"cmd": "paper_decode",
		"uri": uri,
	})
	got, err := hex.DecodeString(resp.PaperPacked)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("python paper decode mismatch")
	}
}

func TestInterop_PNStampValidate(t *testing.T) {
	requireInterop(t)
	lxmData := bytes.Repeat([]byte{0x5a}, 128)
	tid := sha256.Sum256(lxmData)
	stamp, _, err := GenerateStamp(context.Background(), tid[:], 10, WorkblockExpandRoundsPN)
	if err != nil {
		t.Fatal(err)
	}
	transient := append(append([]byte(nil), lxmData...), stamp...)
	resp := interopCall(t, map[string]any{
		"cmd":            "validate_pn_stamp",
		"transient_data": hex.EncodeToString(transient),
		"target_cost":    10,
	})
	if !resp.Valid {
		t.Fatal("python rejected go PN stamp")
	}
	if resp.TransientID != hex.EncodeToString(tid[:]) {
		t.Fatalf("transient id mismatch py=%s", resp.TransientID)
	}
}

func TestInterop_BinaryContent(t *testing.T) {
	requireInterop(t)
	src, _ := identity.NewIdentity()
	dst, _ := identity.NewIdentity()
	content := []byte{0x00, 0xff, 0xfe, 'x'}
	msg, err := NewMessage(outDeliveryHash(t, dst), outDeliveryHash(t, src), []byte("bin"), content, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg.Timestamp = 1700000100
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatal(err)
	}
	resp := interopCall(t, map[string]any{
		"cmd":        "unpack",
		"packed":     hex.EncodeToString(packed),
		"identities": interopIdentities(t, src),
	})
	if !resp.Message.SignatureValidated {
		t.Fatal("signature")
	}
}

func TestInterop_Live_GoMessengerPythonRecv(t *testing.T) {
	if testing.Short() {
		t.Skip("live python interop skipped in -short mode")
	}
	requireInterop(t)

	const (
		goPort = 42940
		pyPort = 42941
		udpGo  = "LXMFIGO"
	)

	cfg := testconfig.New()
	cfg.Interfaces = map[string]*common.InterfaceConfig{
		udpGo: {Type: "UDPInterface", Enabled: true, Address: "127.0.0.1:0", TargetHost: "127.0.0.1:0", Name: udpGo},
	}
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatalf("transport: %v", err)
	}
	defer tr.Close()

	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}

	var iface interfaces.Interface
	iface, err = interfaces.NewUDPInterface(udpGo, "127.0.0.1:42940", "127.0.0.1:42941", true)
	if err != nil {
		t.Fatalf("udp: %v", err)
	}
	iface.SetPacketCallback(func(d []byte, ni common.NetworkInterface) { tr.HandlePacket(d, ni) })
	if err := iface.Start(); err != nil {
		t.Fatalf("udp start: %v", err)
	}
	defer iface.Stop()
	if ni, ok := iface.(common.NetworkInterface); ok {
		if err := tr.RegisterInterface(udpGo, ni); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	messenger, err := NewDeliveryMessenger(id, tr)
	if err != nil {
		t.Fatal(err)
	}

	readyPath := t.TempDir() + "/ready.json"
	wantText := "hello from go lxmf"
	type liveResult struct {
		resp interopResponse
		err  error
	}
	liveCh := make(chan liveResult, 1)
	go func() {
		resp, err := runInterop(map[string]any{
			"cmd":          "live_recv",
			"listen_port":  pyPort,
			"forward_port": goPort,
			"ready_path":   readyPath,
			"timeout_s":    40,
		})
		liveCh <- liveResult{resp, err}
	}()

	ready := waitReadyJSON(t, readyPath, 15*time.Second)
	dstHash, err := hex.DecodeString(ready["dest_hash"])
	if err != nil || len(dstHash) != DestinationLength {
		t.Fatalf("dest_hash=%q err=%v", ready["dest_hash"], err)
	}
	pub, err := hex.DecodeString(ready["public_key"])
	if err != nil || len(pub) == 0 {
		t.Fatalf("public_key=%q err=%v", ready["public_key"], err)
	}
	identity.Remember(nil, dstHash, pub, nil)

	deadline := time.Now().Add(20 * time.Second)
	for !tr.HasPath(dstHash) {
		if time.Now().After(deadline) {
			t.Fatal("path timeout to python lxmf dest")
		}
		_ = tr.RequestPath(dstHash, "", nil, true)
		_ = messenger.Destination().Announce(false, nil, nil)
		time.Sleep(100 * time.Millisecond)
	}

	if _, err := messenger.SendText(dstHash, "live", wantText); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case lr := <-liveCh:
		if lr.err != nil {
			t.Fatalf("live recv: %v", lr.err)
		}
		if !lr.resp.OK {
			t.Fatalf("live recv error: %s\n%s", lr.resp.Error, lr.resp.Trace)
		}
		if lr.resp.Text != wantText {
			t.Fatalf("python text=%q", lr.resp.Text)
		}
	case <-time.After(45 * time.Second):
		t.Fatal("timeout waiting for python lxmf result")
	}
}

func waitReadyJSON(t *testing.T, path string, timeout time.Duration) map[string]string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		raw, err := os.ReadFile(path)
		if err == nil && len(raw) > 0 {
			var out map[string]string
			if json.Unmarshal(raw, &out) == nil && len(out) > 0 {
				return out
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for ready file %s", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
