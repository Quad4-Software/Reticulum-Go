// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

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

// Int64Shrinker returns a shrinker that moves failing int64 values toward zero.
func Int64Shrinker() Shrinker[int64] {
	return int64Shrinker{}
}

// Uint64Shrinker returns a shrinker that moves failing uint64 values toward zero.
func Uint64Shrinker() Shrinker[uint64] {
	return uint64Shrinker{}
}

// IntShrinkerToward returns a shrinker that moves failing integers toward the
// given target instead of zero. Useful when the interesting boundary is a
// non-zero constant.
func IntShrinkerToward(target int) Shrinker[int] {
	return intTargetShrinker[int]{target: target}
}

// Int64ShrinkerToward is IntShrinkerToward for int64 values.
func Int64ShrinkerToward(target int64) Shrinker[int64] {
	return intTargetShrinker[int64]{target: target}
}

// SliceShrinkerOf returns a shrinker that first removes elements like
// SliceShrinker, then shrinks surviving elements in place using elem. Pure
// removal can miss minimal counterexamples when a property fails on element
// values rather than length.
func SliceShrinkerOf[T any](elem Shrinker[T]) Shrinker[[]T] {
	return sliceElemShrinker[T]{elem: elem}
}

// BytesShrinker returns a shrinker that shortens failing byte slices.
func BytesShrinker() Shrinker[[]byte] {
	return sliceShrinker[byte]{}
}

// Tuple2Shrinker minimizes each component of a generated pair, shrinking the
// first component to a local minimum while the second stays fixed, then the
// second. Intended for use with ForAll2 via WithShrinker.
func Tuple2Shrinker[A any, B any](first Shrinker[A], second Shrinker[B]) Shrinker[Tuple2Value[A, B]] {
	return tuple2Shrinker[A, B]{first: first, second: second}
}

// Tuple3Shrinker minimizes each component of a generated triple in order.
// Intended for use with ForAll3 via WithShrinker.
func Tuple3Shrinker[A any, B any, C any](first Shrinker[A], second Shrinker[B], third Shrinker[C]) Shrinker[Tuple3Value[A, B, C]] {
	return tuple3Shrinker[A, B, C]{first: first, second: second, third: third}
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

type int64Shrinker struct{}

func (s int64Shrinker) Shrink(value int64, predicate Predicate[int64]) (int64, bool) {
	if predicate(value) {
		return value, false
	}
	candidate := value
	changed := false
	for candidate != 0 {
		next := candidate / 2
		if next == candidate || predicate(next) {
			break
		}
		candidate = next
		changed = true
	}
	return candidate, changed
}

type uint64Shrinker struct{}

func (s uint64Shrinker) Shrink(value uint64, predicate Predicate[uint64]) (uint64, bool) {
	if predicate(value) {
		return value, false
	}
	candidate := value
	changed := false
	for candidate != 0 {
		next := candidate / 2
		if next == candidate || predicate(next) {
			break
		}
		candidate = next
		changed = true
	}
	return candidate, changed
}

type signedInteger interface {
	~int | ~int64
}

type intTargetShrinker[T signedInteger] struct {
	target T
}

func (s intTargetShrinker[T]) Shrink(value T, predicate Predicate[T]) (T, bool) {
	if predicate(value) {
		return value, false
	}
	candidate := value
	changed := false
	for candidate != s.target {
		next := s.stepToward(candidate)
		if predicate(next) {
			break
		}
		candidate = next
		changed = true
	}
	return candidate, changed
}

// stepToward halves the distance to the target. When candidate and target
// have opposite signs the subtraction could overflow, so the candidate first
// converges on zero until the signs align. A remaining distance of one jumps
// straight to the target.
func (s intTargetShrinker[T]) stepToward(candidate T) T {
	distance := candidate - s.target
	if (candidate > 0) != (s.target > 0) {
		return candidate / 2
	}
	next := s.target + distance/2
	if next == candidate {
		return s.target
	}
	return next
}

type sliceElemShrinker[T any] struct {
	elem Shrinker[T]
}

func (s sliceElemShrinker[T]) Shrink(value []T, predicate Predicate[[]T]) ([]T, bool) {
	if predicate(value) || len(value) == 0 {
		return value, false
	}

	candidate, changed := sliceShrinker[T]{}.Shrink(value, predicate)

	for i := range candidate {
		if s.elem == nil {
			break
		}
		index := i
		current := candidate[index]
		shrunk, elemChanged := s.elem.Shrink(current, func(elem T) bool {
			trial := make([]T, len(candidate))
			copy(trial, candidate)
			trial[index] = elem
			return predicate(trial)
		})
		if elemChanged {
			candidate[index] = shrunk
			changed = true
		}
	}

	return candidate, changed
}

func (s sliceElemShrinker[T]) ShrinkTrace(value []T, predicate Predicate[[]T]) ([][]T, []T, bool) {
	if predicate(value) || len(value) == 0 {
		return nil, value, false
	}

	trace := [][]T{value}
	candidate := append([]T(nil), value...)

	structural, structChanged := sliceShrinker[T]{}.Shrink(candidate, predicate)
	if structChanged {
		candidate = append([]T(nil), structural...)
		trace = append(trace, candidate)
	}

	for i := range candidate {
		index := i
		shrunk, elemChanged := s.elem.Shrink(candidate[index], func(elem T) bool {
			trial := make([]T, len(candidate))
			copy(trial, candidate)
			trial[index] = elem
			return predicate(trial)
		})
		if elemChanged {
			candidate[index] = shrunk
			trace = append(trace, append([]T(nil), candidate...))
		}
	}

	return trace, candidate, structChanged || len(trace) > 1
}

type tuple2Shrinker[A any, B any] struct {
	first  Shrinker[A]
	second Shrinker[B]
}

func (s tuple2Shrinker[A, B]) Shrink(value Tuple2Value[A, B], predicate Predicate[Tuple2Value[A, B]]) (Tuple2Value[A, B], bool) {
	if predicate(value) {
		return value, false
	}
	candidate := value
	changed := false

	if s.first != nil {
		second := candidate.Second
		if shrunk, ok := s.first.Shrink(candidate.First, func(a A) bool {
			return predicate(Tuple2Value[A, B]{First: a, Second: second})
		}); ok {
			candidate.First = shrunk
			changed = true
		}
	}
	if s.second != nil {
		first := candidate.First
		if shrunk, ok := s.second.Shrink(candidate.Second, func(b B) bool {
			return predicate(Tuple2Value[A, B]{First: first, Second: b})
		}); ok {
			candidate.Second = shrunk
			changed = true
		}
	}

	return candidate, changed
}

type tuple3Shrinker[A any, B any, C any] struct {
	first  Shrinker[A]
	second Shrinker[B]
	third  Shrinker[C]
}

func (s tuple3Shrinker[A, B, C]) Shrink(value Tuple3Value[A, B, C], predicate Predicate[Tuple3Value[A, B, C]]) (Tuple3Value[A, B, C], bool) {
	if predicate(value) {
		return value, false
	}
	candidate := value
	changed := false

	if s.first != nil {
		b, c := candidate.Second, candidate.Third
		if shrunk, ok := s.first.Shrink(candidate.First, func(a A) bool {
			return predicate(Tuple3Value[A, B, C]{First: a, Second: b, Third: c})
		}); ok {
			candidate.First = shrunk
			changed = true
		}
	}
	if s.second != nil {
		a, c := candidate.First, candidate.Third
		if shrunk, ok := s.second.Shrink(candidate.Second, func(b B) bool {
			return predicate(Tuple3Value[A, B, C]{First: a, Second: b, Third: c})
		}); ok {
			candidate.Second = shrunk
			changed = true
		}
	}
	if s.third != nil {
		a, b := candidate.First, candidate.Second
		if shrunk, ok := s.third.Shrink(candidate.Third, func(c C) bool {
			return predicate(Tuple3Value[A, B, C]{First: a, Second: b, Third: c})
		}); ok {
			candidate.Third = shrunk
			changed = true
		}
	}

	return candidate, changed
}

// MapShrinker returns a shrinker that removes entries from failing maps. Keys
// are visited in a sorted order so shrinking stays deterministic; the sort
// uses each key's fmt rendering, which is stable for comparable keys.
func MapShrinker[K comparable, V any]() Shrinker[map[K]V] {
	return mapShrinker[K, V]{}
}

// DurationShrinkerToward shrinks time.Duration values toward a target.
func DurationShrinkerToward(target time.Duration) Shrinker[time.Duration] {
	inner := intTargetShrinker[int64]{target: int64(target)}
	return ShrinkerFunc[time.Duration](func(value time.Duration, predicate Predicate[time.Duration]) (time.Duration, bool) {
		shrunk, changed := inner.Shrink(int64(value), func(d int64) bool {
			return predicate(time.Duration(d))
		})
		return time.Duration(shrunk), changed
	})
}

// PtrShrinker shrinks pointer values: nil is tried first, then the pointed-to
// value is shrunk with the element shrinker when one is available.
func PtrShrinker[T any](elem Shrinker[T]) Shrinker[*T] {
	return ptrShrinker[T]{elem: elem}
}

// filterShrinker wraps a shrinker so candidates that fail the filter are
// treated as passing, which keeps filtered generators (SuchThat) inside their
// domain while shrinking.
func filterShrinker[T any](inner Shrinker[T], filter Predicate[T]) Shrinker[T] {
	if inner == nil {
		return nil
	}
	return ShrinkerFunc[T](func(value T, predicate Predicate[T]) (T, bool) {
		return inner.Shrink(value, func(candidate T) bool {
			return !filter(candidate) || predicate(candidate)
		})
	})
}

type mapShrinker[K comparable, V any] struct{}

func (s mapShrinker[K, V]) Shrink(value map[K]V, predicate Predicate[map[K]V]) (map[K]V, bool) {
	if predicate(value) || len(value) == 0 {
		return value, false
	}

	keys := make([]K, 0, len(value))
	for k := range value {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprintf("%v", keys[i]) < fmt.Sprintf("%v", keys[j])
	})

	for _, k := range keys {
		trial := make(map[K]V, len(value)-1)
		for key, v := range value {
			if key != k {
				trial[key] = v
			}
		}
		if !predicate(trial) {
			return s.Shrink(trial, predicate)
		}
	}
	return value, false
}

type ptrShrinker[T any] struct {
	elem Shrinker[T]
}

func (s ptrShrinker[T]) Shrink(value *T, predicate Predicate[*T]) (*T, bool) {
	if predicate(value) || value == nil {
		return value, false
	}
	if !predicate(nil) {
		return nil, true
	}
	if s.elem == nil {
		return value, false
	}
	shrunk, changed := s.elem.Shrink(*value, func(elem T) bool {
		return predicate(&elem)
	})
	if changed {
		return &shrunk, true
	}
	return value, false
}
