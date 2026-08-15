package main

import "time"

type Transport struct{}

func (t *Transport) RequestPath(dest []byte, iface string, tag []byte, recursive bool) error {
	return nil
}
func (t *Transport) HasPath(dest []byte) bool    { return false }
func (t *Transport) AwaitPath(dest []byte) error { return nil }

type Link struct{}

func (l *Link) Establish() error { return nil }

func badPathLoop(tr *Transport, dest []byte) {
	for {
		tr.RequestPath(dest, "", nil, false)
		time.Sleep(100 * time.Millisecond)
	}
}

func badEstablishLoop(l *Link) {
	for {
		l.Establish()
	}
}

func badIgnoredError(tr *Transport, dest []byte) error {
	tr.RequestPath(dest, "", nil, false)
	return nil
}

func badEstablishNoAwait(l *Link) error {
	return l.Establish()
}

func badFixedTimeout() {
	time.Sleep(15 * time.Second)
}
