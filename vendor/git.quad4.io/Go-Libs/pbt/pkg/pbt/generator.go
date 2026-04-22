// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import (
	"math/rand"
	"strings"
)

const asciiAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Generator creates random values for a property check.
type Generator[T any] interface {
	Generate(r *rand.Rand, size int) T
	Name() string
}

// GeneratorFunc adapts plain functions into a Generator.
type GeneratorFunc[T any] struct {
	name string
	fn   func(r *rand.Rand, size int) T
}

// NewGenerator builds a named generator from a function.
func NewGenerator[T any](name string, fn func(r *rand.Rand, size int) T) Generator[T] {
	return GeneratorFunc[T]{
		name: name,
		fn:   fn,
	}
}

// Generate creates a value by invoking the underlying function.
func (g GeneratorFunc[T]) Generate(r *rand.Rand, size int) T {
	return g.fn(r, size)
}

// Name returns the generator name for reporting.
func (g GeneratorFunc[T]) Name() string {
	return g.name
}

// Int generates values across the full int range.
func Int() Generator[int] {
	return NewGenerator("Int", func(r *rand.Rand, _ int) int {
		return r.Int()
	})
}

// IntRange generates integers in the inclusive range [low, high].
func IntRange(low int, high int) Generator[int] {
	if low > high {
		low, high = high, low
	}

	return NewGenerator("IntRange", func(r *rand.Rand, _ int) int {
		// #nosec G115 -- unsigned width arithmetic intentionally handles full int span.
		width := uint(high) - uint(low) + 1
		if width == 0 {
			// #nosec G115 -- full-range random bit pattern mapped directly to int.
			return int(r.Uint64())
		}
		// #nosec G115 -- modulo mapping intentionally uses unsigned arithmetic.
		return int(uint(low) + uint(r.Uint64()%uint64(width)))
	})
}

// Bool generates random boolean values.
func Bool() Generator[bool] {
	return NewGenerator("Bool", func(r *rand.Rand, _ int) bool {
		return r.Intn(2) == 1
	})
}

// Float64 generates finite float64 values in [0, 1).
func Float64() Generator[float64] {
	return NewGenerator("Float64", func(r *rand.Rand, _ int) float64 {
		return r.Float64()
	})
}

// StringASCII generates ASCII-alphanumeric strings within [low, high] length.
func StringASCII(low int, high int) Generator[string] {
	if low < 0 {
		low = 0
	}
	if low > high {
		low, high = high, low
		if low < 0 {
			low = 0
		}
	}

	return NewGenerator("StringASCII", func(r *rand.Rand, size int) string {
		localHigh := high
		if size > 0 && size < localHigh {
			localHigh = size
		}
		if localHigh < low {
			localHigh = low
		}

		length := low
		if localHigh > low {
			length = low + r.Intn(localHigh-low+1)
		}

		var b strings.Builder
		b.Grow(length)
		for i := 0; i < length; i++ {
			b.WriteByte(asciiAlphabet[r.Intn(len(asciiAlphabet))])
		}
		return b.String()
	})
}

// SliceOf generates slices with values from the provided element generator.
func SliceOf[T any](elem Generator[T], low int, high int) Generator[[]T] {
	if low < 0 {
		low = 0
	}
	if low > high {
		low, high = high, low
		if low < 0 {
			low = 0
		}
	}

	return NewGenerator("SliceOf", func(r *rand.Rand, size int) []T {
		localHigh := high
		if size > 0 && size < localHigh {
			localHigh = size
		}
		if localHigh < low {
			localHigh = low
		}

		length := low
		if localHigh > low {
			length = low + r.Intn(localHigh-low+1)
		}

		out := make([]T, 0, length)
		for i := 0; i < length; i++ {
			out = append(out, elem.Generate(r, size))
		}
		return out
	})
}

// Map transforms values produced by a generator.
func Map[A any, B any](name string, source Generator[A], mapper func(A) B) Generator[B] {
	return NewGenerator(name, func(r *rand.Rand, size int) B {
		return mapper(source.Generate(r, size))
	})
}
