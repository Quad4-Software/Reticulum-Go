// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import "math/rand"

// Sample draws n values from a generator with the given seed and returns them
// in generation order. The size parameter grows across the sample the same way
// it does during a check run. Use it to inspect a generator's distribution
// before writing properties.
func Sample[T any](generator Generator[T], n int, seed int64) []T {
	if generator == nil || n <= 0 {
		return nil
	}

	// #nosec G404 -- deterministic PRNG is required for reproducible samples.
	rng := rand.New(rand.NewSource(seed))
	out := make([]T, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, generator.Generate(rng, sizeForRun(i, n, defaultMaxSize)))
	}
	return out
}
