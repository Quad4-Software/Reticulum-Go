// SPDX-License-Identifier: LicenseRef-Reticulum
package hardware_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/hardware"
)

func TestEnableKnownDrivers(t *testing.T) {
	k := hardware.NewKeypad()
	if err := k.Enable("gpio_4x4"); err == nil {
		t.Fatal("gpio keypad should report unimplemented")
	}
	if k.Enabled {
		t.Fatal("unimplemented keypad must stay disabled")
	}
	if err := k.Enable("unknown"); err == nil {
		t.Fatal("unknown keypad")
	}
	if err := k.Enable(""); err != nil {
		t.Fatal(err)
	}
	d := hardware.NewLCD()
	if err := d.Enable("i2c_lcd1602"); err == nil {
		t.Fatal("i2c display should report unimplemented")
	}
	if d.Enabled {
		t.Fatal("unimplemented lcd must stay disabled")
	}
	if err := d.Enable(""); err != nil {
		t.Fatal(err)
	}
}
