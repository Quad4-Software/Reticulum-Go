package adversarial

import "time"

// AdversarialNested ensures nested func literals do not confuse the outer function.
func AdversarialNested(tr interface {
	RequestPath([]byte, string, []byte, bool) error
}, dest []byte) {
	go func() {
		for {
			_ = tr.RequestPath(dest, "", nil, false)
		}
	}()
	_ = time.Now()
}

// AdversarialCleanLiteral has a loop in a closure but the outer func is fine.
func AdversarialCleanLiteral() {
	_ = func() {
		for i := 0; i < 3; i++ {
		}
	}
}
