// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// mcpZeroReader streams filler bytes without allocating a large buffer, so the
// oversized-body server can produce >maxMCPBody cheaply.
type mcpZeroReader struct{}

func (mcpZeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

// TestMCPJSONRejectsOversizedBody proves the official-MCP JSON read REJECTS a
// body over the cap instead of silently truncating it (which would corrupt a
// JSON-RPC envelope) and no longer swallows the read error.
func TestMCPJSONRejectsOversizedBody(t *testing.T) {
	if testing.Short() {
		t.Skip("skips the >16 MiB transfer under -short")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.CopyN(w, mcpZeroReader{}, maxMCPBody+1)
	}))
	t.Cleanup(server.Close)

	_, _, _, err := mcpJSON("GET", server.URL, nil, nil)
	if err == nil {
		t.Fatalf("expected an error for an oversized MCP body, got nil (body was silently truncated)")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want a body-size-limit error", err)
	}
}

// TestMCPFormRejectsOversizedBody proves the same for the form-encoded token
// endpoint read path.
func TestMCPFormRejectsOversizedBody(t *testing.T) {
	if testing.Short() {
		t.Skip("skips the >16 MiB transfer under -short")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.CopyN(w, mcpZeroReader{}, maxMCPBody+1)
	}))
	t.Cleanup(server.Close)

	_, _, err := mcpForm(context.Background(), server.URL, url.Values{"grant_type": {"refresh_token"}})
	if err == nil {
		t.Fatalf("expected an error for an oversized MCP form body, got nil (body was silently truncated)")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want a body-size-limit error", err)
	}
}
