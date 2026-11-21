// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func TestBlackBox_PublicAPI_EnvelopeRoundTrip(t *testing.T) {
	sender := bytes.Repeat([]byte{0x9c}, rrc.IdentityLength)
	env, err := rrc.NewEnvelope(rrc.TypeMsg, sender)
	if err != nil {
		t.Fatal(err)
	}
	env.Room = "#bb"
	env.HasRoom = true
	env.Body = "blackbox"
	env.HasBody = true
	raw, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := rrc.UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := rrc.BodyAsString(got.Body)
	if !ok || s != "blackbox" {
		t.Fatalf("body=%v", got.Body)
	}
}

func TestBlackBox_PublicAPI_Helpers(t *testing.T) {
	if rrc.NormalizeRoom("  #X ") != "#x" {
		t.Fatal("NormalizeRoom")
	}
	if rrc.SanitizeNick("a\nb") != "ab" {
		t.Fatal("SanitizeNick")
	}
	if rrc.AppName != "rrc" || rrc.HubAspect != "hub" {
		t.Fatal("destination constants")
	}
	if rrc.KeyDestination != 8 || rrc.TypeResourceEnvelope != 50 {
		t.Fatal("extension constants")
	}
}

func TestBlackBox_PublicAPI_NilGuards(t *testing.T) {
	_, err := rrc.NewHub(nil, nil, rrc.HubConfig{})
	if !errors.Is(err, rrc.ErrNilArgument) {
		t.Fatalf("err=%v", err)
	}
	_, err = rrc.Dial(nil, nil, nil, rrc.ClientConfig{})
	if !errors.Is(err, rrc.ErrNilArgument) {
		t.Fatalf("err=%v", err)
	}
}

func TestBlackBox_PublicAPI_HelloWelcomeMaps(t *testing.T) {
	h := &rrc.HelloBody{ClientName: "bb", HasName: true}
	m := h.ToMap()
	got, err := rrc.ParseHelloBody(m)
	if err != nil || got.ClientName != "bb" {
		t.Fatalf("hello=%+v err=%v", got, err)
	}
	w := &rrc.WelcomeBody{
		HubName: "hub", HasName: true,
		Limits: rrc.HubLimits{MaxMsgBodyBytes: 100}, HasLimits: true,
	}
	wm := w.ToMap()
	wg, err := rrc.ParseWelcomeBody(wm)
	if err != nil || wg.HubName != "hub" || wg.Limits.MaxMsgBodyBytes != 100 {
		t.Fatalf("welcome=%+v err=%v", wg, err)
	}
}

func TestBlackBox_PublicAPI_JoinedMembers(t *testing.T) {
	id := bytes.Repeat([]byte{0x01}, rrc.IdentityLength)
	members, err := rrc.ParseJoinedMembers([]any{id})
	if err != nil || len(members) != 1 {
		t.Fatalf("members=%v err=%v", members, err)
	}
}
