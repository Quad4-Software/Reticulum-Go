// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build darwin || freebsd || netbsd || openbsd

package backbone

import "errors"

func newEpollPoller() (poller, error) {
	return nil, errors.New("epoll is not available on this platform")
}

func newUringPoller() (poller, error) {
	return nil, errors.New("io_uring is not available on this platform")
}
