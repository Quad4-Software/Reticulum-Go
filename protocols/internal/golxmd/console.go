// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/term"
)

const (
	liveStatusInterval   = 5 * time.Second
	staticStatusInterval = 30 * time.Second
)

var opConsole = newOperatorConsole()

type operatorConsole struct {
	out  *os.File
	live bool
}

func newOperatorConsole() *operatorConsole {
	out := term.FileOf(os.Stdout)
	if out == nil {
		out = os.Stdout
	}
	return &operatorConsole{
		out:  out,
		live: term.ColorEnabled(out),
	}
}

func (c *operatorConsole) writer() io.Writer {
	return c.out
}

func (c *operatorConsole) flush() {
	_ = c.out.Sync()
}

func (c *operatorConsole) isLive() bool {
	return c.live
}

func statusInterval(live bool) time.Duration {
	if v := os.Getenv("GOLXMD_STATUS_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	if live {
		return liveStatusInterval
	}
	return staticStatusInterval
}

func (c *operatorConsole) println(args ...any) {
	fmt.Fprintln(c.out, args...)
	c.flush()
}

func (c *operatorConsole) printf(format string, args ...any) {
	fmt.Fprintf(c.out, format, args...)
	c.flush()
}

func (c *operatorConsole) updateStatus(line string) {
	if c.live {
		fmt.Fprint(c.out, term.ProgressClear(c.out)+line)
		c.flush()
		return
	}
	fmt.Fprintln(c.out, line)
	c.flush()
}
