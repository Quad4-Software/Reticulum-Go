// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

const minShrinkWorkers = 1

func shrinkWithTrace[T any](shrinker Shrinker[T], value T, predicate Predicate[T], workers int) (T, []T) {
	if workers < minShrinkWorkers {
		workers = minShrinkWorkers
	}

	var (
		trace   []T
		final   T
		changed bool
	)
	switch s := shrinker.(type) {
	case ParallelTraceShrinker[T]:
		trace, final, changed = s.ShrinkTraceParallel(value, predicate, workers)
	case TraceShrinker[T]:
		trace, final, changed = s.ShrinkTrace(value, predicate)
	default:
		final, changed = shrinker.Shrink(value, predicate)
	}

	if !changed {
		return value, nil
	}
	// A shrinker must only return values that still fail the predicate. When a
	// custom shrinker violates that contract the original counterexample is
	// kept so the reported value always reproduces the failure.
	if predicate(final) {
		return value, nil
	}
	if len(trace) == 0 {
		trace = []T{value, final}
	}
	return final, trace
}
