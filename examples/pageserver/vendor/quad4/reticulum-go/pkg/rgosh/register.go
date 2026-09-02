// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"context"
	"errors"
	"time"

	"quad4/reticulum-go/pkg/channel"
)

var errNilChannel = errors.New("rgosh: nil channel")

// RegisterNative registers native rgosh message constructors on ch.
func RegisterNative(ch *channel.Channel) error {
	ctors := map[uint16]func() channel.MessageBase{
		NativeNoop:       func() channel.MessageBase { return &NoopMessage{} },
		NativeWinSize:    func() channel.MessageBase { return &WinSizeMessage{} },
		NativeExec:       func() channel.MessageBase { return &ExecMessage{} },
		NativeStream:     func() channel.MessageBase { return &StreamMessage{} },
		NativeVersion:    func() channel.MessageBase { return &VersionMessage{} },
		NativeError:      func() channel.MessageBase { return &ErrorMessage{} },
		NativeExit:       func() channel.MessageBase { return &ExitMessage{} },
		NativeAuthOK:     func() channel.MessageBase { return &AuthOKMessage{} },
		NativeAuthDenied: func() channel.MessageBase { return &AuthDeniedMessage{} },
	}
	for t, ctor := range ctors {
		if err := ch.RegisterMessageType(t, ctor); err != nil {
			return err
		}
	}
	return nil
}

// RegisterCompat registers Python rnsh-compatible message constructors on ch.
func RegisterCompat(ch *channel.Channel) error {
	ctors := map[uint16]func() channel.MessageBase{
		CompatNoop:    func() channel.MessageBase { return &NoopMessage{Compat: true} },
		CompatWinSize: func() channel.MessageBase { return &WinSizeMessage{Compat: true} },
		CompatExec:    func() channel.MessageBase { return &ExecMessage{Compat: true} },
		CompatStream:  func() channel.MessageBase { return &StreamMessage{Compat: true} },
		CompatVersion: func() channel.MessageBase { return &VersionMessage{Compat: true} },
		CompatError:   func() channel.MessageBase { return &ErrorMessage{Compat: true} },
		CompatExit:    func() channel.MessageBase { return &ExitMessage{Compat: true} },
	}
	for t, ctor := range ctors {
		if err := ch.RegisterMessageType(t, ctor); err != nil {
			return err
		}
	}
	return nil
}

// ChannelSender adapts *channel.Channel to Sender.
type ChannelSender struct {
	Ch  *channel.Channel
	Ctx context.Context
}

func (s ChannelSender) Send(msg Message) error {
	if s.Ch == nil {
		return errNilChannel
	}
	return s.Ch.Send(msg)
}

func (s ChannelSender) WaitReady(ctx context.Context) error {
	if s.Ch == nil {
		return errNilChannel
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.Ch.WaitReady(ctx)
}

func (s ChannelSender) MDU() int {
	if s.Ch == nil {
		return channel.DefaultOutletMDU - channel.ChannelHeaderSize
	}
	return s.Ch.MDU()
}

// WaitTxIdle waits until outstanding channel envelopes are acknowledged.
func (s ChannelSender) WaitTxIdle(timeout time.Duration) bool {
	if s.Ch == nil {
		return true
	}
	return s.Ch.WaitTxIdle(timeout)
}
