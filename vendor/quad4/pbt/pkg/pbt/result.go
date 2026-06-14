// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import "fmt"

// Result is the high-level outcome of a property check.
type Result[T any] struct {
	Passed            bool           // True if all runs passed and coverage met.
	TimedOut          bool           // True if the run was aborted by timeout.
	CoverageFailed    bool           // True if label/bucket thresholds were not met.
	PropertyName      string         // Name of the property that was checked.
	Runs              int            // Total number of runs executed.
	Seed              int64          // Random seed used; use WithSeed to reproduce.
	GeneratorName     string         // Name of the generator that produced inputs.
	Counterexample    T              // Failing value when HasCounterexample is true.
	HasCounterexample bool           // True if a counterexample was found.
	FailureLabels     []string       // Classifier output for the failing value.
	ShrinkTrace       []T            // Minimization path when a shrinker was used.
	LabelCounts       map[string]int // Per-label counts for coverage.
	BucketCounts      map[string]int // Per-bucket counts for coverage.
	CoverageErrors    []string       // Threshold violations when CoverageFailed is true.
}

// Error returns a readable message when a result is failing.
func (r Result[T]) Error() string {
	if r.Passed {
		return ""
	}
	if r.TimedOut {
		return fmt.Sprintf(
			"property %q timed out after %d runs budget (seed=%d, generator=%s)",
			r.PropertyName,
			r.Runs,
			r.Seed,
			r.GeneratorName,
		)
	}
	if r.CoverageFailed {
		return fmt.Sprintf(
			"property %q failed coverage thresholds after %d runs (seed=%d, generator=%s, errors=%v)",
			r.PropertyName,
			r.Runs,
			r.Seed,
			r.GeneratorName,
			r.CoverageErrors,
		)
	}
	return fmt.Sprintf(
		"property %q failed after %d runs (seed=%d, generator=%s, counterexample=%v, labels=%v)",
		r.PropertyName,
		r.Runs,
		r.Seed,
		r.GeneratorName,
		r.Counterexample,
		r.FailureLabels,
	)
}
