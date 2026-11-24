// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// Interop tests mapped to RRC 0.1.3 spec sections (https://rrc.kc1awv.net/).

func TestInterop_Spec_1_ProtocolVersionAndHubDest(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "constants"})
	c := resp.Constants
	if c == nil {
		t.Fatal("missing constants")
	}
	if intVal(c["rrc_version"]) != int(ProtocolVersion) {
		t.Fatalf("version=%v", c["rrc_version"])
	}
	if strVal(c["hub_dest"]) != AppName+"."+HubAspect {
		t.Fatalf("hub_dest=%v", c["hub_dest"])
	}
}

func TestInterop_Spec_3_EnvelopeKeyAssignments(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "constants"})
	keys, _ := resp.Constants["envelope_keys"].(map[string]any)
	expect := map[string]uint64{
		"version": KeyVersion, "type": KeyType, "msg_id": KeyMsgID,
		"timestamp": KeyTimestamp, "sender": KeySender, "room": KeyRoom,
		"body": KeyBody, "nick": KeyNick, "destination": KeyDestination,
	}
	for name, want := range expect {
		if intVal(keys[name]) != int(want) {
			t.Fatalf("key %s=%v want %d", name, keys[name], want)
		}
	}
}

func TestInterop_Spec_2_MessageTypeAssignments(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "constants"})
	types, _ := resp.Constants["message_types"].(map[string]any)
	expect := map[string]uint64{
		"hello": TypeHello, "welcome": TypeWelcome,
		"join": TypeJoin, "joined": TypeJoined, "part": TypePart, "parted": TypeParted,
		"msg": TypeMsg, "notice": TypeNotice, "action": TypeAction,
		"ping": TypePing, "pong": TypePong, "error": TypeError,
		"resource_envelope": TypeResourceEnvelope,
	}
	for name, want := range expect {
		if intVal(types[name]) != int(want) {
			t.Fatalf("type %s=%v want %d", name, types[name], want)
		}
	}
}

func TestInterop_Spec_3_HelloWelcomeBodyKeys(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "constants"})
	hello, _ := resp.Constants["hello_keys"].(map[string]any)
	welcome, _ := resp.Constants["welcome_keys"].(map[string]any)
	check := map[string]uint64{
		"name": HelloKeyClientName, "version": HelloKeyClientVersion,
		"capabilities": HelloKeyCapabilities, "nick_legacy": HelloKeyNickLegacy,
	}
	for k, want := range check {
		if intVal(hello[k]) != int(want) {
			t.Fatalf("hello %s=%v", k, hello[k])
		}
	}
	wcheck := map[string]uint64{
		"hub": WelcomeKeyHubName, "version": WelcomeKeyHubVersion,
		"capabilities": WelcomeKeyCapabilities, "limits": WelcomeKeyHubLimits,
	}
	for k, want := range wcheck {
		if intVal(welcome[k]) != int(want) {
			t.Fatalf("welcome %s=%v", k, welcome[k])
		}
	}
}

func TestInterop_Spec_3_WelcomeLimitKeys(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "constants"})
	lim, _ := resp.Constants["limit_keys"].(map[string]any)
	expect := map[string]uint64{
		"max_nick_bytes": LimitMaxNickBytes, "max_room_name_bytes": LimitMaxRoomNameBytes,
		"max_msg_body_bytes": LimitMaxMsgBodyBytes, "max_rooms_per_session": LimitMaxRoomsPerSession,
		"rate_limit_msgs_per_minute": LimitRateLimitMsgsPerMinute,
	}
	for k, want := range expect {
		if intVal(lim[k]) != int(want) {
			t.Fatalf("limit %s=%v", k, lim[k])
		}
	}
}

func TestInterop_Spec_3_CapabilityKeys(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "constants"})
	caps, _ := resp.Constants["capability_keys"].(map[string]any)
	if intVal(caps["resource_envelope"]) != int(CapResourceEnvelope) {
		t.Fatal("resource cap key")
	}
	if intVal(caps["action"]) != int(CapAction) {
		t.Fatal("action cap key")
	}
	if intVal(caps["direct_notice"]) != int(CapDirectNotice) {
		t.Fatal("direct notice cap key")
	}
}

func TestInterop_Spec_3_ResourceEnvelopeKeys(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "constants"})
	rk, _ := resp.Constants["resource_keys"].(map[string]any)
	expect := map[string]uint64{
		"id": ResourceKeyID, "kind": ResourceKeyKind, "size": ResourceKeySize,
		"sha256": ResourceKeySHA256, "encoding": ResourceKeyEncoding,
	}
	for k, want := range expect {
		if intVal(rk[k]) != int(want) {
			t.Fatalf("resource key %s=%v", k, rk[k])
		}
	}
	kinds, _ := resp.Constants["resource_kinds"].(map[string]any)
	if strVal(kinds["notice"]) != ResourceKindNotice {
		t.Fatal("notice kind")
	}
	if strVal(kinds["motd"]) != ResourceKindMOTD {
		t.Fatal("motd kind")
	}
	if strVal(kinds["blob"]) != ResourceKindBlob {
		t.Fatal("blob kind")
	}
}

func TestInterop_Spec_3_RoomNormalization(t *testing.T) {
	requireInterop(t)
	cases := []struct{ in, want string }{
		{"  #Lobby  ", "#lobby"},
		{"#OPS", "#ops"},
		{"lobby", "lobby"},
	}
	for _, tc := range cases {
		resp := interopCall(t, map[string]any{"cmd": "normalize_room", "room": tc.in})
		if resp.Normalized != tc.want {
			t.Fatalf("%q -> %q want %q", tc.in, resp.Normalized, tc.want)
		}
		if NormalizeRoom(tc.in) != tc.want {
			t.Fatalf("go normalize %q", tc.in)
		}
	}
}

func TestInterop_Spec_5_NickNormalization(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "normalize_nick", "nick": "  alice  ", "max_bytes": 32})
	if resp.Normalized != "alice" {
		t.Fatalf("nick=%q", resp.Normalized)
	}
	if SanitizeNick("  alice  ") != "alice" {
		t.Fatal("go sanitize")
	}
	bad := interopCall(t, map[string]any{"cmd": "normalize_nick", "nick": "bad\nnick", "max_bytes": 32})
	if bad.Normalized != "" {
		t.Fatalf("reject newline nick: %q", bad.Normalized)
	}
}

func TestInterop_Spec_3_GoRoundtripPythonReencode(t *testing.T) {
	requireInterop(t)
	sender := bytes.Repeat([]byte{0x77}, IdentityLength)
	types := []uint64{TypeHello, TypeWelcome, TypeJoin, TypeJoined, TypePart, TypeParted, TypeMsg, TypeNotice, TypeAction, TypePing, TypePong, TypeError, TypeResourceEnvelope}
	for _, typ := range types {
		env, err := NewEnvelope(typ, sender)
		if err != nil {
			t.Fatal(err)
		}
		env.MsgID = bytes.Repeat([]byte{0x01}, MessageIDLength)
		env.Timestamp = 1737849600000
		if typ >= TypeJoin && typ <= TypeAction {
			env.Room = "#lobby"
			env.HasRoom = true
		}
		switch typ {
		case TypeMsg, TypeNotice, TypeAction, TypeError, TypePing, TypePong:
			env.Body = "x"
			env.HasBody = true
		case TypeHello:
			env.Body = (&HelloBody{ClientName: "go", HasName: true}).ToMap()
			env.HasBody = true
		case TypeWelcome:
			env.Body = (&WelcomeBody{HubName: "h", HasName: true}).ToMap()
			env.HasBody = true
		case TypeResourceEnvelope:
			env.Body = (&ResourceEnvelopeBody{ID: []byte{1}, HasID: true, Kind: ResourceKindBlob, HasKind: true, Size: 1, HasSize: true}).ToMap()
			env.HasBody = true
		}
		packed, err := env.Marshal()
		if err != nil {
			t.Fatalf("type %d: %v", typ, err)
		}
		rt := interopCall(t, map[string]any{"cmd": "roundtrip", "packed": hex.EncodeToString(packed)})
		if !rt.RoundtripOK {
			t.Fatalf("roundtrip failed type %d packed2=%s", typ, rt.Packed2)
		}
	}
}

func TestInterop_Spec_3_PythonEncodeMatrixGoDecode(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "encode_matrix"})
	matrix := resp.Envelope
	if len(matrix) < 13 {
		t.Fatalf("matrix len=%d", len(matrix))
	}
	for key, hexPacked := range matrix {
		raw, err := hex.DecodeString(strVal(hexPacked))
		if err != nil {
			t.Fatalf("key %s: %v", key, err)
		}
		got, err := UnmarshalEnvelope(raw)
		if err != nil {
			t.Fatalf("key %s decode: %v", key, err)
		}
		if got.Version != ProtocolVersion {
			t.Fatalf("key %s version=%d", key, got.Version)
		}
		_ = interopCall(t, map[string]any{"cmd": "validate", "packed": hex.EncodeToString(raw)})
	}
}

func TestInterop_Spec_2_ResourceEnvelopeValidBody(t *testing.T) {
	requireInterop(t)
	sender := bytes.Repeat([]byte{0x88}, IdentityLength)
	interopCall(t, map[string]any{
		"cmd":    "validate_resource",
		"sender": hex.EncodeToString(sender),
		"body": bodyToJSON(map[uint64]any{
			ResourceKeyID: []byte{1, 2, 3}, ResourceKeyKind: ResourceKindBlob, ResourceKeySize: uint64(3),
		}),
	})
}

func TestInterop_Spec_2_ResourceEnvelopeInvalidRejected(t *testing.T) {
	requireInterop(t)
	sender := hex.EncodeToString(bytes.Repeat([]byte{0x88}, IdentityLength))
	resp, err := runInterop(map[string]any{
		"cmd":    "validate_resource",
		"sender": sender,
		"body":   bodyToJSON(map[uint64]any{ResourceKeyKind: ResourceKindBlob}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK {
		t.Fatal("missing id must be rejected")
	}
}

func TestInterop_Spec_4_FullSession_PyClientGoHub(t *testing.T) {
	if testing.Short() {
		t.Skip("live interop skipped in -short")
	}
	requireInterop(t)
	const goPort, pyPort = 42960, 42961
	gotMsg := make(chan string, 1)
	gotNotice := make(chan string, 1)
	gotAction := make(chan string, 1)
	h := startInteropGoHub(t, goPort, pyPort, "spec4py", HubConfig{
		Name:                   "spec-go-hub",
		Version:                "0.1.0",
		IncludeMemberList:      true,
		EnableResourceTransfer: true,
		Handlers: HubHandlers{
			OnMsg: func(_ []byte, env *Envelope) {
				if s, ok := BodyAsString(env.Body); !ok {
					return
				} else {
					switch env.Type {
					case TypeMsg:
						select {
						case gotMsg <- s:
						default:
						}
					case TypeNotice:
						if env.Room == "" {
							return
						}
						select {
						case gotNotice <- s:
						default:
						}
					case TypeAction:
						select {
						case gotAction <- s:
						default:
						}
					}
				}
			},
		},
	})
	wantText := "spec-protocol-msg"
	type liveResult struct {
		resp interopResponse
		err  error
	}
	liveCh := make(chan liveResult, 1)
	go func() {
		resp, err := runInterop(map[string]any{
			"cmd":          "client_session",
			"hub_hash":     hubHashHex(h),
			"listen_port":  pyPort,
			"forward_port": goPort,
			"room":         "#spec",
			"text":         wantText,
			"nick":         "py-spec",
			"timeout_s":    50,
			"steps": []string{
				"join", "msg", "notice", "action", "ping", "part",
			},
		})
		liveCh <- liveResult{resp, err}
	}()
	deadline := time.Now().Add(50 * time.Second)
	var resp interopResponse
	for {
		_ = h.dest.Announce(false, nil, nil)
		select {
		case lr := <-liveCh:
			if lr.err != nil {
				t.Fatalf("client_session: %v", lr.err)
			}
			if !lr.resp.OK {
				t.Fatalf("session: %s\n%s", lr.resp.Error, lr.resp.Trace)
			}
			resp = lr.resp
			goto done
		case <-time.After(400 * time.Millisecond):
			if time.Now().After(deadline) {
				t.Fatal("timeout python full session")
			}
		}
	}
done:
	if !resp.Joined || !resp.MsgEcho || !resp.Pong || !resp.Parted || !resp.NoticeOK || !resp.ActionOK {
		t.Fatalf("session flags joined=%v msg=%v pong=%v parted=%v notice=%v action=%v",
			resp.Joined, resp.MsgEcho, resp.Pong, resp.Parted, resp.NoticeOK, resp.ActionOK)
	}
	if resp.HubName != "spec-go-hub" {
		t.Fatalf("hub_name=%q", resp.HubName)
	}
	if resp.Text != wantText {
		t.Fatalf("text=%q", resp.Text)
	}
	select {
	case msg := <-gotMsg:
		if msg != wantText {
			t.Fatalf("hub msg=%q", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hub did not receive MSG")
	}
	select {
	case n := <-gotNotice:
		if n != "interop-notice" {
			t.Fatalf("notice=%q", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hub did not receive NOTICE")
	}
	select {
	case a := <-gotAction:
		if a != "interop-action" {
			t.Fatalf("action=%q", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hub did not receive ACTION")
	}
}

func TestInterop_Spec_4_FullSession_GoClientPyHub(t *testing.T) {
	if testing.Short() {
		t.Skip("live interop skipped in -short")
	}
	requireInterop(t)
	const goPort, pyPort = 42970, 42971
	readyPath := t.TempDir() + "/ready.json"
	wantText := "spec-go-client-msg"
	type liveResult struct {
		resp interopResponse
		err  error
	}
	liveCh := make(chan liveResult, 1)
	go func() {
		resp, err := runInterop(map[string]any{
			"cmd":          "live_hub_protocol",
			"listen_port":  pyPort,
			"forward_port": goPort,
			"ready_path":   readyPath,
			"want_text":    wantText,
			"timeout_s":    50,
			"hub_name":     "py-protocol-hub",
		})
		liveCh <- liveResult{resp, err}
	}()
	ready := waitReadyJSON(t, readyPath, 20*time.Second)
	hubHash, err := hex.DecodeString(ready["hub_hash"])
	if err != nil || len(hubHash) != IdentityLength {
		t.Fatalf("hub_hash=%q", ready["hub_hash"])
	}
	pub, err := hex.DecodeString(ready["public_key"])
	if err != nil {
		t.Fatal(err)
	}
	identity.Remember(nil, hubHash, pub, nil)

	cfg := testconfig.New()
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()
	var iface interfaces.Interface
	iface, err = interfaces.NewUDPInterface("RRCI-GOCLI", addr(goPort), addr(pyPort), true)
	if err != nil {
		t.Fatal(err)
	}
	iface.SetPacketCallback(func(d []byte, ni common.NetworkInterface) { tr.HandlePacket(d, ni) })
	if err := iface.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = iface.Stop() }()
	if ni, ok := iface.(common.NetworkInterface); ok {
		_ = tr.RegisterInterface("RRCI-GOCLI", ni)
	}

	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for !tr.HasPath(hubHash) {
		if time.Now().After(deadline) {
			t.Fatal("path timeout")
		}
		_ = tr.RequestPath(hubHash, "", nil, true)
		time.Sleep(100 * time.Millisecond)
	}

	joined := make(chan struct{}, 1)
	parted := make(chan struct{}, 1)
	gotMsg := make(chan string, 1)
	gotNotice := make(chan string, 1)
	gotAction := make(chan string, 1)
	client, err := Dial(tr, id, hubHash, ClientConfig{
		Nick: "go-spec",
		Name: "go-interop",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#spec" {
					select {
					case joined <- struct{}{}:
					default:
					}
				}
			},
			OnParted: func(room string, _ *Envelope) {
				if room == "#spec" {
					select {
					case parted <- struct{}{}:
					default:
					}
				}
			},
			OnMsg: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case gotMsg <- s:
					default:
					}
				}
			},
			OnNotice: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case gotNotice <- s:
					default:
					}
				}
			},
			OnAction: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case gotAction <- s:
					default:
					}
				}
			},
		},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	if client.Welcome() == nil || client.Welcome().HubName != "py-protocol-hub" {
		t.Fatalf("welcome=%+v", client.Welcome())
	}
	if err := client.Join("#spec"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-joined:
	case <-time.After(15 * time.Second):
		t.Fatal("join timeout")
	}
	if err := client.SendMsg("#spec", wantText); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-gotMsg:
		if s != wantText {
			t.Fatalf("msg echo=%q", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no msg echo")
	}
	if err := client.SendNotice("#spec", "go-notice"); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-gotNotice:
		if s != "go-notice" {
			t.Fatalf("notice echo=%q", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no notice echo")
	}
	if err := client.SendAction("#spec", "go-action"); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-gotAction:
		if s != "go-action" {
			t.Fatalf("action echo=%q", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no action echo")
	}
	if err := client.Ping("go-ping"); err != nil {
		t.Fatal(err)
	}
	if err := client.Part("#spec"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-parted:
	case <-time.After(15 * time.Second):
		t.Fatal("part timeout")
	}

	select {
	case lr := <-liveCh:
		if lr.err != nil {
			t.Fatalf("py hub: %v", lr.err)
		}
		if !lr.resp.OK {
			t.Fatalf("py hub: %s", lr.resp.Error)
		}
		if lr.resp.Text != wantText {
			t.Fatalf("py hub text=%q", lr.resp.Text)
		}
		if !lr.resp.NoticeOK || !lr.resp.ActionOK || !lr.resp.Pong || !lr.resp.Parted {
			t.Fatalf("py hub flags notice=%v action=%v pong=%v parted=%v",
				lr.resp.NoticeOK, lr.resp.ActionOK, lr.resp.Pong, lr.resp.Parted)
		}
	case <-time.After(25 * time.Second):
		t.Fatal("timeout py protocol hub")
	}
}

func TestInterop_Spec_3_DirectNoticeDestinationField(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "encode_matrix"})
	raw, err := hex.DecodeString(strVal(resp.Envelope["13"]))
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != TypeNotice {
		t.Fatalf("type=%d", got.Type)
	}
	if !got.HasDestination || len(got.Destination) != IdentityLength {
		t.Fatalf("destination missing has=%v len=%d", got.HasDestination, len(got.Destination))
	}
	if s, ok := BodyAsString(got.Body); !ok || s != "dm" {
		t.Fatalf("body=%v", got.Body)
	}

	sender := bytes.Repeat([]byte{0x51}, IdentityLength)
	dst := bytes.Repeat([]byte{0x52}, IdentityLength)
	py := interopCall(t, map[string]any{
		"cmd":         "encode",
		"type":        TypeNotice,
		"sender":      hex.EncodeToString(sender),
		"destination": hex.EncodeToString(dst),
		"body":        "from-py",
	})
	raw, err = hex.DecodeString(py.Packed)
	if err != nil {
		t.Fatal(err)
	}
	got, err = UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasDestination || !bytes.Equal(got.Destination, dst) {
		t.Fatal("go decode python direct notice")
	}
}

func TestInterop_Spec_2_ResourceEnvelopeRejectionParity(t *testing.T) {
	requireInterop(t)
	sender := hex.EncodeToString(bytes.Repeat([]byte{0x88}, IdentityLength))
	cases := []struct {
		name string
		body map[uint64]any
	}{
		{"missing id", map[uint64]any{ResourceKeyKind: ResourceKindBlob, ResourceKeySize: uint64(1)}},
		{"missing kind", map[uint64]any{ResourceKeyID: []byte{1}, ResourceKeySize: uint64(1)}},
		{"missing size", map[uint64]any{ResourceKeyID: []byte{1}, ResourceKeyKind: ResourceKindBlob}},
		{"invalid sha256", map[uint64]any{
			ResourceKeyID: []byte{1}, ResourceKeyKind: ResourceKindBlob,
			ResourceKeySize: uint64(1), ResourceKeySHA256: "nope",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, reason := ValidateResourceEnvelopeBody(tc.body); reason == "" {
				t.Fatal("go accepted invalid body")
			}
			resp, err := runInterop(map[string]any{
				"cmd":    "validate_resource",
				"sender": sender,
				"body":   bodyToJSON(tc.body),
			})
			if err != nil {
				t.Fatal(err)
			}
			if resp.OK {
				t.Fatal("python accepted invalid body")
			}
			if resp.Error == "" {
				t.Fatal("expected rejection reason")
			}
		})
	}
}

func intVal(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	default:
		return 0
	}
}

func strVal(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
