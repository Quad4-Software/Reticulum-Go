// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sharedinstance

import (
	"crypto/hmac"
	"crypto/md5" // #nosec G501 -- HMAC-MD5 required for multiprocessing.connection auth
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
)

const (
	challengePrefix = "#CHALLENGE#"
	welcomePrefix   = "#WELCOME#"
	failurePrefix   = "#FAILURE#"
	messageLength   = 40
)

var (
	errAuthFailed = errors.New("shared instance authentication failed")
)

// AuthenticateServer performs the multiprocessing.connection server-side
// handshake on conn.
func AuthenticateServer(conn net.Conn, authkey []byte) error {
	if len(authkey) == 0 {
		return errors.New("empty authkey")
	}
	if err := deliverChallenge(conn, authkey); err != nil {
		return err
	}
	return answerChallenge(conn, authkey)
}

// AuthenticateClient performs the multiprocessing.connection client-side
// handshake on conn.
func AuthenticateClient(conn net.Conn, authkey []byte) error {
	if len(authkey) == 0 {
		return errors.New("empty authkey")
	}
	if err := answerChallenge(conn, authkey); err != nil {
		return err
	}
	return deliverChallenge(conn, authkey)
}

func deliverChallenge(conn net.Conn, authkey []byte) error {
	payload := make([]byte, messageLength)
	if _, err := rand.Read(payload); err != nil {
		return err
	}
	message := append([]byte("{sha256}"), payload...)
	if err := sendBytes(conn, append([]byte(challengePrefix), message...)); err != nil {
		return err
	}
	response, err := recvBytes(conn, 256)
	if err != nil {
		return err
	}
	if err := verifyChallenge(authkey, message, response); err != nil {
		_ = sendBytes(conn, []byte(failurePrefix))
		return errAuthFailed
	}
	return sendBytes(conn, []byte(welcomePrefix))
}

func answerChallenge(conn net.Conn, authkey []byte) error {
	buf, err := recvBytes(conn, 256)
	if err != nil {
		return err
	}
	if len(buf) < len(challengePrefix) || string(buf[:len(challengePrefix)]) != challengePrefix {
		return errAuthFailed
	}
	message := buf[len(challengePrefix):]
	if len(message) < 20 {
		return errAuthFailed
	}
	response := createResponse(authkey, message)
	if err := sendBytes(conn, response); err != nil {
		return err
	}
	reply, err := recvBytes(conn, 256)
	if err != nil {
		return err
	}
	if string(reply) != welcomePrefix {
		return errAuthFailed
	}
	return nil
}

func sendBytes(w io.Writer, buf []byte) error {
	if uint64(len(buf)) > math.MaxUint32 {
		return fmt.Errorf("message too large: %d", len(buf))
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(buf))) // #nosec G115 -- guarded by max length check above
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(buf)
	return err
}

// SendFramed writes a length-prefixed multiprocessing.connection frame.
func SendFramed(w io.Writer, buf []byte) error {
	return sendBytes(w, buf)
}

func recvBytes(r io.Reader, maxSize int) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	size := int32(binary.BigEndian.Uint32(header)) // #nosec G115 -- multiprocessing.connection length prefix
	var n int64
	switch {
	case size == -1:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(r, ext); err != nil {
			return nil, err
		}
		n = int64(binary.BigEndian.Uint64(ext)) // #nosec G115 -- wire-format 64-bit length after -1 sentinel
	default:
		n = int64(size)
	}
	if n < 0 {
		return nil, errors.New("negative message size")
	}
	// maxSize must be positive. A zero or negative cap used to skip the
	// bound and allocate from the wire length alone (allocation bomb).
	if maxSize <= 0 {
		return nil, errors.New("maxSize must be positive")
	}
	if n > int64(maxSize) {
		return nil, fmt.Errorf("message too large: %d", n)
	}
	if n > int64(^uint(0)>>1) {
		return nil, fmt.Errorf("message too large: %d", n)
	}
	buf := make([]byte, int(n))
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// RecvFramed reads a length-prefixed multiprocessing.connection frame.
func RecvFramed(r io.Reader, maxSize int) ([]byte, error) {
	return recvBytes(r, maxSize)
}

func createResponse(authkey, message []byte) []byte {
	digestName, _ := parseDigest(message)
	if digestName == "" {
		m := hmac.New(md5.New, authkey)
		_, _ = m.Write(message)
		return m.Sum(nil)
	}
	m := hmac.New(sha256.New, authkey)
	_, _ = m.Write(message)
	sum := m.Sum(nil)
	return append([]byte("{sha256}"), sum...)
}

func verifyChallenge(authkey, message, response []byte) error {
	digestName, responseMAC := parseDigest(response)
	if digestName == "" {
		digestName = "md5"
	}
	var expected []byte
	switch digestName {
	case "md5":
		m := hmac.New(md5.New, authkey)
		_, _ = m.Write(message)
		expected = m.Sum(nil)
	case "sha256":
		m := hmac.New(sha256.New, authkey)
		_, _ = m.Write(message)
		expected = m.Sum(nil)
	default:
		return errAuthFailed
	}
	if len(expected) != len(responseMAC) {
		return errAuthFailed
	}
	if subtle.ConstantTimeCompare(expected, responseMAC) != 1 {
		return errAuthFailed
	}
	return nil
}

func parseDigest(message []byte) (name string, payload []byte) {
	if len(message) == 16 || len(message) == 20 {
		return "", message
	}
	if len(message) > 2 && message[0] == '{' {
		if end := indexByte(message[1:], '}'); end >= 0 {
			end++
			digest := string(message[1:end])
			switch digest {
			case "md5", "sha256", "sha384", "sha3_256", "sha3_384":
				return digest, message[end+1:]
			}
		}
	}
	return "", message
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}
