// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCompletionContainsEveryRegisteredFlag(t *testing.T) {
	specs, err := completionSpecs()
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]string{
		"bash": generateBashCompletion(specs),
		"zsh":  generateZshCompletion(specs),
		"fish": generateFishCompletion(specs),
	}
	for shell, out := range outputs {
		for _, name := range completionNames() {
			if !completionHasToken(out, name) {
				t.Errorf("%s completion missing subcommand %s", shell, name)
			}
		}
		for _, spec := range specs {
			for _, f := range spec.Flags {
				token := "-" + f.Name
				if shell == "fish" {
					if len(f.Name) == 1 {
						token = "-s " + f.Name
					} else {
						token = "-l " + f.Name
					}
				}
				if !completionHasToken(out, token) {
					t.Errorf("%s completion missing %s flag -%s", shell, spec.Command.Name, f.Name)
				}
			}
		}
	}
}

func completionHasToken(text, token string) bool {
	pattern := `(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(token) + `([^A-Za-z0-9_-]|$)`
	return regexp.MustCompile(pattern).MatchString(text)
}

func TestCompletionPathFlags(t *testing.T) {
	specs, err := completionSpecs()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		command string
		flags   []string
	}{
		{CmdSH, []string{"config", "i"}},
		{CmdCP, []string{"config", "i", "j", "s", "save"}},
		{CmdX, []string{"config", "i"}},
		{CmdID, []string{"i", "w"}},
		{CmdPageserver, []string{"config", "c", "pages-dir", "p", "files-dir", "f", "identity", "identity-path"}},
	} {
		spec := specByName(specs, tc.command)
		if spec == nil {
			t.Fatalf("missing spec for %s", tc.command)
		}
		for _, name := range tc.flags {
			found := false
			for _, f := range spec.Flags {
				if f.Name == name {
					found = f.Path
					break
				}
			}
			if !found {
				t.Errorf("%s -%s is not marked as a path flag", tc.command, name)
			}
		}
	}
}

func TestCompletionBashSyntax(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not on PATH")
	}
	specs, err := completionSpecs()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "reticulum-go")
	if err := os.WriteFile(path, []byte(generateBashCompletion(specs)), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, "-n", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}
}

func TestCompletionZshSyntax(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not on PATH")
	}
	specs, err := completionSpecs()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "_reticulum-go")
	if err := os.WriteFile(path, []byte(generateZshCompletion(specs)), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(zsh, "-n", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("zsh -n: %v\n%s", err, out)
	}
}

func TestCompletionCommandViaMain(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		var stdout, stderr bytes.Buffer
		code := Main([]string{"completion", shell}, Options{Stdout: &stdout, Stderr: &stderr, VersionLine: "test"})
		if code != 0 {
			t.Fatalf("%s code=%d stderr=%s", shell, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "reticulum-go") {
			t.Fatalf("%s output missing command name", shell)
		}
	}
}
