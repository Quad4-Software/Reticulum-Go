// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
// Package pbt provides a property-based testing toolkit for Go.
//
// It verifies program behavior against generated inputs instead of hand-written
// test cases. Key features include deterministic reproduction via seeds,
// counterexample shrinking, coverage labels and buckets, stateful model-based
// testing, and replay fixtures for failing cases.
//
// Use ForAll to define a property, Check to run it from tests, and CheckResult
// for programmatic execution. Configure runs, seed, and parallelism via
// WithRuns, WithSeed, WithParallelism, and related options.
package pbt
