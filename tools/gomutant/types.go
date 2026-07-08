// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

type mutantKind string

const (
	kindComparison mutantKind = "comparison"
	kindLogical    mutantKind = "logical"
	kindArithmetic mutantKind = "arithmetic"
)

type mutant struct {
	ID     int
	Pkg    string
	File   string
	Line   int
	Column int
	Kind   mutantKind
	From   string
	To     string
	Offset int
	Length int
	Source []byte
}

type mutantResult string

const (
	resultKilled    mutantResult = "KILLED"
	resultSurvived  mutantResult = "SURVIVED"
	resultSkipped   mutantResult = "SKIPPED"
	resultBuildFail mutantResult = "BUILD_FAIL"
	resultTimeout   mutantResult = "TIMEOUT"
)

type outcome struct {
	Mutant mutant
	Result mutantResult
	Err    string
}
