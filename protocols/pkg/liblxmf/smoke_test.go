// SPDX-License-Identifier: LicenseRef-Reticulum
package liblxmf_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/liblxmf"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

func TestIdentitySaveLoad(t *testing.T) {
	id, code := liblxmf.IdentityGenerate()
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	defer liblxmf.IdentityDestroy(id)

	wantHash, code := liblxmf.IdentityHash(id)
	if code != liblxmf.OK {
		t.Fatal(code)
	}

	path := filepath.Join(t.TempDir(), "identity")
	if code := liblxmf.IdentitySave(id, path); code != liblxmf.OK {
		t.Fatalf("save %d", code)
	}

	loaded, code := liblxmf.IdentityLoad(path)
	if code != liblxmf.OK {
		t.Fatalf("load %d", code)
	}
	defer liblxmf.IdentityDestroy(loaded)

	gotHash, code := liblxmf.IdentityHash(loaded)
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	if !bytes.Equal(wantHash, gotHash) {
		t.Fatalf("hash mismatch after load")
	}
}

func TestLXMFCodecRoundTrip(t *testing.T) {
	id, code := liblxmf.IdentityGenerate()
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	defer liblxmf.IdentityDestroy(id)

	hash, code := liblxmf.IdentityHash(id)
	if code != liblxmf.OK || len(hash) != 16 {
		t.Fatalf("hash %d len=%d", code, len(hash))
	}

	dest := make([]byte, 16)
	copy(dest, hash)
	source := make([]byte, 16)
	copy(source, hash)

	msg, code := liblxmf.MessageCreate(dest, source, "hi", "body")
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	defer liblxmf.MessageDestroy(msg)

	data, code := liblxmf.MessagePack(msg, id)
	if code != liblxmf.OK || len(data) == 0 {
		t.Fatalf("pack %d len=%d", code, len(data))
	}

	inner, code := liblxmf.MessageEncryptedPayload(msg)
	if code != liblxmf.OK || len(inner) == 0 {
		t.Fatalf("encrypted payload %d len=%d", code, len(inner))
	}
	if len(inner) != len(data)-16 {
		t.Fatalf("encrypted payload len=%d want=%d", len(inner), len(data)-16)
	}

	got, code := liblxmf.MessageUnpack(data)
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	defer liblxmf.MessageDestroy(got)

	content, code := liblxmf.MessageGetContent(got)
	if code != liblxmf.OK || content != "body" {
		t.Fatalf("content %q code=%d", content, code)
	}
}

func TestStampCostFromAppData(t *testing.T) {
	raw, err := lxmf.EncodeAnnounceAppDataV5("peer", 12)
	if err != nil {
		t.Fatal(err)
	}
	cost, code := liblxmf.StampCostFromAppData(raw)
	if code != liblxmf.OK || cost != 12 {
		t.Fatalf("cost=%d code=%d", cost, code)
	}

	cost, code = liblxmf.StampCostFromAppData([]byte("legacy-utf8"))
	if code != liblxmf.ErrInvalidArg || cost != 0 {
		t.Fatalf("absent want INVALID_ARG cost=0 got cost=%d code=%d", cost, code)
	}
}

func TestMessagePackPropagated(t *testing.T) {
	src, code := liblxmf.IdentityGenerate()
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	defer liblxmf.IdentityDestroy(src)

	dst, code := liblxmf.IdentityGenerate()
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	defer liblxmf.IdentityDestroy(dst)

	srcHash, code := liblxmf.IdentityDeliveryHash(src)
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	dstHash, code := liblxmf.IdentityDeliveryHash(dst)
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	dstPub, code := liblxmf.IdentityPublicKey(dst)
	if code != liblxmf.OK || len(dstPub) != 64 {
		t.Fatalf("public key code=%d len=%d", code, len(dstPub))
	}

	msg, code := liblxmf.MessageCreate(dstHash, srcHash, "prop", "body")
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	defer liblxmf.MessageDestroy(msg)

	packed, code := liblxmf.MessagePackPropagated(msg, src, dstHash, dstPub, 0)
	if code != liblxmf.OK || len(packed) == 0 {
		t.Fatalf("pack propagated %d len=%d err=%s", code, len(packed), liblxmf.LastError())
	}
}

func TestMessageApplyStamp(t *testing.T) {
	id, code := liblxmf.IdentityGenerate()
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	defer liblxmf.IdentityDestroy(id)

	hash, code := liblxmf.IdentityHash(id)
	if code != liblxmf.OK {
		t.Fatal(code)
	}

	msg, code := liblxmf.MessageCreate(hash, hash, "stamp", "pow")
	if code != liblxmf.OK {
		t.Fatal(code)
	}
	defer liblxmf.MessageDestroy(msg)

	if code := liblxmf.MessageApplyStamp(msg, id, 0, 1000); code != liblxmf.OK {
		t.Fatalf("no-op apply %d", code)
	}

	if code := liblxmf.MessageApplyStamp(msg, id, 4, 15000); code != liblxmf.OK {
		t.Fatalf("apply stamp %d: %s", code, liblxmf.LastError())
	}

	data, code := liblxmf.MessagePack(msg, id)
	if code != liblxmf.OK || len(data) == 0 {
		t.Fatalf("pack after stamp %d len=%d", code, len(data))
	}

	unpacked, err := lxmf.Unpack(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(unpacked.Stamp) == 0 {
		t.Fatal("stamp missing after apply")
	}
	ok, err := unpacked.ValidateStamp(4, nil)
	if err != nil || !ok {
		t.Fatalf("ValidateStamp ok=%v err=%v", ok, err)
	}
}
