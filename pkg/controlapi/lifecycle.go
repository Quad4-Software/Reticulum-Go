// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
)

type refreshPathsRequest struct {
	Destinations []string `json:"destinations"`
}

func (s *Server) handleLifecycleResume(w http.ResponseWriter, r *http.Request) {
	if s.lifecycle == nil {
		writeError(w, http.StatusNotImplemented, "lifecycle not configured")
		return
	}
	if err := s.lifecycle.OnNetworkAvailable(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (s *Server) handleLifecyclePause(w http.ResponseWriter, r *http.Request) {
	if s.lifecycle == nil {
		writeError(w, http.StatusNotImplemented, "lifecycle not configured")
		return
	}
	if err := s.lifecycle.OnNetworkLost(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) handleLifecycleRefreshPaths(w http.ResponseWriter, r *http.Request) {
	if s.lifecycle == nil {
		writeError(w, http.StatusNotImplemented, "lifecycle not configured")
		return
	}
	var req refreshPathsRequest
	if r.Body != nil {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	dests := make([][]byte, 0, len(req.Destinations))
	for _, h := range req.Destinations {
		b, err := hex.DecodeString(h)
		if err != nil || len(b) != 16 {
			writeError(w, http.StatusBadRequest, "invalid destination hash")
			return
		}
		dests = append(dests, b)
	}
	if err := s.lifecycle.RefreshPaths(dests...); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "refreshed"})
}
