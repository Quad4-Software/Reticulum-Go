// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

const bearerPrefix = "Bearer "

// authMiddleware rejects any request without a valid bearer token before it
// reaches the route handlers.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authorized reports whether r carries a bearer token matching the server's
// hex-encoded auth key, compared in constant time.
func (s *Server) authorized(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, bearerPrefix) {
		return false
	}
	token, err := hex.DecodeString(strings.TrimPrefix(header, bearerPrefix))
	if err != nil || len(token) == 0 || len(token) != len(s.authKey) {
		return false
	}
	return subtle.ConstantTimeCompare(token, s.authKey) == 1
}
