// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"bytes"
	"testing"
)

func TestSmoke_ConstantsWired(t *testing.T) {
	if AppName != "rrc" || HubAspect != "hub" {
		t.Fatal("destination identity")
	}
	if ProtocolVersion != 1 {
		t.Fatal("protocol version")
	}
	if MessageIDLength != 8 || IdentityLength != 16 {
		t.Fatal("fixed lengths")
	}
	if KeyDestination != 8 || TypeResourceEnvelope != 50 {
		t.Fatal("extension assignments")
	}
}

func TestSmoke_NewEnvelopeMarshal(t *testing.T) {
	sender := bytes.Repeat([]byte{0x01}, IdentityLength)
	env, err := NewEnvelope(TypePing, sender)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < FixedEnvelopeMin {
		t.Fatalf("len=%d", len(raw))
	}
	got, err := UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != TypePing {
		t.Fatalf("type=%d", got.Type)
	}
}

func TestSmoke_HubConfigApplyDefaults(t *testing.T) {
	cfg := HubConfig{}
	cfg.applyDefaults()
	if cfg.Limits.MaxNickBytes == 0 || cfg.Limits.MaxMsgBodyBytes == 0 {
		t.Fatalf("%+v", cfg.Limits)
	}
}

func TestSmoke_DefaultHubCapabilities(t *testing.T) {
	caps := DefaultHubCapabilities(false)
	if caps[CapAction] != true || caps[CapDirectNotice] != true {
		t.Fatalf("%#v", caps)
	}
}

func TestSmoke_RoomNickHelpers(t *testing.T) {
	if NormalizeRoom("#X") != "#x" {
		t.Fatal("normalize")
	}
	if SanitizeNick(" a ") != "a" {
		t.Fatal("sanitize")
	}
}
