// SPDX-License-Identifier: LicenseRef-Reticulum
package history

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Entry is one completed or attempted call.
type Entry struct {
	Time     time.Time `json:"time"`
	Peer     string    `json:"peer"`
	Incoming bool      `json:"incoming"`
	Duration float64   `json:"duration_sec"`
	Outcome  string    `json:"outcome"`
}

// Log appends JSON lines to a file.
type Log struct {
	mutex sync.Mutex
	path  string
}

func New(path string) *Log {
	return &Log{path: path}
}

func (l *Log) Record(peer []byte, incoming bool, started time.Time, outcome string) error {
	if l == nil || l.path == "" {
		return nil
	}
	duration := 0.0
	if started.IsZero() {
		started = time.Now()
	} else {
		duration = time.Since(started).Seconds()
		if duration < 0 {
			duration = 0
		}
	}
	e := Entry{
		Time:     started,
		Peer:     hex.EncodeToString(peer),
		Incoming: incoming,
		Duration: duration,
		Outcome:  outcome,
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // #nosec G304 -- path is the local call log file
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}

const maxHistoryBytes = 8 << 20

func readTail(path string, limit int) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- path is the local call log file
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	if size == 0 {
		return nil, nil
	}
	if int64(limit) > 0 && size > int64(limit) {
		if _, err := f.Seek(size-int64(limit), io.SeekStart); err != nil {
			return nil, err
		}
	}
	raw, err := io.ReadAll(io.LimitReader(f, int64(limit)))
	if err != nil {
		return nil, err
	}
	if int64(limit) > 0 && size > int64(limit) {
		if i := bytes.IndexByte(raw, '\n'); i >= 0 {
			raw = raw[i+1:]
		}
	}
	return raw, nil
}

func (l *Log) Recent(n int) ([]Entry, error) {
	if l == nil || l.path == "" {
		return nil, nil
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	raw, err := readTail(l.path, maxHistoryBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var all []Entry
	for line := range bytes.SplitSeq(raw, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		all = append(all, e)
	}
	if n <= 0 || n >= len(all) {
		return all, nil
	}
	return all[len(all)-n:], nil
}
