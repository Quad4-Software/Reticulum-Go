// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIdentitySignVerifyAndRSG(t *testing.T) {
	id, code := IdentityGenerate()
	if code != OK || id == 0 {
		t.Fatalf("generate: code=%d", code)
	}
	defer IdentityDestroy(id)

	msg := []byte("renpkg trust root")
	sig, code := IdentitySign(id, msg)
	if code != OK || len(sig) != 64 {
		t.Fatalf("sign: code=%d len=%d", code, len(sig))
	}
	if code := IdentityVerify(id, msg, sig); code != OK {
		t.Fatalf("verify: %d", code)
	}
	if code := IdentityVerify(id, []byte("tampered"), sig); code == OK {
		t.Fatal("expected verify failure")
	}

	pub, code := IdentityPublicKey(id)
	if code != OK || len(pub) != 64 {
		t.Fatalf("pubkey: code=%d len=%d", code, len(pub))
	}
	peer, code := IdentityFromPublicKey(pub)
	if code != OK || peer == 0 {
		t.Fatalf("from pubkey: %d", code)
	}
	defer IdentityDestroy(peer)
	if code := IdentityVerify(peer, msg, sig); code != OK {
		t.Fatalf("peer verify: %d", code)
	}

	rsg, code := RSGCreate(id, msg, false)
	if code != OK || len(rsg) < 65 {
		t.Fatalf("rsg create: code=%d len=%d", code, len(rsg))
	}
	hash, code := IdentityHashBytes(id)
	if code != OK || len(hash) != 16 {
		t.Fatalf("hash bytes: %d", code)
	}
	if code := RSGValidate(rsg, msg, hash); code != OK {
		t.Fatalf("rsg validate: %d last=%s", code, LastError())
	}
	if code := RSGValidate(rsg, []byte("wrong"), hash); code == OK {
		t.Fatal("expected rsg hash failure")
	}

	rsm, code := RSGCreate(id, msg, true)
	if code != OK {
		t.Fatalf("rsm create: %d", code)
	}
	got, code := RSMVerify(rsm, hash)
	if code != OK || !bytes.Equal(got, msg) {
		t.Fatalf("rsm verify: code=%d got=%q", code, got)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.bin")
	if err := os.WriteFile(path, msg, 0o600); err != nil {
		t.Fatal(err)
	}
	fileRSG, code := RSGSignFile(id, path)
	if code != OK {
		t.Fatalf("sign file: %d %s", code, LastError())
	}
	if code := RSGVerifyFile(fileRSG, path, hash); code != OK {
		t.Fatalf("verify file: %d %s", code, LastError())
	}
}
