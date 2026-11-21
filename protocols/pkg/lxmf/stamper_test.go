// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGenerateStampWithDeadline_Expired(t *testing.T) {
	ctx := context.Background()
	id := []byte("deadline-test-material-for-stamp")
	past := time.Now().Add(-time.Minute)
	_, _, err := GenerateStampWithDeadline(ctx, id, 8, WorkblockExpandRounds, past)
	if !errors.Is(err, ErrStampNotFound) {
		t.Fatalf("expected ErrStampNotFound, got %v", err)
	}
}
