// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

// Predicate validates a generated value for a property.
type Predicate[T any] func(value T) bool

// PropertyOption configures optional property behavior.
type PropertyOption[T any] func(*Property[T])

// Property describes a property to evaluate with generated inputs.
type Property[T any] struct {
	Name       string          // Human-readable property name for failure reports.
	Generator  Generator[T]    // Produces random values for each run.
	Predicate  Predicate[T]    // Must return true for the property to hold.
	Shrinker   Shrinker[T]     // Optional; minimizes failing counterexamples.
	Classifier Classifier[T]   // Optional; tags failures for triage.
	Labeler    Labeler[T]      // Optional; labels each sample for coverage.
	Bucketer   Bucketer[T]     // Optional; assigns samples to coverage buckets.
	Coverage   CoverageConfig  // Optional; enforces label/bucket thresholds.
	Hooks      []Hook[T]       // Optional; lifecycle callbacks.
}

// ForAll constructs a property from a generator and predicate.
func ForAll[T any](name string, generator Generator[T], predicate Predicate[T], opts ...PropertyOption[T]) Property[T] {
	property := Property[T]{
		Name:      name,
		Generator: generator,
		Predicate: predicate,
	}
	for _, opt := range opts {
		opt(&property)
	}
	return property
}

// WithShrinker assigns a custom shrinker for minimizing counterexamples.
func WithShrinker[T any](shrinker Shrinker[T]) PropertyOption[T] {
	return func(p *Property[T]) {
		p.Shrinker = shrinker
	}
}

// Classifier tags failing inputs for triage reporting.
type Classifier[T any] func(value T) []string

// Labeler tags every generated input for coverage summaries.
type Labeler[T any] func(value T) []string

// Bucketer assigns each generated input to a coverage bucket.
type Bucketer[T any] func(value T) string

// CoverageRule defines minimum observed coverage for a label or bucket.
type CoverageRule struct {
	Key        string  // Label or bucket identifier.
	MinCount   int     // Minimum absolute count across all runs.
	MinPercent float64 // Minimum percentage of runs (0-100).
}

// CoverageConfig controls optional coverage threshold checks.
type CoverageConfig struct {
	LabelRules  []CoverageRule
	BucketRules []CoverageRule
}

// WithClassifier assigns a failure classifier hook.
func WithClassifier[T any](classifier Classifier[T]) PropertyOption[T] {
	return func(p *Property[T]) {
		p.Classifier = classifier
	}
}

// WithLabeler assigns a per-sample label hook.
func WithLabeler[T any](labeler Labeler[T]) PropertyOption[T] {
	return func(p *Property[T]) {
		p.Labeler = labeler
	}
}

// WithBucketer assigns a per-sample bucket hook.
func WithBucketer[T any](bucketer Bucketer[T]) PropertyOption[T] {
	return func(p *Property[T]) {
		p.Bucketer = bucketer
	}
}

// WithLabelCoverageRules enables minimum threshold checks for labels.
func WithLabelCoverageRules[T any](rules ...CoverageRule) PropertyOption[T] {
	return func(p *Property[T]) {
		p.Coverage.LabelRules = append([]CoverageRule(nil), rules...)
	}
}

// WithBucketCoverageRules enables minimum threshold checks for buckets.
func WithBucketCoverageRules[T any](rules ...CoverageRule) PropertyOption[T] {
	return func(p *Property[T]) {
		p.Coverage.BucketRules = append([]CoverageRule(nil), rules...)
	}
}

// WithHook registers a single lifecycle hook.
func WithHook[T any](hook Hook[T]) PropertyOption[T] {
	return func(p *Property[T]) {
		if hook == nil {
			return
		}
		p.Hooks = append(p.Hooks, hook)
	}
}

// WithHooks registers multiple lifecycle hooks.
func WithHooks[T any](hooks ...Hook[T]) PropertyOption[T] {
	return func(p *Property[T]) {
		for _, hook := range hooks {
			if hook == nil {
				continue
			}
			p.Hooks = append(p.Hooks, hook)
		}
	}
}
