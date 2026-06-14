// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import (
	"time"
)

const (
	defaultRuns              = 100
	defaultMaxSize           = 100
	defaultSeed              = 0
	defaultTimeout           = 0
	defaultParallelism       = 1
	defaultShrinkParallelism = 1
)

// Config defines how property checks are executed.
type Config struct {
	Runs              int           // Number of generated test cases.
	MaxSize           int           // Maximum size parameter passed to generators.
	Seed              int64         // Random seed; set for reproducibility.
	Timeout           time.Duration // Abort after this duration; 0 means no limit.
	Parallelism       int           // Number of deterministic worker partitions.
	ShrinkParallelism int           // Workers used during counterexample shrinking.
}

// Option mutates a Config used by a check.
type Option func(*Config)

// DefaultConfig returns a baseline configuration. Seed defaults to 0 for reproducibility.
func DefaultConfig() Config {
	return Config{
		Runs:              defaultRuns,
		MaxSize:           defaultMaxSize,
		Seed:              defaultSeed,
		Timeout:           defaultTimeout,
		Parallelism:       defaultParallelism,
		ShrinkParallelism: defaultShrinkParallelism,
	}
}

// WithRuns sets the number of generated test cases.
func WithRuns(runs int) Option {
	return func(c *Config) {
		if runs <= 0 {
			panic("pbt: WithRuns requires positive value")
		}
		c.Runs = runs
	}
}

// WithMaxSize sets the maximum size parameter passed to generators.
func WithMaxSize(maxSize int) Option {
	return func(c *Config) {
		if maxSize <= 0 {
			panic("pbt: WithMaxSize requires positive value")
		}
		c.MaxSize = maxSize
	}
}

// WithSeed sets a deterministic random seed for reproducibility.
func WithSeed(seed int64) Option {
	return func(c *Config) {
		c.Seed = seed
	}
}

// WithTimeout sets an optional timeout for the entire check run.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		if timeout < 0 {
			panic("pbt: WithTimeout requires non-negative duration")
		}
		c.Timeout = timeout
	}
}

// WithParallelism sets the number of deterministic worker partitions.
func WithParallelism(parallelism int) Option {
	return func(c *Config) {
		if parallelism <= 0 {
			panic("pbt: WithParallelism requires positive value")
		}
		c.Parallelism = parallelism
	}
}

// WithShrinkParallelism sets workers used during shrink minimization.
func WithShrinkParallelism(parallelism int) Option {
	return func(c *Config) {
		if parallelism <= 0 {
			panic("pbt: WithShrinkParallelism requires positive value")
		}
		c.ShrinkParallelism = parallelism
	}
}

func applyOptions(opts []Option) Config {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}
