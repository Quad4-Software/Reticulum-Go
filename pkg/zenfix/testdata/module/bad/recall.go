package bad

import (
	"context"
)

type Transport struct{}

func (t *Transport) RequestPath(dest []byte, onInterface string, tag []byte, recursive bool) error {
	return nil
}
func (t *Transport) AwaitPath(ctx context.Context, dest []byte) error { return nil }

var identity = struct {
	Recall func([]byte) (any, error)
}{}

func BadRecallBeforePath(tr *Transport, dest []byte) error {
	id, err := identity.Recall(dest)
	if err != nil {
		return err
	}
	_ = id
	return tr.AwaitPath(context.Background(), dest)
}

func BadOnInterface(tr *Transport, dest []byte) error {
	return tr.RequestPath(dest, "LoRaInterface", nil, false)
}
