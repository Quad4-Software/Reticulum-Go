// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package backbone

import "errors"

func newKqueuePoller() (poller, error) {
	return nil, errors.New("kqueue is not available on Linux")
}
