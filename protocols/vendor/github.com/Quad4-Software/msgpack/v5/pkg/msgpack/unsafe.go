//go:build !appengine

package msgpack

import (
	"unsafe"
)

// bytesToString converts byte slice to string.
func bytesToString(b []byte) string {
	// #nosec G103 -- zero-copy string header over b; callers must not mutate b while the string is used.
	return *(*string)(unsafe.Pointer(&b))
}

// stringToBytes converts string to byte slice.
func stringToBytes(s string) []byte {
	// #nosec G103 -- zero-copy slice header over s; result must not be mutated (string immutability).
	return *(*[]byte)(unsafe.Pointer(
		&struct {
			string
			Cap int
		}{s, len(s)},
	))
}
