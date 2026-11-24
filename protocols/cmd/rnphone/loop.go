// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/call"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/history"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/phonebook"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/rnsnode"
)

const (
	historyLimit  = 20
	answerTimeout = 10 * time.Second
	recallTimeout = 10 * time.Second
)

type session struct {
	phone *call.Phone
	book  *phonebook.Book
	log   *history.Log
	id    *identity.Identity
	dest  *destination.Destination
	trans pathTransport
	wait  time.Duration

	mu       sync.Mutex
	active   *call.Call
	started  time.Time
	lastDest string
	callMode int
	scanner  *bufio.Scanner
	quit     bool
}

type pathTransport interface {
	HasPath([]byte) bool
	RequestPath([]byte, string, []byte, bool) error
}

func (s *session) events() call.Events {
	return call.Events{
		OnRinging: func(c *call.Call) {
			fmt.Println("ringing")
			if c.Incoming() {
				fmt.Println("fingerprint", call.Fingerprint(c.RemoteIdentity()))
				fmt.Println("enter to answer, r to reject")
			}
		},
		OnAnswered: func(c *call.Call) {
			fmt.Println("answered")
			s.mu.Lock()
			s.active = c
			s.mu.Unlock()
		},
		OnBusy: func(*call.Call) {
			fmt.Println("busy")
		},
		OnRejected: func(*call.Call) {
			fmt.Println("rejected")
		},
		OnEnded: s.onEnded,
	}
}

func (s *session) onEnded(c *call.Call, reason string) {
	fmt.Printf("ended: %s\n", reason)
	peer := []byte(nil)
	if id := c.RemoteIdentity(); id != nil {
		peer = id.Hash()
	}
	s.mu.Lock()
	recStarted := s.started
	if s.active == c {
		s.active = nil
	}
	s.mu.Unlock()
	_ = s.log.Record(peer, c.Incoming(), recStarted, reason)
}

func (s *session) run() {
	s.scanner = bufio.NewScanner(os.Stdin)
	printHelp()
	for !s.quit {
		fmt.Print("> ")
		if !s.scanner.Scan() {
			return
		}
		if !s.handleLine(strings.TrimSpace(s.scanner.Text())) {
			return
		}
	}
}

func (s *session) handleLine(line string) bool {
	if s.handleRingOrHangup(line) {
		return true
	}
	return s.dispatch(line)
}

func (s *session) handleRingOrHangup(line string) bool {
	cur := s.phone.Switchboard().Active()
	if cur != nil && cur.State() == call.StateRinging && cur.Incoming() {
		if line == "" {
			s.mu.Lock()
			s.started = time.Now()
			s.active = cur
			s.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), answerTimeout)
			_ = cur.Answer(ctx)
			cancel()
			s.enterCallConsole(cur)
			return true
		}
		if line == "r" {
			_ = cur.Reject("user reject")
			return true
		}
	}
	s.mu.Lock()
	curActive := s.active
	s.mu.Unlock()
	if curActive == nil || (line != "" && line != "h") {
		return false
	}
	_ = curActive.Hangup("user hangup")
	s.mu.Lock()
	if s.active == curActive {
		s.active = nil
	}
	s.mu.Unlock()
	return true
}

func (s *session) dispatch(line string) bool {
	switch line {
	case "", "help":
		printHelp()
	case "q", "quit":
		return false
	case "mode half", "mode hdx", "mode ptt":
		s.callMode = proto.ModeHalfDuplex
		fmt.Println("default call mode: half duplex")
	case "mode full", "mode fdx":
		s.callMode = proto.ModeFullDuplex
		fmt.Println("default call mode: full duplex")
	case "i", "identity":
		fmt.Printf("%x\n", s.id.Hash())
	case "d", "desthash":
		fmt.Printf("%x\n", s.dest.GetHash())
	case "a", "announce":
		if err := s.phone.Announce(); err != nil {
			fmt.Println("announce:", err)
		}
	case "p", "phonebook":
		printPhonebook(s.book)
	case "history":
		printHistory(s.log)
	case "r", "redial":
		if s.lastDest == "" {
			fmt.Println("no previous destination")
			return true
		}
		s.dial(s.lastDest)
	default:
		s.dial(line)
	}
	return true
}

func (s *session) dial(token string) {
	s.phone.SetBusy(true)
	remote, err := resolveRemote(s.trans, s.book, token)
	s.phone.SetBusy(false)
	if err != nil {
		fmt.Println(err)
		return
	}
	s.lastDest = hex.EncodeToString(remote.Hash())
	s.mu.Lock()
	s.started = time.Now()
	s.mu.Unlock()
	s.phone.SetMode(s.callMode)
	ctx, cancel := context.WithTimeout(context.Background(), s.wait)
	c, err := s.phone.Dial(ctx, remote)
	cancel()
	if err != nil {
		fmt.Println("dial:", err)
		return
	}
	s.mu.Lock()
	s.active = c
	s.mu.Unlock()
	s.enterCallConsole(c)
}

func printHelp() {
	fmt.Println("enter dest hash or name to dial")
	fmt.Println("p phonebook  r redial  i identity  d desthash  a announce  history")
	fmt.Println("mode half|full  set default duplex mode for new calls")
	fmt.Println("during ring: enter answer, r reject")
	fmt.Println("q quit")
}

func printPhonebook(book *phonebook.Book) {
	ents := book.Entries()
	if len(ents) == 0 {
		fmt.Println("no entries")
		return
	}
	for i, e := range ents {
		fmt.Printf("%d  %s  %x  %s\n", i+1, e.Name, e.Hash, e.Alias)
	}
}

func printHistory(log *history.Log) {
	ents, err := log.Recent(historyLimit)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, e := range ents {
		dir := "out"
		if e.Incoming {
			dir = "in"
		}
		fmt.Printf("%s  %s  %s  %.1fs  %s\n", e.Time.Format(time.RFC3339), dir, e.Peer, e.Duration, e.Outcome)
	}
}

func resolveRemote(t pathTransport, book *phonebook.Book, token string) (*identity.Identity, error) {
	if e, ok := book.Lookup(token); ok {
		token = hex.EncodeToString(e.Hash)
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(token, " ", ""))
	if err != nil {
		return nil, fmt.Errorf("unknown name or hash")
	}
	candidates := [][]byte{raw}
	if len(raw) == proto.IdentityHashLen {
		candidates = append(candidates, proto.TelephonyHash(raw))
	}
	remote, err := rnsnode.WaitRecall(t, candidates, recallTimeout)
	if err != nil {
		return nil, fmt.Errorf("could not recall identity for %s", token)
	}
	return remote, nil
}
