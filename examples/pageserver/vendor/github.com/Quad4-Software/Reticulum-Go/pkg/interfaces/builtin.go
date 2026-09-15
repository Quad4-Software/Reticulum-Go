// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import "sync"

var (
	builtinFromConfigMu sync.RWMutex
	builtinFromConfig   = map[string]ExternalFactory{}
)

func registerBuiltinFromConfig(typeName string, factory ExternalFactory) {
	if typeName == "" || factory == nil {
		return
	}
	builtinFromConfigMu.Lock()
	builtinFromConfig[typeName] = factory
	builtinFromConfigMu.Unlock()
}

func lookupBuiltinFromConfig(typeName string) ExternalFactory {
	builtinFromConfigMu.RLock()
	factory := builtinFromConfig[typeName]
	builtinFromConfigMu.RUnlock()
	return factory
}
