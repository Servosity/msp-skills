// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/config"
)

// hand-fix guard (handfixes.json: error-text-shows-resolved-url): ESET paths
// are absolute templates (https://{region}.<service>.eset.systems/...). Error
// text must name the URL that was actually requested, not the template.
func TestErrorTextShowsResolvedURLNotRegionTemplate(t *testing.T) {
	const tmpl = "https://{region}.device-management.eset.systems/v1/devices"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":5,"message":"not found"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "Bearer test-token", TemplateVars: map[string]string{"region": "eu"}}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), tmpl, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want an APIError, got %v", err)
	}
	if strings.Contains(apiErr.Path, "{region}") || !strings.Contains(apiErr.Path, strings.TrimPrefix(srv.URL, "http://")) {
		t.Fatalf("APIError.Path = %q, want the resolved URL", apiErr.Path)
	}

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := "http://" + ln.Addr().String()
	_ = ln.Close()
	c = New(&config.Config{BaseURL: dead, AuthHeaderVal: "Bearer test-token", TemplateVars: map[string]string{"region": "eu"}}, 2*time.Second, 0)
	c.NoCache = true
	// POST is not retried on a network error, so the error comes back at once.
	_, _, err = c.Post(context.Background(), tmpl, map[string]any{})
	if err == nil || strings.Contains(err.Error(), "{region}") || !strings.Contains(err.Error(), ln.Addr().String()) {
		t.Fatalf("network error names the template, not the resolved URL: %v", err)
	}
}
