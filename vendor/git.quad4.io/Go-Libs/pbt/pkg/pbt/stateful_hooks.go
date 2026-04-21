// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import (
	"sync"
	"time"
)

// StatefulHook receives lifecycle events for stateful model runs.
type StatefulHook[S any] interface {
	OnStatefulRunStart(event StatefulRunStartEvent)
	OnStatefulStep(event StatefulStepEvent[S])
	OnStatefulFailure(event StatefulFailureEvent[S])
	OnStatefulShrinkPass(event StatefulShrinkPassEvent[S])
	OnStatefulRunEnd(event StatefulRunEndEvent)
	OnStatefulReplay(event StatefulReplayEvent[S])
}

// StatefulHookFuncs adapts function callbacks into a StatefulHook.
type StatefulHookFuncs[S any] struct {
	RunStart   func(event StatefulRunStartEvent)
	Step       func(event StatefulStepEvent[S])
	Failure    func(event StatefulFailureEvent[S])
	ShrinkPass func(event StatefulShrinkPassEvent[S])
	RunEnd     func(event StatefulRunEndEvent)
	Replay     func(event StatefulReplayEvent[S])
}

// OnStatefulRunStart dispatches run-start events.
func (h StatefulHookFuncs[S]) OnStatefulRunStart(event StatefulRunStartEvent) {
	if h.RunStart != nil {
		h.RunStart(event)
	}
}

// OnStatefulStep dispatches step events.
func (h StatefulHookFuncs[S]) OnStatefulStep(event StatefulStepEvent[S]) {
	if h.Step != nil {
		h.Step(event)
	}
}

// OnStatefulFailure dispatches failure events.
func (h StatefulHookFuncs[S]) OnStatefulFailure(event StatefulFailureEvent[S]) {
	if h.Failure != nil {
		h.Failure(event)
	}
}

// OnStatefulShrinkPass dispatches shrink-pass events.
func (h StatefulHookFuncs[S]) OnStatefulShrinkPass(event StatefulShrinkPassEvent[S]) {
	if h.ShrinkPass != nil {
		h.ShrinkPass(event)
	}
}

// OnStatefulRunEnd dispatches run-end events.
func (h StatefulHookFuncs[S]) OnStatefulRunEnd(event StatefulRunEndEvent) {
	if h.RunEnd != nil {
		h.RunEnd(event)
	}
}

// OnStatefulReplay dispatches replay events.
func (h StatefulHookFuncs[S]) OnStatefulReplay(event StatefulReplayEvent[S]) {
	if h.Replay != nil {
		h.Replay(event)
	}
}

// StatefulRunStartEvent is emitted at stateful run start.
type StatefulRunStartEvent struct {
	ModelName         string
	Runs              int
	Seed              int64
	MaxSize           int
	ShrinkParallelism int
	StartedAt         time.Time
}

// StatefulStepEvent is emitted for each executed state transition.
type StatefulStepEvent[S any] struct {
	Scenario int
	Step     int
	Command  string
	State    S
}

// StatefulFailureEvent is emitted when model invariant fails.
type StatefulFailureEvent[S any] struct {
	Scenario      int
	Step          int
	FailedCommand string
	Trace         []string
	FinalState    S
}

// StatefulShrinkPassEvent is emitted when shrink removes sequence parts.
type StatefulShrinkPassEvent[S any] struct {
	Pass            int
	ChunkSize       int
	RemovedStart    int
	RemovedEnd      int
	RemainingLength int
	Trace           []string
	FinalState      S
}

// StatefulRunEndEvent is emitted at stateful run end.
type StatefulRunEndEvent struct {
	Passed       bool
	ShrinkPasses int
	Elapsed      time.Duration
}

// StatefulReplayEvent is emitted after replaying a stateful fixture.
type StatefulReplayEvent[S any] struct {
	ModelName    string
	Seed         int64
	ScenarioSeed int64
	Passed       bool
	Trace        []string
	FinalState   S
}

type statefulHookDispatcher[S any] struct {
	hooks []StatefulHook[S]
	mu    sync.Mutex
}

func newStatefulHookDispatcher[S any](hooks []StatefulHook[S]) statefulHookDispatcher[S] {
	out := statefulHookDispatcher[S]{}
	if len(hooks) == 0 {
		return out
	}
	out.hooks = append([]StatefulHook[S](nil), hooks...)
	return out
}

func (d *statefulHookDispatcher[S]) empty() bool {
	return len(d.hooks) == 0
}

func (d *statefulHookDispatcher[S]) runStart(event StatefulRunStartEvent) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnStatefulRunStart(event)
	}
}

func (d *statefulHookDispatcher[S]) step(event StatefulStepEvent[S]) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnStatefulStep(event)
	}
}

func (d *statefulHookDispatcher[S]) failure(event StatefulFailureEvent[S]) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnStatefulFailure(event)
	}
}

func (d *statefulHookDispatcher[S]) shrinkPass(event StatefulShrinkPassEvent[S]) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnStatefulShrinkPass(event)
	}
}

func (d *statefulHookDispatcher[S]) runEnd(event StatefulRunEndEvent) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnStatefulRunEnd(event)
	}
}

func (d *statefulHookDispatcher[S]) replay(event StatefulReplayEvent[S]) {
	if d.empty() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hook := range d.hooks {
		hook.OnStatefulReplay(event)
	}
}
