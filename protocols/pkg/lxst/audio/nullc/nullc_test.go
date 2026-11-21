// SPDX-License-Identifier: LicenseRef-Reticulum
package nullc_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/nullc"
)

func TestNullRoundTrip(t *testing.T) {
	c := nullc.New(4)
	in := []int16{1, -2, 3, -4}
	enc, err := c.Encode(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}
	for i := range in {
		if out[i] != in[i] {
			t.Fatalf("%v vs %v", out, in)
		}
	}
}
