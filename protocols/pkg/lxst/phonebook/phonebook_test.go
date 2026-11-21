// SPDX-License-Identifier: LicenseRef-Reticulum
package phonebook_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/phonebook"
)

func TestRaceAllowAndAdd(t *testing.T) {
	b := phonebook.New()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 64 {
			h := bytes16(byte(i + 1))
			_ = b.Add(phonebook.Entry{Name: string(rune('A'+i%26)) + string(rune('0'+i/26)), Hash: h})
		}
	}()
	go func() {
		defer wg.Done()
		h := bytes16(1)
		for range 200 {
			_ = b.IsAllowed(h)
			_ = b.Policy()
		}
	}()
	wg.Wait()
}

func TestAllowAllAndBlock(t *testing.T) {
	b := phonebook.New()
	h := bytes16(1)
	if !b.IsAllowed(h) {
		t.Fatal("allow all")
	}
	b.SetBlocked([][]byte{h})
	if b.IsAllowed(h) {
		t.Fatal("blocked")
	}
}

func TestSetPolicyUnknownDoesNotAllowAll(t *testing.T) {
	b := phonebook.New()
	b.SetPolicy(0x01)
	if b.IsAllowed(bytes16(2)) {
		t.Fatal("unknown policy must not fail open")
	}
}

func TestAllowNone(t *testing.T) {
	b := phonebook.New()
	b.SetPolicy(phonebook.AllowNone)
	if b.IsAllowed(bytes16(2)) {
		t.Fatal("allow none")
	}
}

func TestEmptyPhonebookDenies(t *testing.T) {
	b := phonebook.New()
	b.AllowPhonebook()
	if b.IsAllowed(bytes16(1)) {
		t.Fatal("empty phonebook must deny")
	}
}

func TestPhonebookAllow(t *testing.T) {
	b := phonebook.New()
	h := bytes16(3)
	if err := b.Add(phonebook.Entry{Name: "Alice", Hash: h, Alias: "1"}); err != nil {
		t.Fatal(err)
	}
	b.AllowPhonebook()
	if !b.IsAllowed(h) {
		t.Fatal("alice should be allowed")
	}
	if b.IsAllowed(bytes16(9)) {
		t.Fatal("unknown should be denied")
	}
	e, ok := b.Lookup("1")
	if !ok || e.Name != "Alice" {
		t.Fatalf("alias lookup %+v %v", e, ok)
	}
}

func TestLoadINIRejectsHuge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1<<20 + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, err := phonebook.LoadINI(path); err == nil {
		t.Fatal("expected size rejection")
	}
}

func TestLoadINI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := "[telephone]\nallowed_callers = phonebook\nblocked_callers = 0000000000000000000000000000000a\nringtone = ringer.opus\nspeaker = HDMI\nmicrophone = USB\nringer = PCM\n\n[phonebook]\nAlice = 00000000000000000000000000000001, 12\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := phonebook.LoadINI(path)
	if err != nil {
		t.Fatal(err)
	}
	b := phonebook.New()
	if err := phonebook.ApplyPolicy(b, cfg); err != nil {
		t.Fatal(err)
	}
	if !b.IsAllowed(bytes16(1)) {
		t.Fatal("alice")
	}
	if b.IsAllowed(bytes16(10)) {
		t.Fatal("blocked")
	}
	if cfg.Ringtone != "ringer.opus" || cfg.Speaker != "HDMI" || cfg.Microphone != "USB" || cfg.Ringer != "PCM" {
		t.Fatalf("devices %+v", cfg)
	}
}

func TestLoadINIRejectsSandbox(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := "[telephone]\nsandbox = off\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := phonebook.LoadINI(path); err == nil {
		t.Fatal("sandbox key")
	}
	body = "[no-sandbox]\nringtone = x\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := phonebook.LoadINI(path); err == nil {
		t.Fatal("sandbox section")
	}
}

func bytes16(v byte) []byte {
	h := make([]byte, 16)
	h[15] = v
	return h
}
