package mf

import (
	"encoding/hex"
	"sync"
	"testing"
)

func TestStress_MF_ParallelPackUnpack(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	senderHash, err := hex.DecodeString(testHashHex)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	const iters = 500

	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Go(func() {
			for range iters {
				msg := &Message{
					SenderHash: senderHash,
					Text:       "parallel stress payload",
				}
				packed, err := msg.Pack()
				if err != nil {
					errs <- err
					return
				}
				if _, err := Unpack(packed); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
}
