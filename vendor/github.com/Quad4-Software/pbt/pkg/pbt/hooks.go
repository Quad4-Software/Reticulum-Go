// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import (
	"sync"
	"time"
)

// Hook receives lifecycle events for a property execution.
type Hook[T any] interface {
	OnRunStart(event RunStartEvent)
	OnCaseGenerated(event CaseGeneratedEvent[T])
	OnFailure(event FailureEvent[T])
	OnShrinkStep(event ShrinkStepEvent[T])
	OnRunEnd(event RunEndEvent)
	OnReplay(event ReplayEvent[T])
}

// HookFuncs adapts plain functions into a Hook.
type HookFuncs[T any] struct {
	RunStart      func(event RunStartEvent)
	CaseGenerated func(event CaseGeneratedEvent[T])
	Failure       func(event FailureEvent[T])
	ShrinkStep    func(event ShrinkStepEvent[T])
	RunEnd        func(event RunEndEvent)
	Replay        func(event ReplayEvent[T])
}

// OnRunStart dispatches a run-start event.
func (h HookFuncs[T]) OnRunStart(event RunStartEvent) {
	if h.RunStart != nil {
		h.RunStart(event)
	}
}

// OnCaseGenerated dispatches a generated-case event.
func (h HookFuncs[T]) OnCaseGenerated(event CaseGeneratedEvent[T]) {
	if h.CaseGenerated != nil {
		h.CaseGenerated(event)
	}
}

// OnFailure dispatches a failure event.
func (h HookFuncs[T]) OnFailure(event FailureEvent[T]) {
	if h.Failure != nil {
		h.Failure(event)
	}
}

// OnShrinkStep dispatches a shrink-step event.
func (h HookFuncs[T]) OnShrinkStep(event ShrinkStepEvent[T]) {
	if h.ShrinkStep != nil {
		h.ShrinkStep(event)
	}
}

// OnRunEnd dispatches a run-end event.
func (h HookFuncs[T]) OnRunEnd(event RunEndEvent) {
	if h.RunEnd != nil {
		h.RunEnd(event)
	}
}

// OnReplay dispatches a replay event.
func (h HookFuncs[T]) OnReplay(event ReplayEvent[T]) {
	if h.Replay != nil {
		h.Replay(event)
	}
}

// RunStartEvent is emitted once when a property run starts.
type RunStartEvent struct {
	PropertyName      string
	Runs              int
	Seed              int64
	GeneratorName     string
	Parallelism       int
	ShrinkParallelism int
	StartedAt         time.Time
}

// CaseGeneratedEvent is emitted for each generated case.
type CaseGeneratedEvent[T any] struct {
	Index  int
	Size   int
	Value  T
	Passed bool
}

// FailureEvent is emitted when the run fails.
type FailureEvent[T any] struct {
	Index          int
	Value          T
	FailureLabels  []string
	CoverageFailed bool
	CoverageErrors []string
}

// ShrinkStepEvent is emitted for each shrink transition.
type ShrinkStepEvent[T any] struct {
	Step int
	From T
	To   T
}

// RunEndEvent is emitted once when a property run completes.
type RunEndEvent struct {
	Passed         bool
	TimedOut       bool
	CoverageFailed bool
	Elapsed        time.Duration
}

// ReplayEvent is emitted after replay evaluation.
type ReplayEvent[T any] struct {
	PropertyName string
	Seed         int64
	Value        T
	StillFailing bool
	Reason       string
}

type hookDispatcher[T any] struct {
	hooks []Hook[T]
	mu    sync.Mutex
}

func newHookDispatcher[T any](hooks []Hook[T]) *hookDispatcher[T] {
	out := &hookDispatcher[T]{}
	if len(hooks) == 0 {
		return out
	}
	out.hooks = append([]Hook[T](nil), hooks...)
	return out
}

func (d *hookDispatcher[T]) empty() bool {
	return len(d.hooks) == 0
}

func (d *hookDispatcher[T]) runStart(event RunStartEvent) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnRunStart(event)
	}
}

func (d *hookDispatcher[T]) caseGenerated(event CaseGeneratedEvent[T]) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnCaseGenerated(event)
	}
}

func (d *hookDispatcher[T]) failure(event FailureEvent[T]) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnFailure(event)
	}
}

func (d *hookDispatcher[T]) shrinkStep(event ShrinkStepEvent[T]) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnShrinkStep(event)
	}
}

func (d *hookDispatcher[T]) runEnd(event RunEndEvent) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnRunEnd(event)
	}
}

func (d *hookDispatcher[T]) replay(event ReplayEvent[T]) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnReplay(event)
	}
}
