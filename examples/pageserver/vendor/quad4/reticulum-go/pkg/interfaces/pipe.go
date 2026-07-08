// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	pipeHWMTU         = 1064
	pipeBitrateGuess  = 1_000_000
	defaultRespawnDly = 5 * time.Second
)

// PipeInterface bridges Reticulum to an external program over stdin/stdout with
// HDLC framing, matching Python PipeInterface.
type PipeInterface struct {
	BaseInterface
	command      string
	respawnDelay time.Duration
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       io.ReadCloser
	done         chan struct{}
	stopOnce     sync.Once
	respawning   bool
	panicOnError bool
	txMu         sync.Mutex
	txFrame      []byte
	readBuf      []byte
}

// NewPipeInterface starts a subprocess identified by command with HDLC framing.
func NewPipeInterface(name, command string, enabled bool, respawnDelay time.Duration, panicOnError bool) (*PipeInterface, error) {
	if strings.TrimSpace(command) == "" {
		return nil, fmt.Errorf("no command specified for PipeInterface")
	}
	if respawnDelay <= 0 {
		respawnDelay = defaultRespawnDly
	}
	pi := &PipeInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypePipe, enabled),
		command:       command,
		respawnDelay:  respawnDelay,
		done:          make(chan struct{}),
		panicOnError:  panicOnError,
		txFrame:       make([]byte, 0, pipeHWMTU*2+4),
		readBuf:       make([]byte, pipeHWMTU),
	}
	pi.In = true
	pi.Out = true
	pi.MTU = pipeHWMTU
	pi.Bitrate = pipeBitrateGuess
	if enabled {
		if err := pi.openPipe(); err != nil {
			return nil, err
		}
		pi.startReadLoop()
	}
	return pi, nil
}

func (pi *PipeInterface) String() string {
	return fmt.Sprintf("PipeInterface[%s]", pi.Name)
}

func (pi *PipeInterface) Start() error {
	pi.Mutex.Lock()
	if pi.Online {
		pi.Mutex.Unlock()
		return nil
	}
	enabled := pi.Enabled
	pi.Mutex.Unlock()
	if !enabled {
		return fmt.Errorf("interface not enabled")
	}
	if err := pi.openPipe(); err != nil {
		return err
	}
	pi.startReadLoop()
	return nil
}

func (pi *PipeInterface) Stop() error {
	pi.Mutex.Lock()
	pi.Enabled = false
	pi.Online = false
	pi.Mutex.Unlock()
	pi.stopOnce.Do(func() {
		close(pi.done)
	})
	pi.killProcess()
	return nil
}

func (pi *PipeInterface) openPipe() error {
	args, err := splitPipeCommand(pi.command)
	if err != nil {
		return err
	}
	cmd := exec.Command(args[0], args[1:]...) // #nosec G204 -- user-configured pipe command
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return err
	}
	pi.Mutex.Lock()
	pi.killLocked()
	pi.cmd = cmd
	pi.stdin = stdin
	pi.stdout = stdout
	pi.Online = true
	pi.Mutex.Unlock()
	debug.Log(debug.DebugVerbose, "Subprocess pipe connected", "name", pi.Name)
	return nil
}

func (pi *PipeInterface) killProcess() {
	pi.Mutex.Lock()
	defer pi.Mutex.Unlock()
	pi.killLocked()
}

func (pi *PipeInterface) killLocked() {
	if pi.stdin != nil {
		_ = pi.stdin.Close()
		pi.stdin = nil
	}
	if pi.stdout != nil {
		_ = pi.stdout.Close()
		pi.stdout = nil
	}
	if pi.cmd != nil && pi.cmd.Process != nil {
		_ = pi.cmd.Process.Kill()
		_, _ = pi.cmd.Process.Wait()
	}
	pi.cmd = nil
}

func (pi *PipeInterface) ProcessOutgoing(data []byte) error {
	pi.txMu.Lock()
	defer pi.txMu.Unlock()

	pi.Mutex.RLock()
	online := pi.Online
	stdin := pi.stdin
	pi.Mutex.RUnlock()
	if !online || stdin == nil {
		return fmt.Errorf("pipe interface offline")
	}
	frame := appendFrameHDLC(pi.txFrame[:0], data)
	pi.txFrame = frame
	n, err := stdin.Write(frame)
	if err != nil {
		pi.handleIOError(err)
		return err
	}
	if n != len(frame) {
		return fmt.Errorf("pipe interface only wrote %d bytes of %d", n, len(frame))
	}
	return nil
}

func (pi *PipeInterface) Send(data []byte, address string) error {
	masked, err := common.ApplyIFACOutbound(pi, data)
	if err != nil {
		return err
	}
	if err := pi.ProcessOutgoing(masked); err != nil {
		return err
	}
	pi.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (pi *PipeInterface) startReadLoop() {
	go pi.readLoop()
}

func (pi *PipeInterface) readLoop() {
	decoder := newHDLCStreamDecoder(pi.MTU, pi.ProcessIncoming)
	buffer := pi.readBuf
	if len(buffer) < pi.MTU {
		buffer = make([]byte, pi.MTU)
		pi.readBuf = buffer
	}

	for {
		pi.Mutex.RLock()
		stdout := pi.stdout
		done := pi.done
		pi.Mutex.RUnlock()
		if stdout == nil {
			return
		}
		select {
		case <-done:
			return
		default:
		}

		n, err := stdout.Read(buffer)
		if n == 0 {
			if err != nil {
				pi.handleProcessExit()
				return
			}
			continue
		}
		decoder.feed(buffer[:n])
		if err != nil {
			pi.handleProcessExit()
			return
		}
	}
}

func (pi *PipeInterface) handleProcessExit() {
	pi.Mutex.Lock()
	pi.Online = false
	enabled := pi.Enabled
	detached := pi.Detached
	command := pi.command
	pi.Mutex.Unlock()
	pi.killProcess()
	debug.Log(debug.DebugInfo, "Subprocess terminated", "name", pi.Name)
	if enabled && !detached && strings.TrimSpace(command) != "" {
		go pi.respawnPipe()
	}
}

func (pi *PipeInterface) handleIOError(err error) {
	debug.Log(debug.DebugError, "Pipe interface write error", "name", pi.Name, "error", err)
	pi.Mutex.Lock()
	pi.Online = false
	pi.Mutex.Unlock()
}

func (pi *PipeInterface) respawnPipe() {
	pi.Mutex.Lock()
	if pi.respawning {
		pi.Mutex.Unlock()
		return
	}
	pi.respawning = true
	pi.Mutex.Unlock()
	defer func() {
		pi.Mutex.Lock()
		pi.respawning = false
		pi.Mutex.Unlock()
	}()

	for {
		pi.Mutex.RLock()
		enabled := pi.Enabled
		detached := pi.Detached
		delay := pi.respawnDelay
		done := pi.done
		pi.Mutex.RUnlock()
		if !enabled || detached {
			return
		}
		select {
		case <-done:
			return
		case <-time.After(delay):
		}
		debug.Log(debug.DebugVerbose, "Attempting to respawn subprocess", "name", pi.Name)
		if err := pi.openPipe(); err != nil {
			debug.Log(debug.DebugError, "Pipe respawn failed", "name", pi.Name, "error", err)
			if pi.panicOnError {
				panic(fmt.Sprintf("pipe interface %s: %v", pi.Name, err))
			}
			continue
		}
		debug.Log(debug.DebugInfo, "Reconnected pipe", "name", pi.Name)
		pi.readLoop()
		return
	}
}

func (pi *PipeInterface) GetConn() net.Conn { return nil }

func (pi *PipeInterface) SendPathRequest([]byte) error { return nil }

func (pi *PipeInterface) SendLinkPacket([]byte, []byte, time.Time) error { return nil }

func (pi *PipeInterface) GetBandwidthAvailable() bool {
	pi.Mutex.RLock()
	defer pi.Mutex.RUnlock()
	return pi.Online && pi.stdin != nil
}

// splitPipeCommand splits a command string like Python shlex.split.
func splitPipeCommand(command string) ([]string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("empty command")
	}
	var (
		args    []string
		cur     strings.Builder
		inQuote bool
		quoteCh byte
		escaped bool
	)
	for i := 0; i < len(command); i++ {
		c := command[i]
		if escaped {
			cur.WriteByte(c)
			escaped = false
			continue
		}
		if inQuote {
			if c == quoteCh {
				inQuote = false
				continue
			}
			if c == '\\' && quoteCh == '"' {
				escaped = true
				continue
			}
			cur.WriteByte(c)
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = true
			quoteCh = c
		case ' ', '\t':
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unclosed quote in command")
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return args, nil
}
