// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package store

import "errors"

// Backend stores identity private blobs keyed by string attributes.
type Backend interface {
	Get(attrs map[string]string) ([]byte, error)
	Set(attrs map[string]string, secret []byte, label string) error
	Delete(attrs map[string]string) error
}

const (
	AttrApplication  = "application"
	AttrIdentityPath = "identity.path"
	AttrIdentityKind = "identity.kind"
	ApplicationName  = "reticulum-go"
)

var (
	ErrNotFound    = errors.New("identity store: secret not found")
	ErrUnsupported = errors.New("identity store: backend unsupported on this platform")
	ErrLocked      = errors.New("identity store: collection locked or session unavailable")
)

// AttrsForPath builds the standard attribute map for an identity path.
func AttrsForPath(path, kind string) map[string]string {
	attrs := map[string]string{
		AttrApplication:  ApplicationName,
		AttrIdentityPath: path,
	}
	if kind != "" {
		attrs[AttrIdentityKind] = kind
	}
	return attrs
}
