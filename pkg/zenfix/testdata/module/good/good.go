package good

import (
	"context"
	"time"
)

type Transport struct{}

func (t *Transport) RequestPath(dest []byte, iface string, tag []byte, recursive bool) error {
	return nil
}
func (t *Transport) AwaitPath(ctx context.Context, dest []byte) error { return nil }

type Link struct{}

func (l *Link) SetEstablishedCallback(func(*Link)) {}
func (l *Link) Establish() error                   { return nil }
func (l *Link) Send([]byte) error                  { return nil }

func GoodPath(ctx context.Context, tr *Transport, dest []byte) error {
	if err := tr.AwaitPath(ctx, dest); err != nil {
		return err
	}
	return nil
}

func GoodLink(ctx context.Context, tr *Transport, dest []byte) error {
	if err := tr.AwaitPath(ctx, dest); err != nil {
		return err
	}
	l := NewLink()
	l.SetEstablishedCallback(func(*Link) {})
	if err := l.Establish(); err != nil {
		return err
	}
	_ = time.Now()
	return l.Send([]byte("ok"))
}

func NewLink() *Link { return &Link{} }
