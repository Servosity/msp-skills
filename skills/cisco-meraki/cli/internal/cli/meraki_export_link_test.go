// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestExportFollowsMerakiLinkHeader guards the hand-fix
// meraki-export-link-pagination: with 250 orgs served 100 per page, export
// must return all 250 once each - not page one only, and not page one again
// and again (the generated offset config sent no offset, so it looped).
func TestExportFollowsMerakiLinkHeader(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) > 10 {
			http.Error(w, "too many requests", http.StatusTeapot)
			return
		}
		start := 0
		if after := r.URL.Query().Get("startingAfter"); after != "" {
			n, _ := strconv.Atoi(after)
			start = n + 1
		}
		end := start + 100
		if end > 250 {
			end = 250
		}
		rows := make([]string, 0, 100)
		for i := start; i < end; i++ {
			rows = append(rows, fmt.Sprintf(`{"id":"%d","name":"Org %d"}`, i, i))
		}
		if end < 250 {
			w.Header().Set("Link", fmt.Sprintf("<http://%s/api/v1/organizations?perPage=100&startingAfter=%d>; rel=next", r.Host, end-1))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[" + strings.Join(rows, ",") + "]"))
	}))
	defer srv.Close()
	t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
	t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")
	out := filepath.Join(t.TempDir(), "orgs.jsonl")
	cmd := newExportCmd(&rootFlags{timeout: 10 * time.Second})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"organizations", "--format", "jsonl", "--output", out})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	seen := map[string]bool{}
	for _, l := range lines {
		seen[l] = true
	}
	if len(lines) != 250 || len(seen) != 250 {
		t.Fatalf("export wrote %d lines (%d distinct) in %d requests, want 250 distinct", len(lines), len(seen), hits)
	}
}
