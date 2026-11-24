//go:build js && wasm

// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"fmt"
	"syscall/js"

	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/call"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
)

func main() {
	js.Global().Set("rgespReady", js.ValueOf(true))
	js.Global().Set("rgespApp", js.ValueOf(proto.AppName+"."+proto.AspectName))
	js.Global().Set("rgespDefaultProfile", js.ValueOf(call.DefaultConfig().Profile))
	fmt.Println("rgesp-wasm ready")
	select {}
}
