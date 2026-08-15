// Package librns provides a stable embedder facade for Reticulum-Go.
//
// The pure Go API in this package is the source of truth for the C ABI
// exported by pkg/librns/capi. Applications can embed via this package
// directly, or link librns.so and use include/rns.h.
package librns
