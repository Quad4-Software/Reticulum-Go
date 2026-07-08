// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type overlayFile struct {
	Replace map[string]string `json:"Replace"`
}

type runner struct {
	workDir     string
	testTimeout time.Duration
	verbose     bool
}

func newRunner(verbose bool, testTimeout time.Duration) (*runner, error) {
	dir, err := os.MkdirTemp("", "gomutant-run-*")
	if err != nil {
		return nil, err
	}
	return &runner{
		workDir:     dir,
		testTimeout: testTimeout,
		verbose:     verbose,
	}, nil
}

func (r *runner) Close() error {
	return os.RemoveAll(r.workDir)
}

func (r *runner) runMutant(ctx context.Context, m mutant) outcome {
	absFile, err := filepath.Abs(m.File)
	if err != nil {
		return outcome{Mutant: m, Result: resultSkipped, Err: err.Error()}
	}

	mutantPath := filepath.Join(r.workDir, fmt.Sprintf("mutant-%d.go", m.ID))
	if err := os.WriteFile(mutantPath, m.Source, 0o600); err != nil {
		return outcome{Mutant: m, Result: resultSkipped, Err: err.Error()}
	}

	overlayPath := filepath.Join(r.workDir, fmt.Sprintf("overlay-%d.json", m.ID))
	payload := overlayFile{Replace: map[string]string{absFile: mutantPath}}
	raw, err := json.Marshal(payload)
	if err != nil {
		return outcome{Mutant: m, Result: resultSkipped, Err: err.Error()}
	}
	if err := os.WriteFile(overlayPath, raw, 0o600); err != nil {
		return outcome{Mutant: m, Result: resultSkipped, Err: err.Error()}
	}

	testCtx, cancel := context.WithTimeout(ctx, r.testTimeout)
	defer cancel()

	cmd := exec.CommandContext(testCtx, "go", "test", m.Pkg,
		"-overlay", overlayPath,
		"-count=1",
		"-timeout", fmt.Sprintf("%ds", int(r.testTimeout.Seconds())),
	)
	cmd.Env = os.Environ()

	out, runErr := cmd.CombinedOutput()
	text := string(out)
	switch {
	case testCtx.Err() == context.DeadlineExceeded:
		return outcome{Mutant: m, Result: resultTimeout, Err: "test timeout"}
	case isBuildFailure(text, runErr):
		return outcome{Mutant: m, Result: resultBuildFail, Err: trimOutput(text)}
	case runErr == nil:
		return outcome{Mutant: m, Result: resultSurvived}
	default:
		return outcome{Mutant: m, Result: resultKilled, Err: trimOutput(text)}
	}
}

func isBuildFailure(output string, err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(output)
	return strings.Contains(lower, "build failed") ||
		strings.Contains(lower, "cannot use") ||
		strings.Contains(lower, "syntax error") ||
		strings.Contains(lower, "# ")
}

func trimOutput(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 240 {
		return s[:240] + "..."
	}
	return s
}

func runMutants(ctx context.Context, mutants []mutant, workers int, verbose bool, testTimeout time.Duration) ([]outcome, error) {
	r, err := newRunner(verbose, testTimeout)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	jobs := make(chan mutant)
	results := make(chan outcome, len(mutants))

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for m := range jobs {
				results <- r.runMutant(ctx, m)
			}
		})
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	go func() {
		for _, m := range mutants {
			select {
			case <-ctx.Done():
				return
			case jobs <- m:
			}
		}
		close(jobs)
	}()

	var out []outcome
	for o := range results {
		out = append(out, o)
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	return out, nil
}

func efficacy(outcomes []outcome) (killed, lived, skipped int, pct float64) {
	for _, o := range outcomes {
		switch o.Result {
		case resultKilled:
			killed++
		case resultSurvived:
			lived++
		default:
			skipped++
		}
	}
	total := killed + lived
	if total == 0 {
		return killed, lived, skipped, 0
	}
	pct = float64(killed) / float64(total) * 100
	return killed, lived, skipped, pct
}
