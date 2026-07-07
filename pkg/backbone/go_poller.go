// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package backbone

type goPoller struct {
	wake chan pollEvent
}

func newGoPoller() poller {
	return &goPoller{wake: make(chan pollEvent, 1)}
}

func (p *goPoller) Add(int, int) error { return nil }
func (p *goPoller) Mod(int, int) error { return nil }
func (p *goPoller) Del(int) error      { return nil }
func (p *goPoller) Wait(int) ([]pollEvent, error) {
	select {
	case ev := <-p.wake:
		return []pollEvent{ev}, nil
	default:
		return nil, nil
	}
}
func (p *goPoller) Close() error { return nil }

func (p *goPoller) signal(fd int, events int) {
	select {
	case p.wake <- pollEvent{fd: fd, events: events}:
	default:
	}
}
