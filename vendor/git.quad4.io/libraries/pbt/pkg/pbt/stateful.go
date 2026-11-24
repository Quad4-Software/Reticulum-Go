// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// StatefulCommand is a single state transition in a model-based test.
type StatefulCommand[S any] interface {
	Name() string
	Precondition(state S) bool
	Next(r *rand.Rand, state S) S
}

// CommandModel defines initial state, commands, and invariants.
type CommandModel[S any] struct {
	Name      string
	Init      func(r *rand.Rand) S
	Commands  []StatefulCommand[S]
	Invariant func(state S) bool
	Hooks     []StatefulHook[S]
}

// StatefulResult is the outcome of a model-based run.
type StatefulResult[S any] struct {
	Passed        bool
	TimedOut      bool
	ModelName     string
	Seed          int64
	ScenarioSeed  int64
	Runs          int
	ScenarioIndex int
	StepIndex     int
	FailedCommand string
	Trace         []string
	OriginalTrace []string
	StepSeeds     []int64
	ShrinkPasses  int
	FinalState    S
}

// Error returns a readable message for failing model checks.
func (r StatefulResult[S]) Error() string {
	if r.Passed {
		return ""
	}
	if len(r.StepSeeds) == 0 {
		return fmt.Sprintf(
			"stateful model %q timed out after %d runs budget (seed=%d)",
			r.ModelName,
			r.Runs,
			r.Seed,
		)
	}
	msg := fmt.Sprintf(
		"stateful model %q failed at scenario=%d step=%d command=%q seed=%d trace=%v",
		r.ModelName,
		r.ScenarioIndex,
		r.StepIndex,
		r.FailedCommand,
		r.Seed,
		r.Trace,
	)
	if r.TimedOut {
		msg += " (timeout during shrinking)"
	}
	return msg
}

// CheckStateful executes a model-based property and fails the test on failure.
func CheckStateful[S any](t TestingT, model CommandModel[S], opts ...Option) {
	t.Helper()

	result := CheckStatefulResult(model, opts...)
	if !result.Passed {
		t.Fatalf("%s", result.Error())
	}
}

// CheckStatefulResult executes command sequences and verifies invariants.
// Scenarios run sequentially; WithParallelism does not apply to stateful
// checks, while WithShrinkParallelism controls counterexample shrinking.
func CheckStatefulResult[S any](model CommandModel[S], opts ...Option) StatefulResult[S] {
	cfg := applyOptions(opts)
	if cfg.Runs <= 0 {
		panic("pbt: stateful runs must be positive")
	}
	if cfg.MaxSize <= 0 {
		panic("pbt: stateful max size must be positive")
	}
	if cfg.Timeout < 0 {
		panic("pbt: stateful timeout must be non-negative")
	}
	if cfg.Parallelism <= 0 {
		panic("pbt: stateful parallelism must be positive")
	}
	if cfg.ShrinkParallelism <= 0 {
		panic("pbt: stateful shrink parallelism must be positive")
	}
	if model.Init == nil {
		panic("pbt: stateful model init cannot be nil")
	}
	if model.Invariant == nil {
		panic("pbt: stateful model invariant cannot be nil")
	}
	if len(model.Commands) == 0 {
		panic("pbt: stateful model requires at least one command")
	}
	for i, cmd := range model.Commands {
		if cmd == nil {
			panic(fmt.Sprintf("pbt: stateful command at index %d cannot be nil", i))
		}
	}

	ctx := context.Background()
	cancel := func() {}
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	}
	defer cancel()

	// #nosec G404 -- deterministic PRNG is required for reproducible model runs.
	rng := rand.New(rand.NewSource(cfg.Seed))
	dispatcher := newStatefulHookDispatcher(model.Hooks)
	startedAt := time.Now()
	dispatcher.runStart(StatefulRunStartEvent{
		ModelName:         model.Name,
		Runs:              cfg.Runs,
		Seed:              cfg.Seed,
		MaxSize:           cfg.MaxSize,
		ShrinkParallelism: cfg.ShrinkParallelism,
		StartedAt:         startedAt,
	})

	out := StatefulResult[S]{
		Passed:    true,
		ModelName: model.Name,
		Seed:      cfg.Seed,
		Runs:      cfg.Runs,
	}

	for scenario := 0; scenario < cfg.Runs; scenario++ {
		if ctx.Err() != nil {
			out.Passed = false
			out.TimedOut = true
			out.ScenarioIndex = scenario
			dispatcher.runEnd(StatefulRunEndEvent{
				Passed:   false,
				TimedOut: true,
				Elapsed:  time.Since(startedAt),
			})
			return out
		}

		scenarioSeed := rng.Int63()
		steps := sizeForRun(scenario, cfg.Runs, cfg.MaxSize)
		run := runStatefulScenario(ctx, model, scenarioSeed, steps, scenario, dispatcher)
		if run.TimedOut {
			out.Passed = false
			out.TimedOut = true
			out.ScenarioIndex = scenario
			out.ScenarioSeed = scenarioSeed
			dispatcher.runEnd(StatefulRunEndEvent{
				Passed:   false,
				TimedOut: true,
				Elapsed:  time.Since(startedAt),
			})
			return out
		}
		if run.Passed {
			continue
		}

		out.Passed = false
		out.ScenarioSeed = scenarioSeed
		out.ScenarioIndex = scenario
		out.StepIndex = run.StepIndex
		out.FailedCommand = run.FailedCommand
		out.Trace = append([]string(nil), run.Trace...)
		out.OriginalTrace = append([]string(nil), run.Trace...)
		out.StepSeeds = append([]int64(nil), run.StepSeeds...)
		out.FinalState = run.FinalState
		dispatcher.failure(StatefulFailureEvent[S]{
			Scenario:      scenario,
			Step:          run.StepIndex,
			FailedCommand: run.FailedCommand,
			Trace:         append([]string(nil), run.Trace...),
			FinalState:    run.FinalState,
		})

		shrinkModel := model
		shrinkModel.Invariant = timeoutAwareInvariant(ctx, model.Invariant)
		shrunk := shrinkStatefulFailure(ctx, shrinkModel, scenarioSeed, run.StepSeeds, cfg.ShrinkParallelism, dispatcher)
		out.TimedOut = ctx.Err() != nil
		if len(shrunk.StepSeeds) > 0 {
			out.Trace = append([]string(nil), shrunk.Trace...)
			out.StepSeeds = append([]int64(nil), shrunk.StepSeeds...)
			out.StepIndex = shrunk.StepIndex
			out.FailedCommand = shrunk.FailedCommand
			out.FinalState = shrunk.FinalState
			out.ShrinkPasses = shrunk.ShrinkPasses
		}
		dispatcher.runEnd(StatefulRunEndEvent{
			Passed:       false,
			TimedOut:     out.TimedOut,
			ShrinkPasses: out.ShrinkPasses,
			Elapsed:      time.Since(startedAt),
		})
		return out
	}

	dispatcher.runEnd(StatefulRunEndEvent{
		Passed:       true,
		ShrinkPasses: 0,
		Elapsed:      time.Since(startedAt),
	})
	return out
}

// timeoutAwareInvariant reports every state as valid once ctx is done so that
// shrink candidate evaluation stops promptly instead of running past the
// timeout. The original failing trace is already recorded, so no
// counterexample is lost.
func timeoutAwareInvariant[S any](ctx context.Context, invariant func(state S) bool) func(state S) bool {
	return func(state S) bool {
		select {
		case <-ctx.Done():
			return true
		default:
		}
		return invariant(state)
	}
}

// CommandSequence generates bounded random command sequences. Commands are
// drawn uniformly without evaluating preconditions, so models must tolerate
// inapplicable commands in a generated sequence.
func CommandSequence[S any](name string, minLen int, maxLen int, commands ...StatefulCommand[S]) Generator[[]StatefulCommand[S]] {
	if minLen < 0 {
		minLen = 0
	}
	if minLen > maxLen {
		minLen, maxLen = maxLen, minLen
	}
	if minLen < 0 {
		minLen = 0
	}
	return NewGenerator(name, func(r *rand.Rand, size int) []StatefulCommand[S] {
		if len(commands) == 0 {
			return nil
		}
		localMax := maxLen
		if size > 0 && size < localMax {
			localMax = size
		}
		if localMax < minLen {
			localMax = minLen
		}
		n := minLen
		if localMax > minLen {
			n = minLen + r.Intn(localMax-minLen+1)
		}
		out := make([]StatefulCommand[S], 0, n)
		for i := 0; i < n; i++ {
			out = append(out, commands[r.Intn(len(commands))])
		}
		return out
	})
}

func enabledCommands[S any](commands []StatefulCommand[S], state S) []StatefulCommand[S] {
	out := make([]StatefulCommand[S], 0, len(commands))
	for _, c := range commands {
		if c.Precondition(state) {
			out = append(out, c)
		}
	}
	return out
}

type statefulRun[S any] struct {
	Passed        bool
	TimedOut      bool
	StepIndex     int
	FailedCommand string
	Trace         []string
	StepSeeds     []int64
	ShrinkPasses  int
	FinalState    S
}

func runStatefulScenario[S any](ctx context.Context, model CommandModel[S], scenarioSeed int64, steps int, scenarioIndex int, dispatcher *statefulHookDispatcher[S]) statefulRun[S] {
	// #nosec G404 -- deterministic PRNG is required for scenario replayability.
	scenarioRng := rand.New(rand.NewSource(scenarioSeed))
	state := model.Init(scenarioRng)
	trace := make([]string, 0, steps)
	stepSeeds := make([]int64, 0, steps)
	out := statefulRun[S]{Passed: true}

	for step := range steps {
		select {
		case <-ctx.Done():
			out.TimedOut = true
			out.Trace = trace
			out.StepSeeds = stepSeeds
			out.FinalState = state
			return out
		default:
		}

		enabled := enabledCommands(model.Commands, state)
		if len(enabled) == 0 {
			out.Trace = trace
			out.StepSeeds = stepSeeds
			out.FinalState = state
			return out
		}

		stepSeed := scenarioRng.Int63()
		// #nosec G404 -- deterministic per-step PRNG enables exact sequence replay.
		stepRng := rand.New(rand.NewSource(stepSeed))
		cmd := enabled[stepRng.Intn(len(enabled))]
		state = cmd.Next(stepRng, state)

		trace = append(trace, cmd.Name())
		stepSeeds = append(stepSeeds, stepSeed)
		if dispatcher != nil {
			dispatcher.step(StatefulStepEvent[S]{
				Scenario: scenarioIndex,
				Step:     step,
				Command:  cmd.Name(),
				State:    state,
			})
		}

		if !model.Invariant(state) {
			out.Passed = false
			out.StepIndex = step
			out.FailedCommand = cmd.Name()
			out.Trace = trace
			out.StepSeeds = stepSeeds
			out.FinalState = state
			return out
		}
	}

	out.Trace = trace
	out.StepSeeds = stepSeeds
	out.FinalState = state
	return out
}

func runStatefulWithSeeds[S any](ctx context.Context, model CommandModel[S], scenarioSeed int64, stepSeeds []int64) statefulRun[S] {
	// #nosec G404 -- deterministic PRNG is required for seeded trace replay.
	scenarioRng := rand.New(rand.NewSource(scenarioSeed))
	state := model.Init(scenarioRng)
	trace := make([]string, 0, len(stepSeeds))
	out := statefulRun[S]{Passed: true}

	for idx, stepSeed := range stepSeeds {
		select {
		case <-ctx.Done():
			out.TimedOut = true
			out.Trace = trace
			out.StepSeeds = append([]int64(nil), stepSeeds[:idx]...)
			out.FinalState = state
			return out
		default:
		}

		enabled := enabledCommands(model.Commands, state)
		if len(enabled) == 0 {
			out.Trace = trace
			out.StepSeeds = append([]int64(nil), stepSeeds[:idx]...)
			out.FinalState = state
			return out
		}

		// #nosec G404 -- deterministic per-step PRNG enables exact sequence replay.
		stepRng := rand.New(rand.NewSource(stepSeed))
		cmd := enabled[stepRng.Intn(len(enabled))]
		state = cmd.Next(stepRng, state)
		trace = append(trace, cmd.Name())

		if !model.Invariant(state) {
			out.Passed = false
			out.StepIndex = idx
			out.FailedCommand = cmd.Name()
			out.Trace = trace
			out.StepSeeds = append([]int64(nil), stepSeeds[:idx+1]...)
			out.FinalState = state
			return out
		}
	}

	out.Trace = trace
	out.StepSeeds = append([]int64(nil), stepSeeds...)
	out.FinalState = state
	return out
}

func shrinkStatefulFailure[S any](ctx context.Context, model CommandModel[S], scenarioSeed int64, failingSeeds []int64, workers int, dispatcher *statefulHookDispatcher[S]) statefulRun[S] {
	current := append([]int64(nil), failingSeeds...)
	best := runStatefulWithSeeds(ctx, model, scenarioSeed, current)
	if best.Passed {
		return statefulRun[S]{Passed: true}
	}

	if workers < 1 {
		workers = 1
	}
	shrinkPasses := 0

	for chunk := len(current) / 2; chunk >= 1; chunk /= 2 {
		progress := true
		for progress {
			progress = false
			if ctx.Err() != nil {
				best.ShrinkPasses = shrinkPasses
				return best
			}
			if workers == 1 {
				for i := 0; i+chunk <= len(current); i++ {
					candidate := removeSeedRange(current, i, i+chunk)
					run := runStatefulWithSeeds(ctx, model, scenarioSeed, candidate)
					if run.Passed {
						continue
					}
					current = candidate
					best = run
					shrinkPasses++
					if dispatcher != nil {
						dispatcher.shrinkPass(StatefulShrinkPassEvent[S]{
							Pass:            shrinkPasses,
							ChunkSize:       chunk,
							RemovedStart:    i,
							RemovedEnd:      i + chunk,
							RemainingLength: len(current),
							Trace:           append([]string(nil), best.Trace...),
							FinalState:      best.FinalState,
						})
					}
					progress = true
					break
				}
				continue
			}

			indexes := make([]int, 0)
			for i := 0; i+chunk <= len(current); i++ {
				indexes = append(indexes, i)
			}
			hit, idx, run := parallelFindFirstFailing(ctx, model, scenarioSeed, current, chunk, indexes, workers)
			if hit {
				current = removeSeedRange(current, idx, idx+chunk)
				best = run
				shrinkPasses++
				if dispatcher != nil {
					dispatcher.shrinkPass(StatefulShrinkPassEvent[S]{
						Pass:            shrinkPasses,
						ChunkSize:       chunk,
						RemovedStart:    idx,
						RemovedEnd:      idx + chunk,
						RemainingLength: len(current),
						Trace:           append([]string(nil), best.Trace...),
						FinalState:      best.FinalState,
					})
				}
				progress = true
			}
		}
	}

	best.ShrinkPasses = shrinkPasses
	return best
}

func parallelFindFirstFailing[S any](
	ctx context.Context,
	model CommandModel[S],
	scenarioSeed int64,
	current []int64,
	chunk int,
	indexes []int,
	workers int,
) (bool, int, statefulRun[S]) {
	type candidate struct {
		index int
		run   statefulRun[S]
	}
	type jobPanic struct {
		index int
		value any
	}

	jobs := make(chan int, len(indexes))
	results := make(chan candidate, len(indexes))
	panics := make(chan jobPanic, workers)
	var wg sync.WaitGroup

	if workers > len(indexes) {
		workers = len(indexes)
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			panicIndex := -1
			defer wg.Done()
			// A panic in a worker goroutine would otherwise crash the whole
			// process. Capture it and let the caller re-raise it below.
			defer func() {
				if r := recover(); r != nil {
					panics <- jobPanic{index: panicIndex, value: r}
				}
			}()
			for idx := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				panicIndex = idx
				candidateSeeds := removeSeedRange(current, idx, idx+chunk)
				run := runStatefulWithSeeds(ctx, model, scenarioSeed, candidateSeeds)
				if run.Passed {
					continue
				}
				results <- candidate{index: idx, run: run}
			}
		}()
	}

	for _, idx := range indexes {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()
	close(results)
	close(panics)

	// Re-raise the panic from the lowest index so the failure is deterministic
	// across repeated runs.
	var recovered *jobPanic
	for p := range panics {
		if recovered == nil || p.index < recovered.index {
			captured := p
			recovered = &captured
		}
	}
	if recovered != nil {
		panic(recovered.value)
	}

	found := false
	bestIdx := 0
	var bestRun statefulRun[S]
	for result := range results {
		if !found || result.index < bestIdx {
			found = true
			bestIdx = result.index
			bestRun = result.run
		}
	}
	return found, bestIdx, bestRun
}

func removeSeedRange(in []int64, start int, end int) []int64 {
	out := make([]int64, 0, len(in)-(end-start))
	out = append(out, in[:start]...)
	out = append(out, in[end:]...)
	return out
}
