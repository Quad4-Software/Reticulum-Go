// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

const minShrinkWorkers = 1

func shrinkWithTrace[T any](shrinker Shrinker[T], value T, predicate Predicate[T], workers int) (T, []T) {
	if workers < minShrinkWorkers {
		workers = minShrinkWorkers
	}
	if parallel, ok := shrinker.(ParallelTraceShrinker[T]); ok {
		trace, final, changed := parallel.ShrinkTraceParallel(value, predicate, workers)
		if changed {
			return final, trace
		}
		return value, trace
	}
	if traced, ok := shrinker.(TraceShrinker[T]); ok {
		trace, final, changed := traced.ShrinkTrace(value, predicate)
		if changed {
			return final, trace
		}
		return value, trace
	}

	final, changed := shrinker.Shrink(value, predicate)
	if !changed {
		return value, nil
	}
	return final, []T{value, final}
}
