// Package msgpack implements MessagePack encoding and decoding.
//
// This is the Quad4-maintained fork; module path quad4/msgpack/v5.
// See the repository README for install and migration notes from upstream.
//
// Encoding and decoding use explicit integer narrowing and bit operations that match
// the MessagePack wire format; gosec rule G115 is suppressed at file scope where needed.
package msgpack
