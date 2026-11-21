// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"fmt"
	"os"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
	"golang.org/x/term"
)

func stdinFd() int {
	const maxInt = int64(^uint(0) >> 1)
	fd := os.Stdin.Fd()
	if fd > uintptr(maxInt) {
		return -1
	}
	return int(fd)
}

func stdinTTY() bool {
	fd := stdinFd()
	return fd >= 0 && term.IsTerminal(fd)
}

func (s *session) enterCallConsole(c *call.Call) {
	if c == nil || c.State() != call.StateActive {
		return
	}
	fmt.Printf("call active | mode %s | profile %s\n", proto.ModeName(c.Mode()), profileLabel(c.Profile()))
	if c.Mode() == proto.ModeHalfDuplex && stdinTTY() {
		s.runHalfDuplexKeys(c)
		return
	}
	s.runLineCallConsole(c)
}

func (s *session) runHalfDuplexKeys(c *call.Call) {
	fmt.Println("half duplex controls (no enter needed):")
	fmt.Println("  space or t  toggle PTT")
	fmt.Println("  h           hang up")
	fmt.Println("  f           switch to full duplex")
	fmt.Println("  m           toggle mute")
	fd := stdinFd()
	if fd < 0 {
		s.runLineCallConsole(c)
		return
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Println("raw terminal:", err)
		s.runLineCallConsole(c)
		return
	}
	defer func() { _ = term.Restore(fd, old) }()

	ptt := false
	buf := make([]byte, 1)
	for c.State() == call.StateActive {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			return
		}
		switch buf[0] {
		case ' ', 't':
			ptt = !ptt
			c.PTT(ptt)
			if ptt {
				fmt.Print("\rPTT on  ")
			} else {
				fmt.Print("\rPTT off ")
			}
		case 'h':
			fmt.Println("\rhangup")
			_ = c.Hangup("user hangup")
			s.clearActive(c)
			return
		case 'f':
			if err := c.SwitchMode(proto.ModeFullDuplex); err != nil {
				fmt.Printf("\rmode: %v  ", err)
			} else {
				fmt.Print("\rswitched to full duplex  ")
				_ = term.Restore(fd, old)
				s.runLineCallConsole(c)
				return
			}
		case 'm':
			_ = c.MuteTX(!c.MutedTX())
			if c.MutedTX() {
				fmt.Print("\rmute on  ")
			} else {
				fmt.Print("\rmute off ")
			}
		case 3, 4:
			_ = c.Hangup("interrupt")
			s.clearActive(c)
			s.quit = true
			return
		}
	}
}

func (s *session) runLineCallConsole(c *call.Call) {
	if c.Mode() == proto.ModeHalfDuplex {
		fmt.Println("half duplex: t or space = toggle PTT | mode full | h = hangup")
	} else {
		fmt.Println("full duplex: m = mute | mode half | h = hangup")
	}
	sc := s.scanner
	for c.State() == call.StateActive {
		fmt.Print("call> ")
		if !sc.Scan() {
			return
		}
		line := sc.Text()
		if !s.handleCallLine(c, line) {
			return
		}
	}
}

func (s *session) handleCallLine(c *call.Call, line string) bool {
	switch line {
	case "", "help":
		if c.Mode() == proto.ModeHalfDuplex {
			fmt.Println("t space  toggle PTT")
		}
		fmt.Println("mode half|full  switch duplex mode")
		fmt.Println("mute            toggle TX mute")
		fmt.Println("h hangup        end call")
	case "h", "hangup", "hang":
		_ = c.Hangup("user hangup")
		s.clearActive(c)
	case "t", " ":
		if c.Mode() != proto.ModeHalfDuplex {
			fmt.Println("PTT only applies in half duplex mode")
			return true
		}
		on := c.Squelched()
		c.PTT(on)
		if on {
			fmt.Println("PTT on")
		} else {
			fmt.Println("PTT off")
		}
	case "mute", "m":
		_ = c.MuteTX(!c.MutedTX())
		if c.MutedTX() {
			fmt.Println("muted")
		} else {
			fmt.Println("unmuted")
		}
	case "mode half", "mode hdx", "mode ptt":
		if err := c.SwitchMode(proto.ModeHalfDuplex); err != nil {
			fmt.Println("mode:", err)
		} else {
			fmt.Println("mode: half duplex (squelched)")
		}
	case "mode full", "mode fdx":
		if err := c.SwitchMode(proto.ModeFullDuplex); err != nil {
			fmt.Println("mode:", err)
		} else {
			fmt.Println("mode: full duplex")
		}
	case "q", "quit":
		_ = c.Hangup("quit")
		s.clearActive(c)
		s.quit = true
		return false
	default:
		fmt.Println("unknown call command, try help")
	}
	return true
}

func profileLabel(p int) string {
	for name, v := range map[string]int{
		"ulbw": proto.ProfileBandwidthUltraLow,
		"vlbw": proto.ProfileBandwidthVeryLow,
		"lbw":  proto.ProfileBandwidthLow,
		"mq":   proto.ProfileQualityMedium,
		"hq":   proto.ProfileQualityHigh,
		"shq":  proto.ProfileQualityMax,
		"ll":   proto.ProfileLatencyLow,
		"ull":  proto.ProfileLatencyUltraLow,
	} {
		if v == p {
			return name
		}
	}
	return fmt.Sprintf("0x%02x", p)
}

func (s *session) clearActive(c *call.Call) {
	s.mu.Lock()
	if s.active == c {
		s.active = nil
	}
	s.mu.Unlock()
}
