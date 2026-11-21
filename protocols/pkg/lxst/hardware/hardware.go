// SPDX-License-Identifier: LicenseRef-Reticulum
package hardware

import (
	"fmt"
	"strings"
)

const (
	DriverKeypadGPIO = "gpio_4x4"
	DriverLCDI2C     = "i2c_lcd1602"
)

// Keypad is a 4x4 matrix used by rnphone on Raspberry Pi.
type Keypad struct {
	Enabled bool
}

func NewKeypad() *Keypad {
	return &Keypad{}
}

func (k *Keypad) Enable(driver string) error {
	driver = strings.TrimSpace(driver)
	if driver == "" {
		k.Enabled = false
		return nil
	}
	if !strings.EqualFold(driver, DriverKeypadGPIO) {
		k.Enabled = false
		return fmt.Errorf("unknown keypad driver %q", driver)
	}
	k.Enabled = false
	return fmt.Errorf("keypad driver %q is not implemented", driver)
}

func (*Keypad) Read() (rune, bool) {
	return 0, false
}

func (*Keypad) Implemented() bool { return false }

// LCD is a 16x2 I2C display used by rnphone.
type LCD struct {
	Enabled bool
}

func NewLCD() *LCD {
	return &LCD{}
}

func (d *LCD) Enable(driver string) error {
	driver = strings.TrimSpace(driver)
	if driver == "" {
		d.Enabled = false
		return nil
	}
	if !strings.EqualFold(driver, DriverLCDI2C) {
		d.Enabled = false
		return fmt.Errorf("unknown display driver %q", driver)
	}
	d.Enabled = false
	return fmt.Errorf("display driver %q is not implemented", driver)
}

func (*LCD) Write(_, _ string) {}

func (*LCD) Clear() {}

func (*LCD) Implemented() bool { return false }
