// SPDX-License-Identifier: LicenseRef-Reticulum
package media_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/media"
)

func TestJitterRejectsEmptyPayloadStillCounted(t *testing.T) {
	jb := media.NewJitterBuffer(20, 8)
	jb.Push(1, 1, nil)
	if jb.Depth() != 1 {
		t.Fatalf("depth %d", jb.Depth())
	}
}

func TestJitterDefaults(t *testing.T) {
	jb := media.NewJitterBuffer(0, 0)
	if jb.TargetMs() != 60 {
		t.Fatalf("target %d", jb.TargetMs())
	}
	jb.Push(1, 1, []byte{1})
	if jb.Depth() != 1 {
		t.Fatalf("depth %d", jb.Depth())
	}
}

func TestJitterDuplicateSeqKeepsLatest(t *testing.T) {
	jb := media.NewJitterBuffer(20, 8)
	jb.Push(1, 1, []byte{1})
	jb.Push(1, 2, []byte{9})
	if jb.Depth() != 1 {
		t.Fatalf("depth %d", jb.Depth())
	}
}
