// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
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

func TestInterop_Ping(t *testing.T) {
	requireInterop(t)
	resp := interopCall(t, map[string]any{"cmd": "ping"})
	if resp.RRCVersion != int(ProtocolVersion) {
		t.Fatalf("rrc_version = %d", resp.RRCVersion)
	}
	if resp.RrcdVersion == "" {
		t.Fatal("expected rrcd_version")
	}
}

func TestInterop_GoEncodePythonDecode(t *testing.T) {
	requireInterop(t)

	sender := bytes.Repeat([]byte{0xab}, IdentityLength)
	env, err := NewEnvelope(TypeMsg, sender)
	if err != nil {
		t.Fatal(err)
	}
	env.MsgID = []byte{1, 2, 3, 4, 5, 6, 7, 8}
	env.Timestamp = 1737849600000
	env.Room = "#lobby"
	env.HasRoom = true
	env.Body = "Hello from Go"
	env.HasBody = true
	env.Nick = "alice"
	env.HasNick = true

	packed, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	resp := interopCall(t, map[string]any{
		"cmd":    "decode",
		"packed": hex.EncodeToString(packed),
	})
	if resp.Type != int(TypeMsg) {
		t.Fatalf("type = %d", resp.Type)
	}
	if resp.Room != "#lobby" {
		t.Fatalf("room = %q", resp.Room)
	}
	if resp.Nick != "alice" {
		t.Fatalf("nick = %q", resp.Nick)
	}
	if resp.Sender != hex.EncodeToString(sender) {
		t.Fatalf("sender = %s", resp.Sender)
	}
	body, ok := resp.Body.(string)
	if !ok || body != "Hello from Go" {
		t.Fatalf("body = %#v", resp.Body)
	}
}

func TestInterop_PythonEncodeGoDecode(t *testing.T) {
	requireInterop(t)

	sender := bytes.Repeat([]byte{0xcd}, IdentityLength)
	resp := interopCall(t, map[string]any{
		"cmd":       "encode",
		"type":      TypeWelcome,
		"sender":    hex.EncodeToString(sender),
		"msg_id":    "aa11bb22cc33dd44",
		"timestamp": 1737849601000,
		"body": bodyToJSON(map[uint64]any{
			WelcomeKeyHubName:    "py-hub",
			WelcomeKeyHubVersion: "0.3.2",
			WelcomeKeyHubLimits: map[uint64]any{
				LimitMaxNickBytes:           uint64(32),
				LimitMaxRoomNameBytes:       uint64(64),
				LimitMaxMsgBodyBytes:        uint64(350),
				LimitMaxRoomsPerSession:     uint64(16),
				LimitRateLimitMsgsPerMinute: uint64(60),
			},
		}),
	})
	packed, err := hex.DecodeString(resp.Packed)
	if err != nil {
		t.Fatal(err)
	}
	env, err := UnmarshalEnvelope(packed)
	if err != nil {
		t.Fatalf("go decode: %v", err)
	}
	if env.Type != TypeWelcome {
		t.Fatalf("type = %d", env.Type)
	}
	wb, err := ParseWelcomeBody(env.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !wb.HasName || wb.HubName != "py-hub" {
		t.Fatalf("welcome = %+v", wb)
	}
	if !wb.HasLimits || wb.Limits.MaxMsgBodyBytes != 350 || wb.Limits.MaxRoomsPerSession != 16 {
		t.Fatalf("limits = %+v", wb.Limits)
	}
}

func TestInterop_PythonValidateGoHELLO(t *testing.T) {
	requireInterop(t)

	sender := bytes.Repeat([]byte{0x11}, IdentityLength)
	env, err := NewEnvelope(TypeHello, sender)
	if err != nil {
		t.Fatal(err)
	}
	env.Nick = "go-client"
	env.HasNick = true
	env.Body = (&HelloBody{
		ClientName:    "reticulum-go-protocols",
		HasName:       true,
		ClientVersion: "0.1.0",
		HasVersion:    true,
	}).ToMap()
	env.HasBody = true
	packed, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	_ = interopCall(t, map[string]any{
		"cmd":    "validate",
		"packed": hex.EncodeToString(packed),
	})
}

func TestInterop_Live_PythonClientGoHub(t *testing.T) {
	if testing.Short() {
		t.Skip("live python interop skipped in -short mode")
	}
	requireInterop(t)

	const (
		goPort = 42920
		pyPort = 42921
		udpGo  = "RRCIGO"
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

	var iface interfaces.Interface
	var err error
	iface, err = interfaces.NewUDPInterface(udpGo, "127.0.0.1:42920", "127.0.0.1:42921", true)
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

	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := NewHubDestination(id, tr)
	if err != nil {
		t.Fatal(err)
	}

	gotMsg := make(chan string, 1)
	hub, err := NewHub(tr, dest, HubConfig{
		Name:    "go-interop-hub",
		Version: "0.1.0",
		Handlers: HubHandlers{
			OnMsg: func(_ []byte, env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case gotMsg <- s:
					default:
					}
				}
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	hub.Start()
	defer hub.Close()

	hubHash := dest.GetHash()
	identity.Remember(nil, hubHash, id.GetPublicKey(), nil)

	wantText := "hello from python rrcd"
	type liveResult struct {
		resp interopResponse
		err  error
	}
	liveCh := make(chan liveResult, 1)
	go func() {
		resp, err := runInterop(map[string]any{
			"cmd":          "live_client",
			"hub_hash":     hex.EncodeToString(hubHash),
			"listen_port":  pyPort,
			"forward_port": goPort,
			"room":         "#lobby",
			"text":         wantText,
			"nick":         "py-alice",
			"timeout_s":    40,
		})
		liveCh <- liveResult{resp, err}
	}()

	deadline := time.Now().Add(45 * time.Second)
	var resp interopResponse
	for {
		_ = dest.Announce(false, nil, nil)
		select {
		case lr := <-liveCh:
			if lr.err != nil {
				t.Fatalf("live client: %v", lr.err)
			}
			if !lr.resp.OK {
				t.Fatalf("live client error: %s\n%s", lr.resp.Error, lr.resp.Trace)
			}
			resp = lr.resp
			goto liveDone
		case <-time.After(400 * time.Millisecond):
			if time.Now().After(deadline) {
				t.Fatal("timeout waiting for python live client")
			}
		}
	}
liveDone:

	if resp.Text != wantText {
		t.Fatalf("python text = %q", resp.Text)
	}

	select {
	case msg := <-gotMsg:
		if msg != wantText {
			t.Fatalf("hub got %q", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for python MSG at go hub")
	}
}

func TestInterop_Live_GoClientPythonHub(t *testing.T) {
	if testing.Short() {
		t.Skip("live python interop skipped in -short mode")
	}
	requireInterop(t)

	const (
		goPort = 42930
		pyPort = 42931
		udpGo  = "RRCGOPY"
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
	iface, err = interfaces.NewUDPInterface(udpGo, "127.0.0.1:42930", "127.0.0.1:42931", true)
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

	readyPath := t.TempDir() + "/ready.json"
	wantText := "hello from go client"
	type liveResult struct {
		resp interopResponse
		err  error
	}
	liveCh := make(chan liveResult, 1)
	go func() {
		resp, err := runInterop(map[string]any{
			"cmd":          "live_hub",
			"listen_port":  pyPort,
			"forward_port": goPort,
			"ready_path":   readyPath,
			"timeout_s":    40,
			"hub_name":     "py-live-hub",
		})
		liveCh <- liveResult{resp, err}
	}()

	ready := waitReadyJSON(t, readyPath, 15*time.Second)
	hubHash, err := hex.DecodeString(ready["hub_hash"])
	if err != nil || len(hubHash) != IdentityLength {
		t.Fatalf("hub_hash=%q err=%v", ready["hub_hash"], err)
	}
	pub, err := hex.DecodeString(ready["public_key"])
	if err != nil || len(pub) == 0 {
		t.Fatalf("public_key=%q err=%v", ready["public_key"], err)
	}
	identity.Remember(nil, hubHash, pub, nil)

	deadline := time.Now().Add(20 * time.Second)
	for !tr.HasPath(hubHash) {
		if time.Now().After(deadline) {
			t.Fatal("path timeout to python hub")
		}
		_ = tr.RequestPath(hubHash, "", nil, true)
		time.Sleep(100 * time.Millisecond)
	}

	joined := make(chan struct{}, 1)
	client, err := Dial(tr, id, hubHash, ClientConfig{
		Nick: "go-alice",
		Name: "go-live",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#lobby" {
					select {
					case joined <- struct{}{}:
					default:
					}
				}
			},
		},
	})
	if err != nil {
		t.Fatalf("dial python hub: %v", err)
	}
	defer client.Close()

	if err := client.Join("#lobby"); err != nil {
		t.Fatalf("join: %v", err)
	}
	select {
	case <-joined:
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for JOINED from python hub")
	}
	if err := client.SendMsg("#lobby", wantText); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case lr := <-liveCh:
		if lr.err != nil {
			t.Fatalf("live hub: %v", lr.err)
		}
		if !lr.resp.OK {
			t.Fatalf("live hub error: %s\n%s", lr.resp.Error, lr.resp.Trace)
		}
		if lr.resp.Text != wantText {
			t.Fatalf("python hub text=%q", lr.resp.Text)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("timeout waiting for python hub result")
	}
}

func TestInterop_AllCoreTypesRoundTrip(t *testing.T) {
	requireInterop(t)
	sender := bytes.Repeat([]byte{0x44}, IdentityLength)
	types := []uint64{TypeHello, TypeWelcome, TypeJoin, TypeJoined, TypePart, TypeParted, TypeMsg, TypeNotice, TypeAction, TypePing, TypePong, TypeError, TypeResourceEnvelope}
	for _, typ := range types {
		env, err := NewEnvelope(typ, sender)
		if err != nil {
			t.Fatal(err)
		}
		env.MsgID = []byte{9, 8, 7, 6, 5, 4, 3, 2}
		env.Timestamp = 1737849600000
		switch typ {
		case TypeJoin, TypeJoined, TypePart, TypeParted, TypeMsg, TypeNotice, TypeAction:
			env.Room = "#lobby"
			env.HasRoom = true
		}
		switch typ {
		case TypeMsg, TypeNotice, TypeAction, TypeError, TypePing, TypePong:
			env.Body = "payload"
			env.HasBody = true
		case TypeHello:
			env.Body = (&HelloBody{ClientName: "go", HasName: true}).ToMap()
			env.HasBody = true
		case TypeWelcome:
			env.Body = (&WelcomeBody{HubName: "h", HasName: true}).ToMap()
			env.HasBody = true
		case TypeResourceEnvelope:
			env.Body = (&ResourceEnvelopeBody{ID: []byte{1}, HasID: true, Kind: ResourceKindBlob, HasKind: true, Size: 4, HasSize: true}).ToMap()
			env.HasBody = true
		}
		packed, err := env.Marshal()
		if err != nil {
			t.Fatalf("marshal type %d: %v", typ, err)
		}
		resp := interopCall(t, map[string]any{
			"cmd":    "validate",
			"packed": hex.EncodeToString(packed),
		})
		_ = resp
		decoded := interopCall(t, map[string]any{
			"cmd":    "decode",
			"packed": hex.EncodeToString(packed),
		})
		if decoded.Type != int(typ) {
			t.Fatalf("type %d decoded as %d", typ, decoded.Type)
		}
	}
}

func TestInterop_DestinationField(t *testing.T) {
	requireInterop(t)
	sender := bytes.Repeat([]byte{0x51}, IdentityLength)
	dst := bytes.Repeat([]byte{0x52}, IdentityLength)
	env, err := NewEnvelope(TypeNotice, sender)
	if err != nil {
		t.Fatal(err)
	}
	env.MsgID = bytes.Repeat([]byte{0x0a}, MessageIDLength)
	env.Timestamp = 1737849600000
	env.Destination = dst
	env.HasDestination = true
	env.Body = "dm"
	env.HasBody = true
	packed, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	resp := interopCall(t, map[string]any{
		"cmd":    "decode",
		"packed": hex.EncodeToString(packed),
	})
	if resp.Destination != hex.EncodeToString(dst) {
		t.Fatalf("python dest=%q", resp.Destination)
	}

	py := interopCall(t, map[string]any{
		"cmd":         "encode",
		"type":        TypeNotice,
		"sender":      hex.EncodeToString(sender),
		"msg_id":      "0b0b0b0b0b0b0b0b",
		"timestamp":   1737849601000,
		"destination": hex.EncodeToString(dst),
		"body":        "from-py",
	})
	raw, err := hex.DecodeString(py.Packed)
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasDestination || !bytes.Equal(got.Destination, dst) {
		t.Fatalf("go dest=%x has=%v", got.Destination, got.HasDestination)
	}
}

func TestInterop_UnknownKeyIgnoredByPython(t *testing.T) {
	requireInterop(t)
	sender := bytes.Repeat([]byte{0x61}, IdentityLength)
	env, err := NewEnvelope(TypePing, sender)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := mustMarshalWithExtra(*env, 99, "future")
	if err != nil {
		t.Fatal(err)
	}
	_ = interopCall(t, map[string]any{
		"cmd":    "validate",
		"packed": hex.EncodeToString(raw),
	})
}

func TestInterop_WrongVersionRejected(t *testing.T) {
	requireInterop(t)
	sender := bytes.Repeat([]byte{0x71}, IdentityLength)
	env, err := NewEnvelope(TypePing, sender)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := mustMarshalWithVersion(env, 99)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := runInterop(map[string]any{
		"cmd":    "validate",
		"packed": hex.EncodeToString(raw),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK {
		t.Fatal("python accepted unsupported version")
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
