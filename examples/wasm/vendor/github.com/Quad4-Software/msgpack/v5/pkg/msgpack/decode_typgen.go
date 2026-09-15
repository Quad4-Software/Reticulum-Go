package msgpack

import "reflect"

// newValue allocates a new addressable zero value of type t, boxed as a
// reflect.Value, for use as a decode destination (a map key/value, the
// pointee of a nil *T, or an ext type instance).
//
// UsePreallocateValues previously routed through a sync.Map of
// *sync.Pool, one pool per distinct reflect.Type, intended to amortize
// allocation for types decoded repeatedly. That pool could never pay off:
// every value it produced was immediately handed to the caller as part of
// a map, struct field, or interface{} and was never returned to the
// decoder, so a Put never happened and every Get was a guaranteed miss
// that fell through to reflect.New(t) regardless. The sync.Map lookup
// (plus the *sync.Pool allocation on first sight of each type, held for
// the life of the process) was pure overhead on top of the same
// reflect.New(t) call, with no amortization ever realized. newValue now
// allocates directly. UsePreallocateValues and its flag bit are kept for
// API compatibility; they no longer change behavior.
func (d *Decoder) newValue(t reflect.Type) reflect.Value {
	return reflect.New(t)
}
