// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestRunMutantKillsOnSamplePackage(t *testing.T) {
	pkg := "./testdata/sample"
	src := filepath.Join("testdata", "sample", "sample.go")
	mutants, _, err := scanFile(pkg, src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(mutants) == 0 {
		t.Fatal("expected mutants")
	}

	r, err := newRunner(false, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	killed := 0
	for _, m := range mutants {
		m.Pkg = pkg
		o := r.runMutant(context.Background(), m)
		if o.Result == resultKilled {
			killed++
		}
	}
	if killed == 0 {
		t.Fatal("expected at least one killed mutant")
	}
}
