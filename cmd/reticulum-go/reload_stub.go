// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build windows || tinygo

package main

import "os"

func startSIGHUPReload(r *Reticulum, opts daemonOptions) {
	_ = r
	_ = opts
}

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
