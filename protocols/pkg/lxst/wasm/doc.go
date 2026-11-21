//go:build js && wasm

// SPDX-License-Identifier: LicenseRef-Reticulum

// Package wasm compiles RGESP against reticulum-go WASM and stub codecs.
// CGO Opus and miniaudio are not available. Audio uses NullDevice unless a
// host page wires Web Audio through syscall/js.
package wasm
