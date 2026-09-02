// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build rns_slim && !js

package interfaces

import (
	"fmt"

	"quad4/reticulum-go/pkg/common"
)

func init() {
	for _, typeName := range []string{
		"QUICClientInterface",
		"QUICServerInterface",
		"I2PInterface",
		"SDRInterface",
		"WebTransportClientInterface",
		"WebTransportServerInterface",
	} {
		registerOmittedFromConfig(typeName)
	}
}

func registerOmittedFromConfig(typeName string) {
	registerBuiltinFromConfig(typeName, func(name string, cfg *common.InterfaceConfig, _ *FromConfigContext) (Interface, error) {
		return nil, fmt.Errorf("interface type %q omitted from this build (built with -tags rns_slim)", typeName)
	})
}
