// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListGoFilesSkipsTestdataAndVendor(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package p\n")
	write("a_test.go", "package p\n")
	write("sub/b.go", "package sub\n")
	write("testdata/hidden.go", "package hidden\n")
	write("vendor/x/x.go", "package x\n")

	all, err := listGoFiles(dir, []string{"./..."}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("recursive files=%v want a.go and sub/b.go", all)
	}

	one, err := listGoFiles(dir, []string{"."}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || filepath.Base(one[0]) != "a.go" {
		t.Fatalf("dir files=%v", one)
	}

	withTests, err := listGoFiles(dir, []string{"."}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(withTests) != 2 {
		t.Fatalf("with tests=%v", withTests)
	}
}
