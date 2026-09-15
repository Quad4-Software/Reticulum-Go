// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package i2p

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// Message is a parsed SAM reply line.
type Message struct {
	Cmd    string
	Action string
	Opts   map[string]string
	raw    string
}

func parseMessage(line string) (*Message, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, ErrInvalidResponse
	}
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 {
		return nil, ErrInvalidResponse
	}
	m := &Message{
		Cmd:    parts[0],
		Action: parts[1],
		Opts:   make(map[string]string),
		raw:    line,
	}
	if len(parts) == 3 {
		m.Opts = parseOpts(parts[2])
	}
	return m, nil
}

// parseOpts parses SAM key=value tokens, including MESSAGE="quoted text".
func parseOpts(s string) map[string]string {
	opts := make(map[string]string)
	i := 0
	for i < len(s) {
		for i < len(s) && s[i] == ' ' {
			i++
		}
		if i >= len(s) {
			break
		}
		rest := s[i:]
		eq := strings.IndexByte(rest, '=')
		if eq < 0 {
			opts[rest] = "true"
			break
		}
		key := rest[:eq]
		i += eq + 1
		if i >= len(s) {
			opts[key] = ""
			break
		}
		if s[i] == '"' {
			i++
			end := strings.IndexByte(s[i:], '"')
			if end < 0 {
				opts[key] = s[i:]
				break
			}
			opts[key] = s[i : i+end]
			i += end + 1
			continue
		}
		sp := strings.IndexByte(s[i:], ' ')
		if sp < 0 {
			opts[key] = s[i:]
			break
		}
		opts[key] = s[i : i+sp]
		i += sp
	}
	return opts
}

func (m *Message) OK() bool {
	if m.Opts["RESULT"] == "OK" {
		return true
	}
	// i2pd and some SAM implementations omit RESULT on successful data
	// replies such as DEST REPLY PUB=... PRIV=...
	if m.Action == "REPLY" && m.Opts["RESULT"] == "" {
		return true
	}
	return false
}

func (m *Message) ResultError() error {
	if m.OK() {
		return nil
	}
	code := m.Opts["RESULT"]
	if code == "" {
		return ErrInvalidResponse
	}
	return samErrorFromResult(code, m.Opts["MESSAGE"])
}

// Client speaks SAM v3.1 to a local I2P router.
type Client struct {
	Address string
	Timeout time.Duration
}

func NewClient(address string) *Client {
	if address == "" {
		address = SAMAddressFromEnv()
	}
	return &Client{
		Address: address,
		Timeout: defaultSAMTimeout * time.Second,
	}
}

func SAMAddressFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("I2P_SAM_ADDRESS")); v != "" {
		return v
	}
	return defaultSAMAddress
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	d := net.Dialer{Timeout: c.Timeout}
	conn, err := d.DialContext(ctx, "tcp", c.Address)
	if err != nil {
		return nil, err
	}
	if err := c.hello(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func (c *Client) hello(ctx context.Context, conn net.Conn) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.Timeout)
	}
	_ = conn.SetDeadline(deadline)
	msg := fmt.Sprintf("HELLO VERSION MIN=%s MAX=%s\n", defaultSAMMinVer, defaultSAMMaxVer)
	if _, err := conn.Write([]byte(msg)); err != nil {
		return err
	}
	reply, err := readLine(conn)
	if err != nil {
		return err
	}
	m, err := parseMessage(reply)
	if err != nil {
		return err
	}
	return m.ResultError()
}

func (c *Client) NamingLookup(ctx context.Context, name string) (string, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if _, err := conn.Write(fmt.Appendf(nil, "NAMING LOOKUP NAME=%s\n", name)); err != nil {
		return "", err
	}
	reply, err := readLine(conn)
	if err != nil {
		return "", err
	}
	m, err := parseMessage(reply)
	if err != nil {
		return "", err
	}
	if err := m.ResultError(); err != nil {
		return "", err
	}
	return m.Opts["VALUE"], nil
}

func (c *Client) DestGenerate(ctx context.Context) (*Destination, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.Write(fmt.Appendf(nil, "DEST GENERATE SIGNATURE_TYPE=%d\n", ed25519SigType)); err != nil {
		return nil, err
	}
	reply, err := readLine(conn)
	if err != nil {
		return nil, err
	}
	m, err := parseMessage(reply)
	if err != nil {
		return nil, err
	}
	if err := m.ResultError(); err != nil {
		return nil, err
	}
	return NewDestinationFromPrivateB64(m.Opts["PRIV"])
}

// Session is an open SAM stream session. The underlying connection must stay
// open for STREAM CONNECT/ACCEPT on other sockets to succeed.
type Session struct {
	ID     string
	conn   net.Conn
	client *Client
}

func (s *Session) Close() error {
	if s.conn == nil {
		return nil
	}
	err := s.conn.Close()
	s.conn = nil
	return err
}

func (c *Client) OpenSession(ctx context.Context, sessionID, destination string) (*Session, error) {
	return c.OpenSessionWithOptions(ctx, sessionID, destination, DefaultSessionOptions)
}

// OpenSessionWithOptions creates a STREAM session with extra I2CP options.
func (c *Client) OpenSessionWithOptions(ctx context.Context, sessionID, destination, options string) (*Session, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	cmd := formatSessionCreate(sessionID, destination, options)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	reply, err := readLine(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	m, err := parseMessage(reply)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := m.ResultError(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &Session{ID: sessionID, conn: conn, client: c}, nil
}

func formatSessionCreate(sessionID, destination, options string) string {
	cmd := fmt.Sprintf("SESSION CREATE STYLE=STREAM ID=%s DESTINATION=%s", sessionID, destination)
	if options = strings.TrimSpace(options); options != "" {
		cmd += " " + options
	}
	return cmd + "\n"
}

func (c *Client) CreateSession(ctx context.Context, sessionID, destination string) error {
	sess, err := c.OpenSession(ctx, sessionID, destination)
	if err != nil {
		return err
	}
	return sess.Close()
}

func (c *Client) StreamConnect(ctx context.Context, sessionID, destinationB64 string) (net.Conn, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf("STREAM CONNECT ID=%s DESTINATION=%s SILENT=false\n", sessionID, destinationB64)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	reply, err := readLine(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	m, err := parseMessage(reply)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := m.ResultError(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func (c *Client) StreamAccept(ctx context.Context, sessionID string) (net.Conn, error) {
	if sessionID == "" {
		return nil, ErrInvalidResponse
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf("STREAM ACCEPT ID=%s SILENT=false\n", sessionID)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	reply, err := readLine(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	m, err := parseMessage(reply)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := m.ResultError(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func readLine(conn net.Conn) (string, error) {
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		if err.Error() == "EOF" {
			return "", ErrSAMOffline
		}
		return "", err
	}
	return line, nil
}

func FreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}
