// SPDX-License-Identifier: LicenseRef-Reticulum
package sounds

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed ringer.opus soft.opus
var FS embed.FS

const DefaultRingtone = "ringer.opus"

const DefaultConfig = `# This is an example rnphone config file.
# You should probably edit it to suit your
# intended usage.

[telephone]
    ringtone = ringer.opus

    # speaker = device name
    # microphone = device name
    # ringer = device name

    # allowed_callers = all
    # allowed_callers = none
    # allowed_callers = phonebook

[phonebook]

[hardware]
    # keypad = gpio_4x4
    # display = i2c_lcd1602
`

func Install(dir string) error {
	if strings.ContainsRune(dir, 0) {
		return fmt.Errorf("invalid path")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, name := range []string{"ringer.opus", "soft.opus"} {
		dst := filepath.Join(dir, name)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		b, err := FS.ReadFile(name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o600); err != nil {
			return err
		}
	}
	cfgPath := filepath.Join(dir, "config")
	if _, err := os.Stat(cfgPath); err == nil {
		return nil
	}
	return os.WriteFile(cfgPath, []byte(DefaultConfig), 0o600)
}
