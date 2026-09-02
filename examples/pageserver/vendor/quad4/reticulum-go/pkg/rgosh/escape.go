// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

// EscapeAction is a client-side tilde escape result.
type EscapeAction int

const (
	// EscapeNone means bytes should be forwarded.
	EscapeNone EscapeAction = iota
	// EscapeQuit disconnects (~.).
	EscapeQuit
	// EscapeToggleLine toggles line-buffered stdin (~L).
	EscapeToggleLine
	// EscapeHelp prints the escape quick reference (~?).
	EscapeHelp
)

// EscapeHelpText is printed for ~?.
const EscapeHelpText = "\r\nSupported rgosh escape sequences:\r\n ~~ Send the escape character by typing it twice\r\n ~. Terminate session and exit immediately\r\n ~L Toggle line-interactive mode\r\n ~? Display this quick reference\r\n\r\n(Escape sequences are only recognized immediately after newline)\r\n"

// EscapeFilter recognizes SSH-style tilde escapes after newline, matching rnsh.
type EscapeFilter struct {
	preEsc bool
	esc    bool
}

// NewEscapeFilter starts ready for a leading ~. like Python rnsh.
func NewEscapeFilter() *EscapeFilter {
	return &EscapeFilter{preEsc: true}
}

// Feed consumes stdin bytes and returns bytes to forward plus an action.
func (e *EscapeFilter) Feed(in []byte) (out []byte, action EscapeAction) {
	for _, c := range in {
		if e.esc {
			e.esc = false
			switch c {
			case '.':
				return out, EscapeQuit
			case 'L':
				action = EscapeToggleLine
				continue
			case '?':
				action = EscapeHelp
				continue
			case '~':
				out = append(out, '~')
				e.preEsc = false
				continue
			default:
				out = append(out, '~', c)
				e.preEsc = c == '\r' || c == '\n'
				continue
			}
		}
		if e.preEsc && c == '~' {
			e.preEsc = false
			e.esc = true
			continue
		}
		e.preEsc = c == '\r' || c == '\n'
		out = append(out, c)
	}
	return out, action
}
