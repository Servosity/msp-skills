// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestTriageMutatesUUIDIssues pins that triage acts on the issue IDs the API
// returns (UUIDs): an --ignore plus --comment reaches the ignore and comments
// endpoints for that UUID, and a token that is neither a UUID nor an integer is
// refused before any request.
func TestTriageMutatesUUIDIssues(t *testing.T) {
	isolateNovelStore(t)
	var mu sync.Mutex
	var writes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			mu.Lock()
			writes = append(writes, r.Method+" "+r.URL.Path)
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	t.Setenv("SERVOSITY_MSP_BASE_URL", srv.URL)
	t.Setenv("SERVOSITY_MSP_TOKEN", "fixture-token")

	const id = "550E8400-E29B-41D4-A716-446655440000"
	run := func(args ...string) error {
		cmd := RootCmd()
		cmd.SetArgs(args)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		return cmd.Execute()
	}
	if err := run("triage", "--ignore", id, "--comment", "scheduled outage"); err != nil {
		t.Fatalf("triage --ignore <uuid>: %v", err)
	}
	want := []string{
		"PUT /issues/550e8400-e29b-41d4-a716-446655440000/ignore/",
		"PUT /issues/550e8400-e29b-41d4-a716-446655440000/comments/",
	}
	mu.Lock()
	got := strings.Join(writes, "\n")
	mu.Unlock()
	if got != strings.Join(want, "\n") {
		t.Fatalf("writes =\n%s\nwant\n%s", got, strings.Join(want, "\n"))
	}
	for _, bad := range []string{"../../companies/4421", "18;drop", "abc"} {
		if err := run("triage", "--archive", bad); err == nil || !strings.Contains(err.Error(), "is not an issue ID") {
			t.Fatalf("triage --archive %q: err = %v, want the ID refusal", bad, err)
		}
	}
	mu.Lock()
	n := len(writes)
	mu.Unlock()
	if n != len(want) {
		t.Fatalf("a refused ID still sent %d write(s)", n-len(want))
	}
}
