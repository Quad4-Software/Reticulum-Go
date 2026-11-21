// SPDX-License-Identifier: LicenseRef-Reticulum
package history_test

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/history"
)

func TestRaceRecordRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calls.jsonl")
	log := history.New(path)
	peer := make([]byte, 16)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 50 {
			_ = log.Record(peer, i%2 == 0, time.Now(), "ended")
		}
	}()
	go func() {
		defer wg.Done()
		for range 50 {
			_, _ = log.Recent(5)
		}
	}()
	wg.Wait()
}

func TestRecordZeroStarted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calls.jsonl")
	log := history.New(path)
	if err := log.Record(nil, true, time.Time{}, "rejected"); err != nil {
		t.Fatal(err)
	}
	got, err := log.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Duration != 0 || got[0].Time.IsZero() {
		t.Fatalf("%+v", got)
	}
}

func TestRecordAndRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calls.jsonl")
	log := history.New(path)
	peer := make([]byte, 16)
	peer[0] = 0xab
	if err := log.Record(peer, true, time.Now().Add(-time.Second), "ended"); err != nil {
		t.Fatal(err)
	}
	if err := log.Record(peer, false, time.Now(), "busy"); err != nil {
		t.Fatal(err)
	}
	got, err := log.Recent(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Outcome != "busy" || got[0].Incoming {
		t.Fatalf("%+v", got)
	}
	all, err := log.Recent(0)
	if err != nil || len(all) != 2 {
		t.Fatalf("all %d %v", len(all), err)
	}
}
