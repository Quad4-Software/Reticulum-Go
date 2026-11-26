// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"flag"
	"io"
	"reflect"
	"testing"
)

func TestParseInterspersed(t *testing.T) {
	newFS := func() (*flag.FlagSet, *string, *bool) {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		cfg := fs.String("config", "", "")
		l := fs.Bool("l", false, "")
		fs.Bool("C", false, "")
		return fs, cfg, l
	}
	fs, cfg, l := newFS()
	if err := parseInterspersed(fs, []string{"-l", "-C", "/bin/sh", "-config", "/x"}); err != nil {
		t.Fatal(err)
	}
	if *cfg != "/x" || !*l || !reflect.DeepEqual(fs.Args(), []string{"/bin/sh"}) {
		t.Fatalf("cfg=%q args=%v", *cfg, fs.Args())
	}
	fs, cfg, _ = newFS()
	if err := parseInterspersed(fs, []string{"hash", "ls", "-l", "-config"}); err != nil {
		t.Fatal(err)
	}
	if *cfg != "" || !reflect.DeepEqual(fs.Args(), []string{"hash", "ls", "-l", "-config"}) {
		t.Fatalf("cfg=%q args=%v", *cfg, fs.Args())
	}
	fs, cfg, _ = newFS()
	if err := parseInterspersed(fs, []string{"hash", "--", "ls", "-config"}); err != nil {
		t.Fatal(err)
	}
	if *cfg != "" || !reflect.DeepEqual(fs.Args(), []string{"hash", "ls", "-config"}) {
		t.Fatalf("cfg=%q args=%v", *cfg, fs.Args())
	}
	fs, cfg, _ = newFS()
	if err := parseInterspersed(fs, []string{"hash", "-config", "/x", "--", "ls", "-compat"}); err != nil {
		t.Fatal(err)
	}
	if *cfg != "/x" || !reflect.DeepEqual(fs.Args(), []string{"hash", "ls", "-compat"}) {
		t.Fatalf("cfg=%q args=%v", *cfg, fs.Args())
	}
}

func TestParseAll(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg := fs.String("config", "", "")
	n := fs.Int("t", 0, "")
	if err := parseAll(fs, []string{"hash", "-config", "/x", "other", "-t", "5"}); err != nil {
		t.Fatal(err)
	}
	if *cfg != "/x" || *n != 5 || !reflect.DeepEqual(fs.Args(), []string{"hash", "other"}) {
		t.Fatalf("cfg=%q n=%d args=%v", *cfg, *n, fs.Args())
	}
	fs = flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Int("t", 0, "")
	if err := parseAll(fs, []string{"a", "--", "-t", "b"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fs.Args(), []string{"a", "-t", "b"}) {
		t.Fatalf("args=%v", fs.Args())
	}
}
