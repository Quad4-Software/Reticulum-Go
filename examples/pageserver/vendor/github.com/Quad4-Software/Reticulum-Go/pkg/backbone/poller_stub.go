// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd

package backbone

import (
	"errors"
	"fmt"
)

func newEpollPoller() (poller, error) {
	return nil, errors.New("epoll not supported on this platform")
}

func newKqueuePoller() (poller, error) {
	return nil, errors.New("kqueue not supported on this platform")
}

func newUringPoller() (poller, error) {
	return nil, fmt.Errorf("io_uring not supported on this platform")
}
