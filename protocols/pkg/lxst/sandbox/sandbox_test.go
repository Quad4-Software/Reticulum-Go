// SPDX-License-Identifier: LicenseRef-Reticulum
package sandbox

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllowRWRejectsSystemRoots(t *testing.T) {
	denied := []string{"/", "/usr", "/usr/bin", "/etc", "/etc/passwd", "/bin", "/sbin", "/lib", "/lib64", "/boot", "/sys", "/proc", "/root", "/root/.ssh"}
	for _, p := range denied {
		if allowRW(p) {
			t.Fatalf("allowed RW %s", p)
		}
	}
	if !allowRW("/home/user/rgesp") {
		t.Fatal("home dir should be eligible")
	}
	if allowRW("") {
		t.Fatal("empty")
	}
}

func TestForbiddenConfigKey(t *testing.T) {
	yes := []string{
		"sandbox", "SANDBOX", "no-sandbox", "no_sandbox", "nosandbox",
		"disable-sandbox", "enable_sandbox", "seccomp", "no-seccomp",
		"landlock", "disable-landlock",
	}
	for _, k := range yes {
		if !ForbiddenConfigKey(k) {
			t.Fatalf("should reject %q", k)
		}
	}
	no := []string{"profile", "identity", "rnsconfig", "server", "allowed_callers"}
	for _, k := range no {
		if ForbiddenConfigKey(k) {
			t.Fatalf("should allow %q", k)
		}
	}
}

func TestBuildPolicyDropsForbiddenRW(t *testing.T) {
	tmp := t.TempDir()
	pol := buildPolicy(Paths{ReadWrite: []string{"/", "/usr", "/etc", tmp}})
	for _, p := range pol.rw {
		if p == "/" || underRoot(p, "/usr") || underRoot(p, "/etc") {
			t.Fatalf("forbidden RW kept %s", p)
		}
	}
	if pathExists(tmp) {
		ok := false
		for _, p := range pol.rw {
			if p == filepath.Clean(tmp) || underRoot(tmp, p) {
				ok = true
				break
			}
		}
		if !ok {
			t.Fatalf("temp dir missing from RW %v", pol.rw)
		}
	}
}

func TestSandboxSourceHasNoDisableSwitch(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{"Getenv", "LookupEnv", "flag.", "os.Args"}
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(src)
		for _, b := range banned {
			if strings.Contains(text, b) {
				t.Errorf("%s contains %q", path, b)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSandboxPackageASTHasNoEnvOrFlag(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, pkg := range pkgs {
		if strings.HasSuffix(name, "_test") {
			continue
		}
		for path := range pkg.Files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(src)
			if strings.Contains(text, "os.Getenv") || strings.Contains(text, "os.LookupEnv") {
				t.Errorf("%s reads environment", path)
			}
			if strings.Contains(text, "flag.") {
				t.Errorf("%s uses flags", path)
			}
		}
	}
}
