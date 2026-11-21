// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import "testing"

func TestDropLeading(t *testing.T) {
	pcm := []int16{1, 2, 3, 4}
	got, left := dropLeading(pcm, 2)
	if left != 0 || len(got) != 2 || got[0] != 3 {
		t.Fatalf("got %v left %d", got, left)
	}
	got, left = dropLeading(pcm, 8)
	if left != 4 || len(got) != 0 {
		t.Fatalf("short frame got %v left %d", got, left)
	}
}

func TestRampIn(t *testing.T) {
	pcm := []int16{1000, 1000, 1000, 1000}
	left := rampIn(pcm, 4, 4)
	if left != 0 {
		t.Fatalf("left %d", left)
	}
	if pcm[0] != 0 {
		t.Fatalf("first sample should start at 0, got %d", pcm[0])
	}
	if pcm[3] == 0 || pcm[3] > 1000 {
		t.Fatalf("last eased sample %d", pcm[3])
	}
}
