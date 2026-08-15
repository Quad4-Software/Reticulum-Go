package bad

import (
	"context"
	"time"
)

type Transport struct{}

func (t *Transport) RequestPath(dest []byte, iface string, tag []byte, recursive bool) error {
	return nil
}
func (t *Transport) HasPath(dest []byte) bool                         { return false }
func (t *Transport) AwaitPath(ctx context.Context, dest []byte) error { return nil }
func (t *Transport) NudgePathRequest(dest []byte) error               { return nil }

type Link struct{}

func (l *Link) Establish() error             { return nil }
func (l *Link) Request(string, []byte) error { return nil }

type Destination struct{}

func (d *Destination) Announce() error { return nil }

func BadPathLoop(tr *Transport, dest []byte) {
	for {
		tr.RequestPath(dest, "", nil, false)
		time.Sleep(100 * time.Millisecond)
	}
}

func BadNudgeLoop(tr *Transport, dest []byte) {
	for {
		_ = tr.NudgePathRequest(dest)
	}
}

func BadHasPathLoop(tr *Transport, dest []byte) {
	for !tr.HasPath(dest) {
		time.Sleep(50 * time.Millisecond)
	}
}

func BadAwaitLoop(tr *Transport, dest []byte, ctx context.Context) {
	for {
		_ = tr.AwaitPath(ctx, dest)
	}
}

func BadEstablishLoop(l *Link) {
	for {
		l.Establish()
	}
}

func BadIgnoredError(tr *Transport, dest []byte) error {
	tr.RequestPath(dest, "", nil, false)
	return nil
}

func BadEstablishNoAwait(l *Link) error {
	return l.Establish()
}

func BadEstablishRepeat(l *Link) error {
	_ = l.Establish()
	return l.Establish()
}

func BadFixedTimeout() {
	time.Sleep(15 * time.Second)
}

func BadAnnounceLoop(d *Destination) {
	for {
		_ = d.Announce()
	}
}

func BadSelectLoop(tr *Transport, dest []byte, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		default:
			tr.RequestPath(dest, "", nil, false)
		}
	}
}
