// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import (
	"math/rand"
	"strings"
	"time"
	"unicode"
)

const asciiAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Generator creates random values for a property check.
type Generator[T any] interface {
	Generate(r *rand.Rand, size int) T
	Name() string
}

// GeneratorFunc adapts plain functions into a Generator.
type GeneratorFunc[T any] struct {
	name     string
	fn       func(r *rand.Rand, size int) T
	shrinker Shrinker[T]
}

// NewGenerator builds a named generator from a function.
func NewGenerator[T any](name string, fn func(r *rand.Rand, size int) T) Generator[T] {
	return GeneratorFunc[T]{
		name: name,
		fn:   fn,
	}
}

// NewShrinkableGenerator builds a named generator that carries a shrinker, so
// properties using it minimize counterexamples without an explicit
// WithShrinker.
func NewShrinkableGenerator[T any](name string, fn func(r *rand.Rand, size int) T, shrinker Shrinker[T]) Generator[T] {
	return GeneratorFunc[T]{
		name:     name,
		fn:       fn,
		shrinker: shrinker,
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

// Shrinker returns the attached shrinker, or nil when the generator was built
// with NewGenerator.
func (g GeneratorFunc[T]) Shrinker() Shrinker[T] {
	return g.shrinker
}

// ShrinkableGenerator is a Generator that knows how to shrink its own values.
// Properties checked without an explicit WithShrinker use it automatically.
type ShrinkableGenerator[T any] interface {
	Generator[T]
	Shrinker() Shrinker[T]
}

// shrinkerFor extracts the attached shrinker from a generator, if any.
func shrinkerFor[T any](generator Generator[T]) Shrinker[T] {
	if sg, ok := generator.(ShrinkableGenerator[T]); ok {
		return sg.Shrinker()
	}
	return nil
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

	shrinker := Shrinker[int](IntShrinker())
	if low != 0 {
		shrinker = IntShrinkerToward(low)
	}
	return NewShrinkableGenerator("IntRange", func(r *rand.Rand, _ int) int {
		// #nosec G115 -- unsigned width arithmetic intentionally handles full int span.
		width := uint(high) - uint(low) + 1
		if width == 0 {
			// #nosec G115 -- full-range random bit pattern mapped directly to int.
			return int(r.Uint64())
		}
		// #nosec G115 -- modulo mapping intentionally uses unsigned arithmetic.
		return int(uint(low) + uint(r.Uint64()%uint64(width)))
	}, shrinker)
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

	return NewShrinkableGenerator("StringASCII", func(r *rand.Rand, size int) string {
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
	}, StringShrinker())
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

	shrinker := Shrinker[[]T](SliceShrinker[T]())
	if es := shrinkerFor(elem); es != nil {
		shrinker = SliceShrinkerOf(es)
	}
	return NewShrinkableGenerator("SliceOf", func(r *rand.Rand, size int) []T {
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
	}, shrinker)
}

// Map transforms values produced by a generator.
func Map[A any, B any](name string, source Generator[A], mapper func(A) B) Generator[B] {
	return NewGenerator(name, func(r *rand.Rand, size int) B {
		return mapper(source.Generate(r, size))
	})
}

// Int64 generates values across the full int64 range.
func Int64() Generator[int64] {
	return NewShrinkableGenerator("Int64", func(r *rand.Rand, _ int) int64 {
		// #nosec G115 -- full-range bit pattern cast is the intended distribution.
		return int64(r.Uint64())
	}, Int64Shrinker())
}

// Int64Range generates int64 values in the inclusive range [low, high].
func Int64Range(low int64, high int64) Generator[int64] {
	if low > high {
		low, high = high, low
	}

	shrinker := Shrinker[int64](Int64Shrinker())
	if low != 0 {
		shrinker = Int64ShrinkerToward(low)
	}
	return NewShrinkableGenerator("Int64Range", func(r *rand.Rand, _ int) int64 {
		// #nosec G115 -- unsigned width arithmetic intentionally handles full int64 span.
		width := uint64(high) - uint64(low) + 1
		if width == 0 {
			// #nosec G115 -- full-range random bit pattern mapped directly to int64.
			return int64(r.Uint64())
		}
		// #nosec G115 -- modulo mapping intentionally uses unsigned arithmetic.
		return int64(uint64(low) + r.Uint64()%width)
	}, shrinker)
}

// Uint64 generates values across the full uint64 range.
func Uint64() Generator[uint64] {
	return NewShrinkableGenerator("Uint64", func(r *rand.Rand, _ int) uint64 {
		return r.Uint64()
	}, Uint64Shrinker())
}

// Bytes generates byte slices with length in [low, high].
func Bytes(low int, high int) Generator[[]byte] {
	if low < 0 {
		low = 0
	}
	if low > high {
		low, high = high, low
		if low < 0 {
			low = 0
		}
	}

	return NewShrinkableGenerator("Bytes", func(r *rand.Rand, size int) []byte {
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

		out := make([]byte, length)
		// rand.Rand.Read fills the slice deterministically and never fails.
		_, _ = r.Read(out)
		return out
	}, BytesShrinker())
}

// String generates UTF-8 strings of printable runes with length in [low, high]
// runes. Unlike StringASCII the alphabet covers the full Unicode printable
// range, which exercises encoding and validation paths that ASCII misses.
func String(low int, high int) Generator[string] {
	if low < 0 {
		low = 0
	}
	if low > high {
		low, high = high, low
		if low < 0 {
			low = 0
		}
	}

	return NewShrinkableGenerator("String", func(r *rand.Rand, size int) string {
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
		for i := 0; i < length; i++ {
			b.WriteRune(randomPrintableRune(r))
		}
		return b.String()
	}, StringShrinker())
}

// randomPrintableRune draws a printable non-surrogate code point. Rejection is
// bounded because roughly a quarter of the code point space is printable.
func randomPrintableRune(r *rand.Rand) rune {
	for i := 0; i < 64; i++ {
		candidate := rune(r.Uint32() % 0x110000)
		if candidate >= 0xD800 && candidate <= 0xDFFF {
			continue
		}
		if unicode.IsPrint(candidate) {
			return candidate
		}
	}
	return 'x'
}

// MapOf generates maps with up to high entries drawn from the key and value
// generators. Duplicate keys collapse, so the resulting map can contain fewer
// than low entries when the key space is small.
func MapOf[K comparable, V any](key Generator[K], value Generator[V], low int, high int) Generator[map[K]V] {
	if low < 0 {
		low = 0
	}
	if low > high {
		low, high = high, low
		if low < 0 {
			low = 0
		}
	}

	return NewShrinkableGenerator("MapOf", func(r *rand.Rand, size int) map[K]V {
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

		out := make(map[K]V, length)
		for i := 0; i < length; i++ {
			out[key.Generate(r, size)] = value.Generate(r, size)
		}
		return out
	}, MapShrinker[K, V]())
}

// DurationRange generates time.Duration values in the inclusive range
// [low, high].
func DurationRange(low time.Duration, high time.Duration) Generator[time.Duration] {
	if low > high {
		low, high = high, low
	}

	return NewShrinkableGenerator("DurationRange", func(r *rand.Rand, _ int) time.Duration {
		// #nosec G115 -- unsigned width arithmetic intentionally handles full duration span.
		width := uint64(high) - uint64(low) + 1
		if width == 0 {
			// #nosec G115 -- full-range random bit pattern mapped directly to duration.
			return time.Duration(r.Uint64())
		}
		// #nosec G115 -- modulo mapping intentionally uses unsigned arithmetic.
		return time.Duration(uint64(low) + r.Uint64()%width)
	}, DurationShrinkerToward(low))
}

// PtrOf generates pointers where nilPercent percent of values are nil. A
// nilPercent of 0 always produces non-nil values, 100 always produces nil.
func PtrOf[T any](elem Generator[T], nilPercent int) Generator[*T] {
	if nilPercent < 0 {
		nilPercent = 0
	}
	if nilPercent > 100 {
		nilPercent = 100
	}

	return NewShrinkableGenerator("PtrOf", func(r *rand.Rand, size int) *T {
		if r.Intn(100) < nilPercent {
			return nil
		}
		value := elem.Generate(r, size)
		return &value
	}, PtrShrinker(shrinkerFor(elem)))
}
