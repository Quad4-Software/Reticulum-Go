// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import (
	"encoding/json"
	"fmt"
	"os"
)

// ReplayFixture stores deterministic replay data for a failing property.
type ReplayFixture struct {
	PropertyName   string          `json:"property_name"`
	Seed           int64           `json:"seed"`
	GeneratorName  string          `json:"generator_name"`
	Counterexample json.RawMessage `json:"counterexample"`
	FailureLabels  []string        `json:"failure_labels,omitempty"`
}

// SerializeCounterexample converts a counterexample into stable JSON.
func SerializeCounterexample[T any](value T) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DeserializeCounterexample decodes a previously serialized counterexample.
func DeserializeCounterexample[T any](payload string) (T, error) {
	var out T
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return out, err
	}
	return out, nil
}

// ToReplayFixture converts a failing result into a replay fixture.
func (r Result[T]) ToReplayFixture() (ReplayFixture, error) {
	if !r.HasCounterexample {
		return ReplayFixture{}, fmt.Errorf("result does not contain a counterexample")
	}

	serialized, err := SerializeCounterexample(r.Counterexample)
	if err != nil {
		return ReplayFixture{}, err
	}

	return ReplayFixture{
		PropertyName:   r.PropertyName,
		Seed:           r.Seed,
		GeneratorName:  r.GeneratorName,
		Counterexample: json.RawMessage(serialized),
		FailureLabels:  append([]string(nil), r.FailureLabels...),
	}, nil
}

// WriteReplayFixture writes a fixture JSON file to disk.
func WriteReplayFixture(path string, fixture ReplayFixture) error {
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, defaultReplayFileMode)
}

// ReadReplayFixture reads a fixture JSON file from disk.
func ReadReplayFixture(path string) (ReplayFixture, error) {
	var fixture ReplayFixture
	// #nosec G304 -- caller controls replay fixture path by design.
	data, err := os.ReadFile(path)
	if err != nil {
		return fixture, err
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		return fixture, err
	}
	return fixture, nil
}

// ReplayResult describes the outcome of replaying a stored fixture.
type ReplayResult[T any] struct {
	Passed        bool
	PropertyName  string
	Seed          int64
	GeneratorName string
	Value         T
	FailureLabels []string
	StillFailing  bool
	Reason        string
}

// ReplayFixtureValue replays a fixture value against a property predicate.
func ReplayFixtureValue[T any](property Property[T], fixture ReplayFixture) (ReplayResult[T], error) {
	if property.Predicate == nil {
		return ReplayResult[T]{}, fmt.Errorf("property predicate cannot be nil")
	}
	if len(fixture.Counterexample) == 0 {
		return ReplayResult[T]{}, fmt.Errorf("fixture counterexample cannot be empty")
	}

	var value T
	if err := json.Unmarshal(fixture.Counterexample, &value); err != nil {
		return ReplayResult[T]{}, err
	}

	stillFailing := !property.Predicate(value)
	reason := "predicate now passes for replay value"
	if stillFailing {
		reason = "predicate still fails for replay value"
	}

	result := ReplayResult[T]{
		Passed:        true,
		PropertyName:  fixture.PropertyName,
		Seed:          fixture.Seed,
		GeneratorName: fixture.GeneratorName,
		Value:         value,
		FailureLabels: append([]string(nil), fixture.FailureLabels...),
		StillFailing:  stillFailing,
		Reason:        reason,
	}

	dispatcher := newHookDispatcher(property.Hooks)
	dispatcher.replay(ReplayEvent[T]{
		PropertyName: result.PropertyName,
		Seed:         result.Seed,
		Value:        result.Value,
		StillFailing: result.StillFailing,
		Reason:       result.Reason,
	})

	return result, nil
}

// ReplayFixtureFile loads and replays a fixture against a property.
func ReplayFixtureFile[T any](property Property[T], path string) (ReplayResult[T], error) {
	fixture, err := ReadReplayFixture(path)
	if err != nil {
		return ReplayResult[T]{}, err
	}
	return ReplayFixtureValue(property, fixture)
}
