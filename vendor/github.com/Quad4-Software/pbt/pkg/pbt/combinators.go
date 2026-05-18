// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import "math/rand"

// Tuple2Value stores a generated pair of values.
type Tuple2Value[A any, B any] struct {
	First  A
	Second B
}

// Tuple3Value stores a generated triple of values.
type Tuple3Value[A any, B any, C any] struct {
	First  A
	Second B
	Third  C
}

// WeightedGenerator holds a weighted generator entry for Frequency.
type WeightedGenerator[T any] struct {
	Weight    int          // Relative weight; higher values are chosen more often.
	Generator Generator[T] // Generator to use when this entry is selected.
}

// Tuple2 combines two generators into a product generator.
func Tuple2[A any, B any](name string, left Generator[A], right Generator[B]) Generator[Tuple2Value[A, B]] {
	return NewGenerator(name, func(r *rand.Rand, size int) Tuple2Value[A, B] {
		return Tuple2Value[A, B]{
			First:  left.Generate(r, size),
			Second: right.Generate(r, size),
		}
	})
}

// Product2 is an alias for Tuple2.
func Product2[A any, B any](name string, left Generator[A], right Generator[B]) Generator[Tuple2Value[A, B]] {
	return Tuple2(name, left, right)
}

// Tuple3 combines three generators into a product generator.
func Tuple3[A any, B any, C any](name string, first Generator[A], second Generator[B], third Generator[C]) Generator[Tuple3Value[A, B, C]] {
	return NewGenerator(name, func(r *rand.Rand, size int) Tuple3Value[A, B, C] {
		return Tuple3Value[A, B, C]{
			First:  first.Generate(r, size),
			Second: second.Generate(r, size),
			Third:  third.Generate(r, size),
		}
	})
}

// OneOf picks one generator uniformly at random.
func OneOf[T any](name string, generators ...Generator[T]) Generator[T] {
	return NewGenerator(name, func(r *rand.Rand, size int) T {
		if len(generators) == 0 {
			panic("pbt: OneOf requires at least one generator")
		}
		pick := generators[r.Intn(len(generators))]
		return pick.Generate(r, size)
	})
}

// Frequency picks a generator using relative integer weights.
func Frequency[T any](name string, entries ...WeightedGenerator[T]) Generator[T] {
	return NewGenerator(name, func(r *rand.Rand, size int) T {
		if len(entries) == 0 {
			panic("pbt: Frequency requires at least one entry")
		}

		total := 0
		for _, e := range entries {
			if e.Weight > 0 {
				total += e.Weight
			}
		}
		if total <= 0 {
			panic("pbt: Frequency requires at least one positive weight")
		}

		target := r.Intn(total)
		acc := 0
		for _, e := range entries {
			if e.Weight <= 0 {
				continue
			}
			acc += e.Weight
			if target < acc {
				return e.Generator.Generate(r, size)
			}
		}

		return entries[len(entries)-1].Generator.Generate(r, size)
	})
}

const defaultSuchThatAttempts = 1000

// SuchThat filters generated values to those satisfying the predicate.
// Tries up to maxAttempts times; panics if no satisfying value is found.
// Use maxAttempts <= 0 for the default (1000).
// For a non-panic path when the predicate may be hard to satisfy, use SuchThatFallback.
func SuchThat[T any](name string, source Generator[T], predicate Predicate[T], maxAttempts int) Generator[T] {
	if maxAttempts <= 0 {
		maxAttempts = defaultSuchThatAttempts
	}
	return NewGenerator(name, func(r *rand.Rand, size int) T {
		for i := 0; i < maxAttempts; i++ {
			v := source.Generate(r, size)
			if predicate(v) {
				return v
			}
		}
		panic("pbt: SuchThat failed to find satisfying value within maxAttempts")
	})
}

// SuchThatFallback filters generated values to those satisfying the predicate.
// When no value is found within maxAttempts, returns fallback instead of panicking.
// Fallback must satisfy the predicate; otherwise the property may fail with a misleading counterexample.
// Use maxAttempts <= 0 for the default (1000).
func SuchThatFallback[T any](name string, source Generator[T], predicate Predicate[T], fallback T, maxAttempts int) Generator[T] {
	if maxAttempts <= 0 {
		maxAttempts = defaultSuchThatAttempts
	}
	return NewGenerator(name, func(r *rand.Rand, size int) T {
		for i := 0; i < maxAttempts; i++ {
			v := source.Generate(r, size)
			if predicate(v) {
				return v
			}
		}
		return fallback
	})
}

// Recursive builds recursive generators with a bounded depth.
func Recursive[T any](name string, base Generator[T], combine func(self Generator[T]) Generator[T], maxDepth int) Generator[T] {
	if maxDepth <= 0 {
		maxDepth = 1
	}

	var genAtDepth func(depth int) Generator[T]
	genAtDepth = func(depth int) Generator[T] {
		if depth <= 0 {
			return base
		}

		child := genAtDepth(depth - 1)
		combined := combine(child)
		return OneOf(name+"-depth", base, combined)
	}

	return NewGenerator(name, func(r *rand.Rand, size int) T {
		depth := maxDepth
		if size > 0 && size < depth {
			depth = size
		}
		return genAtDepth(depth).Generate(r, size)
	})
}
