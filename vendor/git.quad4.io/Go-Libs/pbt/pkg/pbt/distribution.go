// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import (
	"fmt"
	"math/rand"
	"sort"
)

// DistributionReport describes observed sample distribution for a generator.
type DistributionReport struct {
	Total   int
	Buckets map[string]int
}

// DistributionRule defines expected percentage bounds for a bucket.
type DistributionRule struct {
	Bucket     string
	MinPercent float64
	MaxPercent float64
}

// AnalyzeDistribution samples generated values into caller-defined buckets.
func AnalyzeDistribution[T any](
	generator Generator[T],
	bucketer func(T) string,
	runs int,
	seed int64,
	maxSize int,
) DistributionReport {
	if runs <= 0 {
		panic("pbt: AnalyzeDistribution runs must be positive")
	}
	if maxSize <= 0 {
		maxSize = 100
	}
	if bucketer == nil {
		panic("pbt: AnalyzeDistribution bucketer cannot be nil")
	}

	// #nosec G404 -- deterministic PRNG is required for repeatable distribution checks.
	rng := rand.New(rand.NewSource(seed))
	out := DistributionReport{
		Total:   runs,
		Buckets: map[string]int{},
	}

	for i := 0; i < runs; i++ {
		size := sizeForRun(i, runs, maxSize)
		value := generator.Generate(rng, size)
		b := bucketer(value)
		if b == "" {
			b = "unlabeled"
		}
		out.Buckets[b]++
	}

	return out
}

// ValidateDistribution checks whether all rules are satisfied.
func ValidateDistribution(report DistributionReport, rules []DistributionRule) error {
	if report.Total <= 0 {
		return fmt.Errorf("distribution report total must be positive")
	}

	for _, rule := range rules {
		if rule.Bucket == "" {
			return fmt.Errorf("distribution rule bucket cannot be empty")
		}
		if rule.MinPercent < 0 || rule.MaxPercent < 0 || rule.MinPercent > 100 || rule.MaxPercent > 100 {
			return fmt.Errorf("distribution rule percentages must be within [0,100]")
		}
		if rule.MinPercent > rule.MaxPercent {
			return fmt.Errorf("distribution rule min percent cannot exceed max percent")
		}

		count := report.Buckets[rule.Bucket]
		pct := (float64(count) / float64(report.Total)) * 100
		if pct < rule.MinPercent || pct > rule.MaxPercent {
			return fmt.Errorf(
				"bucket %q out of range: got %.2f%% expected [%.2f%%, %.2f%%]",
				rule.Bucket, pct, rule.MinPercent, rule.MaxPercent,
			)
		}
	}

	return nil
}

// SortedBuckets returns deterministic bucket names for reporting.
func (r DistributionReport) SortedBuckets() []string {
	keys := make([]string, 0, len(r.Buckets))
	for k := range r.Buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
