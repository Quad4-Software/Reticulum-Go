// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// TestingT is the subset of testing.TB required by Check.
type TestingT interface {
	Helper()
	Fatalf(format string, args ...any)
}

// Check executes a property and fails the test immediately on failure.
func Check[T any](t TestingT, property Property[T], opts ...Option) {
	t.Helper()

	result := CheckResult(property, opts...)
	if !result.Passed {
		t.Fatalf("%s", result.Error())
	}
}

// CheckResult executes a property and returns a rich result object.
func CheckResult[T any](property Property[T], opts ...Option) Result[T] {
	cfg := applyOptions(opts)
	validate(property, cfg)
	dispatcher := newHookDispatcher(property.Hooks)
	startedAt := time.Now()

	dispatcher.runStart(RunStartEvent{
		PropertyName:      property.Name,
		Runs:              cfg.Runs,
		Seed:              cfg.Seed,
		GeneratorName:     property.Generator.Name(),
		Parallelism:       cfg.Parallelism,
		ShrinkParallelism: cfg.ShrinkParallelism,
		StartedAt:         startedAt,
	})

	ctx := context.Background()
	cancel := func() {}
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), cfg.Timeout)
	}
	defer cancel()

	out := run(ctx, property, cfg, &dispatcher)

	dispatcher.runEnd(RunEndEvent{
		Passed:         out.Passed,
		TimedOut:       out.TimedOut,
		CoverageFailed: out.CoverageFailed,
		Elapsed:        time.Since(startedAt),
	})
	return out
}

func run[T any](ctx context.Context, property Property[T], cfg Config, dispatcher *hookDispatcher[T]) Result[T] {
	if cfg.Parallelism <= 1 {
		return runSequential(ctx, property, cfg, dispatcher)
	}
	return runParallel(ctx, property, cfg, dispatcher)
}

func runSequential[T any](ctx context.Context, property Property[T], cfg Config, dispatcher *hookDispatcher[T]) Result[T] {
	// #nosec G404 -- deterministic PRNG is required for reproducible property runs.
	rng := rand.New(rand.NewSource(cfg.Seed))
	out := Result[T]{
		Passed:        true,
		PropertyName:  property.Name,
		Runs:          cfg.Runs,
		Seed:          cfg.Seed,
		GeneratorName: property.Generator.Name(),
		LabelCounts:   map[string]int{},
		BucketCounts:  map[string]int{},
	}

	for i := 0; i < cfg.Runs; i++ {
		select {
		case <-ctx.Done():
			out.Passed = false
			out.TimedOut = true
			dispatcher.failure(FailureEvent[T]{Index: -1})
			return out
		default:
		}

		size := sizeForRun(i, cfg.Runs, cfg.MaxSize)
		value := property.Generator.Generate(rng, size)
		recordCoverage(&out, property, value)
		passed := property.Predicate(value)
		dispatcher.caseGenerated(CaseGeneratedEvent[T]{
			Index:  i,
			Size:   size,
			Value:  value,
			Passed: passed,
		})
		if passed {
			continue
		}

		out.Passed = false
		out.Counterexample = value
		out.HasCounterexample = true
		out.FailureLabels = classifyFailure(property, value)
		dispatcher.failure(FailureEvent[T]{
			Index:         i,
			Value:         value,
			FailureLabels: append([]string(nil), out.FailureLabels...),
		})

		if property.Shrinker != nil {
			final, trace := shrinkWithTrace(property.Shrinker, value, property.Predicate, cfg.ShrinkParallelism)
			out.Counterexample = final
			out.ShrinkTrace = trace
			emitShrinkTrace(dispatcher, trace)
		}
		return out
	}

	coverageErrors := evaluateCoverage(property.Coverage, out.Runs, out.LabelCounts, out.BucketCounts)
	if len(coverageErrors) > 0 {
		out.Passed = false
		out.CoverageFailed = true
		out.CoverageErrors = coverageErrors
		dispatcher.failure(FailureEvent[T]{
			Index:          -1,
			CoverageFailed: true,
			CoverageErrors: append([]string(nil), coverageErrors...),
		})
	}

	return out
}

func runParallel[T any](ctx context.Context, property Property[T], cfg Config, dispatcher *hookDispatcher[T]) Result[T] {
	workers := cfg.Parallelism
	if workers > cfg.Runs {
		workers = cfg.Runs
	}

	type candidate struct {
		Found bool
		Index int
		Value T
	}

	type workerResult struct {
		Candidate    candidate
		LabelCounts  map[string]int
		BucketCounts map[string]int
	}

	out := Result[T]{
		Passed:        true,
		PropertyName:  property.Name,
		Runs:          cfg.Runs,
		Seed:          cfg.Seed,
		GeneratorName: property.Generator.Name(),
		LabelCounts:   map[string]int{},
		BucketCounts:  map[string]int{},
	}

	baseRuns := cfg.Runs / workers
	extra := cfg.Runs % workers

	results := make(chan workerResult, workers)
	var wg sync.WaitGroup

	start := 0
	for w := 0; w < workers; w++ {
		count := baseRuns
		if w < extra {
			count++
		}
		workerStart := start
		start += count

		wg.Add(1)
		go func(workerIndex int, runStart int, runCount int) {
			defer wg.Done()
			// #nosec G404 -- deterministic PRNG is required for reproducible worker partitions.
			rng := rand.New(rand.NewSource(partitionSeed(cfg.Seed, workerIndex)))
			local := workerResult{
				LabelCounts:  map[string]int{},
				BucketCounts: map[string]int{},
			}

			for i := 0; i < runCount; i++ {
				select {
				case <-ctx.Done():
					results <- local
					return
				default:
				}

				globalIndex := runStart + i
				size := sizeForRun(globalIndex, cfg.Runs, cfg.MaxSize)
				value := property.Generator.Generate(rng, size)
				recordCoverageLocal(local.LabelCounts, local.BucketCounts, property, value)
				passed := property.Predicate(value)
				dispatcher.caseGenerated(CaseGeneratedEvent[T]{
					Index:  globalIndex,
					Size:   size,
					Value:  value,
					Passed: passed,
				})

				if local.Candidate.Found {
					continue
				}
				if passed {
					continue
				}

				local.Candidate = candidate{
					Found: true,
					Index: globalIndex,
					Value: value,
				}
			}

			results <- local
		}(w, workerStart, count)
	}

	wg.Wait()
	close(results)

	select {
	case <-ctx.Done():
		out.Passed = false
		out.TimedOut = true
		dispatcher.failure(FailureEvent[T]{Index: -1})
		return out
	default:
	}

	best := candidate{}
	for r := range results {
		mergeCounts(out.LabelCounts, r.LabelCounts)
		mergeCounts(out.BucketCounts, r.BucketCounts)

		if !r.Candidate.Found {
			continue
		}
		if !best.Found || r.Candidate.Index < best.Index {
			best = r.Candidate
		}
	}

	if !best.Found {
		coverageErrors := evaluateCoverage(property.Coverage, out.Runs, out.LabelCounts, out.BucketCounts)
		if len(coverageErrors) > 0 {
			out.Passed = false
			out.CoverageFailed = true
			out.CoverageErrors = coverageErrors
			dispatcher.failure(FailureEvent[T]{
				Index:          -1,
				CoverageFailed: true,
				CoverageErrors: append([]string(nil), coverageErrors...),
			})
		}
		return out
	}

	out.Passed = false
	out.Counterexample = best.Value
	out.HasCounterexample = true
	out.FailureLabels = classifyFailure(property, best.Value)
	dispatcher.failure(FailureEvent[T]{
		Index:         best.Index,
		Value:         best.Value,
		FailureLabels: append([]string(nil), out.FailureLabels...),
	})

	if property.Shrinker != nil {
		final, trace := shrinkWithTrace(property.Shrinker, best.Value, property.Predicate, cfg.ShrinkParallelism)
		out.Counterexample = final
		out.ShrinkTrace = trace
		emitShrinkTrace(dispatcher, trace)
	}

	return out
}

func emitShrinkTrace[T any](dispatcher *hookDispatcher[T], trace []T) {
	if len(trace) < 2 {
		return
	}
	for i := 0; i < len(trace)-1; i++ {
		dispatcher.shrinkStep(ShrinkStepEvent[T]{
			Step: i + 1,
			From: trace[i],
			To:   trace[i+1],
		})
	}
}

func evaluateCoverage(cfg CoverageConfig, runs int, labelCounts map[string]int, bucketCounts map[string]int) []string {
	if runs <= 0 {
		return nil
	}
	errors := make([]string, 0)

	for _, rule := range cfg.LabelRules {
		if rule.Key == "" {
			continue
		}
		count := labelCounts[rule.Key]
		if !meetsCoverageRule(rule, count, runs) {
			errors = append(errors, fmt.Sprintf("label %q below threshold: count=%d", rule.Key, count))
		}
	}

	for _, rule := range cfg.BucketRules {
		if rule.Key == "" {
			continue
		}
		count := bucketCounts[rule.Key]
		if !meetsCoverageRule(rule, count, runs) {
			errors = append(errors, fmt.Sprintf("bucket %q below threshold: count=%d", rule.Key, count))
		}
	}

	sort.Strings(errors)
	return errors
}

func meetsCoverageRule(rule CoverageRule, count int, runs int) bool {
	if rule.MinCount > 0 && count < rule.MinCount {
		return false
	}
	if rule.MinPercent > 0 {
		pct := (float64(count) / float64(runs)) * 100
		if pct < rule.MinPercent {
			return false
		}
	}
	return true
}

func mergeCounts(dst map[string]int, src map[string]int) {
	for key, value := range src {
		dst[key] += value
	}
}

func classifyFailure[T any](property Property[T], value T) []string {
	if property.Classifier == nil {
		return nil
	}
	return property.Classifier(value)
}

func recordCoverage[T any](out *Result[T], property Property[T], value T) {
	recordCoverageLocal(out.LabelCounts, out.BucketCounts, property, value)
}

func recordCoverageLocal[T any](labelCounts map[string]int, bucketCounts map[string]int, property Property[T], value T) {
	if property.Labeler != nil {
		for _, label := range property.Labeler(value) {
			if label == "" {
				continue
			}
			labelCounts[label]++
		}
	}
	if property.Bucketer != nil {
		bucket := property.Bucketer(value)
		if bucket != "" {
			bucketCounts[bucket]++
		}
	}
}

func partitionSeed(seed int64, worker int) int64 {
	// #nosec G115 -- intentional wraparound mixing for deterministic partition seeds.
	x := uint64(seed) + uint64(worker+1)*0x9e3779b97f4a7c15
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	// #nosec G115 -- bit-pattern cast back to signed seed space is intentional.
	return int64(x)
}

func sizeForRun(index int, runs int, maxSize int) int {
	if runs <= 1 || maxSize <= 1 {
		if maxSize < 1 {
			return 1
		}
		return maxSize
	}
	size := 1 + (index*(maxSize-1))/(runs-1)
	if size < 1 {
		return 1
	}
	return size
}

func validate[T any](property Property[T], cfg Config) {
	if property.Name == "" {
		panic("pbt: property name cannot be empty")
	}
	if property.Generator == nil {
		panic("pbt: generator cannot be nil")
	}
	if property.Predicate == nil {
		panic("pbt: predicate cannot be nil")
	}
	if cfg.Runs <= 0 {
		panic(fmt.Sprintf("pbt: runs must be positive, got %d", cfg.Runs))
	}
	if cfg.MaxSize <= 0 {
		panic(fmt.Sprintf("pbt: max size must be positive, got %d", cfg.MaxSize))
	}
	if cfg.Timeout < 0 {
		panic(fmt.Sprintf("pbt: timeout must be non-negative, got %s", cfg.Timeout))
	}
	if cfg.Parallelism <= 0 {
		panic(fmt.Sprintf("pbt: parallelism must be positive, got %d", cfg.Parallelism))
	}
	if cfg.ShrinkParallelism <= 0 {
		panic(fmt.Sprintf("pbt: shrink parallelism must be positive, got %d", cfg.ShrinkParallelism))
	}
	validateCoverageRules(property.Coverage)
}

func validateCoverageRules(cfg CoverageConfig) {
	validateSet := func(kind string, rules []CoverageRule) {
		for _, rule := range rules {
			if rule.Key == "" {
				panic(fmt.Sprintf("pbt: %s coverage key cannot be empty", kind))
			}
			if rule.MinCount < 0 {
				panic(fmt.Sprintf("pbt: %s coverage min count cannot be negative", kind))
			}
			if rule.MinPercent < 0 || rule.MinPercent > 100 {
				panic(fmt.Sprintf("pbt: %s coverage min percent must be within [0,100]", kind))
			}
		}
	}
	validateSet("label", cfg.LabelRules)
	validateSet("bucket", cfg.BucketRules)
}
