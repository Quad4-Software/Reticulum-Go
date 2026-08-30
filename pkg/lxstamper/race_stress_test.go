package lxstamper_test

import (
	"bytes"
	"context"
	"quad4/reticulum-go/pkg/lxstamper"
	"sync"
	"testing"
)

func TestRaceConcurrentWorkblockAndBatch(t *testing.T) {
	material := bytes.Repeat([]byte{0xAB}, 16)
	stamp, _, err := lxstamper.GenerateStampCPU(context.Background(), material, 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_, _ = lxstamper.StampWorkblock(material, 64)
		}()
		go func() {
			defer wg.Done()
			_, _ = lxstamper.StampWorkblockCPU(material, 25)
		}()
		go func() {
			defer wg.Done()
			cands := []lxstamper.StampCandidate{
				{Material: material, Stamp: stamp},
				{Material: material, Stamp: stamp},
				{Material: material, Stamp: stamp},
				{Material: material, Stamp: stamp},
			}
			_ = lxstamper.ValidateStampBatch(cands, 4, 64)
		}()
	}
	wg.Wait()
}
