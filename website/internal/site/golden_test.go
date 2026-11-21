package site

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Quad4-Software/hugo-reticulum/internal/markdown"
)

// TestExampleSiteGolden builds the example site fixture with Hugo, runs sentinel
// processing, and compares every .mu file against the recorded golden
// output in testdata/example. It skips when hugo is not on PATH.
func TestExampleSiteGolden(t *testing.T) {
	hugo, err := exec.LookPath("hugo")
	if err != nil {
		t.Skip("hugo not on PATH")
	}

	_, thisFile, _, _ := runtime.Caller(0)
	siteDir := filepath.Join(filepath.Dir(thisFile), "testdata", "exampleSite")
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	work := t.TempDir()
	if err := copyDir(siteDir, work); err != nil {
		t.Fatalf("copy example site fixture: %v", err)
	}

	// Point the module import at this repository regardless of where the
	// temporary copy lives.
	env := append(os.Environ(),
		"HUGO_MODULE_REPLACEMENTS=github.com/Quad4-Software/hugo-reticulum -> "+repoRoot)
	cmd := exec.Command(hugo, "--source", work) // #nosec G204 -- test fixture
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hugo build: %v\n%s", err, out)
	}

	publicDir := filepath.Join(work, "public")
	opts := Options{
		Resolver:    &Resolver{PublicDir: publicDir},
		MaxWidth:    72,
		LinkStyle:   markdown.LinkBoth,
		ImagePolicy: markdown.ImageLink,
		TableAlign:  "center",
	}
	if _, err := WalkFiles(publicDir, opts); err != nil {
		t.Fatalf("WalkFiles: %v", err)
	}

	goldenRoot := filepath.Join(filepath.Dir(thisFile), "testdata", "example")
	err = filepath.WalkDir(goldenRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(goldenRoot, path)
		if err != nil {
			return err
		}
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(publicDir, rel))
		if err != nil {
			return err
		}
		if string(want) != string(got) {
			t.Errorf("%s differs from golden output", filepath.ToSlash(rel))
			if testing.Verbose() {
				t.Logf("--- want ---\n%s\n--- got ---\n%s", want, got)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		// Skip build output so the copy stays clean.
		if d.IsDir() && (rel == "public" || rel == "resources" ||
			strings.HasPrefix(rel, "public"+string(os.PathSeparator)) ||
			strings.HasPrefix(rel, "resources"+string(os.PathSeparator))) {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
