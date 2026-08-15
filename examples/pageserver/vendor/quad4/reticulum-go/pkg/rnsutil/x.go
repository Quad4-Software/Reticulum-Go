// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/transport"
)

const (
	// RNXAppName is the destination app name for remote execution.
	RNXAppName = "rnx"
	// RNXAspect is the execute aspect.
	RNXAspect = "execute"
	// RNXCommandPath is the link request path.
	RNXCommandPath = "command"
	// DefaultRNXTimeout is the default path/link/command wait.
	DefaultRNXTimeout = 15 * time.Second
	// RNXRemoteExecGrace matches Python remote_exec_grace.
	RNXRemoteExecGrace = 2 * time.Second

	// Exit codes matching Python rnx.
	ExitRNXInvalidDest    = 241
	ExitRNXPathNotFound   = 242
	ExitRNXLinkFailed     = 243
	ExitRNXRequestFailed  = 244
	ExitRNXNoResult       = 245
	ExitRNXReceiveFailed  = 246
	ExitRNXInvalidResult  = 247
	ExitRNXRemoteExecFail = 248
	ExitRNXNoResponse     = 249
	ExitRNXMirrorNilCode  = 240
)

// RNXIdentityPath returns the default identity path under storage.
func RNXIdentityPath(cfgStorage string) string {
	if cfgStorage == "" {
		return ""
	}
	return filepath.Join(cfgStorage, "identities", RNXAppName)
}

// PrepareRNXIdentity loads or creates the rnx identity file.
func PrepareRNXIdentity(path string) (*identity.Identity, error) {
	if path == "" {
		return nil, fmt.Errorf("empty identity path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return identity.FromFile(path)
	}
	id, err := identity.New()
	if err != nil {
		return nil, err
	}
	if err := id.ToFile(path); err != nil {
		return nil, err
	}
	return id, nil
}

// LoadRNXAllowedIdentities reads allow-list files (Python rnx + Go rgox paths) and CLI hashes.
func LoadRNXAllowedIdentities(extra []string) ([][]byte, error) {
	var out [][]byte
	seen := map[string]struct{}{}
	add := func(h []byte) {
		k := hex.EncodeToString(h)
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, h)
	}
	home := os.Getenv("HOME")
	candidates := []string{
		"/etc/rnx/allowed_identities",
		filepath.Join(home, ".config", "rnx", "allowed_identities"),
		filepath.Join(home, ".rnx", "allowed_identities"),
		filepath.Join(home, ".config", "rgox", "allowed_identities"),
		filepath.Join(home, ".rgox", "allowed_identities"),
	}
	for _, path := range candidates {
		hashes, err := readAllowedFile(path)
		if err != nil {
			continue
		}
		for _, h := range hashes {
			add(h)
		}
	}
	for _, a := range extra {
		h, err := ParseDestHash(strings.TrimSpace(a))
		if err != nil {
			return nil, fmt.Errorf("allowed identity %q: %w", a, err)
		}
		add(h)
	}
	return out, nil
}

// EstablishRNXLink waits for a path and opens an outbound link to rnx.execute.
func EstablishRNXLink(ctx context.Context, tr *transport.Transport, destHash []byte) (*link.Link, error) {
	if err := WaitPath(ctx, tr, destHash); err != nil {
		return nil, fmt.Errorf("path: %w", err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		return nil, fmt.Errorf("recall: %w", err)
	}
	outDest, err := destination.New(remote, destination.Out, destination.Single, RNXAppName, tr, RNXAspect)
	if err != nil {
		return nil, err
	}
	l := link.NewLink(outDest, tr, nil, nil, nil)
	if err := l.Establish(); err != nil {
		return nil, err
	}
	if err := WaitLinkActive(ctx, l); err != nil {
		l.Teardown()
		return nil, fmt.Errorf("link: %w", err)
	}
	return l, nil
}

// RNXRequest is the 5-field command request payload.
type RNXRequest struct {
	Command     string
	TimeoutSec  *float64
	StdoutLimit *int
	StderrLimit *int
	Stdin       []byte
}

// RNXResult is the 8-field command response payload.
type RNXResult struct {
	Executed    bool
	ReturnCode  *int
	Stdout      []byte
	Stderr      []byte
	StdoutTotal int
	StderrTotal int
	StartedAt   float64
	ConcludedAt *float64
}

// PackRNXRequest builds the msgpack-friendly 5-list for link.request.
func PackRNXRequest(req RNXRequest) []any {
	var timeout any
	if req.TimeoutSec != nil {
		timeout = *req.TimeoutSec
	}
	var oLim, eLim any
	if req.StdoutLimit != nil {
		oLim = *req.StdoutLimit
	}
	if req.StderrLimit != nil {
		eLim = *req.StderrLimit
	}
	var stdin any
	if req.Stdin != nil {
		stdin = req.Stdin
	}
	return []any{
		[]byte(req.Command),
		timeout,
		oLim,
		eLim,
		stdin,
	}
}

// ParseRNXRequestPayload unpacks handler data (msgpack 5-list or already-decoded list).
func ParseRNXRequestPayload(data []byte) (RNXRequest, error) {
	var req RNXRequest
	if len(data) == 0 {
		return req, fmt.Errorf("empty request")
	}
	var raw any
	if err := msgpack.Unmarshal(data, &raw); err != nil {
		return req, fmt.Errorf("msgpack: %w", err)
	}
	return parseRNXRequestAny(raw)
}

func parseRNXRequestAny(raw any) (RNXRequest, error) {
	var req RNXRequest
	list, ok := raw.([]any)
	if !ok || len(list) < 5 {
		return req, fmt.Errorf("invalid request format")
	}
	switch v := list[0].(type) {
	case []byte:
		req.Command = string(v)
	case string:
		req.Command = v
	default:
		return req, fmt.Errorf("command must be bytes or string")
	}
	if list[1] != nil {
		f := asFloat64(list[1])
		req.TimeoutSec = &f
	}
	if list[2] != nil {
		n, err := asIntAny(list[2])
		if err != nil {
			return req, fmt.Errorf("stdout limit: %w", err)
		}
		req.StdoutLimit = &n
	}
	if list[3] != nil {
		n, err := asIntAny(list[3])
		if err != nil {
			return req, fmt.Errorf("stderr limit: %w", err)
		}
		req.StderrLimit = &n
	}
	if list[4] != nil {
		switch v := list[4].(type) {
		case []byte:
			req.Stdin = v
		case string:
			req.Stdin = []byte(v)
		default:
			return req, fmt.Errorf("stdin must be bytes or string")
		}
	}
	return req, nil
}

// ParseRNXResult unpacks a link response value into RNXResult.
func ParseRNXResult(v any) (RNXResult, error) {
	var out RNXResult
	if v == nil {
		return out, fmt.Errorf("nil response")
	}
	if b, ok := v.([]byte); ok {
		var raw any
		if err := msgpack.Unmarshal(b, &raw); err != nil {
			return out, fmt.Errorf("msgpack: %w", err)
		}
		v = raw
	}
	list, ok := v.([]any)
	if !ok || len(list) < 8 {
		return out, fmt.Errorf("invalid result format")
	}
	switch x := list[0].(type) {
	case bool:
		out.Executed = x
	case int:
		out.Executed = x != 0
	case int64:
		out.Executed = x != 0
	case uint8:
		out.Executed = x != 0
	default:
		return out, fmt.Errorf("executed field invalid")
	}
	if list[1] != nil {
		n, err := asIntAny(list[1])
		if err != nil {
			return out, fmt.Errorf("returncode: %w", err)
		}
		out.ReturnCode = &n
	}
	if list[2] != nil {
		switch x := list[2].(type) {
		case []byte:
			out.Stdout = x
		case string:
			out.Stdout = []byte(x)
		}
	}
	if list[3] != nil {
		switch x := list[3].(type) {
		case []byte:
			out.Stderr = x
		case string:
			out.Stderr = []byte(x)
		}
	}
	if list[4] != nil {
		if n, err := asIntAny(list[4]); err == nil {
			out.StdoutTotal = n
		}
	}
	if list[5] != nil {
		if n, err := asIntAny(list[5]); err == nil {
			out.StderrTotal = n
		}
	}
	if list[6] != nil {
		out.StartedAt = asFloat64(list[6])
	}
	if list[7] != nil {
		f := asFloat64(list[7])
		out.ConcludedAt = &f
	}
	return out, nil
}

// PackRNXResult builds the 8-list response for the request handler.
func PackRNXResult(r RNXResult) []any {
	var rc any
	if r.ReturnCode != nil {
		rc = *r.ReturnCode
	}
	var concluded any
	if r.ConcludedAt != nil {
		concluded = *r.ConcludedAt
	}
	var stdout, stderr any
	if r.Stdout != nil {
		stdout = r.Stdout
	}
	if r.Stderr != nil {
		stderr = r.Stderr
	}
	return []any{
		r.Executed,
		rc,
		stdout,
		stderr,
		r.StdoutTotal,
		r.StderrTotal,
		r.StartedAt,
		concluded,
	}
}

// RNXRequestTimeout returns the client request timeout: base + RTT*4 + grace.
func RNXRequestTimeout(base time.Duration, rttSec float64) time.Duration {
	if base <= 0 {
		base = DefaultRNXTimeout
	}
	return base + time.Duration(rttSec*4*float64(time.Second)) + RNXRemoteExecGrace
}

// SplitShellCommand splits a command string with quote support (shlex-like).
func SplitShellCommand(command string) ([]string, error) {
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

// ExecuteRNXCommandLocally runs a command for an rnx listener and builds RNXResult.
func ExecuteRNXCommandLocally(req RNXRequest) RNXResult {
	started := float64(time.Now().UnixNano()) / 1e9
	result := RNXResult{StartedAt: started}

	args, err := SplitShellCommand(req.Command)
	if err != nil {
		return result
	}
	cmd := exec.Command(args[0], args[1:]...) // #nosec G204 -- remote-exec allow-listed operator command
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	if err := cmd.Start(); err != nil {
		return result
	}
	result.Executed = true

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var timeout <-chan time.Time
	if req.TimeoutSec != nil && *req.TimeoutSec > 0 {
		timeout = time.After(time.Duration(*req.TimeoutSec * float64(time.Second)))
	}

	timedOut := false
	select {
	case err := <-done:
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code := ee.ExitCode()
				result.ReturnCode = &code
			} else {
				code := 1
				result.ReturnCode = &code
			}
		} else {
			code := 0
			result.ReturnCode = &code
		}
	case <-timeout:
		timedOut = true
		_ = cmd.Process.Kill()
		<-done
		if cmd.ProcessState != nil {
			code := cmd.ProcessState.ExitCode()
			result.ReturnCode = &code
		}
	}

	stdout := stdoutBuf.Bytes()
	stderr := stderrBuf.Bytes()
	result.StdoutTotal = len(stdout)
	result.StderrTotal = len(stderr)
	result.Stdout = truncateBytes(stdout, req.StdoutLimit)
	result.Stderr = truncateBytes(stderr, req.StderrLimit)
	if !timedOut {
		concluded := float64(time.Now().UnixNano()) / 1e9
		result.ConcludedAt = &concluded
	}
	return result
}

func truncateBytes(b []byte, limit *int) []byte {
	if b == nil {
		return nil
	}
	if limit == nil {
		return b
	}
	if *limit == 0 {
		return []byte{}
	}
	if len(b) > *limit {
		return b[:*limit]
	}
	return b
}

func asIntAny(v any) (int, error) {
	switch x := v.(type) {
	case int:
		return x, nil
	case int8:
		return int(x), nil
	case int16:
		return int(x), nil
	case int32:
		return int(x), nil
	case int64:
		return int(x), nil
	case uint:
		if x > math.MaxInt {
			return 0, fmt.Errorf("uint value %d overflows int", x)
		}
		return int(x), nil
	case uint8:
		return int(x), nil
	case uint16:
		return int(x), nil
	case uint32:
		return int(x), nil
	case uint64:
		if x > math.MaxInt {
			return 0, fmt.Errorf("uint64 value %d overflows int", x)
		}
		return int(x), nil
	case float64:
		return int(x), nil
	case float32:
		return int(x), nil
	default:
		return 0, fmt.Errorf("not an int: %T", v)
	}
}
