// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
)

// Developer-facing setup and API error messages.
const (
	MsgDestTransportRequiredForIn = "destination: transport is required when direction includes In"
	MsgDestTransportNotSet        = "destination: transport not set, pass a transport to destination.New"
	MsgDestAnnounceNoInterfaces   = "destination: announce sent on 0 interfaces (none registered with transport)"
	MsgDestAnnounceNoWritable     = "destination: announce sent on 0 interfaces (none online, enabled, and writable)"
	MsgDestNoIncomingLinkHandler  = `destination: no incoming link handler (import the link package, e.g. _ "quad4/reticulum-go/pkg/link")`
	MsgDestAcceptsLinksFalseOnly  = "destination: AcceptsLinks(false) clears the flag only and does not unregister from transport"
	MsgDestNoPacketCallback       = "destination: packet received but no packet callback set (call SetPacketCallback)"
	MsgDestNoRequestHandler       = "destination: no request handler registered for path (call RegisterRequestHandler)"

	MsgLinkNilDestination          = "link: NewLink called with nil destination"
	MsgLinkNilTransport            = "link: NewLink called with nil transport (Establish will fail to send)"
	MsgLinkDestinationRequired     = "link: destination is required"
	MsgLinkTransportRequired       = "link: transport is required for Establish"
	MsgLinkNoPacketCallback        = "link: packet queued with no packet callback (call SetPacketCallback)"
	MsgLinkNoPacketCallbackDropped = "link: packet queued with no packet callback (call SetPacketCallback), prior queued packet dropped"

	MsgTransportNilDestination       = "transport: cannot register nil destination"
	MsgTransportEmptyDestinationHash = "transport: destination hash is empty"
	MsgTransportNoDestForLinkRequest = "transport: no destination registered for hash (create destination with direction In or call RegisterDestination / AcceptsLinks(true))"
	MsgTransportNoDestForData        = "transport: data for unregistered destination (create destination with direction In or call RegisterDestination)"

	MsgControlAPINoAcceptsLinks = "controlapi: destination registered without accepts_links, inbound link events will not be emitted"

	// System and environment error messages.
	MsgOOM          = "out of memory"
	MsgDisk         = "disk unavailable or full"
	MsgCPU          = "cpu resource limit exceeded"
	MsgCorruption   = "data corruption detected"
	MsgSandbox      = "sandbox restriction denied the operation"
	MsgPortConflict = "port already in use"
	MsgConfig       = "invalid configuration"
)

// Developer-facing sentinel errors. Prefer returning these (or wrapping them)
// so apps can use errors.Is.
var (
	ErrDestTransportRequiredForIn = errors.New(MsgDestTransportRequiredForIn)
	ErrDestTransportNotSet        = errors.New(MsgDestTransportNotSet)
	ErrDestAnnounceNoInterfaces   = errors.New(MsgDestAnnounceNoInterfaces)
	ErrDestAnnounceNoWritable     = errors.New(MsgDestAnnounceNoWritable)
	ErrDestNoIncomingLinkHandler  = errors.New(MsgDestNoIncomingLinkHandler)
	ErrDestNoPacketCallback       = errors.New(MsgDestNoPacketCallback)
	ErrDestNoRequestHandler       = errors.New(MsgDestNoRequestHandler)

	ErrLinkDestinationRequired = errors.New(MsgLinkDestinationRequired)
	ErrLinkTransportRequired   = errors.New(MsgLinkTransportRequired)
	ErrLinkNoPath              = errors.New("link: no path to destination")
	ErrLinkNoPacketCallback    = errors.New(MsgLinkNoPacketCallback)

	ErrTransportNilDestination       = errors.New(MsgTransportNilDestination)
	ErrTransportEmptyDestinationHash = errors.New(MsgTransportEmptyDestinationHash)
	ErrTransportNoDestForLinkRequest = errors.New(MsgTransportNoDestForLinkRequest)
	ErrTransportNoDestForData        = errors.New(MsgTransportNoDestForData)
	ErrNoPathToDestination           = errors.New("no path to destination")

	ErrIdentityNotFound = errors.New("identity not found")

	// System and environment sentinel errors.
	ErrOOM          = errors.New(MsgOOM)
	ErrDisk         = errors.New(MsgDisk)
	ErrCPU          = errors.New(MsgCPU)
	ErrCorruption   = errors.New(MsgCorruption)
	ErrSandbox      = errors.New(MsgSandbox)
	ErrPortConflict = errors.New(MsgPortConflict)
	ErrConfig       = errors.New(MsgConfig)
)

// ErrLinkNoPathf returns a path-missing establish error for destHash.
func ErrLinkNoPathf(destHash []byte) error {
	return fmt.Errorf("%w: %x (call RequestPath or wait for an announce)", ErrLinkNoPath, destHash)
}

// ErrNoPathToDestinationf returns a send/path error for destHash.
func ErrNoPathToDestinationf(destHash []byte) error {
	return fmt.Errorf("%w %x (announce the peer or call RequestPath first)", ErrNoPathToDestination, destHash)
}

// ErrIdentityNotFoundf returns a Recall miss for hash.
func ErrIdentityNotFoundf(hash []byte) error {
	return fmt.Errorf("%w for hash %x (wait for an announce or call InitKnownDestinationsPersistence)", ErrIdentityNotFound, hash)
}

// ErrConfigf wraps ErrConfig with a detail message.
func ErrConfigf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConfig, fmt.Sprintf(format, args...))
}

// ErrCorruptionf wraps ErrCorruption with a detail message.
func ErrCorruptionf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrCorruption, fmt.Sprintf(format, args...))
}

// ErrSandboxf wraps ErrSandbox with a detail message.
func ErrSandboxf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrSandbox, fmt.Sprintf(format, args...))
}

// ErrPortConflictf wraps ErrPortConflict with a detail message.
func ErrPortConflictf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPortConflict, fmt.Sprintf(format, args...))
}

// ErrDiskf wraps ErrDisk with a detail message.
func ErrDiskf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrDisk, fmt.Sprintf(format, args...))
}

// ErrOOMf wraps ErrOOM with a detail message.
func ErrOOMf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrOOM, fmt.Sprintf(format, args...))
}

// ErrCPUf wraps ErrCPU with a detail message.
func ErrCPUf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrCPU, fmt.Sprintf(format, args...))
}

// ClassifyIOError maps common OS and net errors onto library sentinels.
// Unknown errors are returned unchanged.
func ClassifyIOError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrOOM) || errors.Is(err, ErrDisk) || errors.Is(err, ErrCPU) ||
		errors.Is(err, ErrCorruption) || errors.Is(err, ErrSandbox) ||
		errors.Is(err, ErrPortConflict) || errors.Is(err, ErrConfig) {
		return err
	}
	if IsPortConflict(err) {
		return fmt.Errorf("%w: %w", ErrPortConflict, err)
	}
	if IsDiskFull(err) {
		return fmt.Errorf("%w: %w", ErrDisk, err)
	}
	if IsOOM(err) {
		return fmt.Errorf("%w: %w", ErrOOM, err)
	}
	return err
}

// IsPortConflict reports whether err indicates a bind/listen address conflict.
func IsPortConflict(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrPortConflict) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		err = opErr.Err
	}
	var sysErr *os.SyscallError
	if errors.As(err, &sysErr) {
		err = sysErr.Err
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") || strings.Contains(msg, "only one usage of each socket address")
}

// IsDiskFull reports whether err indicates insufficient disk space.
func IsDiskFull(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrDisk) {
		return true
	}
	if errors.Is(err, syscall.ENOSPC) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no space left") || strings.Contains(msg, "disk full")
}

// IsOOM reports whether err indicates an out-of-memory condition.
func IsOOM(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrOOM) || errors.Is(err, ErrMemoryBudgetExceeded) {
		return true
	}
	if errors.Is(err, syscall.ENOMEM) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "out of memory") || strings.Contains(msg, "cannot allocate memory")
}

// WrapListenError classifies listen/bind failures, especially port conflicts.
func WrapListenError(err error) error {
	if err == nil {
		return nil
	}
	return ClassifyIOError(err)
}

// WrapWriteError classifies disk and memory failures from write paths.
func WrapWriteError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, io.ErrShortWrite) {
		return err
	}
	return ClassifyIOError(err)
}
