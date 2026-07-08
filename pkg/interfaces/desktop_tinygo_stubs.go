// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build tinygo

package interfaces

// BackboneClientInterface and LocalClientInterface are desktop-only types
// referenced from shared node wiring; TinyGo builds use these minimal stubs.
type BackboneClientInterface struct {
	BaseInterface
}

type LocalClientInterface struct {
	BaseInterface
}

type LocalServerInterface struct {
	BaseInterface
}

type LocalSpawnHook func(client *LocalClientInterface)

func (lc *LocalClientInterface) IsSharedInstanceClient() bool {
	return false
}

func (ls *LocalServerInterface) Stop() error {
	return nil
}
