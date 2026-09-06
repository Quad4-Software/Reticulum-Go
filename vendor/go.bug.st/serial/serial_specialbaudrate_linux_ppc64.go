//
// Copyright 2014-2026 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

//go:build linux && ppc64

package serial

func (port *unixPort) setSpecialBaudrate(speed uint32) error {
	return &PortError{code: InvalidSpeed}
}
