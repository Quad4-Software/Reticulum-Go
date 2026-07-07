// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// gomutant is a lightweight mutation tester for Go packages in this repo.
// It applies same-width operator swaps and runs package tests via go test -overlay.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("gomutant", flag.ExitOnError)
	var (
		pkgs      multiFlag
		threshold = fs.Float64("threshold", 55, "minimum efficacy percent (killed over killed+survived)")
		workers   = fs.Int("workers", 4, "parallel test workers")
		timeout   = fs.Duration("timeout", 45*time.Second, "per-mutant go test timeout")
		maxMut    = fs.Int("max", 0, "maximum mutants to run per package (0 = all)")
		verbose   = fs.Bool("v", false, "verbose output")
	)
	fs.Var(&pkgs, "pkg", "package to test (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(pkgs) == 0 {
		pkgs = []string{"./pkg/packet", "./pkg/cryptography"}
	}

	failed := false
	for _, pkg := range pkgs {
		ok, err := runPackage(pkg, *threshold, *workers, *timeout, *maxMut, *verbose)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gomutant: %s: %v\n", pkg, err)
			failed = true
			continue
		}
		if !ok {
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

func runPackage(pkg string, threshold float64, workers int, timeout time.Duration, maxMut int, verbose bool) (bool, error) {
	mutants, _, err := scanPackage(pkg, 1)
	if err != nil {
		return false, err
	}
	if maxMut > 0 && len(mutants) > maxMut {
		mutants = sampleMutants(mutants, maxMut)
	}
	if len(mutants) == 0 {
		fmt.Printf("gomutant: %s: no mutants found\n", pkg)
		return true, nil
	}

	fmt.Printf("gomutant: %s: running %d mutants (workers=%d)\n", pkg, len(mutants), workers)
	ctx := context.Background()
	outcomes, err := runMutants(ctx, mutants, workers, verbose, timeout)
	if err != nil {
		return false, err
	}

	killed, lived, skipped, pct := efficacy(outcomes)
	for _, o := range outcomes {
		if !verbose && o.Result != resultSurvived {
			continue
		}
		rel, _ := filepath.Rel(moduleRoot(), o.Mutant.File)
		if rel == "" {
			rel = o.Mutant.File
		}
		fmt.Printf("%-10s %s:%d:%d  %s -> %s\n",
			o.Result, rel, o.Mutant.Line, o.Mutant.Column, o.Mutant.From, o.Mutant.To)
	}

	fmt.Printf("gomutant: %s: efficacy %.1f%% (killed=%d lived=%d skipped=%d) threshold=%.1f%%\n",
		pkg, pct, killed, lived, skipped, threshold)

	if killed+lived == 0 {
		return false, fmt.Errorf("no viable mutants")
	}
	if pct < threshold {
		return false, nil
	}
	return true, nil
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func packageDir(pkg string) (string, error) {
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", pkg)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list %s: %w", pkg, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func moduleRoot() string {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func sampleMutants(mutants []mutant, max int) []mutant {
	if max <= 0 || len(mutants) <= max {
		return mutants
	}
	idx := make([]int, len(mutants))
	for i := range idx {
		idx[i] = i
	}
	rng := rand.New(rand.NewPCG(0x6d757461, 0x6e7421))
	rng.Shuffle(len(idx), func(i, j int) {
		idx[i], idx[j] = idx[j], idx[i]
	})
	out := make([]mutant, max)
	for i := range max {
		out[i] = mutants[idx[i]]
	}
	return out
}
