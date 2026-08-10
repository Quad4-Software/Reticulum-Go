// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import (
	"fmt"
	"math"

	"quad4/msgpack/v5/pkg/msgpack"
)

func packCompatVersion(m *VersionMessage) ([]byte, error) {
	sw := m.SoftwareVersion
	if sw == "" {
		sw = DefaultSoftware
	}
	pv := m.ProtocolVersion
	if pv == 0 {
		pv = CompatProtocolVersion
	}
	return msgpack.Marshal([]any{sw, pv})
}

func unpackCompatVersion(m *VersionMessage, raw []byte) error {
	var v any
	if err := msgpack.Unmarshal(raw, &v); err != nil {
		return err
	}
	list, ok := v.([]any)
	if !ok || len(list) < 2 {
		return fmt.Errorf("rgosh: invalid compat version")
	}
	m.SoftwareVersion = asString(list[0])
	m.ProtocolVersion = asInt(list[1])
	return nil
}

func packCompatWinSize(m *WinSizeMessage) ([]byte, error) {
	return msgpack.Marshal([]any{m.Rows, m.Cols, m.HPix, m.VPix})
}

func unpackCompatWinSize(m *WinSizeMessage, raw []byte) error {
	var v any
	if err := msgpack.Unmarshal(raw, &v); err != nil {
		return err
	}
	list, ok := v.([]any)
	if !ok || len(list) < 4 {
		return fmt.Errorf("rgosh: invalid compat winsize")
	}
	m.Rows = asInt(list[0])
	m.Cols = asInt(list[1])
	m.HPix = asInt(list[2])
	m.VPix = asInt(list[3])
	return nil
}

func packCompatExec(m *ExecMessage) ([]byte, error) {
	cmdline := make([]any, len(m.Cmdline))
	for i, a := range m.Cmdline {
		cmdline[i] = a
	}
	return msgpack.Marshal([]any{
		cmdline,
		m.PipeStdin,
		m.PipeStdout,
		m.PipeStderr,
		nil,
		m.Term,
		m.Rows,
		m.Cols,
		m.HPix,
		m.VPix,
	})
}

func unpackCompatExec(m *ExecMessage, raw []byte) error {
	var v any
	if err := msgpack.Unmarshal(raw, &v); err != nil {
		return err
	}
	list, ok := v.([]any)
	if !ok || len(list) < 10 {
		return fmt.Errorf("rgosh: invalid compat exec")
	}
	m.Cmdline = asStringSlice(list[0])
	if len(m.Cmdline) > MaxArgvLen {
		return ErrOversizedField
	}
	m.PipeStdin = asBool(list[1])
	m.PipeStdout = asBool(list[2])
	m.PipeStderr = asBool(list[3])
	m.Term = asString(list[5])
	if len(m.Term) > MaxTermLen {
		return ErrOversizedField
	}
	m.Rows = asInt(list[6])
	m.Cols = asInt(list[7])
	m.HPix = asInt(list[8])
	m.VPix = asInt(list[9])
	return nil
}

func packCompatError(m *ErrorMessage) ([]byte, error) {
	return msgpack.Marshal([]any{m.Msg, m.Fatal, nil})
}

func unpackCompatError(m *ErrorMessage, raw []byte) error {
	var v any
	if err := msgpack.Unmarshal(raw, &v); err != nil {
		return err
	}
	list, ok := v.([]any)
	if !ok || len(list) < 2 {
		return fmt.Errorf("rgosh: invalid compat error")
	}
	m.Msg = asString(list[0])
	m.Fatal = asBool(list[1])
	return nil
}

func packCompatExit(m *ExitMessage) ([]byte, error) {
	return msgpack.Marshal(m.ReturnCode)
}

func unpackCompatExit(m *ExitMessage, raw []byte) error {
	var v any
	if err := msgpack.Unmarshal(raw, &v); err != nil {
		return err
	}
	m.ReturnCode = asInt(v)
	return nil
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return ""
	}
}

func asInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int8:
		return int(x)
	case int16:
		return int(x)
	case int32:
		return int(x)
	case int64:
		return int(x)
	case uint:
		if x > uint(math.MaxInt) {
			return math.MaxInt
		}
		return int(x)
	case uint8:
		return int(x)
	case uint16:
		return int(x)
	case uint32:
		if int64(x) > int64(math.MaxInt) {
			return math.MaxInt
		}
		return int(x)
	case uint64:
		if x > uint64(math.MaxInt) {
			return math.MaxInt
		}
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	default:
		return 0
	}
}

func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case uint8:
		return x != 0
	default:
		return false
	}
}

func asStringSlice(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		out = append(out, asString(item))
	}
	return out
}
