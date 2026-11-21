// SPDX-License-Identifier: LicenseRef-Reticulum
package sounds_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/phonebook"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/sounds"
)

func TestInstallCopiesRingtonesAndConfig(t *testing.T) {
	dir := t.TempDir()
	if err := sounds.Install(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ringer.opus", "soft.opus", "config"} {
		st, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if st.Size() == 0 {
			t.Fatalf("%s empty", name)
		}
	}
	cfg, err := phonebook.LoadINI(filepath.Join(dir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Ringtone != sounds.DefaultRingtone {
		t.Fatalf("ringtone %q", cfg.Ringtone)
	}
	ringer := filepath.Join(dir, "ringer.opus")
	if err := os.WriteFile(ringer, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sounds.Install(dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(ringer)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep" {
		t.Fatal("existing ringtone overwritten")
	}
}
