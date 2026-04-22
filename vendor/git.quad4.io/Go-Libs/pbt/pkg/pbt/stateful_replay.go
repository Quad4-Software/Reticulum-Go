// SPDX-License-Identifier: 0BSD
// Copyright (c) 2026 Quad4
package pbt

import (
	"encoding/json"
	"os"
)

const defaultReplayFileMode = 0o600

// StatefulReplayFixture stores deterministic replay data for stateful failures.
type StatefulReplayFixture struct {
	ModelName    string   `json:"model_name"`
	Seed         int64    `json:"seed"`
	ScenarioSeed int64    `json:"scenario_seed"`
	StepSeeds    []int64  `json:"step_seeds"`
	Trace        []string `json:"trace,omitempty"`
}

// ToStatefulReplayFixture converts a failing stateful result to replay data.
func (r StatefulResult[S]) ToStatefulReplayFixture() (StatefulReplayFixture, error) {
	if r.Passed || len(r.StepSeeds) == 0 {
		return StatefulReplayFixture{}, os.ErrInvalid
	}
	return StatefulReplayFixture{
		ModelName:    r.ModelName,
		Seed:         r.Seed,
		ScenarioSeed: r.ScenarioSeed,
		StepSeeds:    append([]int64(nil), r.StepSeeds...),
		Trace:        append([]string(nil), r.Trace...),
	}, nil
}

// WriteStatefulReplayFixture writes stateful replay data to disk.
func WriteStatefulReplayFixture(path string, fixture StatefulReplayFixture) error {
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, defaultReplayFileMode)
}

// ReadStatefulReplayFixture reads stateful replay data from disk.
func ReadStatefulReplayFixture(path string) (StatefulReplayFixture, error) {
	var fixture StatefulReplayFixture
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

// ReplayStatefulFixture executes the exact recorded failing sequence.
func ReplayStatefulFixture[S any](model CommandModel[S], fixture StatefulReplayFixture) StatefulResult[S] {
	run := runStatefulWithSeeds(model, fixture.ScenarioSeed, fixture.StepSeeds)
	result := StatefulResult[S]{
		Passed:        run.Passed,
		ModelName:     model.Name,
		Seed:          fixture.Seed,
		ScenarioSeed:  fixture.ScenarioSeed,
		Runs:          1,
		ScenarioIndex: 0,
		StepIndex:     run.StepIndex,
		FailedCommand: run.FailedCommand,
		Trace:         append([]string(nil), run.Trace...),
		StepSeeds:     append([]int64(nil), run.StepSeeds...),
		FinalState:    run.FinalState,
	}

	dispatcher := newStatefulHookDispatcher(model.Hooks)
	dispatcher.replay(StatefulReplayEvent[S]{
		ModelName:    model.Name,
		Seed:         fixture.Seed,
		ScenarioSeed: fixture.ScenarioSeed,
		Passed:       result.Passed,
		Trace:        append([]string(nil), result.Trace...),
		FinalState:   result.FinalState,
	})

	return result
}

// ReplayStatefulFixtureFile loads and replays a stateful fixture.
func ReplayStatefulFixtureFile[S any](model CommandModel[S], path string) (StatefulResult[S], error) {
	fixture, err := ReadStatefulReplayFixture(path)
	if err != nil {
		return StatefulResult[S]{}, err
	}
	return ReplayStatefulFixture(model, fixture), nil
}
