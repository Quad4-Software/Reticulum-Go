package msgpack

import (
	"reflect"
	"sync"
)

// valuePools maps reflect.Type to a *sync.Pool that lazily produces
// reflect.New(t) instances. The previous implementation spawned one
// goroutine per distinct type that filled a buffered channel with
// preallocated values forever; those goroutines were never reaped, so
// the goroutine count and resident set grew monotonically with the
// number of distinct Go types ever decoded. sync.Pool gives the same
// amortized allocation pattern (per-P caching) while letting the GC
// drain entries during quiescent periods, and removes the leak.
var valuePools sync.Map

func cachedValue(t reflect.Type) reflect.Value {
	if p, ok := valuePools.Load(t); ok {
		return p.(*sync.Pool).Get().(reflect.Value)
	}
	p, _ := valuePools.LoadOrStore(t, &sync.Pool{
		New: func() interface{} {
			return reflect.New(t)
		},
	})
	return p.(*sync.Pool).Get().(reflect.Value)
}

func (d *Decoder) newValue(t reflect.Type) reflect.Value {
	if d.flags&usePreallocateValues == 0 {
		return reflect.New(t)
	}

	return cachedValue(t)
}
