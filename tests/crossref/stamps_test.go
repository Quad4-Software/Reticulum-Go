// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package crossref

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/announce"
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/cryptography"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/stamps"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

type StampVectors struct {
	Costs     []StampCostVector     `json:"costs"`
	Values    []StampValueVector    `json:"values"`
	Validity  []StampValidityVector `json:"validity"`
	Detached  []StampDetachedVector `json:"detached"`
	Keystamps []StampKeystampVector `json:"keystamps"`
}

type StampCostVector struct {
	Cost      float64 `json:"cost"`
	FP        int     `json:"fp"`
	BytesHex  string  `json:"bytes_hex"`
	TargetHex string  `json:"target_hex"`
	Quantized float64 `json:"quantized"`
}

type StampValueVector struct {
	MaterialHex string  `json:"material_hex"`
	StampHex    string  `json:"stamp_hex"`
	Value       float64 `json:"value"`
}

type StampValidityVector struct {
	WorkblockHex string  `json:"workblock_hex"`
	StampHex     string  `json:"stamp_hex"`
	Cost         float64 `json:"cost"`
	Valid        bool    `json:"valid"`
}

type StampDetachedVector struct {
	MaterialHex string  `json:"material_hex"`
	Cost        float64 `json:"cost"`
	StampHex    string  `json:"stamp_hex"`
	Value       float64 `json:"value"`
}

type StampKeystampVector struct {
	MaterialHex string  `json:"material_hex"`
	Cost        float64 `json:"cost"`
	PrivHex     string  `json:"priv_hex"`
	PubHex      string  `json:"pub_hex"`
	Value       float64 `json:"value"`
}

func loadStampVectors(t *testing.T) *StampVectors {
	t.Helper()
	v := loadVectors(t)
	if v.Stamps == nil {
		t.Skip("stamps section missing (regenerate vectors with format_version 7)")
	}
	return v.Stamps
}

func TestStampCostVectors(t *testing.T) {
	for i, vec := range loadStampVectors(t).Costs {
		t.Run(fmt.Sprintf("cost_%d", i), func(t *testing.T) {
			if fp := stamps.CostToFP(vec.Cost); fp != vec.FP {
				t.Errorf("CostToFP(%v) = %d, want %d", vec.Cost, fp, vec.FP)
			}
			if b := stamps.CostToBytes(vec.Cost); hex.EncodeToString(b[:]) != vec.BytesHex {
				t.Errorf("CostToBytes(%v) = %x, want %s", vec.Cost, b, vec.BytesHex)
			}
			if fp := stamps.FPFromBytes(mustHex(t, vec.BytesHex)); fp != vec.FP {
				t.Errorf("FPFromBytes = %d, want %d", fp, vec.FP)
			}
			want, ok := new(big.Int).SetString(vec.TargetHex, 16)
			if !ok {
				t.Fatalf("bad target hex %q", vec.TargetHex)
			}
			if got := stamps.TargetFromFP(vec.FP); got.Cmp(want) != 0 {
				t.Errorf("TargetFromFP(%d) = %x, want %x", vec.FP, got, want)
			}
			if q := stamps.QuantizeCost(vec.Cost); q != vec.Quantized {
				t.Errorf("QuantizeCost(%v) = %v, want %v", vec.Cost, q, vec.Quantized)
			}
		})
	}
}

func TestStampValueVectors(t *testing.T) {
	for i, vec := range loadStampVectors(t).Values {
		material := mustHex(t, vec.MaterialHex)
		stamp := mustHex(t, vec.StampHex)
		if got := stamps.StampValue(material, stamp); got != vec.Value {
			t.Errorf("vector %d: StampValue = %v, want %v", i, got, vec.Value)
		}
	}
}

func TestStampValidityVectors(t *testing.T) {
	for i, vec := range loadStampVectors(t).Validity {
		workblock := mustHex(t, vec.WorkblockHex)
		stamp := mustHex(t, vec.StampHex)
		if got := stamps.StampValid(stamp, vec.Cost, workblock); got != vec.Valid {
			t.Errorf("vector %d: StampValid(cost=%v) = %v, want %v", i, vec.Cost, got, vec.Valid)
		}
	}
}

func TestStampDetachedVectors(t *testing.T) {
	for i, vec := range loadStampVectors(t).Detached {
		material := mustHex(t, vec.MaterialHex)
		stamp := mustHex(t, vec.StampHex)
		if !stamps.StampValid(stamp, vec.Cost, material) {
			t.Errorf("vector %d: python stamp does not validate at cost %v", i, vec.Cost)
		}
		if got := stamps.StampValue(material, stamp); got != vec.Value {
			t.Errorf("vector %d: StampValue = %v, want %v", i, got, vec.Value)
		}
	}
}

func TestStampKeystampVectors(t *testing.T) {
	for i, vec := range loadStampVectors(t).Keystamps {
		material := mustHex(t, vec.MaterialHex)
		priv := mustHex(t, vec.PrivHex)
		pub := mustHex(t, vec.PubHex)
		derived, err := cryptography.PublicKeyFromPrivate(priv)
		if err != nil {
			t.Fatalf("vector %d: PublicKeyFromPrivate: %v", i, err)
		}
		if hex.EncodeToString(derived) != vec.PubHex {
			t.Errorf("vector %d: derived pub mismatch", i)
		}
		if !stamps.StampValid(pub, vec.Cost, material) {
			t.Errorf("vector %d: python keystamp does not validate at cost %v", i, vec.Cost)
		}
		if got := stamps.StampValue(material, pub); got != vec.Value {
			t.Errorf("vector %d: StampValue = %v, want %v", i, got, vec.Value)
		}
	}
}

// TestGoStampsVerifiedByPython generates a detached stamp and a stamped key in
// Go at low cost and has Python RNS.Stamps validate them.
func TestGoStampsVerifiedByPython(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	material := mustHex(t, "aabbccddee0011223344556677889900")
	stamp, _, err := stamps.GenerateStamp(ctx, material, 10, stamps.StampLength)
	if err != nil {
		t.Fatalf("GenerateStamp: %v", err)
	}
	keyMaterial := mustHex(t, "00112233445566778899aabbccddeeff")
	priv, pub, _, err := stamps.GenerateStampedKey(ctx, keyMaterial, 10)
	if err != nil {
		t.Fatalf("GenerateStampedKey: %v", err)
	}

	payload, err := json.Marshal(map[string]any{
		"detached": []map[string]any{{
			"material_hex": hex.EncodeToString(material),
			"cost":         10,
			"stamp_hex":    hex.EncodeToString(stamp),
		}},
		"keystamps": []map[string]any{{
			"material_hex": hex.EncodeToString(keyMaterial),
			"cost":         10,
			"priv_hex":     hex.EncodeToString(priv[:]),
			"pub_hex":      hex.EncodeToString(pub[:]),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	vecFile := filepath.Join(t.TempDir(), "go_stamps.json")
	if err := os.WriteFile(vecFile, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	_, thisFile, _, _ := runtime.Caller(0)
	script := filepath.Join(filepath.Dir(thisFile), "verify_go_stamps.py")
	python := os.Getenv("PYTHON_INTEROP")
	if python == "" {
		python = "python3"
	}
	cmd := exec.Command(python, script, vecFile)
	cmd.Env = append(os.Environ(), "RETICULUM_PATH="+reticulumPathEnv())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python verify failed: %v\n%s", err, out)
	}
}

type StampedAnnounceVector struct {
	RawHex         string  `json:"raw_hex"`
	DestHashHex    string  `json:"dest_hash_hex"`
	PublicKeyHex   string  `json:"public_key_hex"`
	NameHashHex    string  `json:"name_hash_hex"`
	RandomHashHex  string  `json:"random_hash_hex"`
	RatchetHex     string  `json:"ratchet_hex"`
	SignatureHex   string  `json:"signature_hex"`
	AppDataHex     string  `json:"app_data_hex"`
	OrigAppDataHex string  `json:"orig_app_data_hex"`
	StampHex       string  `json:"stamp_hex"`
	StampValue     float64 `json:"stamp_value"`
	LRCost         float64 `json:"lr_cost"`
	RequiredCost   float64 `json:"required_cost"`
}

// TestPythonStampedAnnounces parses raw HeaderType1 announces generated by
// Python 1.6.0: signature over the unstripped app_data, ext strip, stamp
// value, and lr_cost must all match.
func TestPythonStampedAnnounces(t *testing.T) {
	v := loadVectors(t)
	if len(v.StampedAnnounce) == 0 {
		t.Skip("stamped_announce section missing")
	}
	for i, vec := range v.StampedAnnounce {
		t.Run(fmt.Sprintf("stamped_announce_%d", i), func(t *testing.T) {
			raw := mustHex(t, vec.RawHex)
			destHash := mustHex(t, vec.DestHashHex)
			pub := mustHex(t, vec.PublicKeyHex)
			nameHash := mustHex(t, vec.NameHashHex)
			randomHash := mustHex(t, vec.RandomHashHex)
			ratchet := mustHex(t, vec.RatchetHex)
			sig := mustHex(t, vec.SignatureHex)
			appData := mustHex(t, vec.AppDataHex)

			// Signature covers dest+pub+name+random+ratchet+appData.
			signed := append(append(append(append(append(append(
				[]byte{}, destHash...), pub...), nameHash...), randomHash...), ratchet...), appData...)
			id := identity.FromPublicKey(pub)
			if id == nil || !id.Verify(signed, sig) {
				t.Fatal("signature invalid")
			}

			stripped, stamped, valid, stamp, value, lr := stamps.ParseAnnounceExt(appData, destHash, pub, nameHash, vec.RequiredCost)
			if !stamped || !valid {
				t.Fatal("python ext not recognised/valid")
			}
			if hex.EncodeToString(stripped) != vec.OrigAppDataHex {
				t.Fatalf("stripped appData = %x, want %s", stripped, vec.OrigAppDataHex)
			}
			if hex.EncodeToString(stamp) != vec.StampHex {
				t.Fatal("stamp mismatch")
			}
			if value != vec.StampValue {
				t.Fatalf("value = %v, want %v", value, vec.StampValue)
			}
			if lr != vec.LRCost {
				t.Fatalf("lr_cost = %v, want %v", lr, vec.LRCost)
			}
			_ = raw
		})
	}
}

// TestGoStampedAnnounceValidatedByPython has Python 1.6.0
// Identity.validate_announce accept a Go-built stamped announce and checks
// the recorded stamp value, lr_cost, and stripped app_data.
func TestGoStampedAnnounceValidatedByPython(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destName := "app.stamp.test"
	nh := sha256.Sum256([]byte(destName))
	nameHash := nh[:10]
	dh := sha256.Sum256(append(append([]byte{}, nameHash...), id.Hash()...))
	destHash := dh[:16]
	material := append(append(append([]byte{}, destHash...), id.GetPublicKey()...), nameHash...)
	stamp, wantValue, err := stamps.GenerateStamp(ctx, material, 10, stamps.StampLength)
	if err != nil {
		t.Fatalf("GenerateStamp: %v", err)
	}

	a, err := announce.New(id, destHash, destName, []byte("go-app-data"), false, &common.ReticulumConfig{})
	if err != nil {
		t.Fatal(err)
	}
	a.SetStamp(stamp, 12.5)
	raw, err := a.CreatePacket()
	if err != nil {
		t.Fatal(err)
	}

	_, thisFile, _, _ := runtime.Caller(0)
	script := filepath.Join(filepath.Dir(thisFile), "verify_go_stamped_announce.py")
	python := os.Getenv("PYTHON_INTEROP")
	if python == "" {
		python = "python3"
	}
	cmd := exec.Command(python, script, hex.EncodeToString(raw), "10")
	cmd.Env = append(os.Environ(), "RETICULUM_PATH="+reticulumPathEnv())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python validate_announce failed: %v\n%s", err, out)
	}
	var rec struct {
		AppData    string   `json:"app_data"`
		Stamp      string   `json:"stamp"`
		StampValue float64  `json:"stamp_value"`
		LRCost     *float64 `json:"lr_cost"`
	}
	if err := json.Unmarshal(out, &rec); err != nil {
		t.Fatalf("python output %q: %v", out, err)
	}
	if rec.AppData != "676f2d6170702d64617461" {
		t.Fatalf("python kept app_data %q, want stripped", rec.AppData)
	}
	if rec.Stamp != hex.EncodeToString(stamp) {
		t.Fatal("stamp mismatch")
	}
	if rec.StampValue != wantValue {
		t.Fatalf("stamp_value = %v, want %v", rec.StampValue, wantValue)
	}
	if rec.LRCost == nil || *rec.LRCost != 12.5 {
		t.Fatalf("lr_cost = %v, want 12.5", rec.LRCost)
	}
}

type PathRequestStampVector struct {
	Kind        string  `json:"kind"`
	Flag        int     `json:"flag"`
	PayloadHex  string  `json:"payload_hex"`
	DestHashHex string  `json:"dest_hash_hex"`
	TIDHex      string  `json:"tid_hex"`
	TagHex      string  `json:"tag_hex"`
	StampHex    string  `json:"stamp_hex"`
	StampValue  float64 `json:"stamp_value"`
}

type StampedLRKeyVector struct {
	DestHashHex string  `json:"dest_hash_hex"`
	SigPubHex   string  `json:"sig_pub_hex"`
	PrivHex     string  `json:"priv_hex"`
	PubHex      string  `json:"pub_hex"`
	Cost        float64 `json:"cost"`
	Value       float64 `json:"value"`
}

func TestPythonPathRequestStamps(t *testing.T) {
	v := loadVectors(t)
	if len(v.PathRequestStamp) == 0 {
		t.Skip("path_request_stamp section missing")
	}
	for i, vec := range v.PathRequestStamp {
		t.Run(fmt.Sprintf("pr_%d_%s", i, vec.Kind), func(t *testing.T) {
			payload := mustHex(t, vec.PayloadHex)
			dest, tid, tag, ext, ok := transport.ParsePathRequest(payload)
			if !ok {
				t.Fatal("parse failed")
			}
			if hex.EncodeToString(dest) != vec.DestHashHex {
				t.Fatal("dest hash mismatch")
			}
			if hex.EncodeToString(tid) != vec.TIDHex {
				t.Fatal("tid mismatch")
			}
			if hex.EncodeToString(tag) != vec.TagHex {
				t.Fatal("tag mismatch")
			}
			switch vec.Kind {
			case "stamped":
				if ext == nil || len(ext) != 16 || ext[0] != 0xC1 || int(ext[1]) != vec.Flag {
					t.Fatalf("bad ext %x", ext)
				}
				if !transport.ValidatePRStamp(ext[2:], dest, tag, 8) {
					t.Fatal("stamp does not validate at cost 8")
				}
				material := append(append([]byte{}, dest...), tag...)
				if got := stamps.StampValue(material, ext[2:]); got != vec.StampValue {
					t.Fatalf("stamp value %v want %v", got, vec.StampValue)
				}
			default:
				if ext != nil {
					t.Fatal("unexpected ext")
				}
			}
		})
	}
}

func TestPythonStampedLRKeys(t *testing.T) {
	v := loadVectors(t)
	if len(v.StampedLRKey) == 0 {
		t.Skip("stamped_lr_key section missing")
	}
	for i, vec := range v.StampedLRKey {
		destHash := mustHex(t, vec.DestHashHex)
		sigPub := mustHex(t, vec.SigPubHex)
		priv := mustHex(t, vec.PrivHex)
		pub := mustHex(t, vec.PubHex)
		derived, err := cryptography.PublicKeyFromPrivate(priv)
		if err != nil {
			t.Fatalf("vector %d: %v", i, err)
		}
		if hex.EncodeToString(derived) != vec.PubHex {
			t.Fatalf("vector %d: pub mismatch", i)
		}
		material := append(append([]byte{}, destHash...), sigPub...)
		if got := stamps.StampValue(material, pub); got != vec.Value {
			t.Fatalf("vector %d: value %v want %v", i, got, vec.Value)
		}
		if !stamps.StampValid(pub, vec.Cost, material) {
			t.Fatalf("vector %d: stamp invalid at cost %v", i, vec.Cost)
		}
	}
}

// TestGoStampedPRAndLRValidatedByPython generates a Go stamped PR payload and
// a stamped LR key, and has Python 1.6.0 validate both.
func TestGoStampedPRAndLRValidatedByPython(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	destHash := mustHex(t, "00112233445566778899aabbccddeeff")
	tag := mustHex(t, "ffeeddccbbaa99887766554433221100")
	prMaterial := append(append([]byte{}, destHash...), tag...)
	prStamp, _, err := stamps.GenerateStamp(ctx, prMaterial, 8, stamps.StampLength)
	if err != nil {
		t.Fatalf("GenerateStamp: %v", err)
	}

	sigPub := mustHex(t, "abababababababababababababababababababababababababababababababab")
	lrMaterial := append(append([]byte{}, destHash...), sigPub...)
	priv, pub, _, err := stamps.GenerateStampedKey(ctx, lrMaterial, 8)
	if err != nil {
		t.Fatalf("GenerateStampedKey: %v", err)
	}

	payload, err := json.Marshal(map[string]any{
		"dest_hash_hex": hex.EncodeToString(destHash),
		"tag_hex":       hex.EncodeToString(tag),
		"pr_stamp_hex":  hex.EncodeToString(prStamp),
		"sig_pub_hex":   hex.EncodeToString(sigPub),
		"lr_priv_hex":   hex.EncodeToString(priv[:]),
		"lr_pub_hex":    hex.EncodeToString(pub[:]),
		"cost":          8,
	})
	if err != nil {
		t.Fatal(err)
	}
	vecFile := filepath.Join(t.TempDir(), "go_pr_lr.json")
	if err := os.WriteFile(vecFile, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	_, thisFile, _, _ := runtime.Caller(0)
	script := filepath.Join(filepath.Dir(thisFile), "verify_go_pr_stamps.py")
	python := os.Getenv("PYTHON_INTEROP")
	if python == "" {
		python = "python3"
	}
	cmd := exec.Command(python, script, vecFile)
	cmd.Env = append(os.Environ(), "RETICULUM_PATH="+reticulumPathEnv())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python verify failed: %v\n%s", err, out)
	}
}
