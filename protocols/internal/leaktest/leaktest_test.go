package leaktest

import (
	"sync"
	"testing"
	"time"
)

func TestAssertStable_NoLeakAfterWork(t *testing.T) {
	base := Baseline()
	var wg sync.WaitGroup
	for range 40 {
		wg.Go(func() {
			time.Sleep(5 * time.Millisecond)
		})
	}
	wg.Wait()
	AssertStable(t, base, 16, 3*time.Second)
}
