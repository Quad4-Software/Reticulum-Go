// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import "strings"

// Shrinker attempts to minimize a failing value while preserving failure.
type Shrinker[T any] interface {
	Shrink(value T, predicate Predicate[T]) (T, bool)
}

// TraceShrinker extends Shrinker with shrink path introspection.
type TraceShrinker[T any] interface {
	Shrinker[T]
	ShrinkTrace(value T, predicate Predicate[T]) ([]T, T, bool)
}

// ParallelTraceShrinker supports shrink strategy tuning with worker count.
type ParallelTraceShrinker[T any] interface {
	Shrinker[T]
	ShrinkTraceParallel(value T, predicate Predicate[T], workers int) ([]T, T, bool)
}

// ShrinkerFunc adapts a function into a Shrinker.
type ShrinkerFunc[T any] func(value T, predicate Predicate[T]) (T, bool)

// Shrink minimizes a value by invoking the wrapped function.
func (s ShrinkerFunc[T]) Shrink(value T, predicate Predicate[T]) (T, bool) {
	return s(value, predicate)
}

// IntShrinker returns a shrinker that moves failing integers toward zero.
func IntShrinker() Shrinker[int] {
	return intShrinker{}
}

// StringShrinker returns a shrinker that shortens failing strings.
func StringShrinker() Shrinker[string] {
	return stringShrinker{}
}

// SliceShrinker returns a shrinker that shortens failing slices by removing elements.
func SliceShrinker[T any]() Shrinker[[]T] {
	return sliceShrinker[T]{}
}

type intShrinker struct{}

func (s intShrinker) Shrink(value int, predicate Predicate[int]) (int, bool) {
	_, final, changed := s.ShrinkTrace(value, predicate)
	return final, changed
}

func (s intShrinker) ShrinkTraceParallel(value int, predicate Predicate[int], _ int) ([]int, int, bool) {
	return s.ShrinkTrace(value, predicate)
}

func (s intShrinker) ShrinkTrace(value int, predicate Predicate[int]) ([]int, int, bool) {
	if predicate(value) {
		return nil, 0, false
	}

	candidate := value
	trace := []int{value}
	changed := false

	for candidate != 0 {
		next := candidate / 2
		if next == candidate {
			break
		}
		if predicate(next) {
			break
		}
		candidate = next
		trace = append(trace, candidate)
		changed = true
	}

	return trace, candidate, changed
}

type stringShrinker struct{}

func (s stringShrinker) Shrink(value string, predicate Predicate[string]) (string, bool) {
	_, final, changed := s.ShrinkTrace(value, predicate)
	return final, changed
}

func (s stringShrinker) ShrinkTraceParallel(value string, predicate Predicate[string], _ int) ([]string, string, bool) {
	return s.ShrinkTrace(value, predicate)
}

func (s stringShrinker) ShrinkTrace(value string, predicate Predicate[string]) ([]string, string, bool) {
	if predicate(value) {
		return nil, "", false
	}

	candidate := value
	trace := []string{value}
	changed := false

	for len(candidate) > 0 {
		next := candidate[:len(candidate)/2]
		if predicate(next) {
			break
		}
		candidate = next
		trace = append(trace, candidate)
		changed = true
	}

	trimmed := strings.TrimSpace(candidate)
	if trimmed != candidate && !predicate(trimmed) {
		candidate = trimmed
		trace = append(trace, candidate)
		changed = true
	}

	return trace, candidate, changed
}

type sliceShrinker[T any] struct{}

func (s sliceShrinker[T]) Shrink(value []T, predicate Predicate[[]T]) ([]T, bool) {
	_, final, changed := s.ShrinkTrace(value, predicate)
	return final, changed
}

func (s sliceShrinker[T]) ShrinkTrace(value []T, predicate Predicate[[]T]) ([][]T, []T, bool) {
	if predicate(value) {
		return nil, value, false
	}
	if len(value) == 0 {
		return nil, value, false
	}

	trace := [][]T{value}

	if len(value) > 1 {
		half := len(value) / 2
		first := append([]T(nil), value[:half]...)
		if !predicate(first) {
			subTrace, final, changed := s.ShrinkTrace(first, predicate)
			fullTrace := append(trace, subTrace...)
			if len(subTrace) == 0 {
				fullTrace = append(trace, first)
			}
			return fullTrace, final, changed || true
		}
		second := append([]T(nil), value[half:]...)
		if !predicate(second) {
			subTrace, final, changed := s.ShrinkTrace(second, predicate)
			fullTrace := append(trace, subTrace...)
			if len(subTrace) == 0 {
				fullTrace = append(trace, second)
			}
			return fullTrace, final, changed || true
		}
	}

	for i := range value {
		shorter := make([]T, 0, len(value)-1)
		shorter = append(shorter, value[:i]...)
		shorter = append(shorter, value[i+1:]...)
		if !predicate(shorter) {
			subTrace, final, changed := s.ShrinkTrace(shorter, predicate)
			fullTrace := append(trace, subTrace...)
			if len(subTrace) == 0 {
				fullTrace = append(trace, shorter)
			}
			return fullTrace, final, changed || true
		}
	}
	return trace, value, false
}

func (s sliceShrinker[T]) ShrinkTraceParallel(value []T, predicate Predicate[[]T], _ int) ([][]T, []T, bool) {
	return s.ShrinkTrace(value, predicate)
}
